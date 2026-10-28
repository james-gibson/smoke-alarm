package mcp_faults

import (
	"testing"
	"time"
)

var policy = RotationPolicy{MaxGrace: time.Second}

// ref is the logical route the mirror watches; routeID is derived from it, as the
// mirror would derive it.
var ref = RouteRef{Scope: "project-a", Tool: "list-tools"}

var routeID = ref.ID(policy)

// TestVerifyRotationAnnounced is the honest case: the mirror observed the
// rotation and the node's claim matches it.
func TestVerifyRotationAnnounced(t *testing.T) {
	at := time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)
	claim := &RotationClaim{NodeID: "n1", RotatedAt: at, Grace: 400 * time.Millisecond, Prefix: "abcd1234_", RouteID: routeID}

	verdict := VerifyRotation(ref, nil, at, claim, policy)
	if verdict.State != RotationAnnounced {
		t.Errorf("expected announced, got %q", verdict.State)
	}
	if verdict.Grace != 400*time.Millisecond {
		t.Errorf("claimed grace under the ceiling must be honored: got %s", verdict.Grace)
	}
}

// TestUnannouncedRotationIsAFinding is the compromised-node case: the node
// rotated and told nobody. Silence is recorded, not assumed benign, and the
// cluster keeps its own ceiling rather than the node's silence.
func TestUnannouncedRotationIsAFinding(t *testing.T) {
	at := time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)

	verdict := VerifyRotation(ref, nil, at, nil, policy)
	if verdict.State != RotationUnannounced {
		t.Errorf("silent rotation must be a finding, got %q", verdict.State)
	}
	if verdict.Grace != policy.MaxGrace {
		t.Errorf("cluster should keep its ceiling when unannounced, got %s", verdict.Grace)
	}
}

// TestOverclaimedGraceIsCapped is the replay case: a node grants itself an hour
// of grace so a leaked stamp keeps resolving. The peer must apply the ceiling.
func TestOverclaimedGraceIsCapped(t *testing.T) {
	at := time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)
	claim := &RotationClaim{NodeID: "n1", RotatedAt: at, Grace: time.Hour, Prefix: "abcd1234_", RouteID: routeID}

	verdict := VerifyRotation(ref, nil, at, claim, policy)
	if verdict.State != RotationOverclaimedGrace {
		t.Errorf("an hour of grace against a second ceiling is an overclaim, got %q", verdict.State)
	}
	if verdict.Grace != policy.MaxGrace {
		t.Errorf("honored grace must be capped at %s, got %s", policy.MaxGrace, verdict.Grace)
	}
	if verdict.Grace >= time.Hour {
		t.Fatal("the node's claim must never be honored beyond policy")
	}
}

// TestClaimedWithoutRotation covers the inverse lie: announcing a rotation that
// the mirror never observed, which would otherwise imply a route had moved.
func TestClaimedWithoutRotation(t *testing.T) {
	claim := &RotationClaim{
		NodeID:    "n1",
		RotatedAt: time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC),
		Grace:     100 * time.Millisecond,
		RouteID:   routeID,
	}
	verdict := VerifyRotation(ref, nil, time.Time{}, claim, policy)
	if verdict.State != RotationClaimedWithoutRotate {
		t.Errorf("expected claimed-without-rotation, got %q", verdict.State)
	}
	if verdict.Grace != 0 {
		t.Errorf("no observed rotation means no grace, got %s", verdict.Grace)
	}
}

// TestMismatchedRotationTimestamp catches a claim that describes a different
// rotation than the one observed, so a node cannot backdate or borrow a rotation.
func TestMismatchedRotationTimestamp(t *testing.T) {
	observed := time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)
	claim := &RotationClaim{NodeID: "n1", RotatedAt: observed.Add(-2 * time.Second), Grace: 0, RouteID: routeID}

	verdict := VerifyRotation(ref, nil, observed, claim, policy)
	if verdict.State != RotationMismatched {
		t.Errorf("expected mismatched-rotation, got %q", verdict.State)
	}
}

// TestNodeCannotSelfGrantGrace is the in-process half: even our own code caps a
// configured grace at the ceiling. The control is the verdict above; this only
// stops the honest path from drifting.
func TestNodeCannotSelfGrantGrace(t *testing.T) {
	clock := &testClock{t: time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)}
	proxy := hashedProxyAt(t, "list-tools", clock.Now)

	proxy.GraceWindow = time.Hour // as if misconfigured, or negotiated by a liar
	stamp := proxy.IssueSkill("reader")
	proxy.Rotate()

	clock.Advance(2 * time.Second) // past the ceiling, inside the claim
	if ok, _ := proxy.ValidateCall(stamp); ok {
		t.Fatal("a node must not resolve stamps past the cluster grace ceiling")
	}
}

