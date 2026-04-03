package health

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// SelfDescriptionFunc returns the live self-description document as a
// JSON-serializable value. Called on each request to /.well-known/smoke-alarm.json.
type SelfDescriptionFunc func() any

// Options configures the HTTP health server.
type Options struct {
	ServiceName         string
	Version             string
	ListenAddr          string
	HealthzPath         string
	ReadyzPath          string
	StatusPath          string
	SelfDescriptionPath string
	SelfDescriptionFunc SelfDescriptionFunc
	ShutdownTimeout     time.Duration
}

// ComponentStatus tracks readiness contributors (scheduler, discovery, probe engine, etc).
type ComponentStatus struct {
	Name      string    `json:"name"`
	Healthy   bool      `json:"healthy"`
	Detail    string    `json:"detail,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TargetStatus is a lightweight snapshot of a monitored target.
type TargetStatus struct {
	ID         string    `json:"id"`
	Protocol   string    `json:"protocol,omitempty"`
	Endpoint   string    `json:"endpoint,omitempty"`
	State      string    `json:"state"` // healthy|degraded|unhealthy|outage|regression|unknown
	Severity   string    `json:"severity,omitempty"`
	Message    string    `json:"message,omitempty"`
	Regression bool      `json:"regression"`
	CheckedAt  time.Time `json:"checked_at"`
	LatencyMS  int64     `json:"latency_ms,omitempty"`
}

// StatusResponse is returned by /status.
type StatusResponse struct {
	Service    string            `json:"service"`
	Version    string            `json:"version,omitempty"`
	Now        time.Time         `json:"now"`
	StartedAt  time.Time         `json:"started_at"`
	UptimeSec  int64             `json:"uptime_sec"`
	Live       bool              `json:"live"`
	Ready      bool              `json:"ready"`
	ReadyError string            `json:"ready_error,omitempty"`
	Summary    StatusSummary     `json:"summary"`
	Components []ComponentStatus `json:"components"`
	Targets    []TargetStatus    `json:"targets"`
}

// StatusSummary aggregates target states.
type StatusSummary struct {
	Total      int `json:"total"`
	Healthy    int `json:"healthy"`
	Degraded   int `json:"degraded"`
	Unhealthy  int `json:"unhealthy"`
	Outage     int `json:"outage"`
	Regression int `json:"regression"`
	Unknown    int `json:"unknown"`
}

// Server provides liveness, readiness, and status endpoints.
type Server struct {
	opts            Options
	httpSrv         *http.Server
	startedAt       time.Time
	selfDescFactory SelfDescriptionFunc
	listener        net.Listener // set by BindWithRetry; nil means use ListenAndServe

	live  atomic.Bool
	ready atomic.Bool

	mu             sync.RWMutex
	readyError     string
	components     map[string]ComponentStatus
	targets        map[string]TargetStatus
	shutdownSignal chan struct{}
}

// NewServer constructs a health server with sensible defaults.
func NewServer(opts Options) *Server {
	if strings.TrimSpace(opts.ServiceName) == "" {
		opts.ServiceName = "ocd-smoke-alarm"
	}
	if strings.TrimSpace(opts.ListenAddr) == "" {
		opts.ListenAddr = "127.0.0.1:8088"
	}
	if strings.TrimSpace(opts.HealthzPath) == "" {
		opts.HealthzPath = "/healthz"
	}
	if strings.TrimSpace(opts.ReadyzPath) == "" {
		opts.ReadyzPath = "/readyz"
	}
	if strings.TrimSpace(opts.StatusPath) == "" {
		opts.StatusPath = "/status"
	}
	if strings.TrimSpace(opts.SelfDescriptionPath) == "" {
		opts.SelfDescriptionPath = "/.well-known/smoke-alarm.json"
	}
	if opts.ShutdownTimeout <= 0 {
		opts.ShutdownTimeout = 10 * time.Second
	}

	s := &Server{
		opts:            opts,
		startedAt:       time.Now().UTC(),
		selfDescFactory: opts.SelfDescriptionFunc,
		components:      make(map[string]ComponentStatus),
		targets:         make(map[string]TargetStatus),
		shutdownSignal:  make(chan struct{}),
	}
	s.live.Store(true)
	s.ready.Store(false)

	mux := http.NewServeMux()
	mux.HandleFunc(opts.HealthzPath, s.handleHealthz)
	mux.HandleFunc(opts.ReadyzPath, s.handleReadyz)
	mux.HandleFunc(opts.StatusPath, s.handleStatus)
	mux.HandleFunc(opts.SelfDescriptionPath, s.handleSelfDescription)
	mux.HandleFunc("/federation/report", s.handleFederationReport)

	s.httpSrv = &http.Server{
		Addr:              opts.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second,
	}
	return s
}

// BindWithRetry pre-binds the health server's listen address, scanning up to
// maxRetries higher ports on EADDRINUSE. Returns the address actually bound.
// Must be called before Start. The bound port is what callers should advertise
// (e.g. via mDNS) — it may differ from opts.ListenAddr when ports are in use.
func (s *Server) BindWithRetry(maxRetries int) (string, error) {
	host, portStr, err := net.SplitHostPort(s.opts.ListenAddr)
	if err != nil {
		return "", fmt.Errorf("health server: invalid listen addr %q: %w", s.opts.ListenAddr, err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return "", fmt.Errorf("health server: invalid port in %q: %w", s.opts.ListenAddr, err)
	}
	for i := 0; i <= maxRetries; i++ {
		addr := net.JoinHostPort(host, strconv.Itoa(port+i))
		var lc net.ListenConfig
		ln, err := lc.Listen(context.Background(), "tcp", addr)
		if err == nil {
			s.listener = ln
			return addr, nil
		}
		if !isAddrInUse(err) {
			return "", fmt.Errorf("health server: listen %s: %w", addr, err)
		}
	}
	return "", fmt.Errorf("health server: no free port in range %s – %s:%d",
		s.opts.ListenAddr, host, port+maxRetries)
}

func isAddrInUse(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "address already in use") ||
		strings.Contains(err.Error(), "bind: can't assign requested address"))
}

// Start runs the HTTP server and blocks until ctx cancellation or a fatal serve error.
//
// If BindWithRetry was called first, Start serves on the pre-bound listener.
// Otherwise it falls back to ListenAndServe on opts.ListenAddr.
//
// On context cancellation, Start attempts graceful shutdown and returns nil
// unless shutdown itself fails.
func (s *Server) Start(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		var err error
		if s.listener != nil {
			err = s.httpSrv.Serve(s.listener)
		} else {
			err = s.httpSrv.ListenAndServe()
		}
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		return s.Shutdown(context.Background())
	case err := <-errCh:
		return err
	}
}

// Shutdown gracefully stops the HTTP server.
func (s *Server) Shutdown(ctx context.Context) error {
	select {
	case <-s.shutdownSignal:
		// already shutting down/shut down
	default:
		close(s.shutdownSignal)
	}

	s.live.Store(false)

	timeoutCtx, cancel := context.WithTimeout(ctx, s.opts.ShutdownTimeout)
	defer cancel()
	return s.httpSrv.Shutdown(timeoutCtx)
}

// SetReady updates readiness and optional reason when unready.
func (s *Server) SetReady(ready bool, reason string) {
	s.ready.Store(ready)
	s.mu.Lock()
	defer s.mu.Unlock()
	if ready {
		s.readyError = ""
		return
	}
	s.readyError = strings.TrimSpace(reason)
}

// SetSelfDescription sets or replaces the self-description factory function.
func (s *Server) SetSelfDescription(fn SelfDescriptionFunc) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.selfDescFactory = fn
}

// SetLive updates liveness explicitly (normally true while process is running).
func (s *Server) SetLive(live bool) {
	s.live.Store(live)
}

// SetComponent records readiness component status.
func (s *Server) SetComponent(name string, healthy bool, detail string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.components[name] = ComponentStatus{
		Name:      name,
		Healthy:   healthy,
		Detail:    strings.TrimSpace(detail),
		UpdatedAt: time.Now().UTC(),
	}
}

// RemoveComponent removes a readiness component.
func (s *Server) RemoveComponent(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.components, name)
}

// UpsertTargetStatus stores or updates a target snapshot.
func (s *Server) UpsertTargetStatus(st TargetStatus) {
	if strings.TrimSpace(st.ID) == "" {
		return
	}
	if st.CheckedAt.IsZero() {
		st.CheckedAt = time.Now().UTC()
	}
	state := strings.ToLower(strings.TrimSpace(st.State))
	if state == "" {
		st.State = "unknown"
	} else {
		st.State = state
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.targets[st.ID] = st
}

// RemoveTarget removes a target from status output.
func (s *Server) RemoveTarget(targetID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.targets, targetID)
}

// Snapshot returns a consistent status view.
func (s *Server) Snapshot() StatusResponse {
	now := time.Now().UTC()
	live := s.live.Load()
	readyFlag := s.ready.Load()

	s.mu.RLock()
	defer s.mu.RUnlock()

	components := make([]ComponentStatus, 0, len(s.components))
	for _, c := range s.components {
		components = append(components, c)
	}
	sort.Slice(components, func(i, j int) bool { return components[i].Name < components[j].Name })

	targets := make([]TargetStatus, 0, len(s.targets))
	for _, t := range s.targets {
		targets = append(targets, t)
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].ID < targets[j].ID })

	summary := summarizeTargets(targets)

	ready := readyFlag && allComponentsHealthy(components)
	readyErr := s.readyError
	if !ready {
		if readyErr == "" && !allComponentsHealthy(components) {
			readyErr = "one or more components are unhealthy"
		}
	}

	return StatusResponse{
		Service:    s.opts.ServiceName,
		Version:    s.opts.Version,
		Now:        now,
		StartedAt:  s.startedAt,
		UptimeSec:  int64(now.Sub(s.startedAt).Seconds()),
		Live:       live,
		Ready:      ready,
		ReadyError: readyErr,
		Summary:    summary,
		Components: components,
		Targets:    targets,
	}
}

func (s *Server) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	if !s.live.Load() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status":  "unhealthy",
			"service": s.opts.ServiceName,
			"reason":  "service not live",
			"ts":      time.Now().UTC(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ok",
		"service": s.opts.ServiceName,
		"ts":      time.Now().UTC(),
	})
}

func (s *Server) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	snap := s.Snapshot()
	if !snap.Ready {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status":      "not_ready",
			"service":     s.opts.ServiceName,
			"ready_error": snap.ReadyError,
			"summary":     snap.Summary,
			"ts":          time.Now().UTC(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  "ready",
		"service": s.opts.ServiceName,
		"summary": snap.Summary,
		"ts":      time.Now().UTC(),
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.Snapshot())
}

func (s *Server) handleSelfDescription(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	fn := s.selfDescFactory
	s.mu.RUnlock()
	if fn == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "self-description not configured"})
		return
	}
	writeJSON(w, http.StatusOK, fn())
}

func (s *Server) handleFederationReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	var incoming StatusResponse
	if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
		return
	}

	for _, t := range incoming.Targets {
		s.UpsertTargetStatus(t)
	}

	s.SetComponent("federation", true, fmt.Sprintf("received %d targets from upstream", len(incoming.Targets)))
	writeJSON(w, http.StatusOK, map[string]any{"status": "accepted"})
}

func summarizeTargets(targets []TargetStatus) StatusSummary {
	var out StatusSummary
	out.Total = len(targets)

	for _, t := range targets {
		switch strings.ToLower(strings.TrimSpace(t.State)) {
		case "healthy":
			out.Healthy++
		case "degraded":
			out.Degraded++
		case "unhealthy", "failed":
			out.Unhealthy++
		case "outage":
			out.Outage++
		case "regression":
			out.Regression++
		default:
			out.Unknown++
		}
	}
	return out
}

func allComponentsHealthy(components []ComponentStatus) bool {
	for _, c := range components {
		if !c.Healthy {
			return false
		}
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
