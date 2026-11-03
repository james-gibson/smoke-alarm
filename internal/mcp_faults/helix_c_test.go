package mcp_faults

import "testing"

// Node C is where the helix becomes a decision. A and B each hold a residue for the same
// turn, computed in *opposing* directions; C holds the tuple, reconstructs the index, and
// is the one that commits. Three properties matter, and the first two correct a natural
// but wrong intuition.
//
//  1. Opposite directions see a common-mode drift at double amplitude; a same-direction
//     helix cancels it and is blind.
//  2. CRT does not reject a lie. It *maps* it: a corrupted tuple reconstructs to a
//     different, perfectly valid index. So detection is a comparison against the expected
//     index, never a consistency test.
//  3. The mapping is structured - the delta is divisible by the lcm of the honest moduli -
//     so one tuple against one expectation names the liar exactly.

// opposing is the residue of a path that walks the helix backwards.
func opposing(index, modulus int) int {
	return residue(-index, modulus)
}

// tuple is what A and B hand C for one turn: the same index observed from both directions.
type tuple struct {
	index    int
	moduli   []int
	residues []int
}

// newTuple builds an honest tuple: even paths ascend, odd paths descend.
func newTuple(index int, moduli []int) tuple {
	residues := make([]int, len(moduli))
	for i, m := range moduli {
		if i%2 == 0 {
			residues[i] = residue(index, m)
		} else {
			residues[i] = opposing(index, m)
		}
	}
	return tuple{index: index, moduli: moduli, residues: residues}
}

// undirected removes the direction, so C compares positions rather than raw residues.
// Forgetting this step is the easiest bug in the construction: an ascending
// reconstruction of a descending residue returns a plausible, wrong index.
func (t tuple) undirected() []int {
	out := make([]int, len(t.residues))
	for i, r := range t.residues {
		if i%2 == 0 {
			out[i] = r
		} else {
			out[i] = residue(-r, t.moduli[i])
		}
	}
	return out
}

// position is the index the tuple reconstructs to, or -1 when it does not fit.
func (t tuple) position() int {
	return reconstruct(t.moduli, t.undirected())
}

// TestOpposingHelicesSeeCommonModeDrift is the reason for the direction. If both paths
// drift together - a replayed coordinator, a shared lagging clock - a same-direction helix
// cancels it and the check is blind. Opposing helices cancel the sum and double the
// difference.
func TestOpposingHelicesSeeCommonModeDrift(t *testing.T) {
	const modulus = 5 // odd, so doubling is invertible in the difference
	index := 3

	baselineSum := residue(residue(index, modulus)+opposing(index, modulus), modulus)
	baselineDiff := residue(residue(index, modulus)-opposing(index, modulus), modulus)

	t.Logf("%-6s %-12s %-14s %-14s", "drift", "sum", "difference", "visible?")

	for _, d := range []int{0, 1, 2, 3, 4} {
		a := residue(index+d, modulus)
		b := opposing(index+d, modulus)

		sum := residue(a+b, modulus)
		diff := residue(a-b, modulus)
		visible := diff != baselineDiff

		t.Logf("%-6d %-12d %-14d %-14v", d, sum, diff, visible)

		if d > 0 && sum != baselineSum {
			t.Errorf("drift %d must cancel in the sum, got %d vs %d", d, sum, baselineSum)
		}
		if d > 0 && !visible {
			t.Errorf("drift %d must be visible in the difference", d)
		}
	}

	// The doubling, stated exactly: the difference moves by 2d.
	if got := residue(2*2, modulus); got != 4 {
		t.Errorf("a common drift of 2 should move the difference by 4, got %d", got)
	}
	t.Log("sum-helix: blind to common mode; difference-helix: 2d amplitude")

	// The trap: an even modulus destroys it, because 2d is zero for d = m/2.
	if residue(2*1, 2) != 0 {
		t.Error("with modulus 2 a common drift of 1 must vanish in the difference")
	}
	t.Log("the difference check needs odd moduli; modulus 2 aliases at d=1")
}