// TestPeerIgnoresNodeGraceClaim wires the two halves together: a node that
// overclaims grace does not extend the replay window for anyone who asks the
// mirror first. The claim is forged by hand because our own code self-caps -
// a compromised node would simply ship code that does not.
func TestPeerIgnoresNodeGraceClaim(t *testing.T) {
	clock := &testClock{t: time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)}
	node := hashedProxyAt(t, "list-tools", clock.Now)

	stamp := node.IssueSkill("reader")
	node.Rotate()

	lying := &RotationClaim{
		NodeID:    node.ProxyID,
		RotatedAt: node.LastRotation,
		Grace:     time.Hour,
		Prefix:    node.Prefix,
		RouteID:   node.RouteID(),
	}

	verdict := VerifyRotation(node.Ref(), nil, node.LastRotation, lying, policy)
	if verdict.State != RotationOverclaimedGrace {
		t.Errorf("expected overclaimed-grace, got %q", verdict.State)
	}

	// The peer's window is the verdict's, so the stamp dies on schedule.
	clock.Advance(verdict.Grace + time.Millisecond)
	if ok, _ := node.ValidateCall(stamp); ok {
		t.Error("stamp outlived the honored grace")
	}
	if ok, _ := node.ValidateCall(stamp); ok {
		t.Error("stamp revived on a second call")
	}
}

// TestRouteIDIsInvariantUnderRotation is the inverse statement: the hot path
// tumbles, the identity does not.
func TestRouteIDIsInvariantUnderRotation(t *testing.T) {
	clock := &testClock{t: time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)}
	a := hashedProxyAt(t, "list-tools", clock.Now)
	b := hashedProxyAt(t, "list-tools", clock.Now)

	if a.Prefix == b.Prefix {
		t.Fatal("fixture: two nodes should have tumbled to different hot paths")
	}
	if a.RouteID() != b.RouteID() {
		t.Errorf("nodes serving one route must share its identity: %s != %s", a.RouteID(), b.RouteID())
	}

	before := a.RouteID()
	a.Rotate()
	if a.RouteID() != before {
		t.Errorf("rotation must not change what the route is: %s != %s", a.RouteID(), before)
	}

	// A different definition is a different route, even with the same tool name.
	if ref.ID(RotationPolicy{MaxGrace: time.Hour}) == before {
		t.Error("a different grace ceiling is a different route definition")
	}
}

// TestCanonicalRouteIDIsUnambiguous checks the encoding: two definitions must not
// concatenate to the same id.
func TestCanonicalRouteIDIsUnambiguous(t *testing.T) {
	otherTool := RouteRef{Scope: "project-a", Tool: "list-tool"}
	longerCeiling := RotationPolicy{MaxGrace: time.Hour}
	otherScope := RouteRef{Scope: "project-b", Tool: "list-tools"}

	if a, b := ref.ID(policy), otherTool.ID(policy); a == b {
		t.Error("differing tool names must not collide")
	}
	if a, b := ref.ID(policy), ref.ID(longerCeiling); a == b {
		t.Error("differing grace ceilings must not collide")
	}
	if a, b := ref.ID(policy), otherScope.ID(policy); a == b {
		t.Error("the same tool name in another project must not collide")
	}
}

// TestForgedRouteIdentityIsRejected is the mirror's trust boundary: a node cannot
// answer for a route it is not serving, and a claim about another route earns no
// grace at all.
func TestForgedRouteIdentityIsRejected(t *testing.T) {
	at := time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)
	otherRoute := RouteRef{Scope: "project-a", Tool: "call-tool"}.ID(policy)

	claim := &RotationClaim{
		NodeID:    "n1",
		RotatedAt: at,
		Grace:     100 * time.Millisecond,
		RouteID:   otherRoute,
	}

	verdict := VerifyRotation(ref, nil, at, claim, policy)
	if verdict.State != RotationIdentityMismatch {
		t.Errorf("a claim about another route must be rejected, got %q", verdict.State)
	}
	if verdict.Grace != 0 {
		t.Errorf("identity mismatch grants no grace, got %s", verdict.Grace)
	}
	if verdict.RouteID != routeID {
		t.Errorf("the verdict must report the derived id, not the claimed one: %s", verdict.RouteID)
	}
}

