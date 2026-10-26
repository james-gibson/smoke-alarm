package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/netbind"
)

// MockRedirectMode controls how the mock OAuth redirect endpoint behaves.
type MockRedirectMode string

const (
	MockRedirectAllow MockRedirectMode = "allow"
	MockRedirectFail  MockRedirectMode = "fail"
)

// MockRedirectOptions configures a standalone mock OAuth redirect server.
type MockRedirectOptions struct {
	ListenAddr      string
	Path            string
	Mode            MockRedirectMode
	ShutdownTimeout time.Duration
}

// MockRedirectServer hosts a lightweight OAuth redirect callback endpoint.
// It is useful for local smoke tests where you need deterministic allow/fail
// behavior without relying on a full external OAuth provider callback stack.
type MockRedirectServer struct {
	opts    MockRedirectOptions
	httpSrv *http.Server

	started   atomic.Bool
	listener  net.Listener // set by BindWithRetry; nil means use ListenAndServe
	boundAddr string       // address actually bound, "" until BindWithRetry
	hits      atomic.Int64
}

// NewMockRedirectServer creates a new mock redirect server with sane defaults.
func NewMockRedirectServer(opts MockRedirectOptions) *MockRedirectServer {
	if strings.TrimSpace(opts.ListenAddr) == "" {
		opts.ListenAddr = "localhost:8877"
	}
	if strings.TrimSpace(opts.Path) == "" {
		opts.Path = "/oauth/callback"
	}
	if !strings.HasPrefix(opts.Path, "/") {
		opts.Path = "/" + opts.Path
	}
	if opts.Mode == "" {
		opts.Mode = MockRedirectAllow
	}
	if opts.ShutdownTimeout <= 0 {
		opts.ShutdownTimeout = 8 * time.Second
	}

	s := &MockRedirectServer{opts: opts}
	mux := http.NewServeMux()
	mux.HandleFunc(opts.Path, s.handleCallback)
	mux.HandleFunc("/oauth/mock/status", s.handleStatus)

	s.httpSrv = &http.Server{
		Addr:              opts.ListenAddr,
		Handler:           mux,
		ReadHeaderTimeout: 4 * time.Second,
	}
	return s
}

