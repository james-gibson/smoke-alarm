package mcp_faults

import (
	"fmt"
	"math"
	"testing"
)

// Two ways to answer "has anything returned to the start?":
//
//	structure - make return impossible (a torsion-free carrier), so the answer is always no
//	memory    - test whether the state has been seen (a Bloom filter), so the answer is
//	            "no" or "maybe"
//
// They are inverses in exactly the way the braid/Bloom contrast suggests: the group precludes
// the cycle, the filter detects it. And they are substitutes, not complements - you spend
// algebra or you spend memory.
//
// The trap is the polarity of the filter's one-sided error. A Bloom filter never misses a
// real return, which sounds ideal for a detector, but its false positives invert the meaning:
// "present" here means "you have already been here", so a false positive is a **false
// accusation of replay**. In the parental filter, "present" means "a rotation I know", and
// the same error admits a stranger. Same structure, opposite security polarity.

// TestBloomAnswersReturnBraidPrecludesIt is the duality, on the real filter.
func TestBloomAnswersReturnBraidPrecludesIt(t *testing.T) {
	// Memory: a filter over visited states.
	// A deliberately small filter, so the one-sided error is visible rather than theoretical.
	seen := NewRotationFilter(16, 2)

	// Six distinct states, then a return to the second one.
	states := []string{"s0", "s1", "s2", "s3", "s4", "s5"}
	for _, s := range states {
		if seen.Present(s) {
			t.Fatalf("a fresh state must not read as returned: %s", s)
		}
		seen.Add(s)
	}

	// A real return is always caught: no false negatives, by construction.
	returned := seen.Present("s1")
	t.Logf("return to s1 detected: %v (no false negatives)", returned)
	if !returned {
		t.Fatal("a filter must always detect a real return")
	}

	// And a state never visited may still read as returned, which is the false accusation.
	falseReturns := 0
	for i := 0; i < 1000; i++ {
		if seen.Present(fmt.Sprintf("never-%d", i)) {
			falseReturns++
		}
	}
	rate := float64(falseReturns) / 1000
	t.Logf("false returns: %d/1000 (%.1f%%), theory %.1f%%", falseReturns, rate*100, seen.TheoryFalsePositive()*100)
	if falseReturns == 0 {
		t.Error("a filter this small must produce false returns, or the fixture proves nothing")
	}

	// Structure: the same question, answered by algebra, with no memory at all.
	// A torsion-free generator never returns, so "has it returned?" is always no.
	state := unit
	returnedToStart := false
	for turn := 0; turn < 100; turn++ {
		state = state.mul(sigma1)
		if state == unit {
			returnedToStart = true
			break
		}
	}
	t.Logf("torsion-free state after 100 turns: returned=%v, no filter consulted", returnedToStart)
	if returnedToStart {
		t.Error("a torsion-free carrier must never return")
	}
}

// TestFalseReturnIsAnAccusation prices the polarity. A filter used as a return detector does
// not admit a stranger - it accuses an honest node, once per query, at the false-positive
// rate.
func TestFalseReturnIsAnAccusation(t *testing.T) {
	const (
		nodes        = 1000
		turnsPerHour = 3600 / 5 // one turn per epoch, 5 s
		designBits   = 10       // the parental filter's design size
	)

	// The false-positive rate at the design size, from the filter's own formula.
	atDesign := math.Pow(0.6185, designBits)
	accusationsPerHour := float64(nodes) * turnsPerHour * atDesign

	t.Logf("%d nodes, %d turns/hour, %.2f%% false returns -> %.0f false accusations per hour",
		nodes, turnsPerHour, atDesign*100, accusationsPerHour)

	if accusationsPerHour < 1 {
		t.Fatalf("the polarity cost should be visible at design size, got %.2f", accusationsPerHour)
	}

	// To make it rare, the rate must scale with the audience: epsilon <= 1/(N * turns).
	epsilon := 1.0 / (nodes * turnsPerHour)
	bitsNeeded := math.Log(epsilon) / math.Log(0.6185)

	t.Logf("for under one accusation per hour: epsilon <= %.2e, i.e. %.0f bits per item", epsilon, bitsNeeded)

	// That is roughly triple the parental filter's design size, for a job the torsion-free
	// carrier does with no bits at all.
	if bitsNeeded < 25 {
		t.Errorf("the accusation budget should force a much larger filter, got %.0f bits", bitsNeeded)
	}
	t.Logf("design %d bits -> %.0f accusations/hour; %0.f bits -> under one", designBits, accusationsPerHour, bitsNeeded)
}

// TestTorsionFreeSubstitutesForBoth compares the three carriers on what they cost to answer
// "returned?", so the choice is a budget rather than a preference.
func TestTorsionFreeSubstitutesForBoth(t *testing.T) {
	type carrier struct {
		name string
		// memory in machine words per node, and the period it can still miss.
		words int
		blind int
		note  string
	}

	carriers := []carrier{
		{"coprime residues {2,3,5}", 3, 30, "period is the blind spot"},
		{"bloom over visited states", 1, -1, "no period, but false returns"},
		{"torsion-free matrix", 4, -1, "no period and no false returns"},
	}

	t.Logf("%-28s %-8s %-10s %s", "carrier", "words", "blind", "note")
	for _, c := range carriers {
		blind := "none"
		if c.blind > 0 {
			blind = fmt.Sprintf("%d", c.blind)
		}
		t.Logf("%-28s %-8d %-10s %s", c.name, c.words, blind, c.note)
	}

	// The ordering that makes the decision: residues are cheapest but keep a period; the
	// filter removes the period and buys one-sided error; the matrix removes both at four
	// words, which is the cost of the state that is already being carried.
	residues, bloom, matrix := carriers[0], carriers[1], carriers[2]
	if residues.words >= matrix.words {
		t.Error("the matrix should cost more words than a residue triple, or the comparison is pointless")
	}
	if bloom.blind != -1 || matrix.blind != -1 {
		t.Error("both the filter and the matrix must be free of a period")
	}
	if matrix.words != 4 {
		t.Errorf("a 2x2 integer matrix is four words, got %d", matrix.words)
	}

	// And the substitution, stated: the filter's job disappears when the carrier cannot
	// return, because the question it answers cannot arise.
	t.Log("if the state is already carried, torsion-free removes the period AND the filter")
	t.Logf("residues: %d words, still blind at %d; matrix: %d words, blind at nothing",
		residues.words, residues.blind, matrix.words)
}
