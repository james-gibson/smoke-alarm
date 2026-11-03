package mcp_faults

import (
	"testing"
	"time"
)

// TestGraceCoversNotificationLatency pins the grace window as a notification
// budget: after a node rotates, a caller still holding the old stamp resolves
// until word of the rotation could plausibly have reached it.
func TestGraceCoversNotificationLatency(t *testing.T) {
	t0 := time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)
	clock := &testClock{t: t0}
	proxy := hashedProxyAt(t, "list-tools", clock.Now)

	stamp := proxy.IssueSkill("reader")
	proxy.Rotate()

	// Mid-grace: the notification is still in flight, so the call is valid.
	clock.Advance(DefaultGraceWindow / 2)
	if ok, reason := proxy.ValidateCall(stamp); !ok || reason != "" {
		t.Fatalf("stamp inside grace must resolve, got (%v, %q)", ok, reason)
	}
	if m := proxy.GetInvalidCallMetrics(); m.TotalInvalid != 0 || m.Distance42i != 0 {
		t.Fatalf("grace-window call must leave no trace: %d invalid, %d distance", m.TotalInvalid, m.Distance42i)
	}

	// One tick past grace: the stamp is burned but still attributable.
	clock.Advance(DefaultGraceWindow/2 + time.Millisecond)
	ok, reason := proxy.ValidateCall(stamp)
	if ok {
		t.Fatal("stamp past grace must not resolve")
	}
	if reason != "burned-skill-route" {
		t.Errorf("past-grace stamp must read as burned, got %q", reason)
	}

	m := proxy.GetInvalidCallMetrics()
	if m.FuzzAttempts != 0 {
		t.Errorf("attributable staleness counted as fuzz: %d", m.FuzzAttempts)
	}
	if m.Distance42i != burnedSkillCost {
		t.Errorf("expected %d distance, got %d", burnedSkillCost, m.Distance42i)
	}
}

// TestSingleNodeRotatesWithoutClusterRotation checks that rotation is per-node:
// one node can move mid-cycle while its peers stay put, and no client is scored
// for the divergence.
func TestSingleNodeRotatesWithoutClusterRotation(t *testing.T) {
	clock := &testClock{t: time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)}

	nodes := []*ToolProxy{
		hashedProxyAt(t, "list-tools", clock.Now),
		hashedProxyAt(t, "list-tools", clock.Now),
		hashedProxyAt(t, "list-tools", clock.Now),
	}
	stamps := make([]string, len(nodes))
	for i, node := range nodes {
		stamps[i] = node.IssueSkill("reader")
	}

	// Off-cadence: 2.3s into a 5s cluster cycle, so no cluster rotation is due.
	clock.Advance(2300 * time.Millisecond)
	if got := nodes[0].Rotate(); got != 1 {
		t.Fatalf("single-node rotation should burn 1 skill, burned %d", got)
	}

	if nodes[0].Rotations != 1 || nodes[1].Rotations != 0 || nodes[2].Rotations != 0 {
		t.Errorf("rotation must not propagate as a cluster event: %d/%d/%d",
			nodes[0].Rotations, nodes[1].Rotations, nodes[2].Rotations)
	}

	// Peers are untouched: their routes still resolve cleanly.
	for _, i := range []int{1, 2} {
		if ok, reason := nodes[i].ValidateCall(stamps[i]); !ok || reason != "" {
			t.Errorf("peer %d route stopped resolving after another node rotated: (%v, %q)", i, ok, reason)
		}
		if m := nodes[i].GetInvalidCallMetrics(); m.Distance42i != 0 {
			t.Errorf("peer %d accrued %d distance from another node's rotation", i, m.Distance42i)
		}
	}

	// The rotated node's own client: valid during grace, priced after it.
	if ok, _ := nodes[0].ValidateCall(stamps[0]); !ok {
		t.Error("rotated node must still honor its stamp during grace")
	}
	clock.Advance(DefaultGraceWindow + time.Millisecond)
	if ok, reason := nodes[0].ValidateCall(stamps[0]); ok || reason != "burned-skill-route" {
		t.Errorf("stale stamp past grace should be attributed as burned, got (%v, %q)", ok, reason)
	}
}

// TestSilentRotationStillAttributes covers the client's side of a rotation the
// mirror was never told about: nothing tells this node's clients the route moved,
// so grace expires unannounced and attribution alone has to carry the call. The
// mirror still records the silence as a finding (VerifyRotation), which is why
// attribution - not grace - has to be the safety net.
func TestSilentRotationStillAttributes(t *testing.T) {
	clock := &testClock{t: time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)}
	proxy := hashedProxyAt(t, "list-tools", clock.Now)

	stamp := proxy.IssueSkill("reader")
	proxy.Rotate()

	// No notification is sent anywhere: this node rotated alone and silently.
	clock.Advance(10 * time.Minute) // far past grace, and two cluster cycles later

	ok, reason := proxy.ValidateCall(stamp)
	if ok {
		t.Fatal("a stamp must not survive an unannounced rotation")
	}
	if reason != "burned-skill-route" {
		t.Errorf("silent rotation must still read as burned, got %q", reason)
	}

	metrics := proxy.GetInvalidCallMetrics()
	if metrics.FuzzAttempts != 0 {
		t.Errorf("an unannounced rotation turned a stamped call into fuzz (%d)", metrics.FuzzAttempts)
	}
	if metrics.Distance42i != burnedSkillCost {
		t.Errorf("silent staleness should cost %d, got %d", burnedSkillCost, metrics.Distance42i)
	}
}

