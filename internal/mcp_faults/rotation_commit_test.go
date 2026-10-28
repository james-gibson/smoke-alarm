package mcp_faults

import (
	"testing"
	"time"
)

func commitFixture(t *testing.T) (*testClock, RouteRef, *ToolProxy, *ToolProxy) {
	t.Helper()

	clock := &testClock{t: time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)}
	ref := RouteRef{Scope: "lab", Tool: "isOdd"}

	a := hashedProxyAt(t, "isOdd", clock.Now)
	a.Scope = ref.Scope
	a.ProxyID = "node-a"

	b := hashedProxyAt(t, "isOdd", clock.Now)
	b.Scope = ref.Scope
	b.ProxyID = "node-b"

	return clock, ref, a, b
}

// TestCommitAcceptsBothRedundantStores checks the base case: two stores rotate, the
// committer observes both, and both rotations land in one commit.
func TestCommitAcceptsBothRedundantStores(t *testing.T) {
	clock, ref, a, b := commitFixture(t)
	policy := DefaultRotationPolicy()

	a.IssueSkill("reader")
	b.IssueSkill("reader")

	clock.Advance(ClusterRotationPeriod)
	observed := map[string]time.Time{}
	claims := map[string]*RotationClaim{}

	for _, node := range []*ToolProxy{a, b} {
		node.Rotate()
		observed[node.ProxyID] = node.LastRotation
		claims[node.ProxyID] = node.Announcement()
	}

	commit := Commit(ref, "node-c", clock.Now(), policy, observed, claims)

	if len(commit.Rejected) != 0 {
		t.Fatalf("no store should be rejected: %+v", commit.Rejected)
	}
	for _, id := range []string{"node-a", "node-b"} {
		if !commit.Accepted(id) {
			t.Errorf("%s should be committed", id)
		}
	}
	if commit.Grace <= 0 {
		t.Errorf("a commit over honest stores should carry a positive grace, got %s", commit.Grace)
	}
	if commit.RouteID != ref.ID(policy) {
		t.Errorf("commit must name the derived route id")
	}

	// Both stores agree with their own observation.
	for _, node := range []*ToolProxy{a, b} {
		if ok, reason := commit.Check(node.ProxyID, node.LastRotation); !ok {
			t.Errorf("%s raised %q against an honest commit", node.ProxyID, reason)
		}
	}
}

// TestCommitRejectsCompromisedStoreIndependently checks that a bad store is judged
// on its own evidence: one store lying does not taint the other, and it cannot
// widen the route's grace.
func TestCommitRejectsCompromisedStoreIndependently(t *testing.T) {
	clock, ref, a, b := commitFixture(t)
	policy := DefaultRotationPolicy()

	clock.Advance(ClusterRotationPeriod)
	a.Rotate()
	b.Rotate()

	observed := map[string]time.Time{
		"node-a": a.LastRotation,
		"node-b": b.LastRotation,
	}
	claims := map[string]*RotationClaim{
		// A lies about grace; B tells the truth.
		"node-a": {NodeID: "node-a", RotatedAt: a.LastRotation, Grace: time.Hour, RouteID: ref.ID(policy), Prefix: a.Prefix},
		"node-b": b.Announcement(),
	}

	commit := Commit(ref, "node-c", clock.Now(), policy, observed, claims)

	if commit.Accepted("node-a") {
		t.Error("an overclaiming store must not be committed")
	}
	if !commit.Accepted("node-b") {
		t.Error("the honest store must still be committed")
	}
	if len(commit.Rejected) != 1 || commit.Rejected[0].NodeID != "node-a" {
		t.Fatalf("expected exactly node-a rejected, got %+v", commit.Rejected)
	}
	if commit.Rejected[0].State != RotationOverclaimedGrace {
		t.Errorf("rejection should name the state, got %q", commit.Rejected[0].State)
	}
	if commit.Grace > policy.MaxGrace {
		t.Errorf("a rejected store must not widen grace: %s", commit.Grace)
	}
}

