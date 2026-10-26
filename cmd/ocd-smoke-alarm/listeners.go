package main

// listeners.go — every listener this process owns claims its port instead of
// failing on it.
//
// Only the health listener and the federation slot used to negotiate a port
// (health.BindWithRetry, federation.ClaimSlot). The hosted MCP/ACP server and
// the OAuth mock redirect bound their fixed port and died if it was held, so a
// lab that had one of those ports taken saw an unreachable member and no
// statement of which port was lost (smoke-alarm#53). The launcher used to avoid
// that by offsetting every port, which broke federation followers because they
// announce at the root base_port.
//
// The claiming itself — scan upward, record the bound address in /status, report
// a port that could not be taken as a named component — is
// health.Server.ClaimListener, so the CLI and the Gherkin suite exercise the
// same code. This file only renders the outcome for the startup log.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/james-gibson/smoke-alarm/internal/health"
)

// listenerRetries is how many ports above a configured one a listener scans
// before it reports the whole range as taken.
const listenerRetries = 10

// boundListenerSummary renders the bound addresses for a log line, so a startup
// log states where each surface ended up without reading /status.
func boundListenerSummary(hs *health.Server) string {
	if hs == nil {
		return ""
	}
	listeners := hs.Listeners()
	if len(listeners) == 0 {
		return ""
	}
	names := make([]string, 0, len(listeners))
	for name := range listeners {
		names = append(names, name)
	}
	// Deterministic order: the log line is compared in tests and by operators.
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s=%s", name, listeners[name]))
	}
	return strings.Join(parts, " ")
}
