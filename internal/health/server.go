package health

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/james-gibson/isotope"
	"github.com/james-gibson/smoke-alarm/internal/netbind"
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
	// IsotopeTTL is how long a self-registered isotope stays live without
	// re-registering. A registration is a heartbeat, not a name: a record the
	// registrant stopped renewing is swept from the registry. Zero uses
	// DefaultIsotopeTTL.
	IsotopeTTL time.Duration
	// VerifyTrust resolves the initial trust rung for a registering endpoint.
	// Defaults to isotope.VerifyTrust; tests inject a stub.
	VerifyTrust func(ctx context.Context, endpoint string) int
}

// DefaultIsotopeTTL is how long a registration lasts without renewal. Short
// enough that a node that dies without deregistering stops resolving, long
// enough that a client renewing on a one-minute cadence can miss several
// heartbeats before its name expires (smoke-alarm#35).
const DefaultIsotopeTTL = 5 * time.Minute

// Registry status values reported by GET /isotope/list.
const (
	// IsotopeStatusLive is a self-registered record whose heartbeat is current.
	IsotopeStatusLive = "live"
	// IsotopeStatusRegistered is a record the alarm mirrors from a trusted
	// source (federation membership). Its lifecycle belongs to that source, not
	// to a heartbeat, so the registry reports the registration without claiming
	// the name is live.
	IsotopeStatusRegistered = "registered"
)

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
	// Listeners maps a listener name (health, hosted, oauth_redirect_mock) to
	// the address it actually bound. A listener whose configured port was held
	// reports the port it moved to, not the port that was asked for.
	Listeners map[string]string `json:"listeners,omitempty"`
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

