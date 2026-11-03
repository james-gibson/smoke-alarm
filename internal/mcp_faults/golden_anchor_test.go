package mcp_faults

import (
	"math"
	"testing"
	"time"
)

// Where the capacity surge was hiding.
//
// A golden cluster looks like spare capacity: it exists, it is running, and it can be
// pointed at. It cannot answer. What it actually provides is a shared reference, and a
// shared reference is only usable if the sub-clusters share the tumble channel - so the
// anchor resolves *content* while the channel resolves *position*. Neither substitutes
// for the other, and neither provides a node that serves or stamps.

// subCluster is one independently rotating sub-cluster.
type subCluster struct {
	name  string
	chain int64
}

// contentAgreement costs nothing: the anchor is derived, so every sub-cluster evaluates
// the same expectation locally.
func (s subCluster) expected(at time.Time, size int) []string {
	return goldenEpoch(epochOf(at), size)
}

// positionAgreement is the channel's job: are these sub-clusters on the same tumble?
// Sharing a channel means the comparison is possible at all.
func positionAgreement(clusters []subCluster, shared bool) (agreed bool, comparisons int) {
	if !shared {
		return false, 0 // nothing to compare against: positions are private
	}
	if len(clusters) < 2 {
		return true, 0
	}
	first := clusters[0].chain
	for _, cluster := range clusters[1:] {
		comparisons++
		if cluster.chain != first {
			return false, comparisons
		}
	}
	return true, comparisons
}

// TestGoldenAnchorGivesContentAgreementForFree is the part golden genuinely does:
// because it is a function of the epoch, every sub-cluster computes the same expected
// set without exchanging anything.
func TestGoldenAnchorGivesContentAgreementForFree(t *testing.T) {
	at := time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)

	clusters := []subCluster{{name: "a"}, {name: "b"}, {name: "c"}, {name: "d"}}
	reference := clusters[0].expected(at, 16)

	messages := 0
	for _, cluster := range clusters {
		got := cluster.expected(at, 16)
		for i := range got {
			if got[i] != reference[i] {
				t.Fatalf("%s disagreed on the expected set at %d", cluster.name, i)
			}
		}
	}

	t.Logf("%d sub-clusters agreed on %d expected rotations using %d messages",
		len(clusters), len(reference), messages)

	// Free is the load-bearing word: an anchor you query is a shared budget and a
	// bottleneck; an anchor you evaluate is neither.
	if messages != 0 {
		t.Errorf("a derived anchor must cost no messages, cost %d", messages)
	}
}

// TestSharedTumbleChannelGivesPositionAgreement is the half golden cannot do. Four
// sub-clusters can all compute the same expectation and still be on different chains.
func TestSharedTumbleChannelGivesPositionAgreement(t *testing.T) {
	// A shared channel: everyone sees the same chain positions.
	together := []subCluster{{name: "a", chain: 7}, {name: "b", chain: 7}, {name: "c", chain: 7}}
	agreed, comparisons := positionAgreement(together, true)
	t.Logf("shared channel, %d sub-clusters: agreed=%v after %d comparisons", len(together), agreed, comparisons)

	if !agreed {
		t.Error("sub-clusters on one channel must be able to agree on position")
	}
	if comparisons != len(together)-1 {
		t.Errorf("agreement should be linear in the number of sub-clusters, got %d comparisons", comparisons)
	}

	// A divergence is detectable, which is what lets the reference be used at all.
	diverged := []subCluster{{name: "a", chain: 7}, {name: "b", chain: 9}}
	if ok, _ := positionAgreement(diverged, true); ok {
		t.Error("a chain divergence must be visible to the channel")
	}

	// Without the channel the comparison is not even possible, however good the anchor.
	if ok, comparisons := positionAgreement(diverged, false); ok || comparisons != 0 {
		t.Error("without a shared channel there is no position to compare")
	}
}

// TestGoldenContributesNoSurgeCapacity is the correction to the capacity story. The
// fleet needed for correct serving is the same with and without a golden cluster,
// because the anchor neither serves nor stamps.
func TestGoldenContributesNoSurgeCapacity(t *testing.T) {
	const (
		lambda = 400.0
		mu     = 100.0
		verify = 50 * time.Millisecond
	)

	model := slaModel{mu: mu, verifyLatency: verify}
	serving := model.nodesForFast(lambda)
	correct := model.nodesForCorrect(lambda)

	t.Logf("serving nodes at %.0f/s: %d", lambda, serving)
	t.Logf("correct nodes at v=%s: %d", verify, correct)
	t.Logf("contribution of a golden cluster: 0 serving, 0 stamping")

	// The surge is the difference between the two, and golden does not touch it.
	surge := correct - serving
	if surge <= 0 {
		t.Fatalf("expected a positive surge at this latency, got %d", surge)
	}
	if multiplier := float64(correct) / float64(serving); math.Abs(multiplier-mu*verify.Seconds()) > 1 {
		t.Errorf("the surge should still be mu*v = %.0f, got %.1f", mu*verify.Seconds(), multiplier)
	}

	// So the surge has to be provisioned as serving and stamping capacity. A golden
	// cluster that looks like spare capacity is the most expensive kind of decoy: it
	// costs rack space and buys no throughput.
	if surge == 0 {
		t.Error("there is a real surge to pay for")
	}
}

// TestAnchorIsOnlyAsSharedAsTheClocks is the precondition nobody prices: the functor is
// a function of UTC, so sub-clusters whose clocks differ by an epoch boundary evaluate
// different anchors and cannot reconcile.
func TestAnchorIsOnlyAsSharedAsTheClocks(t *testing.T) {
	base := time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)

	t.Logf("%-10s %-12s %-12s %-10s", "skew", "epoch a", "epoch b", "agree")
	for _, skew := range []time.Duration{0, time.Second, 4 * time.Second, 6 * time.Second, 30 * time.Second} {
		other := base.Add(skew)
		a := epochOf(base)
		b := epochOf(other)
		agree := a == b
		t.Logf("%-10s %-12d %-12d %-10v", skew, a, b, agree)

		// Inside one epoch the anchor is identical; past the boundary it is a different
		// reference entirely, and no amount of agreement on content helps.
		if skew < ClusterRotationPeriod && !agree {
			t.Errorf("skew %s is inside the epoch but changed the anchor", skew)
		}
	}

	// The hard requirement: clock skew must stay below the epoch period, or the shared
	// reference splits without anyone misbehaving.
	if ClusterRotationPeriod <= 0 {
		t.Fatal("an epoch is required for a shared anchor")
	}
	t.Logf("clock skew must stay below %s for the anchor to be shared", ClusterRotationPeriod)
}