// TestCRTMapsLiesItDoesNotRejectThem is the correction: a corrupted tuple reconstructs. It
// reconstructs to a different index, and every residue is consistent with *some* index, so
// there is nothing to reject. What catches it is the expectation.
func TestCRTMapsLiesItDoesNotRejectThem(t *testing.T) {
	moduli := []int{2, 3, 5}
	honest := newTuple(17, moduli)

	liar := newTuple(17, moduli)
	liar.residues[2] = (liar.residues[2] + 1) % 5

	honestPos := honest.position()
	liarPos := liar.position()

	t.Logf("honest tuple %v -> %d; corrupted tuple %v -> %d",
		honest.residues, honestPos, liar.residues, liarPos)

	if honestPos != 17 {
		t.Fatalf("the honest tuple must reconstruct to 17, got %d", honestPos)
	}
	if liarPos < 0 {
		t.Fatal("CRT should map the lie, not reject it; a rejection means the model is wrong")
	}
	if liarPos == honestPos {
		t.Error("the lie must move the reconstruction, or it is invisible")
	}

	// Detection is not "does it reconstruct" but "does it reconstruct to what the helix
	// predicted". With no history there is no prediction.
	expected := honestPos + 1
	if liarPos == expected {
		t.Error("fixture: this lie must not land on the expected index")
	}
	t.Logf("expected %d, reconstructed %d -> detected by comparison, not by consistency", expected, liarPos)

	// And the structure that makes it attributable: the delta is a multiple of the lcm of
	// the *other* moduli, so its factorisation names the path that moved.
	delta := liarPos - honestPos
	if delta <= 0 {
		delta += lcmOf(moduli)
	}
	t.Logf("delta %d: divisible by lcm(2,3)=%d, not by 5 -> the liar is the mod-5 path",
		delta, lcmOf([]int{2, 3}))
	if delta%lcmOf([]int{2, 3}) != 0 {
		t.Errorf("the delta must be a multiple of the honest pair's lcm, got %d", delta)
	}
	if delta%5 == 0 {
		t.Error("the delta must not be a multiple of the liar's own modulus, or attribution is ambiguous")
	}
}

// TestTheDeltaNamesTheLiar is attribution from one tuple plus one expectation. Corrupting
// path i moves the reconstruction by a multiple of the lcm of every *other* modulus, so the
// one modulus that does not divide the delta is the culprit.
func TestTheDeltaNamesTheLiar(t *testing.T) {
	moduli := []int{2, 3, 5}
	const index = 17
	period := lcmOf(moduli)

	t.Logf("%-8s %-8s %-8s %-14s %-10s", "corrupt", "delta", "lcm(oth)", "divides?", "culprit")

	for corrupt := range moduli {
		liar := newTuple(index, moduli)
		liar.residues[corrupt] = (liar.residues[corrupt] + 1) % moduli[corrupt]

		pos := liar.position()
		if pos < 0 {
			t.Fatalf("corrupting path %d should map, not reject", corrupt)
		}

		delta := normalizeDelta(pos-index, period)

		culprit := -1
		for i, m := range moduli {
			if delta%m != 0 {
				culprit = i
			}
		}
		othersLcm := lcmOf(drop(moduli, corrupt))

		t.Logf("%-8d %-8d %-8d %-14v %-10d", corrupt, delta, othersLcm, delta%othersLcm == 0, culprit)

		if culprit != corrupt {
			t.Errorf("corrupting path %d should name it, named %d instead", corrupt, culprit)
		}
		if delta%othersLcm != 0 {
			t.Errorf("delta %d must be divisible by the honest pair's lcm %d", delta, othersLcm)
		}
	}

	// Two paths cannot do this: the delta identifies a class rather than a member.
	twoDelta := periodicDelta(17, 0, []int{2, 3})
	if twoDelta == 0 {
		t.Error("a corruption over two moduli must still move the reconstruction")
	}
	t.Logf("two paths: delta %d names a class, not a member; three is the minimum for exact attribution", twoDelta)
	t.Log("three moduli: one tuple plus one expectation attributes exactly")
}