// Start runs the server until the context is canceled or a fatal serve error occurs.
//
// If BindWithRetry was called first, Start serves on the pre-bound listener;
// otherwise it falls back to ListenAndServe on the configured address.
func (s *MockRedirectServer) Start(ctx context.Context) error {
	if s.started.Load() {
		return nil
	}
	s.started.Store(true)

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

// BindWithRetry pre-binds the callback endpoint's listen address, scanning up
// to maxRetries higher ports on EADDRINUSE, and returns the address actually
// bound. Must be called before Start.
//
// Without this the mock either kills the process or leaves the OAuth surface
// absent while /status still says live, with no statement of which port was
// taken (smoke-alarm#53).
func (s *MockRedirectServer) BindWithRetry(maxRetries int) (string, error) {
	ln, boundAddr, err := netbind.Listen(s.opts.ListenAddr, maxRetries)
	if err != nil {
		return "", fmt.Errorf("oauth mock redirect: %w", err)
	}
	s.listener = ln
	s.boundAddr = boundAddr
	s.httpSrv.Addr = boundAddr
	return boundAddr, nil
}

// Addr returns the address the callback endpoint actually serves on: the bound
// address once BindWithRetry has run, otherwise the configured one.
func (s *MockRedirectServer) Addr() string {
	if s.boundAddr != "" {
		return s.boundAddr
	}
	return s.opts.ListenAddr
}

// Shutdown gracefully stops the server.
func (s *MockRedirectServer) Shutdown(ctx context.Context) error {
	if !s.started.Load() {
		return nil
	}
	s.started.Store(false)

	timeoutCtx, cancel := context.WithTimeout(ctx, s.opts.ShutdownTimeout)
	defer cancel()
	return s.httpSrv.Shutdown(timeoutCtx)
}

// CallbackURL returns the full callback URL served by this mock endpoint. It
// follows the bound address, so a callback that moved ports is still reachable.
func (s *MockRedirectServer) CallbackURL() string {
	return fmt.Sprintf("http://%s%s", s.Addr(), s.opts.Path)
}

// Hits returns the total number of callback requests observed.
func (s *MockRedirectServer) Hits() int64 {
	return s.hits.Load()
}

func (s *MockRedirectServer) handleCallback(w http.ResponseWriter, r *http.Request) {
	s.hits.Add(1)

	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"ok":      false,
			"error":   "method_not_allowed",
			"message": "use GET or POST for oauth redirect callback",
		})
		return
	}

	_ = r.ParseForm()

	// Common OAuth callback fields.
	code := strings.TrimSpace(firstNonEmpty(
		r.URL.Query().Get("code"),
		r.Form.Get("code"),
	))
	state := strings.TrimSpace(firstNonEmpty(
		r.URL.Query().Get("state"),
		r.Form.Get("state"),
	))
	callbackID := strings.TrimSpace(firstNonEmpty(
		r.URL.Query().Get("callback_id"),
		r.Form.Get("callback_id"),
	))

	// Keep optional fields visible for diagnostics without exposing secrets.
	errorCode := strings.TrimSpace(firstNonEmpty(
		r.URL.Query().Get("error"),
		r.Form.Get("error"),
	))
	errorDesc := strings.TrimSpace(firstNonEmpty(
		r.URL.Query().Get("error_description"),
		r.Form.Get("error_description"),
	))

	mode := strings.ToLower(strings.TrimSpace(string(s.opts.Mode)))
	if mode == "" {
		mode = string(MockRedirectAllow)
	}

	switch mode {
	case string(MockRedirectAllow):
		if callbackID == "" {
			callbackID = "mock-callback"
		}
		if code == "" {
			code = "mock_auth_code"
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":          true,
			"mode":        mode,
			"callback_id": callbackID,
			"code":        code,
			"state":       state,
			"received_at": time.Now().UTC().Format(time.RFC3339Nano),
			"path":        s.opts.Path,
		})
		return

	case string(MockRedirectFail):
		if errorCode == "" {
			errorCode = "access_denied"
		}
		if errorDesc == "" {
			errorDesc = "mock redirect endpoint configured in fail mode"
		}
		writeJSON(w, http.StatusUnauthorized, map[string]any{
			"ok":                false,
			"mode":              mode,
			"error":             errorCode,
			"error_description": errorDesc,
			"callback_id":       callbackID,
			"state":             state,
			"received_at":       time.Now().UTC().Format(time.RFC3339Nano),
			"path":              s.opts.Path,
		})
		return

	default:
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"ok":      false,
			"error":   "invalid_mode",
			"message": fmt.Sprintf("unsupported mode %q", s.opts.Mode),
		})
		return
	}
}

func (s *MockRedirectServer) handleStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"service":    "oauth-mock-redirect",
		"listen":     s.Addr(),
		"path":       s.opts.Path,
		"mode":       strings.ToLower(string(s.opts.Mode)),
		"callback":   s.CallbackURL(),
		"hits":       s.Hits(),
		"started":    s.started.Load(),
		"updated_at": time.Now().UTC().Format(time.RFC3339Nano),
	})
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// BuildCallbackURL helps construct a callback URL from base + callback_id if needed
// for external integrations (for example when prebuilding redirect links).
func BuildCallbackURL(baseCallbackURL, callbackID, state string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(baseCallbackURL))
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("callback URL must include scheme and host")
	}

	q := u.Query()
	if strings.TrimSpace(callbackID) != "" {
		q.Set("callback_id", callbackID)
	}
	if strings.TrimSpace(state) != "" {
		q.Set("state", state)
	}
	u.RawQuery = q.Encode()
	return u.String(), nil
}
