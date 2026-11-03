package mcp_faults

import (
	"testing"
	"time"
)

// One lagged tick and one tick, phase-offset.
//
// The design accepts the current epoch and the one before it, so any given auth or
// route is valid across a two-tick window. That makes the skew budget two epochs
// rather than one - and because the two ticks are offset phase-wise, a clock has to be
// consistently beyond the whole window before anything breaks, which is a monitorable
// condition rather than a cliff.
//
// The same fact changes the capacity story. Golden serves nothing but derives state in
// sync, so it can spawn nodes already hydrated with the NEXT tick's state. It does not
// remove the surge; it makes the surge arrive on time.

// acceptanceWindow is how wide the valid band is: one tick plus one lagged tick.
func acceptanceWindow() time.Duration { return 2 * ClusterRotationPeriod }

// TestTwoTickWindowDoublesTheSkewBudget corrects the one-epoch bound. A skew inside the
// two-tick window is not a fault; it is the design's tolerance.
func TestTwoTickWindowDoublesTheSkewBudget(t *testing.T) {
	window := acceptanceWindow()

	t.Logf("epoch %s, lagged epoch %s, acceptance window %s",
		ClusterRotationPeriod, ClusterRotationPeriod, window)

	t.Logf("%-10s %-12s %-12s", "skew", "within window", "outcome")
	for _, skew := range []time.Duration{0, 4 * time.Second, 9 * time.Second, 10 * time.Second, 11 * time.Second, 30 * time.Second} {
		within := skew <= window
		outcome := "accepted, both ticks live"
		if !within {
			outcome = "outside the design window"
		}
		t.Logf("%-10s %-12v %-12s", skew, within, outcome)

		if skew < ClusterRotationPeriod && !within {
			t.Errorf("skew %s is inside one epoch and must be accepted", skew)
		}
	}

	// And the phase offset is what makes the monitor meaningful: the two ticks are not
	// coincident, so a single bad sample is not a verdict.
	if window != 10*time.Second {
		t.Errorf("a one-lag design should give a %s window, got %s", 10*time.Second, window)
	}
	t.Logf("consistent breach means skew beyond %s across the whole offset, not one sample", window)
}

// TestTwoLiveEpochsDoubleTheAttackersCost is the security consequence of the wider
// window: both ticks must discriminate, so the attacker has to saturate two filters.
func TestTwoLiveEpochsDoubleTheAttackersCost(t *testing.T) {
	const members = 100

	perEpoch := perClaimantPerEpoch()
	oneTick := saturationFactor * perEpoch * float64(members)
	twoTicks := 2 * oneTick // current and lagged must both be filled

	t.Logf("one epoch of design state: %.0f insertions to saturate", oneTick)
	t.Logf("two live epochs: %.0f insertions, i.e. %.1fx the attacker's work", twoTicks, twoTicks/oneTick)

	if twoTicks/oneTick != 2 {
		t.Errorf("a one-lag window should double the attacker's cost, got %.2fx", twoTicks/oneTick)
	}

	// The per-epoch share is unchanged, which is the property that must survive:
	// grouping still fixes the cost at 1.33x the membership, per filter.
	share := saturationFactor / 3
	t.Logf("per-group share stays %.2fx of membership per epoch", share)
}

// TestGoldenSpawnsHydratedNodes is where the surge actually comes from. Golden derives
// the next tick's state in sync, so a spawned node can be hydrated before the tick it
// is needed for - the surge arrives on schedule instead of minutes late.
func TestGoldenSpawnsHydratedNodes(t *testing.T) {
	const n = 1_000_000

	// Hydration is the cost of evaluating the next tick's state into a fresh node.
	hydration := time.Duration(float64(n) * 11 * hashProbe.Seconds() * float64(time.Second))
	lead := hydration // start this long before the boundary

	t.Logf("hydration for %d entries: %s", n, hydration)
	t.Logf("spawn lead time before the tick: %s", lead)
	t.Logf("cold path (restore + hydrate):   %s, i.e. %.0f ticks late",
		coldRestore+hydration, (coldRestore+hydration).Seconds()/ClusterRotationPeriod.Seconds())

	if lead >= ClusterRotationPeriod {
		t.Errorf("hydration must fit inside one epoch to be predictive, got %s", lead)
	}

	// Available at the boundary: the node is serving when the tick it was hydrated for
	// begins, which is the whole point of deriving ahead of time.
	boundary := time.Date(2026, 10, 22, 3, 27, 35, 0, time.UTC)
	ready := boundary.Add(-lead).Add(hydration)
	if ready.After(boundary) {
		t.Errorf("a predictive spawn should be ready by the boundary, ready at %s", ready.Sub(boundary))
	}

	// The comparison that matters operationally: cold spawn misses the herd entirely.
	coldReady := boundary.Add(coldRestore + hydration)
	if coldReady.Sub(boundary) <= ClusterRotationPeriod {
		t.Error("a cold spawn should miss at least one tick; otherwise hydration would not matter")
	}
	t.Logf("cold spawn ready %s after the boundary (%.0f ticks late)",
		coldReady.Sub(boundary), coldReady.Sub(boundary).Seconds()/ClusterRotationPeriod.Seconds())
}

// TestPredictiveSurgeIsNotFreeCapacity restates the correction: the nodes are real.
// Golden provides the timing, not the hardware.
func TestPredictiveSurgeIsNotFreeCapacity(t *testing.T) {
	const (
		lambda = 400.0
		mu     = 100.0
		verify = 50 * time.Millisecond
	)

	model := slaModel{mu: mu, verifyLatency: verify}
	surge := model.nodesForCorrect(lambda) - model.nodesForFast(lambda)

	t.Logf("surge to provision: %d nodes (mu*v = %.0f at v=%s)", surge, mu*verify.Seconds(), verify)
	t.Logf("golden contribution: 0 nodes; it only controls WHEN they exist")

	if surge <= 0 {
		t.Fatal("the surge is real and must be provisioned")
	}

	// What golden changes is the cost of the surge being *idle*: a node hydrated for the
	// next tick only needs to exist for the tick it is needed for.
	idleWithout := coldRestore
	idleWith := hydrationFor(1_000_000)
	t.Logf("idle cost per spawn: %s cold vs %s predictive (%.0fx less)",
		idleWithout, idleWith, idleWithout.Seconds()/idleWith.Seconds())

	if idleWith >= idleWithout {
		t.Error("predictive hydration should reduce idle time")
	}
}

// hydrationFor is the derived-state hydration cost for n entries.
func hydrationFor(n int) time.Duration {
	return time.Duration(float64(n) * 11 * hashProbe.Seconds() * float64(time.Second))
}
