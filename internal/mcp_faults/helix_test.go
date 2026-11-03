package mcp_faults

import (
	"math"
	"testing"
	"time"
)

// Two paths, two complementary invariants: that is not two independent proofs, it is one
// entanglement. Each path records its own index under a modulus, and the pair constrains
// their *relative* position - which is exactly what measuring rotation needs. The result is
// a helix: a state you can predict one turn ahead from the previous turn's residue.
//
// Three things have to be true for the helix to be an expected state rather than a
// tautology, and each one is measured here:
//
//  1. The values must be *possessed*, not derived. Anyone can compute n and n+1; only a
//     participant holds the issued pair.
//  2. A single modulus aliases. A parity helix cannot see a drift of two, ever.
//  3. Coprime moduli reconstruct the absolute index, so the aliasing disappears up to
//     their product - which is the Chinese Remainder Theorem doing the work.

// residue is the invariant a path records: its index reduced by the path's modulus.
func residue(index, modulus int) int {
	if modulus <= 0 {
		return -1
	}
	r := index % modulus
	if r < 0 {
		r += modulus
	}
	return r
}

// reconstruct is the helix inversion: the unique index below lcm whose residues match, or
// -1 when no such index exists. Brute force on purpose - the loop is the definition.
func reconstruct(moduli, residues []int) int {
	if len(moduli) != len(residues) {
		return -1
	}
	lcm := lcmOf(moduli)
	for index := 0; index < lcm; index++ {
		match := true
		for i, m := range moduli {
			if residue(index, m) != residues[i] {
				match = false
				break
			}
		}
		if match {
			return index
		}
	}
	return -1
}

