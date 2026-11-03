package health

// listeners.go — claiming a free port and saying which one was taken.
//
// BindWithRetry gives one listener the ability to yield to the next free port.
// ClaimListener is the same step applied to any listener the process owns, and
// records the outcome on the health server so /status carries it: the bound
// address on success, an unhealthy component naming the configured port on
// failure (smoke-alarm#53).
//
// The alternative — a fixed listener that hard-fails, or one that runs without
// its surface while /status still says live — looks identical from outside the
// process: an unreachable member with no statement of which port was lost.

import (
	"log/slog"
)

// Listener names used as /status components and as keys of
// StatusResponse.Listeners.
const (
	ListenerHealth        = "health"
	ListenerHosted        = "hosted"
	ListenerOAuthRedirect = "oauth_redirect_mock"
	ListenerFederation    = "federation"
)

// ListenerBinder is a server that claims a free port instead of failing when
// its configured one is held. Server itself, hosted.Server and
// auth.MockRedirectServer all satisfy it.
type ListenerBinder interface {
	BindWithRetry(maxRetries int) (string, error)
}

// ClaimListener binds one of the process's own listeners with retry and reports
// the address it actually bound.
//
// On success the bound address is recorded under name, so /status names the port
// the surface is on rather than the one the config asked for. On failure the
// listener is recorded as an unhealthy component carrying the error, which names
// the port: the surface is absent, and the report says which one.
func (s *Server) ClaimListener(name, configured string, srv ListenerBinder, logger *slog.Logger, maxRetries int) (string, error) {
	addr, err := srv.BindWithRetry(maxRetries)
	if s == nil {
		return addr, err
	}
	if err != nil {
		s.SetComponent(name, false, err.Error())
		if logger != nil {
			logger.Error("listener could not bind; its surface is unavailable",
				"listener", name, "configured", configured, "error", err.Error())
		}
		return "", err
	}
	s.SetListener(name, addr)
	s.SetComponent(name, true, "listening on "+addr)
	if addr != configured && logger != nil {
		logger.Warn("listener port in use, using next available",
			"listener", name, "preferred", configured, "actual", addr)
	}
	return addr, nil
}
