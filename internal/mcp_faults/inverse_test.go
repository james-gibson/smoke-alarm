package mcp_faults

import (
	"fmt"
	"math/cmplx"
	"math/rand"
	"testing"
)

// The inverse run. Forward, one planted stake had to be found among filler that
// made no claim to be a flag. Here every point is a flag: identity carries no
// information at all, and the only discriminator is whether an item's mirror
// inversion is *proper* - two rotations that compose back to the claim instead of
// drifting away from it.
//
// So blue cannot ask "is this the stake". It can only ask "does this inversion
// close", and the answer is a residual. Precision comes from the residual, recall
// from whether the item is ever admitted to a slot.

// flag is one candidate: a claim and its mirror inversion.
type flag struct {
	id     string
	value  complex128 // the claim
	mirror complex128 // the mirror-inverted claim
}

// residual is how far the pair is from closing: zero means a proper inversion.
func (f flag) residual() float64 { return cmplx.Abs(f.value + f.mirror) }

// inversePopulation builds one planted flag among decoys that all claim to be
// flags. The planted mirror is exact; decoys carry a drift, which is the whole of
// what distinguishes them.
func inversePopulation(decoys int, driftMax float64, seed int64) (flag, []flag) {
	rng := rand.New(rand.NewSource(seed))

	value := complex(42, 0)
	planted := flag{id: "planted", value: value, mirror: -value}

	population := make([]flag, 0, decoys)
	for i := 0; i < decoys; i++ {
		phase := rng.Float64() * 0.01 // decoys differ in claim, not only in drift
		drift := cmplx.Rect(rng.Float64()*driftMax, rng.Float64()*2*3.141592653589793)
		v := cmplx.Rect(42, phase)
		population = append(population, flag{
			id:     fmt.Sprintf("decoy-%d", i),
			value:  v,
			mirror: -v + drift,
		})
	}
	return planted, population
}

// inverseOutcome is what blue could conclude.
type inverseOutcome struct {
	admitted     int     // items that ever occupied a slot
	coverage     float64 // admitted / population
	candidates   int     // admitted items whose inversion closed within tolerance
	plantedFound bool    // the planted flag was admitted and closed
	falsePos     int     // decoys that closed within tolerance
	precision    float64 // planted among candidates
}

// runInverse floods the queue with the whole population and checks every item that
// was ever admitted. The check is the mirror inversion, never the identity.
func runInverse(capacity int, policy DropPolicy, decoys int, driftMax, tolerance float64, seed int64) inverseOutcome {
	planted, population := inversePopulation(decoys, driftMax, seed)

	q := NewFIFO(capacity, policy)

	// Push the planted flag first, then the crowd.
	q.Push(planted.id)
	for _, f := range population {
		q.Push(f.id)
	}

	// Service drains while the flood keeps arriving, as in the forward run.
	for step := 0; step < 4*capacity+8; step++ {
		for a := 0; a < arrivalsPerStep; a++ {
			q.Push(fmt.Sprintf("late-%d-%d", step, a))
		}
		q.Pop()
	}

	closes := func(f flag) bool { return f.residual() <= tolerance }

	out := inverseOutcome{}
	if q.Observed(planted.id) {
		out.admitted++
		out.plantedFound = closes(planted)
		if out.plantedFound {
			out.candidates++
		}
	}

	for _, f := range population {
		if !q.Observed(f.id) {
			continue // never admitted, therefore never examined
		}
		out.admitted++
		if closes(f) {
			out.candidates++
			out.falsePos++
		}
	}

	out.coverage = float64(out.admitted) / float64(len(population)+1)
	if out.candidates > 0 && out.plantedFound {
		out.precision = 1 / float64(out.candidates)
	}
	return out
}

