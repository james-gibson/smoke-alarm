package federation

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/health"
)

// ServerOptions configures the federation introducer HTTP server.
type ServerOptions struct {
	// Listener is the already-claimed local slot listener.
	Listener net.Listener
	// Registry holds the local membership state.
	Registry *Registry
	// Logger is optional structured logging output.
	Logger *slog.Logger
	// AgeOutInterval controls how frequently stale peers are culled.
	AgeOutInterval time.Duration
	// SharedSecret authenticates federation endpoints. Required whenever the
	// listener is reachable beyond loopback (see RequireSharedSecret), because
	// membership is itself a trust grant.
	SharedSecret string
	// PeerStatusTimeout marks a peer's status stale when no report has arrived
	// within the window. Zero disables staleness marking.
	PeerStatusTimeout time.Duration
	// SelfStatus supplies the introducer's own status, which is included in the
	// aggregate so followers see the introducer's alerts as well as each
	// other's. Optional.
	SelfStatus func() health.StatusResponse
	// OnStatus is invoked with the peer-reported status set whenever it changes,
	// so the introducer can render its peers into its own health view. Self is
	// excluded — the introducer's own status is local, not mirrored.
	OnStatus func(PeerStatuses)
}

// Server exposes HTTP endpoints for introductions, heartbeats and status reports.
type Server struct {
	opts    ServerOptions
	logger  *slog.Logger
	httpSrv *http.Server

	// statusMu guards statuses, the aggregated view of every follower's
	// reported status that is handed back to all followers.
	statusMu sync.RWMutex
	statuses PeerStatuses

	stopOnce sync.Once
}

// NewServer builds a federation introducer server.
func NewServer(opts ServerOptions) (*Server, error) {
	if opts.Listener == nil {
		return nil, errors.New("federation: introducer server requires a listener")
	}
	if opts.Registry == nil {
		return nil, errors.New("federation: introducer server requires a registry")
	}
	if opts.AgeOutInterval <= 0 {
		opts.AgeOutInterval = 5 * time.Second
	}
	// Fail closed: a reachable introducer with no secret lets any host claim
	// cluster membership, which is a trust grant, not just a roster entry.
	if err := RequireSharedSecret(opts.Listener.Addr().String(), opts.SharedSecret); err != nil {
		return nil, err
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	mux := http.NewServeMux()
	srv := &Server{
		opts:     opts,
		logger:   logger,
		statuses: make(PeerStatuses),
		httpSrv: &http.Server{
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}

	mux.HandleFunc("/introductions", srv.handleIntroduction)
	mux.HandleFunc("/heartbeats", srv.handleHeartbeat)
	mux.HandleFunc("/membership", srv.handleMembership)
	mux.HandleFunc("/reports", srv.handleReport)

	return srv, nil
}

// authorized enforces the shared secret, writing 401 and returning false when
// the request may not proceed.
func (s *Server) authorized(w http.ResponseWriter, r *http.Request) bool {
	if CheckSharedSecret(r, s.opts.SharedSecret) {
		return true
	}
	writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
	return false
}

// Start begins serving HTTP and blocks until the context is canceled or an error occurs.
func (s *Server) Start(ctx context.Context) error {
	errCh := make(chan error, 1)

	go func() {
		s.logger.Info("federation introducer listening",
			"addr", s.opts.Listener.Addr().String(),
		)
		if err := s.httpSrv.Serve(s.opts.Listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	go s.ageOutLoop(ctx)

	select {
	case <-ctx.Done():
		return s.shutdown()
	case err := <-errCh:
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

// Shutdown stops the HTTP server gracefully.
func (s *Server) Shutdown() error {
	return s.shutdown()
}

func (s *Server) shutdown() error {
	var retErr error
	s.stopOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		retErr = s.httpSrv.Shutdown(ctx)
	})
	return retErr
}

type introductionRequest struct {
	Record InstanceRecord `json:"record"`
}

type membershipResponse struct {
	Self         InstanceRecord   `json:"self"`
	IntroducerID string           `json:"introducer_id"`
	Peers        []InstanceRecord `json:"peers"`
	Version      uint64           `json:"version"`
	GeneratedAt  time.Time        `json:"generated_at"`
	// Statuses is the aggregated status of every peer the introducer has heard
	// from. It rides the membership response so every follower converges on the
	// same cluster-wide view: the alert A raises reaches B without B having to
	// poll A.
	Statuses PeerStatuses `json:"statuses,omitempty"`
}

func (s *Server) handleIntroduction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if !s.authorized(w, r) {
		return
	}

	var req introductionRequest
	defer func() { _ = r.Body.Close() }()
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
		return
	}
	rec := req.Record
	if stringsTrim(rec.ID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "record.id is required"})
		return
	}
	if rec.Role == "" {
		rec.Role = RoleFollower
	}
	rec.Introducer = s.opts.Registry.Self().ID
	rec.AnnouncedAt = time.Now().UTC()

	s.opts.Registry.Upsert(rec, "introduction")
	if err := s.opts.Registry.SaveSnapshot(); err != nil {
		s.logger.Warn("federation snapshot persist failed", "error", err)
	}

	writeJSON(w, http.StatusOK, s.snapshotResponse())
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if !s.authorized(w, r) {
		return
	}

	var rec InstanceRecord
	defer func() { _ = r.Body.Close() }()
	if err := json.NewDecoder(r.Body).Decode(&rec); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
		return
	}
	if stringsTrim(rec.ID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "record.id is required"})
		return
	}

	s.opts.Registry.Upsert(rec, "heartbeat")
	writeJSON(w, http.StatusOK, s.snapshotResponse())
}