// TestClusterRotationStaggerStaysInsideGrace models the cluster cadence: every
// node rotates each ClusterRotationPeriod, but notifications arrive staggered.
// A client holding a peer's pre-rotation stamp must not be scored during the
// stagger.
func TestClusterRotationStaggerStaysInsideGrace(t *testing.T) {
	clock := &testClock{t: time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)}

	const nodes = 4
	proxies := make([]*ToolProxy, nodes)
	stamps := make([]string, nodes)
	for i := range proxies {
		proxies[i] = hashedProxyAt(t, "list-tools", clock.Now)
		stamps[i] = proxies[i].IssueSkill("reader")
	}

	// Cluster rotation: the mirror observes it, and word fans out staggered.
	clock.Advance(ClusterRotationPeriod)
	lag := []time.Duration{0, 120 * time.Millisecond, 300 * time.Millisecond, 800 * time.Millisecond}

	for i, proxy := range proxies {
		clock.Advance(0)
		if got := proxy.Rotate(); got != 1 {
			t.Fatalf("node %d burned %d skills, want 1", i, got)
		}
		// The notification reaches this node's clients `lag[i]` after the event.
		before := clock.Now()
		clock.t = before.Add(lag[i])
		if lag[i] < DefaultGraceWindow {
			if ok, reason := proxy.ValidateCall(stamps[i]); !ok || reason != "" {
				t.Errorf("node %d: pre-rotation stamp inside notification lag must resolve, got (%v, %q)", i, ok, reason)
			}
		}
		clock.t = before
	}

	// Every lag above fits inside grace, so a whole staggered cluster rotation
	// costs nothing: no false positives from the rotation itself.
	for i, proxy := range proxies {
		if m := proxy.GetInvalidCallMetrics(); m.Distance42i != 0 {
			t.Errorf("node %d: staggered cluster rotation charged %d distance", i, m.Distance42i)
		}
	}

	// Past grace on each node, the old stamps are attributable staleness, not fuzz.
	for i, proxy := range proxies {
		clock.t = clock.t.Add(DefaultGraceWindow + time.Millisecond)
		ok, reason := proxy.ValidateCall(stamps[i])
		if ok || reason != "burned-skill-route" {
			t.Errorf("node %d: post-grace stamp should be burned, got (%v, %q)", i, ok, reason)
		}
		if m := proxy.GetInvalidCallMetrics(); m.FuzzAttempts != 0 {
			t.Errorf("node %d: staleness counted as fuzz", i)
		}
	}
}

// TestPromotionKeepsAttribution is the failover case: the mirror is standby
// capacity, so when it steps in as the departed replica it must already hold that
// replica's stamp history - otherwise failover manufactures fuzz.
func TestPromotionKeepsAttribution(t *testing.T) {
	clock := &testClock{t: time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)}

	// Node A serves project-a, stamps a skill, then rotates.
	a := hashedProxyAt(t, "list-tools", clock.Now)
	a.Scope = "project-a"
	stamp := a.IssueSkill("reader")
	a.Rotate()
	clock.Advance(DefaultGraceWindow + time.Millisecond) // grace has expired

	// Control: a standby that never observed A's rotations inherits nothing.
	naive := hashedProxyAt(t, "list-tools", clock.Now)
	naive.Scope = "project-a"

	if ok, reason := naive.ValidateCall(stamp); ok || reason != "fuzzed-or-wrong-prefix" {
		t.Fatalf("fixture: an uninformed standby should read the stamp as fuzz, got (%v, %q)", ok, reason)
	}
	if m := naive.GetInvalidCallMetrics(); m.FuzzAttempts != 1 {
		t.Fatalf("fixture: expected 1 fuzz, got %d", m.FuzzAttempts)
	}

	// Promoted: the standby adopts the ledger it built by watching A.
	promoted := hashedProxyAt(t, "list-tools", clock.Now)
	promoted.Scope = "project-a"

	if got := promoted.AdoptLedger(a); got != 1 {
		t.Fatalf("promotion should take on 1 stamp, took %d", got)
	}

	ok, reason := promoted.ValidateCall(stamp)
	if ok {
		t.Fatal("a stamp from the departed replica must not resolve")
	}
	if reason != "burned-skill-route" {
		t.Errorf("promotion must preserve attribution, got %q", reason)
	}

	m := promoted.GetInvalidCallMetrics()
	if m.FuzzAttempts != 0 {
		t.Errorf("a promoted node must not score its predecessor's clients as fuzz (%d)", m.FuzzAttempts)
	}
	if m.Distance42i != burnedSkillCost {
		t.Errorf("expected %d distance, got %d", burnedSkillCost, m.Distance42i)
	}
}

// TestPromotionSharesRouteIdentityButNotHotPath pins the two identities apart:
// replicas agree on the route, never on the prefix, so failover looks like a
// rotation to the client and like continuity to the mirror.
func TestPromotionSharesRouteIdentityButNotHotPath(t *testing.T) {
	clock := &testClock{t: time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)}

	a := hashedProxyAt(t, "list-tools", clock.Now)
	a.Scope = "project-a"
	b := hashedProxyAt(t, "list-tools", clock.Now)
	b.Scope = "project-a"

	if a.RouteID() != b.RouteID() {
		t.Fatal("replicas must share the route identity")
	}
	if a.Prefix == b.Prefix {
		t.Fatal("fixture: replicas should hold different hot paths")
	}

	// B is already authorized to speak for the route, so promotion needs no
	// re-registration - only the ledger.
	registry := NewRouteRegistry()
	registry.Register(a.Ref(), DefaultRotationPolicy(), a.ProxyID)
	registry.Register(b.Ref(), DefaultRotationPolicy(), b.ProxyID)

	b.Rotate()
	verdict := VerifyRotation(a.Ref(), registry, b.LastRotation, b.Announcement(), DefaultRotationPolicy())
	if verdict.State != RotationAnnounced {
		t.Errorf("a promoted replica should verify as itself, got %q", verdict.State)
	}
}
