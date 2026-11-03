package mcp_faults

import (
	"testing"
	"time"
)

// isOdd is the example task: total, cheap to check independently, and the answer
// does not depend on which replica resolved it. Only the epoch does.
func isOdd(n int) bool { return n%2 != 0 }

// TestIsOddAcrossReplicasWithReproducibleRotation models the map-reduce case:
// several replicas provide one route, several agents invoke it, and the rotation
// they must reproduce is verifiable rather than agreed by convention.
func TestIsOddAcrossReplicasWithReproducibleRotation(t *testing.T) {
	clock := &testClock{t: time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)}
	ref := RouteRef{Scope: "lab", Tool: "isOdd"}

	east := hashedProxyAt(t, "isOdd", clock.Now)
	east.Scope = ref.Scope
	west := hashedProxyAt(t, "isOdd", clock.Now)
	west.Scope = ref.Scope

	// One route, two hot paths: the identity is what makes the answers comparable.
	if east.RouteID() != west.RouteID() {
		t.Fatal("replicas of isOdd must share the route identity")
	}
	if east.Prefix == west.Prefix {
		t.Fatal("fixture: replicas should hold different hot paths")
	}

	registry := NewRouteRegistry()
	registry.Register(ref, DefaultRotationPolicy(), east.ProxyID)
	registry.Register(ref, DefaultRotationPolicy(), west.ProxyID)

	northStamps := map[string]string{
		// Both agents hold stamps from the replica that will serve them. A stamp
		// from the other replica would be unattributable here until its ledger is
		// adopted - that is the promotion case, tested separately.
		"north":  east.IssueSkill("agent-north"),
		"north2": east.IssueSkill("agent-north-west"),
	}

	// The cadence: both replicas rotate, and the mirror verifies each for itself.
	clock.Advance(ClusterRotationPeriod)
	for name, replica := range map[string]*ToolProxy{"east": east, "west": west} {
		replica.Rotate()
		verdict := VerifyRotation(ref, registry, replica.LastRotation, replica.Announcement(), DefaultRotationPolicy())
		if verdict.State != RotationAnnounced {
			t.Errorf("%s: replica rotation did not verify: %s", name, verdict.State)
		}
	}

	// Non-conforming agents, in the order they are handled.
	type outcome struct {
		agent    string
		ok       bool
		reason   string
		counted  bool // whether a map-reduce reduction would take this answer
		expected int  // distance this call should add
	}

	cases := []outcome{
		// Refreshed inside grace: the notification was in flight.
		{"north (still in grace)", true, "", true, 0},
		// Never refreshed: attributable staleness, not an attack.
		{"north2 (past grace)", false, "burned-skill-route", false, burnedSkillCost},
		// Invented a prefix: the capability was never issued.
		{"south (fabricated)", false, "fuzzed-or-wrong-prefix", false, 8},
	}

	before := east.GetInvalidCallMetrics().Distance42i
	for _, c := range cases {
		var gotOK bool
		var gotReason string
		switch c.agent {
		case "north (still in grace)":
			gotOK, gotReason = east.ValidateCall(northStamps["north"])
		case "north2 (past grace)":
			clock.Advance(DefaultGraceWindow + time.Millisecond)
			gotOK, gotReason = east.ValidateCall(northStamps["north2"])
		default:
			gotOK, gotReason = east.ValidateCall("deadbeef_isOdd")
		}

		if gotOK != c.ok || gotReason != c.reason {
			t.Errorf("%s: got (%v, %q), want (%v, %q)", c.agent, gotOK, gotReason, c.ok, c.reason)
		}
	}

	metrics := east.GetInvalidCallMetrics()
	delta := metrics.Distance42i - before
	if want := burnedSkillCost + 8; delta != want {
		t.Errorf("two non-conforming agents should cost %d, got %d", want, delta)
	}
	if metrics.FuzzAttempts != 1 {
		t.Errorf("only the fabricated prefix is fuzz, got %d", metrics.FuzzAttempts)
	}
	if metrics.SkillRouteCalls != 1 {
		t.Errorf("exactly one call is attributable staleness, got %d", metrics.SkillRouteCalls)
	}

	// The reduce is over counted answers only, and the surviving answers agree
	// because the replicas share an identity.
	counted := 0
	results := map[bool]int{}
	for _, c := range cases {
		if !c.counted {
			continue
		}
		counted++
		results[isOdd(7)]++
	}
	if counted != 1 {
		t.Errorf("expected 1 counted answer, got %d", counted)
	}
	if len(results) != 1 {
		t.Errorf("counted answers must agree, got %v", results)
	}
	if !isOdd(7) || isOdd(8) {
		t.Error("isOdd disagrees with its own independent check")
	}
}

// TestNonConformingAgentsAreClassifiedNotRejected is the point of the example:
// the cluster does not need agents to reproduce the rotation, only to classify
// their failure to. Every class gets a distinct disposition and price, and nothing
// is silently dropped.
func TestNonConformingAgentsAreClassifiedNotRejected(t *testing.T) {
	clock := &testClock{t: time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)}
	node := hashedProxyAt(t, "isOdd", clock.Now)
	node.Scope = "lab"

	issued := node.IssueSkill("north")
	node.Rotate()
	clock.Advance(DefaultGraceWindow + time.Millisecond)

	// A skill on the current route, so a rewritten suffix lands on an active
	// capability rather than on nothing.
	node.IssueSkill("north-fresh")

	dispositions := map[string]struct {
		call   string
		reason string
	}{
		"refreshed":  {node.PrefixedToolName(), ""},
		"stale":      {issued, "burned-skill-route"},
		"rewritten":  {node.Prefix + "other-tool", "skill-route-mismatch"},
		"fabricated": {"deadbeef_isOdd", "fuzzed-or-wrong-prefix"},
		"bare":       {"isOdd", "direct-call-to-tool-bypasses-proxy"},
		"malformed":  {"isOddnoprefix", "unprefixed-malformed"},
	}

	for name, want := range dispositions {
		ok, reason := node.ValidateCall(want.call)
		if ok != (want.reason == "") || reason != want.reason {
			t.Errorf("%s: got (%v, %q), want reason %q", name, ok, reason, want.reason)
		}
	}

	m := node.GetInvalidCallMetrics()
	if m.TotalInvalid != 5 {
		t.Errorf("expected 5 classified failures, got %d", m.TotalInvalid)
	}
	if m.FuzzAttempts != 1 {
		t.Errorf("only the fabricated prefix is fuzz, got %d", m.FuzzAttempts)
	}
	// Something must price higher than rotation staleness, or the classes are
	// indistinguishable in the score.
	if m.Distance42i <= burnedSkillCost {
		t.Errorf("non-conforming classes should cost more than staleness alone: %d", m.Distance42i)
	}
}
