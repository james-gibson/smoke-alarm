package mcp_faults

import (
	"testing"
	"time"
)

// The base math was chosen so that a full cluster collapse is repairable before the next
// tick, twice over. That is a design inequality, and it deserves to be a test rather than a
// sentence:
//
//	2 x repair(n) <= ClusterRotationPeriod
//
// The repair is a warm rebuild from derived state: re-hash every entry and transfer the
// golden set. It is *not* the 550 ms hashing figure alone - the transfer adds 320 ms at a
// million entries - so the number the inequality has to satisfy is 870 ms.
//
// This is the reason the tick is 5 s and not 1 s: at 1 s, twice-over holds only below ~574k
// entries, and the design would have to shrink or stop claiming the margin.

// repairPerEntry is the measured cost of rebuilding one entry: hashes and transfer.
func repairPerEntry(hashes int) time.Duration {
	return time.Duration(hashes)*hashProbe + time.Duration(bytesPerEntry)*time.Second/warmTransfer
}

// repairFor is the warm rebuild time for n entries.
func repairFor(n, hashes int) time.Duration {
	return time.Duration(n) * repairPerEntry(hashes)
}

// TestRepairFitsTwiceInATick is the inequality, at the size the ADRs quote.
func TestRepairFitsTwiceInATick(t *testing.T) {
	const (
		n      = 1_000_000
		hashes = 11
	)

	repair := repairFor(n, hashes)
	double := 2 * repair
	tick := ClusterRotationPeriod

	t.Logf("repair at %d entries: %s (hashing %s + transfer %s)",
		n, repair, time.Duration(n)*hashes*hashProbe, time.Duration(n)*bytesPerEntry*time.Second/warmTransfer)
	t.Logf("twice over: %s against a %s tick (margin %.2fx)", double, tick, float64(tick)/float64(double))

	if double > tick {
		t.Fatalf("the collapse must be fixable twice within a tick: %s > %s", double, tick)
	}

	// The margin, and the number of full rebuilds a single tick can absorb.
	margin := float64(tick) / float64(double)
	perTick := int(tick / repair)
	t.Logf("margin %.2fx; full rebuilds per tick: %d", margin, perTick)

	if margin < 2 {
		t.Errorf("the design claims twice-over, margin is only %.2fx", margin)
	}
	if perTick < 2 {
		t.Errorf("a tick should absorb at least two rebuilds, got %d", perTick)
	}

	// And the size at which the claim stops holding, which is what makes it a constraint
	// rather than a boast.
	maxN := int(float64(tick) / 2 / float64(repairPerEntry(hashes)))
	t.Logf("twice-over holds up to %d entries (%.1fM) at this tick", maxN, float64(maxN)/1e6)

	if maxN < n {
		t.Errorf("the quoted size must be inside the budget, budget is %d", maxN)
	}
}

// TestRepairBudgetAcrossSizes is the table, so the choice of tick is visible as a budget.
func TestRepairBudgetAcrossSizes(t *testing.T) {
	const hashes = 11

	t.Logf("%-10s %-10s %-12s %-10s %-12s", "entries", "repair", "per tick", "twice over?", "max tick")

	for _, n := range []int{10_000, 100_000, 1_000_000, 2_000_000, 4_000_000} {
		repair := repairFor(n, hashes)
		perTick := int(ClusterRotationPeriod / repair)
		twice := 2*repair <= ClusterRotationPeriod
		maxTick := 2 * repair // the smallest tick that still satisfies twice-over

		t.Logf("%-10d %-10s %-12d %-10v %-12s", n, repair, perTick, twice, maxTick)

		if n <= 1_000_000 && !twice {
			t.Errorf("%d entries must satisfy twice-over, repair %s", n, repair)
		}
	}

	// Sanity on the constants the inequality depends on: if either moves, the claim moves.
	if hashProbe != 50*time.Nanosecond {
		t.Errorf("the inequality assumes a 50 ns hash probe, got %s", hashProbe)
	}
	if warmTransfer != 100*1000*1000 {
		t.Errorf("the inequality assumes a 100 MB/s warm transfer, got %d", warmTransfer)
	}
}

// TestObservableCollapseSkipsDetection is why the inequality is about repair alone. A
// collapse that has to be *detected* costs a turn before the repair starts, so it spans two
// ticks; a collapse that is *observable* - a saturated filter, an evicted cohort - starts
// repairing immediately, which is the case the twice-over budget covers.
func TestObservableCollapseSkipsDetection(t *testing.T) {
	const hashes = 11
	repair := repairFor(1_000_000, hashes)

	observable := repair
	hidden := ClusterRotationPeriod + repair

	t.Logf("observable collapse: %s (fits a tick: %v)", observable, observable <= ClusterRotationPeriod)
	t.Logf("hidden collapse:     %s (fits a tick: %v)", hidden, hidden <= ClusterRotationPeriod)

	if observable > ClusterRotationPeriod {
		t.Error("an observable collapse must repair within one tick")
	}
	if hidden <= ClusterRotationPeriod {
		t.Error("a collapse needing detection must cost a turn, or the inequality is over-claimed")
	}

	// The rule: predict the collapse (hydration), or make it observable (saturation is
	// visible in the canary), so that repair never has to wait for a statistical decision.
	t.Log("either the collapse is visible, or hydration has already pre-built the replacement")
}