// Feature represents a Gherkin feature that this smoke-alarm knows about and
// can report its certification status for to ADHD dashboards.
type Feature struct {
	ID          string    `json:"id"`   // unique identifier, e.g. "ocd/federation-mesh"
	Name        string    `json:"name"` // human-readable feature name
	FilePath    string    `json:"file_path,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	Scenarios   int       `json:"scenarios"`
	Status      string    `json:"status"` // "certified", "unclaimed", "failed"
	CertifiedAt time.Time `json:"certified_at,omitempty"`
	FailedAt    time.Time `json:"failed_at,omitempty"`
}

// IsotopeDependency names another project this registration depends on: the
// route says where that project is (lezz `seed.json` project.route), and the
// contract pins the signed spec the dependency was built against. Neither is a
// trust claim; the rung remains the registry's judgement.
type IsotopeDependency struct {
	ProjectRoute string `json:"project_route"`
	Contract     string `json:"contract,omitempty"`
}

// IsotopeView is a registered isotope plus what only the registry knows about
// the name. isotope.Record is the wire shape shared with isotope.Client; the
// view adds the heartbeat and the project identity, so a consumer that resolves
// by name can tell a live registration from one the registry has not heard from,
// and a dependency can be resolved by project rather than by URL.
type IsotopeView struct {
	isotope.Record
	LastSeen     time.Time           `json:"last_seen"`
	ExpiresAt    *time.Time          `json:"expires_at,omitempty"`
	ProjectRoute string              `json:"project_route,omitempty"`
	Contract     string              `json:"contract,omitempty"`
	DependsOn    []IsotopeDependency `json:"depends_on,omitempty"`
	// Status is IsotopeStatusLive for a self-registered record within its TTL,
	// or IsotopeStatusRegistered for a record mirrored from a trusted source.
	Status string `json:"status"`
}

// isotopeEntry is a record plus its heartbeat and identity. A zero expiresAt
// means the record is durable: the alarm mirrors it from federation membership,
// whose lifecycle removes it, rather than from a registration that must renew.
type isotopeEntry struct {
	record       isotope.Record
	lastSeen     time.Time
	expiresAt    time.Time
	projectRoute string
	contract     string
	dependsOn    []IsotopeDependency
}

// Server provides liveness, readiness, and status endpoints.
type Server struct {
	opts            Options
	httpSrv         *http.Server
	startedAt       time.Time
	selfDescFactory SelfDescriptionFunc
	listener        net.Listener // set by BindWithRetry; nil means use ListenAndServe
	verifyTrust     func(ctx context.Context, endpoint string) int

	live  atomic.Bool
	ready atomic.Bool

	mu             sync.RWMutex
	readyError     string
	components     map[string]ComponentStatus
	targets        map[string]TargetStatus
	isotopes       map[string]isotopeEntry
	features       map[string]Feature // keyed by Feature.ID
	listeners      map[string]string  // listener name -> address actually bound
	shutdownOnce   sync.Once
	shutdownSignal chan struct{}
}

// NewServer constructs a health server with sensible defaults.
func NewServer(opts Options) *Server {
	if strings.TrimSpace(opts.ServiceName) == "" {
		opts.ServiceName = "ocd-smoke-alarm"
	}
	if strings.TrimSpace(opts.ListenAddr) == "" {
		opts.ListenAddr = "localhost:8088"
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
	if opts.IsotopeTTL <= 0 {
		opts.IsotopeTTL = DefaultIsotopeTTL
	}
	if opts.VerifyTrust == nil {
		opts.VerifyTrust = isotope.VerifyTrust
	}

	s := &Server{
		opts:            opts,
		startedAt:       time.Now().UTC(),
		selfDescFactory: opts.SelfDescriptionFunc,
		verifyTrust:     opts.VerifyTrust,
		components:      make(map[string]ComponentStatus),
		targets:         make(map[string]TargetStatus),
		isotopes:        make(map[string]isotopeEntry),
		features:        make(map[string]Feature),
		listeners:       make(map[string]string),
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
	mux.HandleFunc("/isotope/", s.handleIsotopeRoute)
	mux.HandleFunc("/features", s.handleFeaturesList)
	mux.HandleFunc("/features/certify", s.handleFeatureCertify)

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
	ln, boundAddr, err := netbind.Listen(s.opts.ListenAddr, maxRetries)
	if err != nil {
		return "", fmt.Errorf("health server: %w", err)
	}
	s.listener = ln
	s.SetListener(ListenerHealth, boundAddr)
	return boundAddr, nil
}

// SetListener records the address a named listener actually bound, so /status
// can report a port that moved instead of the one that was configured. The
// health listener is recorded by BindWithRetry; the process records its other
// listeners here because they are the ones a lab notices missing (smoke-alarm#53).
func (s *Server) SetListener(name, addr string) {
	name = strings.TrimSpace(name)
	addr = strings.TrimSpace(addr)
	if name == "" || addr == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.listeners[name] = addr
}

// ListenerAddr returns the address a named listener actually bound, or "" when
// the process does not run that listener.
func (s *Server) ListenerAddr(name string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listeners[strings.TrimSpace(name)]
}

// Listeners returns a copy of the bound addresses keyed by listener name.
func (s *Server) Listeners() map[string]string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.listeners) == 0 {
		return nil
	}
	out := make(map[string]string, len(s.listeners))
	for name, addr := range s.listeners {
		out[name] = addr
	}
	return out
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
//
// It is safe to call more than once and from more than one goroutine:
// Start calls it when ctx is canceled, and the CLI calls it from a defer, so
// the two can overlap on the way out. shutdownOnce is what makes the "shutdown
// started" transition happen exactly once — a select/default around close() is
// not atomic, and two callers that both took the default branch raced and
// panicked with "close of closed channel" (smoke-alarm#40). Repeated calls
// still wait for in-flight connections: http.Server.Shutdown is idempotent.
func (s *Server) Shutdown(ctx context.Context) error {
	s.shutdownOnce.Do(func() { close(s.shutdownSignal) })

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

	listeners := make(map[string]string, len(s.listeners))
	for name, addr := range s.listeners {
		listeners[name] = addr
	}

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
		Listeners:  listeners,
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

	// Merge upstream components too — an upstream (e.g. adhd) reports security
	// events (honeypot contact) as components, not targets. Dropping them here
	// silently loses the event.
	for _, c := range incoming.Components {
		s.SetComponent(c.Name, c.Healthy, c.Detail)
	}

	s.SetComponent("federation", true, fmt.Sprintf("received %d targets from upstream", len(incoming.Targets)))
	writeJSON(w, http.StatusOK, map[string]any{"status": "accepted"})
}

// isotopeRegistration is the request body for POST /isotope/register: the wire
// shape shared with isotope.Client plus the heartbeat override and the project
// identity a dependency needs to resolve this registration.
type isotopeRegistration struct {
	isotope.Registration
	// TTL overrides the server's isotope TTL for this registration, as a Go
	// duration ("90s"). Re-registering renews it.
	TTL string `json:"ttl,omitempty"`
	// ProjectRoute is the project this instance belongs to (lezz `seed.json`
	// project.route). An instance name identifies one process; the route
	// identifies the project a dependent is trying to reach.
	ProjectRoute string `json:"project_route,omitempty"`
	// Contract is the signed spec id this instance implements.
	Contract string `json:"contract,omitempty"`
	// DependsOn references the projects this instance depends on, by route and
	// pinned contract.
	DependsOn []IsotopeDependency `json:"depends_on,omitempty"`
}

func (s *Server) handleIsotopeRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	var body isotopeRegistration
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
		return
	}
	reg := body.Registration
	reg.Name = strings.TrimSpace(reg.Name)
	reg.Endpoint = strings.TrimSpace(reg.Endpoint)
	if reg.Name == "" || reg.Endpoint == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name and endpoint are required"})
		return
	}

	ttl, err := s.isotopeTTL(body.TTL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}

	now := time.Now().UTC()

	// Re-registration is a heartbeat, not a new claim. Re-verifying an unchanged
	// endpoint on every beat would run the trust probe (up to 4s) for an answer
	// that cannot have changed; a changed endpoint is a new claim and is
	// verified again.
	s.mu.RLock()
	existing, found := s.isotopes[reg.Name]
	s.mu.RUnlock()

	rung, rungName := 0, ""
	if found && existing.record.Endpoint == reg.Endpoint {
		rung, rungName = existing.record.TrustRung, existing.record.RungName
	} else {
		rung = s.verifyTrust(r.Context(), reg.Endpoint)
		rungName = rungNameFor(rung)
	}

	registeredAt := now
	if found {
		registeredAt = existing.record.RegisteredAt
	}

	entry := isotopeEntry{
		record: isotope.Record{
			Name:         reg.Name,
			Role:         reg.Role,
			Endpoint:     reg.Endpoint,
			Protocol:     reg.Protocol,
			TrustRung:    rung,
			RungName:     rungName,
			RegisteredAt: registeredAt,
		},
		lastSeen:     now,
		expiresAt:    now.Add(ttl),
		projectRoute: strings.TrimSpace(body.ProjectRoute),
		contract:     strings.TrimSpace(body.Contract),
		dependsOn:    normalizeDependencies(body.DependsOn),
	}
	if entry.projectRoute != "" && !isoRoutePattern.MatchString(entry.projectRoute) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "project_route must use letters, digits, _ . -"})
		return
	}
	if found {
		// A renewal may be minimal. Do not let a heartbeat erase the identity a
		// dependent resolved.
		if entry.projectRoute == "" {
			entry.projectRoute = existing.projectRoute
		}
		if entry.contract == "" {
			entry.contract = existing.contract
		}
		if len(entry.dependsOn) == 0 {
			entry.dependsOn = existing.dependsOn
		}
	}

	s.mu.Lock()
	s.isotopes[reg.Name] = entry
	s.mu.Unlock()

	writeJSON(w, http.StatusOK, viewOf(entry))
}

func (s *Server) handleIsotopeList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}

	writeJSON(w, http.StatusOK, s.liveIsotopeViews(func(isotopeEntry) bool { return true }))
}

// handleIsotopeRoute dispatches the isotope registry:
//
//	POST   /isotope/register     register or renew a name
//	GET    /isotope/list         live records
//	GET    /isotope/<name>       resolve one name (404 when expired)
//	DELETE /isotope/<name>       deregister now
//
// It is one handler rather than ServeMux method patterns because the literal
// /isotope/register route and a /isotope/{name} pattern overlap in a way the
// mux rejects.
func (s *Server) handleIsotopeRoute(w http.ResponseWriter, r *http.Request) {
	item := strings.TrimPrefix(r.URL.Path, "/isotope/")
	if unescaped, err := url.PathUnescape(item); err == nil {
		item = unescaped
	}

	switch item {
	case "register":
		s.handleIsotopeRegister(w, r)
		return
	case "list":
		s.handleIsotopeList(w, r)
		return
	case "":
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "isotope not found"})
		return
	}

	switch {
	case strings.HasPrefix(item, "by-route/"):
		s.handleIsotopeByRoute(w, r, strings.TrimPrefix(item, "by-route/"))
		return
	case strings.HasPrefix(item, "by-contract/"):
		s.handleIsotopeByContract(w, r, strings.TrimPrefix(item, "by-contract/"))
		return
	}

	name := strings.TrimSpace(item)
	switch r.Method {
	case http.MethodGet:
		s.handleIsotopeGet(w, name)
	case http.MethodDelete:
		s.handleIsotopeDelete(w, name)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
	}
}

// handleIsotopeByRoute resolves a project route to the instances registered
// under it. A route names the project, not one process: a restarted or
// additional instance shares it.
func (s *Server) handleIsotopeByRoute(w http.ResponseWriter, r *http.Request, route string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	route = strings.TrimSpace(route)
	if route == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "project route is required"})
		return
	}

	views := s.liveIsotopeViews(func(entry isotopeEntry) bool {
		return entry.projectRoute == route
	})
	if len(views) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "no isotope registered for that project route"})
		return
	}
	writeJSON(w, http.StatusOK, views)
}

// handleIsotopeByContract resolves a pinned contract id to what implements or
// depends on it. A dependent pins the contract it was built against; this is
// how it finds the project that still offers it.
func (s *Server) handleIsotopeByContract(w http.ResponseWriter, r *http.Request, contract string) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	contract = strings.TrimSpace(contract)
	if contract == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "contract is required"})
		return
	}

	views := s.liveIsotopeViews(func(entry isotopeEntry) bool {
		if entry.contract == contract {
			return true
		}
		for _, dep := range entry.dependsOn {
			if dep.Contract == contract {
				return true
			}
		}
		return false
	})
	if len(views) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "no isotope registered for that contract"})
		return
	}
	writeJSON(w, http.StatusOK, views)
}

// liveIsotopeViews sweeps expired names and returns the matching views, ordered
// by name so responses are stable.
func (s *Server) liveIsotopeViews(match func(isotopeEntry) bool) []IsotopeView {
	s.mu.Lock()
	s.sweepIsotopesLocked(time.Now().UTC())
	views := make([]IsotopeView, 0, len(s.isotopes))
	for _, entry := range s.isotopes {
		if match(entry) {
			views = append(views, viewOf(entry))
		}
	}
	s.mu.Unlock()

	sort.Slice(views, func(i, j int) bool { return views[i].Name < views[j].Name })
	return views
}

// handleIsotopeGet resolves one name. An expired registration is gone, so this
// answers 404 rather than handing a consumer a dead endpoint.
func (s *Server) handleIsotopeGet(w http.ResponseWriter, name string) {
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name is required"})
		return
	}

	s.mu.Lock()
	s.sweepIsotopesLocked(time.Now().UTC())
	entry, found := s.isotopes[name]
	s.mu.Unlock()

	if !found {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "isotope not found"})
		return
	}
	writeJSON(w, http.StatusOK, viewOf(entry))
}

// handleIsotopeDelete removes a registration explicitly, for a node that gets
// the chance to say goodbye.
func (s *Server) handleIsotopeDelete(w http.ResponseWriter, name string) {
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "name is required"})
		return
	}

	s.mu.Lock()
	_, found := s.isotopes[name]
	delete(s.isotopes, name)
	s.mu.Unlock()

	if !found {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "isotope not found"})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// sweepIsotopesLocked drops heartbeat-managed records whose expiry has passed.
// Durable records (federation-mirrored) have no expiry and are removed by the
// membership feed instead. Callers must hold s.mu.
func (s *Server) sweepIsotopesLocked(now time.Time) {
	for name, entry := range s.isotopes {
		if !entry.expiresAt.IsZero() && !now.Before(entry.expiresAt) {
			delete(s.isotopes, name)
		}
	}
}

// rungNameFor renders a rung with the name the isotope module assigns it.
func rungNameFor(rung int) string {
	for _, rt := range isotope.DefaultRungThresholds {
		if rt.Rung == rung {
			return rt.Name
		}
	}
	return ""
}

// isotopeTTL resolves the heartbeat window for one registration.
func (s *Server) isotopeTTL(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return s.opts.IsotopeTTL, nil
	}
	ttl, err := time.ParseDuration(raw)
	if err != nil || ttl <= 0 {
		return 0, fmt.Errorf("ttl must be a positive duration, e.g. \"90s\"")
	}
	return ttl, nil
}

// viewOf renders a stored entry as the liveness- and identity-bearing wire view.
func viewOf(entry isotopeEntry) IsotopeView {
	view := IsotopeView{
		Record:       entry.record,
		LastSeen:     entry.lastSeen,
		ProjectRoute: entry.projectRoute,
		Contract:     entry.contract,
		DependsOn:    append([]IsotopeDependency(nil), entry.dependsOn...),
		Status:       IsotopeStatusRegistered,
	}
	if !entry.expiresAt.IsZero() {
		expires := entry.expiresAt
		view.ExpiresAt = &expires
		view.Status = IsotopeStatusLive
	}
	return view
}

// isoRoutePattern is lezz's project-id shape: letters, digits, and _ . - only.
var isoRoutePattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// normalizeDependencies trims a dependency list and drops entries that name no
// project. An empty list means "no dependencies", not "keep whatever was there".
func normalizeDependencies(in []IsotopeDependency) []IsotopeDependency {
	if len(in) == 0 {
		return nil
	}
	out := make([]IsotopeDependency, 0, len(in))
	for _, dep := range in {
		route := strings.TrimSpace(dep.ProjectRoute)
		if route == "" {
			continue
		}
		out = append(out, IsotopeDependency{
			ProjectRoute: route,
			Contract:     strings.TrimSpace(dep.Contract),
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// UpsertIsotope adds or replaces an isotope record. Modules (for example the
// federation mesh) use it to register internal isotopes for members they trust
// by construction, without a network round-trip through /isotope/register.
//
// A record added this way is durable: the membership feed owns its lifecycle
// and removes it when the member leaves, so it is not subject to the heartbeat
// sweep. Updating a self-registered record preserves that record's heartbeat,
// because a membership event says the peer is a member, not that its own
// registration is current.
func (s *Server) UpsertIsotope(rec isotope.Record) {
	if strings.TrimSpace(rec.Name) == "" {
		return
	}
	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	entry, found := s.isotopes[rec.Name]
	if found && rec.RegisteredAt.IsZero() {
		rec.RegisteredAt = entry.record.RegisteredAt
	}
	if rec.RegisteredAt.IsZero() {
		rec.RegisteredAt = now
	}
	if !found {
		s.isotopes[rec.Name] = isotopeEntry{record: rec, lastSeen: now}
		return
	}
	entry.record = rec
	if entry.expiresAt.IsZero() {
		entry.lastSeen = now
	}
	s.isotopes[rec.Name] = entry
}

// RemoveIsotope deletes a previously registered isotope by name. Unknown names
// are ignored.
func (s *Server) RemoveIsotope(name string) {
	if strings.TrimSpace(name) == "" {
		return
	}
	s.mu.Lock()
	delete(s.isotopes, name)
	s.mu.Unlock()
}

// RegisterFeature adds or replaces a feature in the registry with status "unclaimed".
// Idempotent: re-registering an already-certified feature preserves its certified status.
func (s *Server) RegisterFeature(f Feature) {
	if f.Status == "" {
		f.Status = "unclaimed"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.features[f.ID]; ok && existing.Status == "certified" {
		return // don't downgrade certified to unclaimed on re-registration
	}
	s.features[f.ID] = f
}

// CertifyFeature marks a feature as certified at the current time.
func (s *Server) CertifyFeature(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.features[id]
	if !ok {
		return
	}
	f.Status = "certified"
	f.CertifiedAt = time.Now().UTC()
	s.features[id] = f
}

// FailFeature marks a feature as failed.
func (s *Server) FailFeature(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	f, ok := s.features[id]
	if !ok {
		return
	}
	f.Status = "failed"
	f.FailedAt = time.Now().UTC()
	s.features[id] = f
}

// handleFeaturesList returns all registered features as JSON.
func (s *Server) handleFeaturesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	s.mu.RLock()
	out := make([]Feature, 0, len(s.features))
	for _, f := range s.features {
		out = append(out, f)
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	writeJSON(w, http.StatusOK, out)
}

// handleFeatureCertify accepts a POST body {"id":"..."} and certifies that feature.
// Called by the Godog step suite after a scenario passes.
func (s *Server) handleFeatureCertify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	var body struct {
		ID     string `json:"id"`
		Status string `json:"status"` // "certified" or "failed"; defaults to "certified"
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "id required"})
		return
	}
	if body.Status == "failed" {
		s.FailFeature(body.ID)
	} else {
		s.CertifyFeature(body.ID)
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "id": body.ID})
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