// TestStoreCatchesCommitterDivergence is the point of having two stores: C is
// authoritative, so the only check on C is a store comparing the commit to what it
// saw itself.
func TestStoreCatchesCommitterDivergence(t *testing.T) {
	clock, ref, a, b := commitFixture(t)
	policy := DefaultRotationPolicy()

	clock.Advance(ClusterRotationPeriod)
	a.Rotate()
	b.Rotate()

	// A compromised committer commits a rotation for a that a never performed.
	invented := clock.Now().Add(time.Minute)
	rotten := RotationCommit{
		CommittedBy: "node-c",
		At:          clock.Now(),
		RouteID:     ref.ID(policy),
		Rotations:   map[string]time.Time{"node-a": invented},
		Grace:       policy.MaxGrace,
	}

	if ok, reason := rotten.Check("node-a", a.LastRotation); ok || reason != CommitMismatchedRotation {
		t.Errorf("store must reject a rotation it did not perform: (%v, %q)", ok, reason)
	}

	// Omission is equally a discrepancy: the committer silently dropping a store's
	// rotation is how a route's state gets rewritten.
	omitted := RotationCommit{
		CommittedBy: "node-c",
		At:          clock.Now(),
		RouteID:     ref.ID(policy),
		Rotations:   map[string]time.Time{},
		Grace:       0,
	}
	if ok, reason := omitted.Check("node-b", b.LastRotation); ok || reason != CommitOmittedRotation {
		t.Errorf("store must reject an omitted rotation: (%v, %q)", ok, reason)
	}

	// And inventing a rotation for a store that saw nothing.
	if ok, reason := omitted.Check("node-a", time.Time{}); !ok || reason != CommitAgrees {
		t.Errorf("agreement on absence should hold: (%v, %q)", ok, reason)
	}
	inventedForSilent := RotationCommit{Rotations: map[string]time.Time{"node-a": invented}}
	if ok, reason := inventedForSilent.Check("node-a", time.Time{}); ok || reason != CommitInventedRotation {
		t.Errorf("store must reject a rotation it never saw: (%v, %q)", ok, reason)
	}
}

// TestCommitIsDeterministic checks reproducibility: the same observations commit
// the same decision, so two committers cannot disagree by iteration order.
func TestCommitIsDeterministic(t *testing.T) {
	clock, ref, a, b := commitFixture(t)
	policy := DefaultRotationPolicy()

	clock.Advance(ClusterRotationPeriod)
	a.Rotate()
	b.Rotate()

	observed := map[string]time.Time{
		"node-a": a.LastRotation,
		"node-b": b.LastRotation,
		"node-z": {}, // observed, never announced
	}
	claims := map[string]*RotationClaim{
		"node-a": a.Announcement(),
		"node-b": b.Announcement(),
	}

	first := Commit(ref, "node-c", clock.Now(), policy, observed, claims)
	for i := 0; i < 20; i++ {
		again := Commit(ref, "node-c", clock.Now(), policy, observed, claims)
		if len(again.Rejected) != len(first.Rejected) {
			t.Fatalf("commit changed between runs: %d vs %d rejections", len(again.Rejected), len(first.Rejected))
		}
		for j := range first.Rejected {
			if again.Rejected[j] != first.Rejected[j] {
				t.Fatalf("rejection order unstable: %+v vs %+v", again.Rejected[j], first.Rejected[j])
			}
		}
		if again.Grace != first.Grace {
			t.Fatalf("grace changed between runs: %s vs %s", again.Grace, first.Grace)
		}
	}

	if len(first.Rejected) != 1 || first.Rejected[0].NodeID != "node-z" {
		t.Fatalf("the silent node should be the single rejection, got %+v", first.Rejected)
	}
	if first.Rejected[0].State != RotationUnannounced {
		t.Errorf("a silent but observed node is unannounced, got %q", first.Rejected[0].State)
	}
}

// TestCommitWithNoValidStoresCommitsNothing checks the floor: when no store verifies,
// the committer commits no rotation and no grace, rather than inventing one.
func TestCommitWithNoValidStoresCommitsNothing(t *testing.T) {
	clock, ref, a, b := commitFixture(t)
	policy := DefaultRotationPolicy()

	clock.Advance(ClusterRotationPeriod)
	a.Rotate()
	b.Rotate()

	// Both announce a rotation that contradicts what C observed.
	observed := map[string]time.Time{
		"node-a": a.LastRotation,
		"node-b": b.LastRotation,
	}
	claims := map[string]*RotationClaim{
		"node-a": {NodeID: "node-a", RotatedAt: a.LastRotation.Add(-time.Second), RouteID: ref.ID(policy)},
		"node-b": {NodeID: "node-b", RotatedAt: b.LastRotation.Add(-time.Second), RouteID: ref.ID(policy)},
	}

	commit := Commit(ref, "node-c", clock.Now(), policy, observed, claims)

	if len(commit.Rotations) != 0 {
		t.Errorf("nothing should be committed, got %+v", commit.Rotations)
	}
	if commit.Grace != 0 {
		t.Errorf("no accepted store means no grace, got %s", commit.Grace)
	}
	if len(commit.Rejected) != 2 {
		t.Errorf("both stores should be rejected, got %d", len(commit.Rejected))
	}
}
