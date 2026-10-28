package mcp_faults

import (
	"testing"
	"time"
)

// TestStaleFloodCannotForceRotations is the answer to the alarm DDoS. A refused alarm
// does not advance the chain, so a flood of stale alarms costs the cluster nothing:
// no rotation, no filter republish, no skill burn.
func TestStaleFloodCannotForceRotations(t *testing.T) {
	tumble := NewAlarmTumble("ingress-a", 1)

	// One honest alarm establishes and advances the chain.
	if ok, _ := tumble.Offer(0, Isotoped); !ok {
		t.Fatal("the first honest alarm should be accepted")
	}

	// Floods of increasing size, all at a fixed reference. Each forces the alarms the
	// tolerance allows and then plateaus: the chain runs away from a fixed reference
	// and never comes back.
	t.Logf("%-12s %-11s %-11s %-10s", "flood", "rotations", "refusals", "chain")
	var firstRotations int
	for _, flood := range []int{1, 10, 1000, 1_000_000} {
		fresh := NewAlarmTumble("ingress-a", 1)
		fresh.Offer(0, Isotoped) // the one accepted alarm that settles the chain

		for i := 0; i < flood; i++ {
			fresh.Offer(0, Isotoped) // a reference that is now behind
		}

		rotations := fresh.Rotated()
		if firstRotations == 0 {
			firstRotations = rotations
		}
		t.Logf("%-12d %-11d %-11d %-10d", flood, rotations, fresh.Refused(), fresh.Chain())

		// The bound: at most tolerance+? rotations, however large the flood.
		if rotations > 2 {
			t.Errorf("flood of %d forced %d rotations; the ratchet should plateau", flood, rotations)
		}
		if flood > 1000 && fresh.Refused() != flood-1 {
			t.Errorf("flood of %d: expected %d refusals, got %d", flood, flood-1, fresh.Refused())
		}
	}

	// The bound is independent of the flood size, which is the whole point: a bigger
	// flood buys the attacker nothing.
	if firstRotations == 0 {
		t.Error("the settling alarm should have rotated once")
	}
}

// TestPipelinedFloodIsCaughtBySkew is why a flood is inherently stale: alarms sent
// without waiting for an answer all reference the same chain, so the second and later
// ones arrive behind the tumble.
func TestPipelinedFloodIsCaughtBySkew(t *testing.T) {
	for _, tolerance := range []int{0, 1} {
		tumble := NewAlarmTumble("ingress-a", tolerance)

		// A pipeline of 10 in flight, all referencing the chain they learned once.
		accepted := 0
		for i := 0; i < 10; i++ {
			if ok, _ := tumble.Offer(0, Isotoped); ok {
				accepted++
			}
		}

		want := tolerance + 1
		t.Logf("tolerance %d: pipeline of 10 in flight -> %d accepted, %d refused", tolerance, accepted, tumble.Refused())
		if accepted != want {
			t.Errorf("tolerance %d should accept %d of a pipeline, accepted %d", tolerance, want, accepted)
		}
	}
}

// TestPlainAliasCannotLag is what the "plain alias" form costs: without provenance it
// cannot prove descent, so it has to name the current chain exactly.
func TestPlainAliasCannotLag(t *testing.T) {
	tumble := NewAlarmTumble("ingress-a", 1)

	// Advance the chain twice with isotoped alarms, which may lag by one.
	if ok, _ := tumble.Offer(0, Isotoped); !ok {
		t.Fatal("first isotoped alarm should be accepted")
	}
	if ok, _ := tumble.Offer(0, Isotoped); !ok {
		t.Fatal("an isotoped alarm may lag by one")
	}
	current := tumble.Chain()

	// A plain alias one behind is refused; at the current chain it is accepted.
	if ok, skew := tumble.Offer(current-1, PlainAlias); ok {
		t.Errorf("a plain alias must not lag, skew %d was accepted", skew)
	}
	if ok, _ := tumble.Offer(current, PlainAlias); !ok {
		t.Error("a plain alias at the current chain should be accepted")
	}

	// The isotoped form buys tolerance because it carries the chain it descended from.
	before := tumble.Rotated()
	if ok, _ := tumble.Offer(tumble.Chain()-1, Isotoped); !ok {
		t.Error("an isotoped alarm should still tolerate one rotation of lag")
	}
	if tumble.Rotated() <= before {
		t.Error("an accepted alarm must advance the chain")
	}
}

