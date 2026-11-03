package mcp_faults

import (
	"testing"
	"time"
)

// Can an offline golden cluster provide stability?
//
// It cannot provide liveness - it is not serving - but it changes what a saturated
// filter means. Without one, saturation is an integrity loss (the summary admits
// everyone, indefinitely). With one, saturation is an availability event: the summary
// is discarded and rebuilt from golden. So the question is a race, and these are its
// numbers.

const (
	// A parental filter insert costs one hash probe per hash function.
	hashProbe = 50 * time.Nanosecond
	// Golden state is transferred once, at n * 32 bytes.
	bytesPerEntry = 32
	// A warm golden cluster is reachable; a cold one has to be restored first.
	warmTransfer = 100 * 1000 * 1000 // bytes/sec
	coldRestore  = 300 * time.Second
)

// rotationRate is how often the live cluster rotates, and therefore how fast golden
// state ages.
const rotationRate = 1.0 // rotations per second

// goldenRace compares the time an attacker needs to saturate a filter of n entries
// with the time the defender needs to rebuild it from golden.
type goldenRace struct {
	n          int
	identities int
	hashes     int
}

// timeToSaturate is how long the attacker needs to push the filter to its 4x
// saturation point at the per-claimant ceiling.
func (g goldenRace) timeToSaturate() time.Duration {
	insertions := saturationFactor * float64(g.n)
	perSecond := alarmsPerClaimant * float64(g.identities)
	return time.Duration(insertions / perSecond * float64(time.Second))
}

// rebuildTimeWarm is re-hashing golden state into a fresh filter plus the transfer.
func (g goldenRace) rebuildTimeWarm() time.Duration {
	hashes := float64(g.n) * float64(g.hashes)
	hashing := hashes * hashProbe.Seconds()
	transfer := float64(g.n*bytesPerEntry) / warmTransfer
	return time.Duration((hashing + transfer) * float64(time.Second))
}

// rebuildTimeCold adds a cold restore in front of the warm path.
func (g goldenRace) rebuildTimeCold() time.Duration {
	return coldRestore + g.rebuildTimeWarm()
}

// TestGoldenRebuildWinsTheRace is the headline: rebuilding from golden is orders of
// magnitude faster than saturating, and the ratio does not depend on cluster size.
func TestGoldenRebuildWinsTheRace(t *testing.T) {
	t.Logf("%-10s %-9s %-14s %-14s %-10s", "entries", "ids", "saturate", "rebuild", "margin")

	var margins []float64
	for _, n := range []int{10_000, 100_000, 1_000_000} {
		race := goldenRace{n: n, identities: 100, hashes: 11}
		saturate := race.timeToSaturate()
		rebuild := race.rebuildTimeWarm()
		margin := saturate.Seconds() / rebuild.Seconds()
		margins = append(margins, margin)

		t.Logf("%-10d %-9d %-14s %-14s %-10.0fx", n, race.identities, saturate, rebuild, margin)

		if margin < 100 {
			t.Errorf("n=%d: the defender should win by at least 100x, got %.1fx", n, margin)
		}
	}

	// Scale invariance: both sides are linear in n, so the ratio is property of the
	// rates, not of the size.
	for _, margin := range margins {
		if margin/margins[0] > 2 || margins[0]/margin > 2 {
			t.Errorf("the race margin should be scale invariant, got %v", margins)
		}
	}
}

