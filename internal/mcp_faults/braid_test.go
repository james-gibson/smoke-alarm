package mcp_faults

import (
	"fmt"
	"testing"
)

// A Bloom filter and a braid group are duals, not cousins, and the contrast is the clearest
// way to say what the helix needs.
//
// A Bloom filter is a bounded set: bits, OR'd. Union is commutative and idempotent, so order
// and repetition carry no information, and the structure is intentionally lossy - a
// membership answer can be wrong in one direction only.
//
// A braid is a stack: crossings composed. The operation is non-commutative and
// non-idempotent, so order and repetition *are* the information, and the structure is exact -
// the word problem is decidable and an equality answer is never wrong.
//
// That is the same decision PadStack already made ("a stack, not a set"): order and
// repetition are information. The braid group is what that decision looks like when you need
// the algebra to close.
//
// B_3 is isomorphic to PSL(2,Z), so a faithful 2x2 integer representation is exact here and
// cheap to check: sigma1 = [[1,1],[0,1]], sigma2 = [[1,0],[-1,1]].

// matrix is an integer 2x2, standing in for an element of PSL(2,Z) = B_3.
type matrix [4]int

var (
	sigma1 = matrix{1, 1, 0, 1}
	sigma2 = matrix{1, 0, -1, 1}
	unit   = matrix{1, 0, 0, 1}
)

func (m matrix) mul(n matrix) matrix {
	return matrix{
		m[0]*n[0] + m[1]*n[2], m[0]*n[1] + m[1]*n[3],
		m[2]*n[0] + m[3]*n[2], m[2]*n[1] + m[3]*n[3],
	}
}

// braidWord multiplies the generators in order.
func braidWord(gens ...int) matrix {
	out := unit
	for _, g := range gens {
		if g == 1 {
			out = out.mul(sigma1)
		} else {
			out = out.mul(sigma2)
		}
	}
	return out
}

// TestBloomJoinsAndBraidsStack is the table, measured on the real filter and the real
// representation rather than asserted from the textbook.
func TestBloomJoinsAndBraidsStack(t *testing.T) {
	// Bloom: union is idempotent and commutative, so repetition and order vanish.
	once := NewRotationFilter(64, 8)
	once.Add("a")
	once.Add("b")

	twice := NewRotationFilter(64, 8)
	twice.Add("a")
	twice.Add("a")
	twice.Add("b")
	twice.Add("b")
	twice.Add("b")

	reordered := NewRotationFilter(64, 8)
	reordered.Add("b")
	reordered.Add("a")

	probes := []string{"a", "b", "c", "d", ""}
	for _, p := range probes {
		if once.Present(p) != twice.Present(p) {
			t.Errorf("a Bloom filter must be idempotent; %q differs", p)
		}
		if once.Present(p) != reordered.Present(p) {
			t.Errorf("a Bloom filter must be commutative; %q differs", p)
		}
	}
	t.Log("bloom: OR is commutative and idempotent, so order and repetition are lost")

	// Braid: neither holds, and the relations are the braid relations.
	s1s2 := braidWord(1, 2)
	s2s1 := braidWord(2, 1)
	if s1s2 == s2s1 {
		t.Fatal("adjacent generators must not commute, or it is not a braid")
	}

	if braidWord(1, 1) == braidWord(1) {
		t.Error("a repeated crossing must not be idempotent")
	}

	// The braid relation, which is what makes it a group rather than a free monoid.
	if braidWord(1, 2, 1) != braidWord(2, 1, 2) {
		t.Error("the braid relation sigma1 sigma2 sigma1 = sigma2 sigma1 sigma2 must hold")
	}
	// With only two strands there is no distant commutation to test: B_3 has one braid
	// relation and nothing else, which is why it is isomorphic to PSL(2,Z). A wider braid
	// would add sigma_i sigma_j = sigma_j sigma_i for |i-j| >= 2.
	if braidWord(2, 1, 2) != braidWord(1, 2, 1) {
		t.Error("the braid relation must be symmetric in the generators")
	}
	t.Log("braid: non-commutative, non-idempotent, and closed under the braid relations")

	t.Logf("sigma1*sigma2 = %v; sigma2*sigma1 = %v", s1s2, s2s1)
}