// handleReport accepts a follower's local status and folds it into the
// aggregated view that every follower receives on its next membership response.
func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if !s.authorized(w, r) {
		return
	}

	var report StatusReport
	defer func() { _ = r.Body.Close() }()
	if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid JSON"})
		return
	}
	if stringsTrim(report.InstanceID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "instance_id is required"})
		return
	}

	s.statusMu.Lock()
	s.statuses[report.InstanceID] = report.Status
	s.statusMu.Unlock()

	s.logger.Debug("federation status report",
		"instance_id", report.InstanceID,
		"targets", len(report.Status.Targets),
		"components", len(report.Status.Components),
	)

	s.notifyStatus()
	writeJSON(w, http.StatusOK, map[string]any{"status": "accepted"})
}

// aggregateStatuses returns a copy of the peer status view, including the
// introducer's own status so a follower's view is the whole cluster.
func (s *Server) aggregateStatuses() PeerStatuses {
	out := s.peerStatuses()
	if s.opts.SelfStatus != nil {
		self := s.opts.Registry.Self()
		if self.ID != "" {
			out[self.ID] = s.opts.SelfStatus()
		}
	}
	return out
}

// peerStatuses returns a copy of the reported peer statuses, excluding self.
func (s *Server) peerStatuses() PeerStatuses {
	s.statusMu.RLock()
	defer s.statusMu.RUnlock()
	out := make(PeerStatuses, len(s.statuses))
	for id, st := range s.statuses {
		out[id] = st
	}
	return out
}

// notifyStatus hands the current peer status set to the observer, if any.
func (s *Server) notifyStatus() {
	if s.opts.OnStatus == nil {
		return
	}
	s.opts.OnStatus(s.peerStatuses())
}

// forgetStatuses drops the status of peers that have left the registry.
func (s *Server) forgetStatuses(ids ...string) {
	if len(ids) == 0 {
		return
	}
	s.statusMu.Lock()
	for _, id := range ids {
		delete(s.statuses, id)
	}
	s.statusMu.Unlock()
	s.notifyStatus()
}

func (s *Server) handleMembership(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	if !s.authorized(w, r) {
		return
	}
	writeJSON(w, http.StatusOK, s.snapshotResponse())
}

func (s *Server) snapshotResponse() membershipResponse {
	snap := s.opts.Registry.Snapshot()
	return membershipResponse{
		Self:         snap.Self,
		IntroducerID: snap.IntroducerID,
		Peers:        snap.Peers,
		Version:      snap.Version,
		GeneratedAt:  snap.GeneratedAt,
		Statuses:     s.aggregateStatuses(),
	}
}

func (s *Server) ageOutLoop(ctx context.Context) {
	ticker := time.NewTicker(s.opts.AgeOutInterval)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			events := s.opts.Registry.AgeOut()
			departed := make([]string, 0, len(events))
			for _, ev := range events {
				departed = append(departed, ev.Record.ID)
				s.logger.Info("federation peer removed",
					"peer_id", ev.Record.ID,
					"reason", ev.Reason,
				)
			}
			// A peer that aged out must not keep contributing status, or its
			// last known alert would linger as if the peer were still present.
			s.forgetStatuses(departed...)
			if len(events) > 0 {
				if err := s.opts.Registry.SaveSnapshot(); err != nil {
					s.logger.Warn("federation snapshot persist failed", "error", err)
				}
			}
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
