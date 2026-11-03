package mcp_faults

import (
	"fmt"
	"testing"
)

// Two questions the side channel might be asked, and they are not the same question:
//
//	freshness - "is this rotation one of the ones issued for this tick?"
//	replay    - "has this rotation been used before, at any tick?"
//
// Keying the filter on (tick, rotation) answers the first and destroys the second: a replay
// presented at a new tick is a *new key*, so it reads as fresh. That is the same shape as the
// existing parental filter, which is rebuilt every epoch and is therefore an admission
// filter - false positive means admit a stranger - not a replay detector.
//
// Getting the polarity and the key wrong gives a filter that appears to work and answers
// neither question, which is the failure mode this test exists to make loud.

// TestTickKeyedFilterAnswersFreshnessNotReplay is the trap, measured.
func TestTickKeyedFilterAnswersFreshnessNotReplay(t *testing.T) {
	// The rotation value and the tick it was issued for.
	const (
		rotation = "rot-42"
		tick     = 17
	)

	// Keyed on (tick, rotation): each tick is a fresh key, so a replay is invisible.
	tickKeyed := NewRotationFilter(256, 8)
	tickKeyed.Add(fmt.Sprintf("%d/%s", tick, rotation))

	replayedNextTick := tickKeyed.Present(fmt.Sprintf("%d/%s", tick+1, rotation))
	t.Logf("tick-keyed: replay at tick %d reads fresh: %v", tick+1, !replayedNextTick)
	if replayedNextTick {
		t.Error("fixture: a new tick is a new key, so the replay must read fresh")
	}

	// Keyed on the rotation alone: the replay is a repeat, so it reads as seen.
	valueKeyed := NewRotationFilter(256, 8)
	valueKeyed.Add(rotation)

	replayedAnyTick := valueKeyed.Present(rotation)
	t.Logf("value-keyed: replay at any tick reads seen: %v", replayedAnyTick)
	if !replayedAnyTick {
		t.Error("a value-keyed filter must see the replay")
	}

	// So: the per-tick form is an admission side channel (helps a legitimate rotation in
	// quickly), the value-keyed form is a replay side channel (flags a repeat). Running one
	// and believing it is the other is the whole hazard.
	t.Log("per-tick key = admission; per-value key = replay; they cannot be the same filter")
}

// TestCanaryFilterWarnsSixteenTimesEarlier is the first alarm gap the side channel closes.
// A deliberately undersized filter saturates at the same *ratio* but far fewer insertions, so
// it warns long before the real filter is compromised.
func TestCanaryFilterWarnsSixteenTimesEarlier(t *testing.T) {
	const (
		fullSize   = 1000
		bitsPer    = 10
		saturation = 0.40 // the level that counts as "no longer discriminating"
	)

	crossing := func(size int) int {
		f := NewRotationFilter(size, bitsPer)
		for i := 0; i < 20*size; i++ {
			f.Add(fmt.Sprintf("p-%d", i))
			if f.TheoryFalsePositive() > saturation {
				return i + 1
			}
		}
		return -1
	}

	fullAt := crossing(fullSize)
	canaryAt := crossing(fullSize / 16)

	t.Logf("full filter (n=%d) crosses %.0f%% at %d insertions", fullSize, saturation*100, fullAt)
	t.Logf("canary    (n=%d) crosses %.0f%% at %d insertions", fullSize/16, saturation*100, canaryAt)

	if fullAt <= 0 || canaryAt <= 0 {
		t.Fatal("both filters must have a crossing point")
	}

	ratio := float64(fullAt) / float64(canaryAt)
	t.Logf("warning lead: %.1fx earlier (%d vs %d insertions)", ratio, canaryAt, fullAt)
	if ratio < 8 {
		t.Errorf("a 1/16 canary should warn at least 8x earlier, got %.1fx", ratio)
	}

	// And the baseline is not degraded: at design load both are at their design rate.
	fullDesign := NewRotationFilter(fullSize, bitsPer)
	for i := 0; i < fullSize; i++ {
		fullDesign.Add(fmt.Sprintf("d-%d", i))
	}
	canaryDesign := NewRotationFilter(fullSize/16, bitsPer)
	for i := 0; i < fullSize/16; i++ {
		canaryDesign.Add(fmt.Sprintf("d-%d", i))
	}
	t.Logf("design-load FP: full %.2f%%, canary %.2f%%",
		fullDesign.TheoryFalsePositive()*100, canaryDesign.TheoryFalsePositive()*100)

	if canaryDesign.TheoryFalsePositive() > 2*fullDesign.TheoryFalsePositive() {
		t.Error("the canary must not pay a large false-positive premium at design load")
	}
	t.Log("a canary is the same filter with a smaller n, so it warns early at no design cost")
}

