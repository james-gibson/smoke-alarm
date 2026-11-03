package mcp_faults

import (
	"math"
	"testing"
	"time"
)

// Churn as a maintained base load, yielding above a 50% work share.
//
// Churn is not idle traffic: it is the rotation machinery's clock. The rate it runs at
// sets the epoch, the acceptance window, and the filter's insertion budget, so yielding
// churn to work has a cost curve - and the first thing the curve says is that the yield
// band is *empty* in the shipped configuration.
//
// The reason is that two constraints pin the epoch from both sides:
//
//	window = 2 x epoch must be >= the skew budget   (ADR 0003: one lagged tick + one tick)
//	window = 2 x epoch must be <= the replay tolerance
//
// With ClusterRotationPeriod = 5s both are 10s. The rotation rate is therefore not a free
// parameter; it is exactly skew/2, and "slow the churn" is not a lever, it is a decision
// to widen the replay tolerance. Adding capacity is the only replay-safe response.

const (
	// Capacity in work units per second, and the churn capacity a nominal epoch costs.
	clusterCapacity   = 200.0
	nominalChurnUnits = 100.0
	// Queries the sidechannel sees, independent of the rotation rate.
	queryRate = 400.0
	// Per-epoch false-admission budget, from ADR 0002.
	epochBudget = 0.01
	// How long a leaked chain may remain acceptable before it is a replay problem.
	replayTolerance = 10 * time.Second
)

// nominalChurnRate is the rotation rate at nominal churn: one rotation per epoch.
func nominalChurnRate() float64 {
	return 1 / ClusterRotationPeriod.Seconds()
}

// churnFor is the yield rule: work has priority past the setpoint, so churn takes
// whatever capacity is left.
func churnFor(work float64) float64 {
	remaining := clusterCapacity - work
	if remaining < 0 {
		return 0
	}
	if remaining > nominalChurnUnits {
		return nominalChurnUnits
	}
	return remaining
}

// workShare is the controlled variable: work as a fraction of work plus churn.
func workShare(work, churn float64) float64 {
	if work+churn == 0 {
		return 1
	}
	return work / (work + churn)
}

// churnRate maps yielded churn capacity onto the rotation rate it can still afford.
func churnRate(units float64) float64 {
	if units <= 0 {
		return 0
	}
	return nominalChurnRate() * units / nominalChurnUnits
}

// epochAt is the epoch length implied by a yield of churn capacity.
func epochAt(units float64) time.Duration {
	rate := churnRate(units)
	if rate <= 0 {
		return 0
	}
	return time.Duration(1 / rate * float64(time.Second))
}

// windowAt is the acceptance window: two live epochs.
func windowAt(units float64) time.Duration {
	return 2 * epochAt(units)
}

// bitsAt is the filter budget needed when an epoch contains this many queries.
//
// Slower churn means fewer epochs per second, which means more queries land inside one
// epoch, so the per-query admission target tightens and the filter grows.
func bitsAt(units float64) int {
	epoch := epochAt(units)
	if epoch <= 0 {
		return -1
	}
	queries := int(math.Ceil(queryRate * epoch.Seconds()))
	if queries < 1 {
		queries = 1
	}
	return BitsForFalseAdmission(queries, epochBudget)
}

// replaySafe reports whether a yield keeps the window inside the replay tolerance.
//
// Zero churn is not safe: a stopped clock has no window at all, and nothing is accepted.
func replaySafe(units float64) bool {
	if churnRate(units) <= 0 {
		return false
	}
	return windowAt(units) <= replayTolerance
}

// TestFiftyPercentSetpointIsTheProvisioningRule states the rule the policy implies, and
// finds that the setpoint is simultaneously the replay ceiling.
func TestFiftyPercentSetpointIsTheProvisioningRule(t *testing.T) {
	t.Logf("%-6s %-6s %-7s %-9s %-9s %-8s", "work", "churn", "share", "epoch", "window", "safe")

	for _, work := range []float64{50, 100, 150, 200} {
		churn := churnFor(work)
		share := workShare(work, churn)
		t.Logf("%-6.0f %-6.0f %-7.2f %-9s %-9s %-8v",
			work, churn, share, epochAt(churn), windowAt(churn), replaySafe(churn))

		if work <= clusterCapacity/2 && churn != nominalChurnUnits {
			t.Errorf("work %.0f is under the setpoint but churn yielded to %.0f", work, churn)
		}
		if work > clusterCapacity/2 && churn >= nominalChurnUnits {
			t.Errorf("work %.0f is past the setpoint but churn did not yield", work)
		}
	}

	// The coincidence that makes the policy simple: at the setpoint churn equals work,
	// and the window there is exactly the replay tolerance.
	atSetpoint := churnFor(clusterCapacity / 2)
	if atSetpoint != clusterCapacity/2 {
		t.Errorf("at the setpoint churn should equal work, got %.0f vs %.0f", atSetpoint, clusterCapacity/2)
	}
	if !replaySafe(atSetpoint) {
		t.Errorf("the setpoint must be replay-safe, window %s", windowAt(atSetpoint))
	}
	if replaySafe(atSetpoint * 0.99) {
		t.Errorf("any yield past the setpoint must breach the replay tolerance, window %s",
			windowAt(atSetpoint*0.99))
	}

	t.Logf("setpoint churn == work: provisioned at 2x the work load, window %s == replay tolerance",
		windowAt(atSetpoint))
	t.Log("the setpoint and the replay floor coincide, so the yield band is empty by configuration")
}