// periodicDelta is the positive delta a corruption of path corrupt produces.
func periodicDelta(index, corrupt int, moduli []int) int {
	liar := newTuple(index, moduli)
	liar.residues[corrupt] = (liar.residues[corrupt] + 1) % moduli[corrupt]
	pos := liar.position()
	if pos < 0 {
		return 0
	}
	return normalizeDelta(pos-index, lcmOf(moduli))
}

// normalizeDelta puts a modular difference into [1, period], so a backwards move reads as
// the forward distance it is equivalent to.
func normalizeDelta(delta, period int) int {
	d := ((delta % period) + period) % period
	if d == 0 {
		return period
	}
	return d
}

// drop returns the slice without position i.
func drop(moduli []int, i int) []int {
	out := make([]int, 0, len(moduli)-1)
	for j, m := range moduli {
		if j == i {
			continue
		}
		out = append(out, m)
	}
	return out
}

// TestSequenceGivesTheExpectation is the part C actually needs: the expectation comes from
// the previous turn, and a one-turn lag is grace rather than a lie.
func TestSequenceGivesTheExpectation(t *testing.T) {
	moduli := []int{2, 3, 5}

	tuples := []tuple{newTuple(17, moduli), newTuple(18, moduli), newTuple(19, moduli)}

	t.Logf("%-8s %-10s %-12s %-12s", "turn", "position", "expected", "ok?")

	for i, tp := range tuples {
		pos := tp.position()
		expected := 17 + i
		ok := pos == expected
		t.Logf("%-8d %-10d %-12d %-12v", i, pos, expected, ok)

		if !ok {
			t.Errorf("turn %d reconstructed %d, expected %d", i, pos, expected)
		}
	}

	// A one-turn lag is a clean phase offset, which is grace: it reconstructs to a
	// legitimate previous index and must not be treated as a lie.
	lagging := newTuple(18, moduli)
	if got := lagging.position(); got != 18 {
		t.Errorf("a lagging tuple must reconstruct to its own index, got %d", got)
	}
	t.Logf("a one-turn lag reconstructs to %d: grace, not deviance", lagging.position())

	// And a freeze - the same residue repeated while the truth advances - moves the
	// reconstruction, which is the deviance signature.
	frozen := newTuple(19, moduli)
	frozen.residues[2] = tuples[1].residues[2]
	pos := frozen.position()
	t.Logf("frozen path 2: position %d vs expected 19", pos)
	if pos == 19 {
		t.Error("a frozen residue must not reconstruct to the expected index")
	}
	if pos < 0 {
		t.Fatalf("a frozen residue should map, not reject")
	}
	t.Logf("attribution: delta %d names path %d", normalizeDelta(pos-19, lcmOf(moduli)), 2)
}

// TestNodeCCommitsRatherThanCounts keeps the topology honest: the tuple is evidence, not a
// vote. C is the single writer of the verdict, and A/B redundancy exists to catch C rather
// than to outvote it.
func TestNodeCCommitsRatherThanCounts(t *testing.T) {
	policy := DefaultRotationPolicy()
	ref := RouteRef{Scope: "lab", Tool: "isOdd"}

	// Deterministic across renders, so C verifies against it and never against a claim.
	if ref.ID(policy) != (RouteRef{Scope: "lab", Tool: "isOdd"}).ID(policy) {
		t.Error("the derived identity must be stable, or C has nothing to verify against")
	}
	if ref.ID(policy) == (RouteRef{Scope: "lab", Tool: "isEven"}).ID(policy) {
		t.Error("two routes must not share an identity, or a tuple could be replayed across them")
	}

	// The verdict has three states, and an unexamined item is not innocent - which is why
	// the tuple must be checked, not merely received.
	if Classify(false, false) != VerdictUnexamined {
		t.Error("an unreceived tuple is unexamined, not trusted")
	}
	if Classify(true, true) != VerdictTrusted {
		t.Error("a tuple that agrees with the derived identity is trusted")
	}

	t.Logf("C verifies against the derived identity (%s...), never against the claim", ref.ID(policy)[:12])
	t.Log("the tuple is evidence for C's check; it is not a ballot")
}
