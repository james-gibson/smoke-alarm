package federation

import (
	"crypto/subtle"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/health"
)

// peerPrefix namespaces peer-derived entries in the local health view. A
// mirrored alert is therefore distinguishable from a locally-originated one,
// and the path names the route it traveled ("federation/a/security/...").
const peerPrefix = "federation"

// maxPeerComponents bounds how many entries one peer may contribute, so a peer
// reporting a very large target list cannot grow the local component set without
// limit. Unhealthy entries are kept first — they are the ones that matter.
const maxPeerComponents = 64

// StatusReport is a follower's own health status, sent to the introducer on the
// report cadence. It deliberately carries only locally-originated status: peer
// entries the follower has already mirrored are not re-published, which keeps a
// report the size of one alarm rather than the size of the cluster, and keeps
// mirrored names from nesting into "federation/a/federation/b/..." chains.
type StatusReport struct {
	InstanceID  string                `json:"instance_id"`
	ServiceName string                `json:"service_name"`
	ReportedAt  time.Time             `json:"reported_at"`
	Status      health.StatusResponse `json:"status"`
}

// PeerStatuses maps instance id to that peer's most recent reported status.
type PeerStatuses map[string]health.StatusResponse

// StatusSink receives peer status for rendering into the local health view.
// The health server satisfies it.
type StatusSink interface {
	SetComponent(name string, healthy bool, detail string)
	RemoveComponent(name string)
}

// PeerStatusSink renders aggregated peer status into a StatusSink and tracks the
// component names it owns so they can be withdrawn cleanly when a peer leaves or
// its status goes stale.
type PeerStatusSink struct {
	sink StatusSink

	mu    sync.Mutex
	owned map[string][]string
}

// NewPeerStatusSink wraps a StatusSink for peer-status rendering.
func NewPeerStatusSink(sink StatusSink) *PeerStatusSink {
	return &PeerStatusSink{sink: sink, owned: make(map[string][]string)}
}

// ApplyAll renders every known peer and withdraws any peer absent from the set.
// Staleness is re-evaluated on every call, so a peer that keeps heartbeating but
// stops reporting is marked degraded rather than shown as healthy forever.
func (p *PeerStatusSink) ApplyAll(statuses PeerStatuses, now time.Time, timeout time.Duration) {
	if p == nil || p.sink == nil {
		return
	}

	p.mu.Lock()
	previous := p.owned
	p.owned = make(map[string][]string, len(statuses))
	p.mu.Unlock()

	for id := range previous {
		if _, keep := statuses[id]; !keep {
			p.withdraw(previous[id])
		}
	}

	for id, st := range statuses {
		p.render(id, st, now, timeout)
	}
}

// Remove withdraws a single peer's entries, e.g. when it ages out of the
// registry. Safe to call for an unknown peer.
func (p *PeerStatusSink) Remove(peerID string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	names, ok := p.owned[peerID]
	delete(p.owned, peerID)
	p.mu.Unlock()
	if ok {
		p.withdraw(names)
	}
}

func (p *PeerStatusSink) render(peerID string, st health.StatusResponse, now time.Time, timeout time.Duration) {
	var names []string
	set := func(name string, healthy bool, detail string) {
		p.sink.SetComponent(name, healthy, detail)
		names = append(names, name)
	}

	prefix := peerPrefix + "/" + peerID

	// Roll-up first, so the peer's overall state is visible even when the
	// individual entries are trimmed below. A peer is unhealthy or in outage if
	// any target is; degraded alone is still "up".
	sum := st.Summary
	rollupHealthy := sum.Unhealthy == 0 && sum.Outage == 0
	set(prefix, rollupHealthy, fmt.Sprintf("targets %d/%d healthy, degraded=%d unhealthy=%d outage=%d, ready=%t",
		sum.Healthy, sum.Total, sum.Degraded, sum.Unhealthy, sum.Outage, st.Ready))

	for _, c := range unhealthyFirst(st.Components, func(c health.ComponentStatus) bool { return c.Healthy }) {
		if len(names) >= maxPeerComponents {
			break
		}
		set(prefix+"/"+c.Name, c.Healthy, c.Detail)
	}

	for _, t := range unhealthyFirst(st.Targets, func(t health.TargetStatus) bool { return t.State == stateHealthy }) {
		if len(names) >= maxPeerComponents {
			break
		}
		set(prefix+"/target/"+t.ID, t.State == stateHealthy, formatTarget(t))
	}

	// Re-apply staleness after rendering so a silent peer cannot look healthy.
	if timeout > 0 && !st.Now.IsZero() && now.Sub(st.Now) > timeout {
		set(prefix, false, fmt.Sprintf("no status report for %s (last %s)",
			now.Sub(st.Now).Truncate(time.Second), st.Now.UTC().Format(time.RFC3339)))
	}

	p.mu.Lock()
	p.owned[peerID] = names
	p.mu.Unlock()
}

func (p *PeerStatusSink) withdraw(names []string) {
	if p.sink == nil {
		return
	}
	for _, n := range names {
		p.sink.RemoveComponent(n)
	}
}

// stateHealthy is the canonical healthy target state reported by health.
const stateHealthy = "healthy"

// unhealthyFirst returns entries ordered unhealthy-first, stably, so trimming
// never silently drops an alert in favor of a healthy row.
func unhealthyFirst[T any](in []T, healthy func(T) bool) []T {
	out := make([]T, 0, len(in))
	for _, e := range in {
		if !healthy(e) {
			out = append(out, e)
		}
	}
	for _, e := range in {
		if healthy(e) {
			out = append(out, e)
		}
	}
	return out
}

func formatTarget(t health.TargetStatus) string {
	parts := []string{t.State}
	if t.Severity != "" {
		parts = append(parts, t.Severity)
	}
	if t.Message != "" {
		parts = append(parts, t.Message)
	}
	return strings.Join(parts, " · ")
}

// ---- authentication ---------------------------------------------------------

// secretHeader is the request header carrying the shared secret.
const secretHeader = "X-Federation-Token"

// ErrNoSharedSecret is returned when a federation endpoint that is reachable
// beyond loopback has no shared secret configured.
var ErrNoSharedSecret = fmt.Errorf("federation: a shared secret is required for non-loopback federation")

// IsLoopbackAddr reports whether addr resolves to the loopback interface. It
// accepts a URL ("http://host:port"), a bare "host:port", or a bare host. An
// unparseable or empty value is treated as non-loopback, so a malformed address
// fails closed rather than open.
func IsLoopbackAddr(addr string) bool {
	host := strings.TrimSpace(addr)
	if host == "" {
		return false
	}
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	host = strings.TrimPrefix(host, "//")
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// CheckSharedSecret reports whether a request carries the configured secret. It
// compares in constant time and treats an unset server-side secret as "no auth
// required" — which is only safe while the listener is loopback-bound, so
// RequireSharedSecret must be used to enforce that at startup.
func CheckSharedSecret(r *http.Request, want string) bool {
	if want == "" {
		return true
	}
	got := r.Header.Get(secretHeader)
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// RequireSharedSecret refuses a federation listener that other hosts can reach
// without a secret. Membership here is a trust grant (a matching ClusterID
// yields MeshMemberRung by construction), so exposing the introducer without
// authentication would let any reachable host claim the cluster.
func RequireSharedSecret(bindAddr, sharedSecret string) error {
	if IsLoopbackAddr(bindAddr) {
		return nil
	}
	if strings.TrimSpace(sharedSecret) == "" {
		return ErrNoSharedSecret
	}
	return nil
}
