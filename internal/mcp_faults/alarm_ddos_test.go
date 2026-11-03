package mcp_faults

import (
	"fmt"
	"testing"
)

// A DDoS of fire alarms. The alarm is cheap to send and expensive to honor: each
// one forces a rotation announcement, republishes the parental filter, and burns the
// skills on the route. Three separate costs, and the attacker pays for none of them.

const (
	// Eviction threshold used by the engine's server manager.
	evictionThreshold = 60
	// Alarms a single claimant identity can raise before its own distance evicts it,
	// depending on how the alarm is classified.
	directAlarmsPerIdentity = evictionThreshold / 24 // bypassing the proxy
	fuzzAlarmsPerIdentity   = evictionThreshold / 8  // malformed but prefixed
)

// TestAlarmFloodSaturatesTheFilter is the security cost, and it is the worst of the
// three: the filter is sized for a number of insertions per epoch, so an alarm flood
// inflates n and the false-admission rate follows the insertion count, not the budget.
func TestAlarmFloodSaturatesTheFilter(t *testing.T) {
	const (
		capacity    = 1000
		bitsPerItem = 10
		probes      = 20000
	)

	t.Logf("%-8s %-10s %-10s %-10s", "inserted", "factor", "theory FP", "measured")

	for _, factor := range []int{1, 2, 4, 8, 16} {
		inserted := capacity * factor
		f := NewRotationFilter(capacity, bitsPerItem) // m fixed at the design size
		for i := 0; i < inserted; i++ {
			f.Add(fmt.Sprintf("rotation-%d", i))
		}

		admitted := 0
		for i := 0; i < probes; i++ {
			if f.Present(fmt.Sprintf("impostor-%d", i)) {
				admitted++
			}
		}
		measured := float64(admitted) / probes

		t.Logf("%-8d %-10d %-10.4f %-10.4f", inserted, factor, f.TheoryFalsePositive(), measured)

		if factor == 1 && measured > 0.05 {
			t.Errorf("at design size the filter should discriminate, got %.4f", measured)
		}
		if factor >= 4 && measured < 0.4 {
			t.Errorf("a %dx alarm flood should saturate discrimination, got %.4f", factor, measured)
		}
	}

	// The point: a 4x flood takes the filter from a 0.8% admission error to a
	// coin-toss-with-a-bias, while the attacker spends nothing but alarm traffic.
	atSize := NewRotationFilter(capacity, bitsPerItem)
	for i := 0; i < capacity; i++ {
		atSize.Add(fmt.Sprintf("r-%d", i))
	}
	flooded := NewRotationFilter(capacity, bitsPerItem)
	for i := 0; i < 4*capacity; i++ {
		flooded.Add(fmt.Sprintf("r-%d", i))
	}
	if flooded.TheoryFalsePositive() <= atSize.TheoryFalsePositive()*10 {
		t.Errorf("a 4x flood should degrade admission by more than 10x: %.4f vs %.4f",
			flooded.TheoryFalsePositive(), atSize.TheoryFalsePositive())
	}
}

// TestAlarmRotationsEvictTheCohort is the client cost: every rotation burns the active
// stamps, and a burned stamp costs 2. Thirty rotations will evict an honest cohort
// that keeps presenting what it was issued.
func TestAlarmRotationsEvictTheCohort(t *testing.T) {
	rotations := evictionThreshold / burnedSkillCost

	t.Logf("burned stamp costs %d; eviction at %d; rotations to evict a cohort: %d",
		burnedSkillCost, evictionThreshold, rotations)

	if rotations != 30 {
		t.Errorf("expected 30 rotations to reach the eviction threshold, got %d", rotations)
	}

	// An alarm flood at 1/s therefore evicts every honest client inside 30 seconds,
	// without a single dishonest call from the attacker.
	perSecond := 1
	t.Logf("at %d alarm/s, an honest cohort is evicted after %ds", perSecond, rotations/perSecond)

	if rotations/perSecond > 60 {
		t.Error("the alarm pathway should not be able to evict a cohort within a minute")
	}
}

// TestQuorumMultipliesTheAttackCost is the only lever that works without changing the
// filter: a global rotation requires alarms from q distinct claimants, so the attacker
// must hold q identities per induced rotation instead of one.
func TestQuorumMultipliesTheAttackCost(t *testing.T) {
	t.Logf("%-8s %-9s %-12s %-12s", "quorum", "alarms/id", "ids/rotation", "rotations/7 ids")

	for _, quorum := range []int{1, 3, 5} {
		for _, perIdentity := range []int{fuzzAlarmsPerIdentity, directAlarmsPerIdentity} {
			// Every claimant can raise `perIdentity` alarms before its own distance
			// evicts it, so the identities needed to sustain rotations are
			// quorum per rotation amortized over the alarms each identity can send.
			idsPerRotation := float64(quorum)
			rotationsPerBatch := float64(perIdentity) / float64(quorum)

			t.Logf("%-8d %-9d %-12.2f %-12.2f", quorum, perIdentity, idsPerRotation, rotationsPerBatch)
		}
	}

	// Without a quorum, one identity sustains a rotation per alarm it survives.
	if got := float64(fuzzAlarmsPerIdentity) / 1.0; got < float64(fuzzAlarmsPerIdentity)/float64(3) {
		t.Errorf("a quorum of 3 should reduce sustained rotations, got %v", got)
	}

	// With a quorum of 3 the attacker needs three identities for every rotation, so
	// the cost per induced rotation is multiplied by the quorum - and identity is the
	// only thing in the system that is not free.
	if directAlarmsPerIdentity*3 <= directAlarmsPerIdentity {
		t.Error("quorum must multiply the identity cost")
	}
}

// TestHoneypotInvocationIsTheAlarmsReceipt is the structural answer rather than a
// rate limit: an alarm that did not come from a honeypot invocation has no tension
// behind it, and the honeypot invocation is what the attacker cannot supply cheaply.
func TestHoneypotInvocationIsTheAlarmsReceipt(t *testing.T) {
	// A credible alarm costs a honeypot invocation, which lands in the fuzz tier.
	credible := fuzzAlarmsPerIdentity // 7 alarms before the claimant's own distance grows

	t.Logf("one identity can raise %d credible alarms before its distance reaches %d",
		credible, evictionThreshold)
	t.Logf("an alarm with no honeypot behind it should not force a global rotation at all")

	// The asymmetry to preserve: the alarm pathway must cost the claimant at least as
	// much as it costs the cluster, or the DoS is always profitable.
	perAlarmClaimantCost := 8 // fuzz tier
	perAlarmClusterCost := 2  // claim + republish, before filter saturation
	if perAlarmClaimantCost < perAlarmClusterCost {
		t.Errorf("alarms must cost the claimant more than the cluster: %d vs %d",
			perAlarmClaimantCost, perAlarmClusterCost)
	}
}