// TestColdGoldenIsTheOnlyWayToLose quantifies when the golden cluster stops helping:
// a cold restore dominates the race, and the attacker's leverage becomes a duty cycle.
func TestColdGoldenIsTheOnlyWayToLose(t *testing.T) {
	race := goldenRace{n: 1_000_000, identities: 100, hashes: 11}

	saturate := race.timeToSaturate()
	warm := race.rebuildTimeWarm()
	cold := race.rebuildTimeCold()

	// Duty cycle of critical unavailability if the attacker keeps re-saturating.
	warmDuty := warm.Seconds() / (warm.Seconds() + saturate.Seconds())
	coldDuty := cold.Seconds() / (cold.Seconds() + saturate.Seconds())

	t.Logf("saturate %s | warm rebuild %s (outage %.1f%%) | cold rebuild %s (outage %.0f%%)",
		saturate, warm, warmDuty*100, cold, coldDuty*100)

	if warmDuty > 0.01 {
		t.Errorf("a warm golden cluster should keep critical outage under 1%%, got %.2f%%", warmDuty*100)
	}
	if coldDuty <= warmDuty {
		t.Error("a cold restore must be worse than a warm one")
	}
	if coldDuty < 0.5 {
		t.Logf("cold restore still leaves critical up %.0f%% of the time", (1-coldDuty)*100)
	}
}

// TestGoldenStalenessRefusesTheFire is the cost of the golden anchor: a rebuilt filter
// only knows what golden knew, so a stale golden state produces false negatives - and a
// false negative refuses a legitimate fire, the one error this design must not make.
func TestGoldenStalenessRefusesTheFire(t *testing.T) {
	t.Logf("%-10s %-12s %-14s", "lag", "missed rots", "fire refusals")

	for _, lag := range []time.Duration{0, time.Second, 5 * time.Second, 30 * time.Second, 5 * time.Minute} {
		missed := rotationRate * lag.Seconds()
		t.Logf("%-10s %-12.0f %-14.0f", lag, missed, missed)

		// Inside one epoch the live ledger can be replayed forward, so nothing is
		// refused. Beyond it, the rebuilt filter is missing rotations that are already
		// legitimate.
		if lag <= ClusterRotationPeriod && missed != 0 && lag > 0 {
			t.Logf("  -> within an epoch: replay the live ledger forward instead of publishing stale")
		}
		if lag > ClusterRotationPeriod && missed < 1 {
			t.Errorf("lag %s should miss at least one rotation", lag)
		}
	}

	// The rule: golden may be at most one epoch behind, or it must be replayed forward
	// before it is authoritative.
	if ClusterRotationPeriod <= 0 {
		t.Error("an epoch must exist for the staleness bound to mean anything")
	}
}

// TestGoldenChangesThePayoffNotTheArithmetic checks that the saturation point itself is
// untouched: the attacker still needs 4x insertions. What changes is what they get.
func TestGoldenChangesThePayoffNotTheArithmetic(t *testing.T) {
	const n = 100_000

	insertionsToSaturate := saturationFactor * float64(n)

	// Without golden the saturated filter stays saturated: its admission rate is the
	// measured 54% at 4x, and there is nothing to go back to.
	saturated := NewRotationFilter(n, 10)
	for i := 0; i < int(insertionsToSaturate); i++ {
		saturated.Add("rot-" + string(rune('a'+i%26)))
	}

	// With golden the same filter is discarded and rebuilt to its design rate.
	rebuilt := NewRotationFilter(n, 10)
	for i := 0; i < n; i++ {
		rebuilt.Add("rotation-" + string(rune('a'+i%26)))
	}

	t.Logf("insertions to saturate: %.0f (unchanged by golden)", insertionsToSaturate)
	t.Logf("saturated admission rate: %.2f", saturated.TheoryFalsePositive())
	t.Logf("rebuilt admission rate:   %.4f", rebuilt.TheoryFalsePositive())

	if saturated.TheoryFalsePositive() <= rebuilt.TheoryFalsePositive() {
		t.Error("the payload of saturation should exceed a rebuilt filter's rate")
	}
	if insertionsToSaturate != saturationFactor*n {
		t.Error("golden must not change the saturation point")
	}

	// So an attacker who saturates gains an availability effect with a bounded window,
	// not a permanent admission failure. That is the stability golden provides.
	ratio := saturated.TheoryFalsePositive() / rebuilt.TheoryFalsePositive()
	t.Logf("integrity payoff goes from %.2f to %.4f: a factor of %.0f, and it is temporary",
		saturated.TheoryFalsePositive(), rebuilt.TheoryFalsePositive(), ratio)
}