// TestInverseAllPointsAreFlags is the inverse matrix: precision from the inversion
// residual, recall from admission.
func TestInverseAllPointsAreFlags(t *testing.T) {
	const (
		decoys    = 1000
		seed      = 20261022
		capacity  = 64
		tolerance = 1e-6
	)

	t.Logf("%-10s %-9s %-11s %-9s %-9s %-9s",
		"driftMax", "admitted", "candidates", "planted", "falsePos", "precision")

	for _, driftMax := range []float64{1e-9, 1e-6, 1e-3, 1e-1} {
		out := runInverse(capacity, DropOldest, decoys, driftMax, tolerance, seed)
		t.Logf("%-10.0e %-9d %-11d %-9v %-9d %-9.4f",
			driftMax, out.admitted, out.candidates, out.plantedFound, out.falsePos, out.precision)

		// The planted flag closes exactly, whatever the crowd does.
		if !out.plantedFound {
			t.Errorf("driftMax %g: the planted flag must close", driftMax)
		}
		// Decoys close only when their drift falls inside the tolerance, so the
		// expected false positives are decoys*tolerance/driftMax - a rate, not zero.
		if driftMax > tolerance {
			expected := float64(decoys) * tolerance / driftMax
			if float64(out.falsePos) > 3*expected+1 {
				t.Errorf("driftMax %g: %d decoys closed, expected about %.1f", driftMax, out.falsePos, expected)
			}
		}
	}

	tight := runInverse(capacity, DropOldest, decoys, 1e-3, tolerance, seed)
	loose := runInverse(capacity, DropOldest, decoys, 1e-1, tolerance, seed)
	if loose.falsePos > tight.falsePos {
		t.Errorf("more drift produced more false positives: %d > %d", loose.falsePos, tight.falsePos)
	}
}

// TestQueueSizeMovesRecallNotPrecision is the inverse counterpart of the forward
// result. Queue discipline decides which items ever get a verdict; the inversion
// residual decides who is right. Sizing the queue cannot buy precision, because
// precision never involved the queue.
func TestQueueSizeMovesRecallNotPrecision(t *testing.T) {
	const (
		decoys    = 1000
		driftMax  = 1e-3
		tolerance = 1e-9
		seed      = 20261022
	)

	t.Logf("%-5s %-9s %-9s %-9s %-11s", "cap", "policy", "admitted", "coverage", "candidates")

	precisions := map[DropPolicy]float64{}
	for _, capacity := range []int{8, 64, 256} {
		for _, policy := range []DropPolicy{DropNewest, DropOldest} {
			out := runInverse(capacity, policy, decoys, driftMax, tolerance, seed)
			name := "newest"
			if policy == DropOldest {
				name = "oldest"
			}
			t.Logf("%-5d %-9s %-9d %-9.2f %-11d",
				capacity, name, out.admitted, out.coverage, out.candidates)

			if !out.plantedFound {
				t.Errorf("cap %d %s: the planted flag must be nameable once admitted", capacity, name)
			}
			precisions[policy] = out.precision

			// At a tight tolerance only the proper inversion closes.
			if out.candidates != 1 {
				t.Errorf("cap %d %s: expected 1 candidate, got %d", capacity, name, out.candidates)
			}
		}
	}

	if precisions[DropNewest] != precisions[DropOldest] {
		t.Errorf("precision differed by queue discipline: %v", precisions)
	}
}

// TestRefusedCandidatesAreNotVerdicts is the inverse failure mode: an item that is
// never admitted is not innocent, it is unexamined. Blue must never read "not a
// candidate" as "not the flag".
func TestRefusedCandidatesAreNotVerdicts(t *testing.T) {
	const (
		decoys    = 1000
		driftMax  = 1e-1
		tolerance = 1e-9
	)

	out := runInverse(8, DropNewest, decoys, driftMax, tolerance, 7)

	if out.coverage >= 1.0 {
		t.Fatalf("fixture: a refusal policy should abandon most of the population, coverage %.2f", out.coverage)
	}
	if out.admitted >= decoys+1 {
		t.Errorf("a refusal policy cannot adjudicate the whole flood: admitted %d", out.admitted)
	}

	// The unexamined remainder is the real blind spot: absence of a candidate is not
	// evidence of innocence.
	unexamined := int(float64(decoys+1) * (1 - out.coverage))
	if unexamined <= 0 {
		t.Errorf("expected an unexamined remainder, got %d", unexamined)
	}
	t.Logf("coverage %.2f: %d items never examined, therefore never cleared", out.coverage, unexamined)
}