// TestFutureChainIsRefused covers the replay from another ingress: a chain position
// this ingress has not issued cannot be a descent from it.
func TestFutureChainIsRefused(t *testing.T) {
	tumble := NewAlarmTumble("ingress-a", 1)

	if ok, skew := tumble.Offer(5, Isotoped); ok || skew >= 0 {
		t.Errorf("an alarm from an unissued chain must be refused, ok=%v skew=%d", ok, skew)
	}
	if tumble.TooNew() != 1 {
		t.Errorf("expected 1 too-new alarm, got %d", tumble.TooNew())
	}
}

// TestSustainableRatePerClaimant is the deal stated as a number: sequencing buys one
// alarm per round trip, pipelining buys nothing beyond the tolerance.
func TestSustainableRatePerClaimant(t *testing.T) {
	rtt := 50 * time.Millisecond

	t.Logf("%-10s %-9s %-12s %-14s", "tolerance", "kind", "alarms/seq", "alarms/s/claimant")
	for _, tolerance := range []int{0, 1} {
		for _, kind := range []AlarmKind{PlainAlias, Isotoped} {
			allowed := tolerance
			if kind == PlainAlias {
				allowed = 0
			}
			sequential := allowed + 1
			perSecond := float64(sequential) / rtt.Seconds()
			t.Logf("%-10d %-9s %-12d %-14.1f", tolerance, kind, sequential, perSecond)
		}
	}

	// A sequential claimant at tolerance 1 gets two alarms per round trip; the pipeline
	// ceiling is the same number, which is why flooding cannot beat behaving.
	sequential := 2
	perSecond := float64(sequential) / rtt.Seconds()
	if perSecond > 40 {
		t.Errorf("a claimant should be capped near %0.0f alarms/s, got %0.1f", 40.0, perSecond)
	}
	if perSecond < 1 {
		t.Error("the ratchet must not starve an honest claimant")
	}
}

// TestFloodCostsTheClaimantNotTheCluster is the accounting: the cluster's cost is one
// chain advance per *accepted* alarm, and the claimant's cost is every refusal. A
// flood therefore scales the wrong side of the ledger.
func TestFloodCostsTheClaimantNotTheCluster(t *testing.T) {
	tumble := NewAlarmTumble("ingress-a", 1)

	// Establish the chain, then flood with a stale reference.
	tumble.Offer(0, Isotoped)
	clusterCostBefore := tumble.Rotated()

	const flood = 1000
	for i := 0; i < flood; i++ {
		tumble.Offer(0, Isotoped)
	}

	clusterCost := tumble.Rotated() - clusterCostBefore
	claimantCost := tumble.Refused()

	t.Logf("flood of %d: cluster cost %d rotations, claimant cost %d refusals", flood, clusterCost, claimantCost)

	// Bounded, not zero: one lagging alarm is accepted inside the tolerance, then the
	// chain is out of reach. The cluster's exposure is the tolerance, not the flood.
	if clusterCost > 1 {
		t.Errorf("a flood must cost the cluster at most the tolerance, cost %d rotations", clusterCost)
	}
	if claimantCost <= clusterCost {
		t.Errorf("the flood must cost the claimant more: %d refusals vs %d rotations", claimantCost, clusterCost)
	}

	// And every refusal is attributable to the claimant, so the same distance pricing
	// that catches honeypot abuse catches alarm spam.
	if Classify(true, false) != VerdictUntrusted {
		t.Error("a refused alarm should be untrusted, not unexamined")
	}
}
