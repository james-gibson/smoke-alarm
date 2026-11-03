package mcp_faults

import (
	"fmt"
	"math"
	"math/cmplx"
	"testing"
)

// The two-path design: a small, cheap side channel that updates every turn, and the exact
// state traveling a slower path. That is the right shape, and the base math decides whether
// it is torsion-free - which turns on one distinction.
//
// U(1) is the quotient R/2*pi*Z. A quotient has torsion: every rational angle has finite
// order. So a carrier in the *phase* has a period, and the period is whatever the denominator
// is - 30, for a thirtieth of a turn. The carrier in the *lift* - the accumulated angle or
// action, living in R - is torsion-free, because R is.
//
// So "already torsion-free by design" is true exactly while the state is kept in the lift.
// Project to the phase for the wire and you have reintroduced the period, and with it the
// blind spot and the replay.
//
// The side channel must follow the same rule: a filter keyed on the phase saturates after q
// turns, because there are only q distinct keys. Keyed on the lift, it keeps working.

// TestAdditiveLiftIsTorsionFree is the carrier property, stated as the definition: no
// non-zero element has finite order.
func TestAdditiveLiftIsTorsionFree(t *testing.T) {
	// Integer lifts, exact arithmetic: k*x is never zero for non-zero x.
	for _, x := range []int64{1, 3, 7, -5} {
		for k := int64(1); k <= 100; k++ {
			if x*k == 0 {
				t.Errorf("the additive lift must be torsion-free: %d * %d = 0", k, x)
			}
		}
	}

	// A real-valued action behaves the same way: summing a non-zero step never returns.
	step := 0.25
	state := 0.0
	returned := false
	for turn := 1; turn <= 1000; turn++ {
		state += step
		if state == 0 {
			returned = true
			break
		}
	}
	t.Logf("1000 additive steps of %v: state %v, returned=%v", step, state, returned)
	if returned {
		t.Error("an additive real state must never return to zero")
	}

	t.Log("R and R^n are torsion-free: the lift remembers how far it has wound")
}

// TestPhasesAreAQuotientAndHaveTorsion is the hazard: the same state projected onto the phase
// has order q, and therefore a period.
func TestPhasesAreAQuotientAndHaveTorsion(t *testing.T) {
	const q = 30 // a thirtieth of a turn, the lcm of {2,3,5}

	// The smallest non-zero angle is 2*pi/30, and it is plainly distinguishable from 1.
	smallest := cmplx.Abs(cmplx.Exp(complex(0, 2*math.Pi/q)) - 1)
	t.Logf("phase step 1/%d: |e^(i*2pi/%d) - 1| = %.3f", q, q, smallest)
	if smallest < 0.2 {
		t.Errorf("the smallest phase step should be well separated, got %.3f", smallest)
	}

	// At exactly q the phase returns, with no residue left to show for the winding.
	if got := cmplx.Abs(cmplx.Exp(complex(0, 2*math.Pi)) - 1); got > 1e-12 {
		t.Errorf("the phase must return after %d steps, got %.3e", q, got)
	}

	// Before q it does not: so the phase's period is exactly q.
	for k := 1; k < q; k++ {
		if d := cmplx.Abs(cmplx.Exp(complex(0, 2*math.Pi*float64(k)/q)) - 1); d < 1e-9 {
			t.Errorf("the phase returned early at %d", k)
		}
	}

	t.Logf("U(1) has torsion: a 1/%d phase has order exactly %d, so drift %d is invisible", q, q, q)
	t.Log("keep the state in the universal cover (the lift); the phase is a projection")
}

// TestFilterOverTheQuotientSaturates is the same hazard in the side channel. Keyed on the
// phase, the filter has only q distinct keys, so after q turns it cannot tell a new winding
// from a visited one, and it reports every subsequent turn as already seen.
func TestFilterOverTheQuotientSaturates(t *testing.T) {
	const q = 30
	filter := NewRotationFilter(64, 6)

	// Insert q windings, keyed by phase. Each is a *different* lift.
	for k := 0; k < q; k++ {
		filter.Add(fmt.Sprintf("phase-%d", k%q))
	}

	// The next winding is a new lift, but the key repeats, so it reads as seen - a false
	// return, and one that will recur forever.
	falseReturns := 0
	for k := q; k < 2*q; k++ {
		if filter.Present(fmt.Sprintf("phase-%d", k%q)) {
			falseReturns++
		}
	}

	t.Logf("after %d windings, %d/%d new lifts read as already seen", q, falseReturns, q)
	if falseReturns != q {
		t.Errorf("the phase-keyed filter must saturate completely, got %d/%d", falseReturns, q)
	}

	// Keyed on the lift, the same filter keeps discriminating: every winding is a new key.
	liftFilter := NewRotationFilter(4096, 8)
	for k := 0; k < q; k++ {
		liftFilter.Add(fmt.Sprintf("lift-%d", k))
	}
	misread := 0
	for k := q; k < 2*q; k++ {
		if liftFilter.Present(fmt.Sprintf("lift-%d", k)) {
			misread++
		}
	}
	t.Logf("keyed on the lift: %d/%d new windings misread (theory %.2f%%)",
		misread, q, liftFilter.TheoryFalsePositive()*100)

	if misread > 3 {
		t.Errorf("a lift-keyed filter should stay discriminating, misread %d/%d", misread, q)
	}
	t.Log("the side channel must be keyed on the lift, for the same reason the state is")
}

// TestFastPathIsAnExclusionChannel is the division of labor, and the direction in which a
// one-sided error is safe: absent is certain, present is a question for the slow path.
func TestFastPathIsAnExclusionChannel(t *testing.T) {
	const turns = 100_000

	filter := NewRotationFilter(1<<16, 10)
	slowQueries := 0
	missedReplays := 0

	// A replay re-presents an earlier lift; everything else is fresh.
	replayAt := map[int]int{1000: 1, 50_000: 1, 99_999: 1}

	for turn := 0; turn < turns; turn++ {
		key := fmt.Sprintf("lift-%d", turn)
		if replayOf, ok := replayAt[turn]; ok {
			key = fmt.Sprintf("lift-%d", replayOf)
			if !filter.Present(key) {
				missedReplays++
			}
		}

		// The fast path answers one of two ways: certainly absent, or maybe present.
		if filter.Present(key) {
			slowQueries++ // escalate
		}
		filter.Add(key)
	}

	resolved := 100 * (1 - float64(slowQueries)/turns)
	t.Logf("turns=%d escalations=%d (%.2f%%) resolved locally=%.2f%%",
		turns, slowQueries, 100*float64(slowQueries)/turns, resolved)

	// No false negatives: every real replay is escalated, never waved through.
	if missedReplays != 0 {
		t.Errorf("the fast path must never miss a replay, missed %d", missedReplays)
	}

	// And the escalation rate is bounded by the false-positive rate, not by the load.
	if slowQueries > turns/100 {
		t.Errorf("escalation should stay under 1%% at 10 bits/item, got %.2f%%",
			100*float64(slowQueries)/turns)
	}

	// Bandwidth: the side channel is bits per turn, the state is words per turn.
	const (
		sideBits  = 10
		stateBits = 4 * 64
	)
	t.Logf("per turn: side channel %d bits, authoritative state %d bits (%.1fx less, no round trip)",
		sideBits, stateBits, float64(stateBits)/sideBits)
}