// TestSameToolAcrossClustersIsOneRoute is the replication case: several clusters
// providing the same tool for the same project are replicas of one route, so they
// share an id while their hot paths differ.
func TestSameToolAcrossClustersIsOneRoute(t *testing.T) {
	clock := &testClock{t: time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)}

	east := hashedProxyAt(t, "list-tools", clock.Now)
	east.Scope = "project-a"
	west := hashedProxyAt(t, "list-tools", clock.Now)
	west.Scope = "project-a"

	if east.Prefix == west.Prefix {
		t.Fatal("fixture: replicas should have tumbled to different hot paths")
	}
	if east.RouteID() != west.RouteID() {
		t.Errorf("replicas of one route must share its id: %s != %s", east.RouteID(), west.RouteID())
	}

	registry := NewRouteRegistry()
	registry.Register(ref, policy, east.ProxyID)
	registry.Register(ref, policy, west.ProxyID)
	if got := len(registry.Replicas(ref, policy)); got != 2 {
		t.Errorf("expected 2 replicas of one route, got %d", got)
	}
}

// TestSameToolDifferentProjectsAreDistinctRoutes is the collision case: two
// projects exposing a tool of the same name are different routes, so a claim
// about one must not verify against the other.
func TestSameToolDifferentProjectsAreDistinctRoutes(t *testing.T) {
	clock := &testClock{t: time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)}

	a := hashedProxyAt(t, "list-tools", clock.Now)
	a.Scope = "project-a"
	b := hashedProxyAt(t, "list-tools", clock.Now)
	b.Scope = "project-b"

	if a.RouteID() == b.RouteID() {
		t.Fatal("same tool name in different projects must not share an id")
	}

	// B announces its rotation; the mirror watching A must reject it.
	b.Rotate()
	claim := b.Announcement()

	verdict := VerifyRotation(a.Ref(), nil, b.LastRotation, claim, policy)
	if verdict.State != RotationIdentityMismatch {
		t.Errorf("a cross-project claim must not verify, got %q", verdict.State)
	}
	if verdict.Grace != 0 {
		t.Errorf("cross-project claim grants no grace, got %s", verdict.Grace)
	}

	// And the mirror watching B accepts the same claim.
	if got := VerifyRotation(b.Ref(), nil, b.LastRotation, claim, policy); got.State != RotationAnnounced {
		t.Errorf("the owning project's mirror should verify it, got %q", got.State)
	}
}

// TestUnauthorizedReplicaIsRejected covers the stranger case: the right route, but
// a node the mirror does not hold as a replica of it.
func TestUnauthorizedReplicaIsRejected(t *testing.T) {
	at := time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)

	registry := NewRouteRegistry()
	registry.Register(ref, policy, "known-replica")

	stranger := &RotationClaim{
		NodeID:    "stranger",
		RotatedAt: at,
		Grace:     100 * time.Millisecond,
		RouteID:   routeID, // right route, wrong voice
	}

	verdict := VerifyRotation(ref, registry, at, stranger, policy)
	if verdict.State != RotationUnauthorizedReplica {
		t.Errorf("an unregistered node must not speak for the route, got %q", verdict.State)
	}
	if verdict.Grace != 0 {
		t.Errorf("unauthorized replica grants no grace, got %s", verdict.Grace)
	}

	known := &RotationClaim{NodeID: "known-replica", RotatedAt: at, Grace: 100 * time.Millisecond, RouteID: routeID}
	if got := VerifyRotation(ref, registry, at, known, policy); got.State != RotationAnnounced {
		t.Errorf("a registered replica should verify, got %q", got.State)
	}
}

// TestRegistryScopesReplicaMembership checks that being a replica of a route in one
// project does not authorize the same node for the same tool name in another.
func TestRegistryScopesReplicaMembership(t *testing.T) {
	other := RouteRef{Scope: "project-b", Tool: "list-tools"}

	registry := NewRouteRegistry()
	registry.Register(ref, policy, "node-1")

	if !registry.Authorized(ref, policy, "node-1") {
		t.Error("node-1 should be authorized for project-a's route")
	}
	if registry.Authorized(other, policy, "node-1") {
		t.Error("replica membership in one project must not carry into another")
	}
}