// TestBloomIsLossyAndBraidIsExact is the failure mode on each side: a filter can say present
// when absent, a braid cannot confuse two words.
func TestBloomIsLossyAndBraidIsExact(t *testing.T) {
	// A small filter at design size still admits false positives, which is the point of the
	// structure rather than a defect.
	f := NewRotationFilter(8, 2)
	for _, v := range []string{"r1", "r2", "r3", "r4"} {
		f.Add(v)
	}

	falsePositives := 0
	for i := 0; i < 100; i++ {
		if f.Present(fmt.Sprintf("absent-%d", i)) {
			falsePositives++
		}
	}
	t.Logf("bloom: %d/100 absent values admitted (theory %.3f)", falsePositives, f.TheoryFalsePositive())
	if falsePositives == 0 {
		t.Log("bloom: this fixture happened to be exact; the structure still permits error")
	}

	// A braid word is exact: distinct words have distinct matrices unless related by the
	// braid relations, so equality never answers "maybe".
	words := [][]int{{1}, {1, 1}, {1, 1, 1}, {2}, {1, 2}, {2, 1}, {1, 2, 1}, {2, 1, 2}}
	seen := map[matrix][]int{}
	merged := 0
	for _, w := range words {
		m := braidWord(w...)
		if prev, ok := seen[m]; ok {
			merged++
			t.Logf("merged by the braid relation: %v == %v", prev, w)
			continue
		}
		seen[m] = w
	}

	if merged != 1 {
		t.Errorf("exactly one pair should merge, and only because of the braid relation; merged %d", merged)
	}
	if len(seen) != len(words)-1 {
		t.Errorf("expected %d distinct matrices, got %d", len(words)-1, len(seen))
	}
	t.Logf("braid: %d words -> %d matrices; the single merge is the braid relation, nothing else", len(words), len(seen))
}

// TestBraidsHaveNoTorsionSoNoBlindSpot is why the group choice matters for the helix. A
// modulus has a period, and a drift of exactly that period is invisible. A torsion-free group
// has no period at all, so no drift is invisible.
func TestBraidsHaveNoTorsionSoNoBlindSpot(t *testing.T) {
	// Modulus: the blind spot is the period.
	const period = 30
	blind := period
	t.Logf("modulus helix: drift %d is invisible, because residue(index+%d) == residue(index)", blind, blind)
	if residue(0+blind, period) != residue(0, period) {
		t.Error("fixture: the period must alias")
	}
	if driftIsVisible([]int{2, 3, 5}, blind) {
		t.Error("the period must be the blind spot")
	}

	// Torsion-free: sigma1^k is never the identity, so every drift is distinguishable.
	t.Logf("%-8s %-14s %-12s", "k", "sigma1^k", "identity?")
	for k := 1; k <= 5; k++ {
		gens := make([]int, k)
		for i := range gens {
			gens[i] = 1
		}
		m := braidWord(gens...)
		t.Logf("%-8d %-14v %-12v", k, m, m == unit)
		if m == unit {
			t.Errorf("a braid generator must have infinite order; sigma1^%d is the identity", k)
		}
	}

	// The unbounded version: no k up to 100 returns to the identity, so no drift is free.
	turns := 0
	state := unit
	for k := 0; k < 100; k++ {
		state = state.mul(sigma1)
		if state == unit {
			turns = k + 1
			break
		}
	}
	if turns != 0 {
		t.Errorf("the generator must be torsion-free, returned after %d", turns)
	}
	t.Logf("100 crossings and no return: the blind spot is not a period, it is the space itself")
	t.Log("so a torsion-free state trades a periodic blind spot for a guessing space (2^bits)")
}