// lcmOf is the period of the helix: the point at which the residues repeat.
func lcmOf(moduli []int) int {
	if len(moduli) == 0 {
		return 0
	}
	l := 1
	for _, m := range moduli {
		l = l / gcd(l, m) * m
	}
	return l
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// driftIsVisible reports whether a drift of d steps must show up in at least one residue.
func driftIsVisible(moduli []int, d int) bool {
	for _, m := range moduli {
		if residue(d, m) != 0 {
			return true
		}
	}
	return false
}

// TestComplementaryInvariantsGiveRelativePosition is the mechanism: one odd path and one
// even path, and the pair tells you where each is relative to the other.
func TestComplementaryInvariantsGiveRelativePosition(t *testing.T) {
	// Path A records under mod 2 and is expected to sit on an odd index; path B on an even
	// one, one turn behind. Together they are one rotation observed twice.
	t.Logf("%-6s %-8s %-12s %-12s %-10s", "turn", "index", "A mod 2", "B mod 2", "agree?")

	agree := 0
	for turn := 0; turn < 10; turn++ {
		aIndex := 2*turn + 1 // odd
		bIndex := 2 * turn   // even, one behind

		a, b := residue(aIndex, 2), residue(bIndex, 2)
		expected := (b + 1) % 2 // the helix: A is B advanced by one
		ok := a == expected
		if ok {
			agree++
		}
		t.Logf("%-6d %-8d %-12d %-12d %-10v", turn, aIndex, a, b, ok)

		if !ok {
			t.Errorf("turn %d: the helix must hold, got A=%d B=%d", turn, a, b)
		}
	}

	// A node that guesses its residue has one chance in two per turn, so k turns give
	// 1 - 2^-k. This is per-turn verification, not the quadratic sample test of ADR 0006.
	t.Logf("%-6s %-12s %-12s", "turns", "detection", "latency")
	for _, turns := range []int{1, 3, 5, 10, 20} {
		detection := 1 - math.Pow(2, -float64(turns))
		latency := time.Duration(turns) * ClusterRotationPeriod
		t.Logf("%-6d %-12.4f %-12s", turns, detection, latency)
	}

	// Ten turns is 99.9% at the shipped epoch: exact, and 100x slower than the bias test.
	if lat := 10 * ClusterRotationPeriod; lat <= 0 {
		t.Fatal("the helix latency must be finite")
	}
	t.Logf("the helix verifies state in %s at 99.9%%; the bias test detects intent in 469ms",
		10*ClusterRotationPeriod)
	t.Log("so they are complements: the helix measures *where* a node is, the bias test measures *why* it is there")
}

// TestParityHelixIsBlindToEvenDrift is the aliasing failure, and the reason one modulus is
// never enough: drift by two is invisible forever, so an attacker who always skips exactly
// two passes every check.
func TestParityHelixIsBlindToEvenDrift(t *testing.T) {
	t.Logf("%-8s %-14s %-14s", "drift", "mod 2 visible", "mod 2,3,5 visible")

	for d := 1; d <= 8; d++ {
		t.Logf("%-8d %-14v %-14v", d, driftIsVisible([]int{2}, d), driftIsVisible([]int{2, 3, 5}, d))
	}

	// The specific hole: a node silently skipping one rotation pair is undetectable on a
	// parity helix, no matter how many turns pass.
	for _, d := range []int{2, 4, 6, 8} {
		if driftIsVisible([]int{2}, d) {
			t.Errorf("drift %d should be invisible to a parity helix", d)
		}
	}
	if !driftIsVisible([]int{2}, 1) {
		t.Error("drift 1 must be visible to a parity helix")
	}

	// With a coprime set the only invisible drift is a multiple of the product.
	if driftIsVisible([]int{2, 3, 5}, 30) {
		t.Error("drift 30 is the helix's blind spot, so it must be invisible")
	}
	if !driftIsVisible([]int{2, 3, 5}, 29) {
		t.Error("drift 29 must be visible with coprime moduli")
	}
	t.Log("a single modulus has a period-2 blind spot; coprime moduli move it to the product")
}

// TestCoprimeHelicesReconstructTheIndex is the "actual expected state": with moduli that
// share no factor, the residues determine the index below their product.
func TestCoprimeHelicesReconstructTheIndex(t *testing.T) {
	t.Logf("%-16s %-8s %-14s %-14s", "moduli", "period", "unique?", "blind spot")

	for _, moduli := range [][]int{{2}, {2, 3}, {2, 3, 5}, {2, 3, 5, 7}} {
		period := lcmOf(moduli)

		// Every index below the period must reconstruct to itself, and no two may collide.
		seen := map[int]int{}
		unique := true
		for index := 0; index < period; index++ {
			residues := make([]int, len(moduli))
			for i, m := range moduli {
				residues[i] = residue(index, m)
			}
			got := reconstruct(moduli, residues)
			if got != index {
				unique = false
				break
			}
			if prev, ok := seen[got]; ok && prev != index {
				unique = false
				break
			}
			seen[got] = index
		}

		t.Logf("%-16v %-8d %-14v %-14d", moduli, period, unique, period)
		if !unique {
			t.Errorf("moduli %v must reconstruct the index uniquely below %d", moduli, period)
		}
	}

	// The cost of the fourth helix is a fourth path to record and check per turn, and the
	// benefit is a blind spot seven times further away.
	if lcmOf([]int{2, 3, 5, 7}) != 210 {
		t.Errorf("four coprime moduli should reach 210, got %d", lcmOf([]int{2, 3, 5, 7}))
	}
	if lcmOf([]int{2, 3, 5}) != 30 {
		t.Errorf("three coprime moduli should reach 30, got %d", lcmOf([]int{2, 3, 5}))
	}

	// Concrete: three helices reconstruct the epoch index exactly, which is what turns a
	// residue into an *expected* state at the next turn.
	moduli := []int{2, 3, 5}
	index := 17
	residues := []int{residue(index, 2), residue(index, 3), residue(index, 5)}
	got := reconstruct(moduli, residues)
	t.Logf("index %d recorded as residues %v reconstructs to %d", index, residues, got)
	if got != index {
		t.Errorf("expected %d, reconstructed %d", index, got)
	}
}

// TestHelixOverDerivedValuesIsATautology is the precondition, and it is the same one
// [ADR 0002] makes about the golden reference: a public function is computable by the
// attacker, so a helix over derived values constrains nothing.
func TestHelixOverDerivedValuesIsATautology(t *testing.T) {
	// A forger computes both paths from the epoch number, which it can read.
	derivedHelix := func(epoch int) (int, int) {
		return residue(2*epoch+1, 2), residue(2*epoch, 2)
	}

	// It satisfies the helix on every epoch, having done no work at all.
	for epoch := 0; epoch < 100; epoch++ {
		a, b := derivedHelix(epoch)
		if a != (b+1)%2 {
			t.Errorf("epoch %d: a derived helix should always satisfy the relation", epoch)
		}
	}
	t.Log("derived helix: 100/100 epochs satisfy the relation, zero information")

	// A possessed helix breaks when the holder is stale: the residue comes from the issued
	// value, so a node that did not rotate cannot produce the current one.
	moduli := []int{2, 3, 5}
	current := 23
	issued := []int{residue(current, 2), residue(current, 3), residue(current, 5)}

	// The node claims it is at 24 (one turn on) but presents the older residues.
	claimed := []int{residue(24, 2), residue(24, 3), residue(24, 5)}
	mismatched := issued[0] != claimed[0] || issued[1] != claimed[1] || issued[2] != claimed[2]

	t.Logf("issued residues %v vs claimed %v: mismatched=%v", issued, claimed, mismatched)
	if !mismatched {
		t.Error("a stale holder must fail the helix, or possession is not being checked")
	}

	// And the reconstruction is the expected state: the pair pins the index, so the next
	// turn is predictable rather than merely plausible.
	reconstructed := reconstruct(moduli, issued)
	if reconstructed != current {
		t.Errorf("issued residues must reconstruct to %d, got %d", current, reconstructed)
	}
	expectedNext := reconstruct(moduli, []int{residue(current+1, 2), residue(current+1, 3), residue(current+1, 5)})
	t.Logf("helix: current %d implies next %d", reconstructed, expectedNext)
	if expectedNext != (reconstructed+1)%lcmOf(moduli) {
		t.Errorf("the helix must predict the next index, got %d", expectedNext)
	}
}

// TestHelixIsALoadInvariantCheck is the operational difference from the bias test, and the
// reason both exist: the helix is per-turn, so more load does not make it faster.
func TestHelixIsALoadInvariantCheck(t *testing.T) {
	type check struct {
		name    string
		latency time.Duration
		units   string
	}

	bias := check{name: "bias test (ADR 0006)", latency: 469 * time.Millisecond, units: "samples"}
	helix := check{name: "three-helix CRT", latency: 10 * ClusterRotationPeriod, units: "turns"}

	for _, c := range []check{bias, helix} {
		t.Logf("%-24s %-10s drives detection", c.name, c.units)
	}

	// Load accelerates the sample test and does nothing for the turn test.
	if bias.latency >= helix.latency {
		t.Error("the bias test should be faster, that is the trade it buys")
	}
	// The helix's latency is a schedule, not a statistic: fixed by the epoch and the turn
	// count, unaffected by arrival rate, which makes it the thing you can put an SLO on.
	if helix.latency%ClusterRotationPeriod != 0 {
		t.Error("the helix latency must be a whole number of turns")
	}
	t.Logf("bias: %s, load-dependent; helix: %s, load-invariant and exact", bias.latency, helix.latency)

	// And the split, stated as a rule: the helix answers "is this node where it claims",
	// which is an attestation of state; the bias test answers "is this client following",
	// which is behavior and needs no attestation at all.
	t.Log("the helix attests where a node is; the bias test measures whether it is following")
}

// TestNonCoprimeModuliWasteAPath is the trap in choosing the set: more moduli is not more
// coverage unless they are coprime. A pair that shares a factor adds a path and extends the
// period by less than the modulus, so the helix looks stronger while its blind spot barely
// moves.
func TestNonCoprimeModuliWasteAPath(t *testing.T) {
	t.Logf("%-14s %-8s %-10s", "moduli", "period", "vs product")

	for _, moduli := range [][]int{{2, 4}, {2, 3}, {3, 4}, {2, 3, 6}} {
		period := lcmOf(moduli)
		product := 1
		for _, m := range moduli {
			product *= m
		}
		t.Logf("%-14v %-8d %-10d", moduli, period, product)

		if period > product {
			t.Errorf("moduli %v: the period cannot exceed the product", moduli)
		}
	}

	// {2,4} reaches 4, not 8: the mod-2 path is redundant, because every mod-4 residue
	// already determines it.
	if got := lcmOf([]int{2, 4}); got != 4 {
		t.Errorf("a shared factor must collapse the period to 4, got %d", got)
	}
	if got := lcmOf([]int{3, 4}); got != 12 {
		t.Errorf("coprime 3 and 4 must reach 12, got %d", got)
	}
	if lcmOf([]int{2, 3, 6}) >= 36 {
		t.Error("a redundant modulus must not reach the product")
	}

	// The rule: the period is the lcm, so coverage per path is maximized only when every
	// pair is coprime - primes are the simple way to guarantee it.
	t.Log("coverage per path: choose moduli pairwise coprime; primes make that structural")
}