// TestYieldingCostsBitsPerItem is the surprising half: slowing churn makes the epoch
// longer, so more queries land in one epoch and the filter must grow.
func TestYieldingCostsBitsPerItem(t *testing.T) {
	t.Logf("%-6s %-7s %-9s %-9s %-6s", "churn", "rate", "epoch", "queries", "bits")

	previous := 0
	for _, units := range []float64{100, 50, 25, 12.5, 6.25} {
		bits := bitsAt(units)
		epoch := epochAt(units)
		queries := int(math.Ceil(queryRate * epoch.Seconds()))
		t.Logf("%-6.2f %-7.2f %-9s %-9d %-6d", units, churnRate(units), epoch, queries, bits)

		if units < nominalChurnUnits && bits <= previous {
			t.Errorf("halving churn from %.2f should need more bits, %d vs %d", units, bits, previous)
		}
		previous = bits
	}

	// Each halving adds about ln2/ln(1/0.6185) = 1.44 bits per item, rounded up.
	oneHalving := bitsAt(50) - bitsAt(100)
	if oneHalving < 1 || oneHalving > 3 {
		t.Errorf("one halving should add 1-3 bits per item, added %d", oneHalving)
	}
	t.Logf("each halving of churn adds %d bits per item (theory 1.44)", oneHalving)
}

// TestTheYieldBandIsEmpty is the correction that matters: any yield at all breaches the
// replay tolerance, because the nominal epoch is already half of it.
func TestTheYieldBandIsEmpty(t *testing.T) {
	t.Logf("%-6s %-9s %-9s", "churn", "window", "safe")

	for _, units := range []float64{100, 90, 50, 25, 10, 1} {
		t.Logf("%-6.2f %-9s %-9v", units, windowAt(units), replaySafe(units))
	}

	if !replaySafe(nominalChurnUnits) {
		t.Error("the nominal configuration must be replay-safe, or the design ships broken")
	}

	// Even a ten percent yield breaches it. That is the finding: the band is empty, so
	// "slow the churn" cannot be a free response.
	if replaySafe(90) {
		t.Errorf("a 10%% yield should already breach the tolerance, window %s", windowAt(90))
	}

	// The two constraints that pin the epoch, stated as an equation.
	skewBudget := 2 * ClusterRotationPeriod
	if windowAt(nominalChurnUnits) != skewBudget {
		t.Errorf("the nominal window should equal the skew budget: %s vs %s",
			windowAt(nominalChurnUnits), skewBudget)
	}
	if skewBudget != replayTolerance {
		t.Errorf("this cluster's skew budget (%s) and replay tolerance (%s) differ, so a band exists",
			skewBudget, replayTolerance)
	}

	t.Logf("epoch is pinned: window %s = skew budget = replay tolerance", skewBudget)
	t.Log("a non-empty band requires the two to differ, which is a declared trade, not a free lever")
}

// TestAddingCapacityIsTheReplaySafeResponse is the only response that does not move the
// replay tolerance, and therefore the default.
func TestAddingCapacityIsTheReplaySafeResponse(t *testing.T) {
	const work = 150.0

	yielded := churnFor(work)
	needed := work + nominalChurnUnits
	extra := needed - clusterCapacity

	t.Logf("work %.0f: yielding gives churn %.0f, window %s, replay-safe %v",
		work, yielded, windowAt(yielded), replaySafe(yielded))
	t.Logf("adding capacity instead: %.0f units, i.e. +%.0f (%.0f%% more fleet), window unchanged at %s",
		needed, extra, 100*extra/clusterCapacity, windowAt(nominalChurnUnits))

	if replaySafe(yielded) {
		t.Error("this fixture assumes the yield breaches the tolerance")
	}
	if extra <= 0 {
		t.Error("restoring nominal churn under load must cost capacity, or the policy is free")
	}
	if windowAt(nominalChurnUnits) >= windowAt(yielded) {
		t.Error("capacity must not widen the window; that is what it is buying")
	}

	t.Logf("capacity buys back the epoch; the yield bought a %s window for %.0f units",
		windowAt(yielded), nominalChurnUnits-yielded)
}

// TestChurnShareIsMonitorable is the last operational requirement: the controlled
// variable has to be observable, or the setpoint is a hope.
func TestChurnShareIsMonitorable(t *testing.T) {
	for _, work := range []float64{60, 100, 140} {
		churn := churnFor(work)
		share := workShare(work, churn)
		rate := churnRate(churn)
		bits := bitsAt(churn)

		// Six numbers that must be exported for the policy to be enforceable.
		t.Logf("work=%3.0f churn=%3.0f share=%.2f rate=%.3f/s epoch=%s window=%s bits=%d",
			work, churn, share, rate, epochAt(churn), windowAt(churn), bits)

		if share > 0.5 && churn == nominalChurnUnits {
			t.Error("a share above the setpoint with nominal churn means the yield did not fire")
		}
		if bits <= 0 {
			t.Error("bits must remain measurable as churn moves")
		}
	}
}