// TestSideChannelDrainsTheUnexaminedBacklog is the second gap: the slow path and the mirror
// only see the escalations, not every item.
func TestSideChannelDrainsTheUnexaminedBacklog(t *testing.T) {
	const (
		items = 100_000
		bits  = 10
	)
	filter := NewRotationFilter(1<<16, bits)

	escalations := 0
	for i := 0; i < items; i++ {
		key := fmt.Sprintf("item-%d", i)
		if filter.Present(key) {
			escalations++
		}
		filter.Add(key)
	}

	examined := 100 * float64(escalations) / items
	t.Logf("items=%d escalated=%d (%.2f%%); certainly-absent=%.2f%%",
		items, escalations, examined, 100-examined)

	// The backlog that used to be 100% unexamined is now the escalation rate.
	if examined > 2 {
		t.Errorf("the slow path should see under 2%% of items, saw %.2f%%", examined)
	}
	t.Logf("unexamined work drops from 100%% to %.2f%% at %d bits/item", examined, bits)

	// What it does not close: the escalated 1% still needs the slow path, so the mirror's
	// load scales with the false-positive rate, not with the traffic.
	t.Log("it drains the backlog, it does not eliminate the slow path")
}

// TestPerRouteCanariesLocalizeAFlood is the third partial gap, and it needs one filter per
// route rather than one filter with route-prefixed keys. A shared filter only shows a flood
// once the *whole* filter is saturated - 4x its total design size - so the signal arrives at
// the same time as the damage. A per-route canary saturates after ~4x that route's expected
// volume, which is early and localized.
func TestPerRouteCanariesLocalizeAFlood(t *testing.T) {
	const (
		routes       = 8
		expectedN    = 64  // per-route volume one epoch expects
		floodInserts = 300 // the attacker's volume on one route
		floodRoute   = 3
	)

	canaries := make([]*RotationFilter, routes)
	for r := range canaries {
		canaries[r] = NewRotationFilter(expectedN, 10)
	}

	// One epoch of normal traffic on every route.
	for r := 0; r < routes; r++ {
		for i := 0; i < expectedN; i++ {
			canaries[r].Add(fmt.Sprintf("r-%d/rot-%d", r, i))
		}
	}

	// The flood lands on one route.
	for i := 0; i < floodInserts; i++ {
		canaries[floodRoute].Add(fmt.Sprintf("r-%d/flood-%d", floodRoute, i))
	}

	t.Logf("%-8s %-10s %-12s", "route", "load", "FP")

	worst, worstRoute := 0.0, -1
	for r, c := range canaries {
		fp := c.TheoryFalsePositive()
		t.Logf("%-8d %-10d %-12.3f", r, expectedN+boolToInt(r == floodRoute)*floodInserts, fp)
		if fp > worst {
			worst, worstRoute = fp, r
		}
	}

	if worstRoute != floodRoute {
		t.Errorf("the saturated canary must name the flooded route, named %d", worstRoute)
	}
	if worst < 0.4 {
		t.Errorf("a 300-insertion flood over a 64-entry canary should saturate it, worst %.3f", worst)
	}
	t.Logf("route %d saturated at FP %.2f%%; every other route stayed at design", worstRoute, worst*100)

	// Why per-route matters: a single shared 4096-entry filter is not even close to
	// saturated by 2000 insertions, so a shared key space hides the flood until the damage
	// is global.
	shared := NewRotationFilter(4096, 10)
	for i := 0; i < 2000; i++ {
		shared.Add(fmt.Sprintf("shared-%d", i))
	}
	t.Logf("shared filter after 2000 insertions: FP %.3f (no signal)", shared.TheoryFalsePositive())
	if shared.TheoryFalsePositive() > 0.1 {
		t.Error("fixture: the shared filter should still be far from saturation")
	}

	t.Log("per-route canaries localize a flood; attribution to an identity still costs the slow path")
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
