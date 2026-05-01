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
}

// Server exposes HTTP endpoints for introductions and heartbeats.
type Server struct {
	opts    ServerOptions
	logger  *slog.Logger
	httpSrv *http.Server

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
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	mux := http.NewServeMux()
	srv := &Server{
		opts:   opts,
		logger: logger,
		httpSrv: &http.Server{
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}

	mux.HandleFunc("/introductions", srv.handleIntroduction)
	mux.HandleFunc("/heartbeats", srv.handleHeartbeat)
	mux.HandleFunc("/membership", srv.handleMembership)

	return srv, nil
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
}

func (s *Server) handleIntroduction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
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

func (s *Server) handleMembership(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "method not allowed"})
		return
	}
	writeJSON(w, http.StatusOK, s.snapshotResponse())
}

func (s *Server) snapshotResponse() membershipResponse {
	snap := s.opts.Registry.Snapshot()
	return membershipResponse(snap)
}

func (s *Server) ageOutLoop(ctx context.Context) {
	ticker := time.NewTicker(s.opts.AgeOutInterval)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			events := s.opts.Registry.AgeOut()
			for _, ev := range events {
				s.logger.Info("federation peer removed",
					"peer_id", ev.Record.ID,
					"reason", ev.Reason,
				)
			}
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
