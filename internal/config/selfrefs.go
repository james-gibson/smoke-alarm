package config

// selfrefs.go — a target that names one of this process's own listeners must
// follow the port that listener actually bound.
//
// The health listener already feeds its bound port to the mDNS advertisement.
// The hosted MCP/ACP server and the OAuth mock redirect are also probed by the
// process's own targets (the hosted sample records four targets against its own
// hosted endpoints), so when one of them yields to the next free port, the
// targets that name it would otherwise probe a port this process does not own
// and report its own surface as an outage (smoke-alarm#53).

import (
	"net"
	"net/url"
	"strings"
)

// RewriteSelfReferences points targets that name one of this process's own
// listeners at the address that listener actually bound.
//
// moved maps a configured address to the address bound for it, and contains
// only the listeners that had to move. A remote target is never rewritten:
// the port alone identifies a local listener only when both hosts name this
// machine.
func RewriteSelfReferences(cfg Config, moved map[string]string) Config {
	if len(moved) == 0 {
		return cfg
	}
	for i := range cfg.Targets {
		t := &cfg.Targets[i]
		t.Endpoint = rewriteSelfEndpoint(t.Endpoint, moved)
		t.URL = rewriteSelfEndpoint(t.URL, moved)
		t.Auth.RedirectURL = rewriteSelfEndpoint(t.Auth.RedirectURL, moved)
		for j := range t.Check.HURLTests {
			t.Check.HURLTests[j].Endpoint = rewriteSelfEndpoint(t.Check.HURLTests[j].Endpoint, moved)
		}
	}
	return cfg
}

// rewriteSelfEndpoint swaps the port of one URL when it addresses a listener
// that moved, keeping the host the target chose: 0.0.0.0, 127.0.0.1 and
// localhost all address the same local surface in the samples.
func rewriteSelfEndpoint(raw string, moved map[string]string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return raw
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Host == "" {
		return raw
	}
	for configured, bound := range moved {
		if !SameListenerAddr(u.Host, configured) {
			continue
		}
		_, boundPort, splitErr := net.SplitHostPort(bound)
		if splitErr != nil {
			return raw
		}
		u.Host = net.JoinHostPort(u.Hostname(), boundPort)
		return u.String()
	}
	return raw
}

// SameListenerAddr reports whether target, a host:port a target points at, names
// the listener configured at configured.
func SameListenerAddr(target, configured string) bool {
	targetHost, targetPort, targetErr := net.SplitHostPort(target)
	configHost, configPort, configErr := net.SplitHostPort(configured)
	if targetErr != nil || configErr != nil || targetPort != configPort {
		return false
	}
	return strings.EqualFold(targetHost, configHost) || (IsLocalHost(targetHost) && IsLocalHost(configHost))
}

// IsLocalHost reports whether host names this machine or every interface on it.
func IsLocalHost(host string) bool {
	switch strings.ToLower(strings.Trim(strings.TrimSpace(host), "[]")) {
	case "", "localhost", "127.0.0.1", "0.0.0.0", "::", "::1":
		return true
	default:
		return false
	}
}
