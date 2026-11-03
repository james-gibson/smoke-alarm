package mcp_faults

import (
	"testing"
)

// Forced inversion: we supply a number, downstream must invert it as part of its rotation,
// and the check is composition to the identity. The relation is exact, public, and reveals
// nothing beyond equality - no modulus structure, no lcm, no opening.
//
// Two findings, and the second is the sharp edge:
//
//  1. Attribution comes free, because the pairing is explicit. Each downstream gets its own
//     number, so the pair that fails to compose names the node. That is the job the lcm was
//     doing, and here it needs no arithmetic structure at all.
//  2. Any rotation defined as "invert" is an involution. f(f(x)) = x, so the period is 2 in
//     every group, abelian or not, and a node can replay x, x^-1, x, x^-1 forever and pass
//     every check. Beating that needs a non-abelian group, where the period becomes the
//     order of the conjugating element.

// inverseOf is the supplied number's inverse in the multiplicative group mod p.
func inverseOf(x, p int) int {
	// Brute force is the definition here: find y with x*y = 1.
	for y := 1; y < p; y++ {
		if x*y%p == 1 {
			return y
		}
	}
	return 0
}

// TestInversionGatesRotation is the mechanism, and it is strictly stronger than a claimed
// rotation: the node cannot turn without producing the inverse of *our* number.
func TestInversionGatesRotation(t *testing.T) {
	const p = 101
	supplied := 17

	// The rotation: hand down x, require y with x*y = 1.
	rotated := inverseOf(supplied, p)
	t.Logf("supplied %d -> required inverse %d (product %d)", supplied, rotated, supplied*rotated%p)

	if supplied*rotated%p != 1 {
		t.Fatalf("the pair must compose to the identity, got %d", supplied*rotated%p)
	}

	// Every other answer fails, so "did it rotate" is a single equality, not a statistic.
	for y := 1; y <= 5; y++ {
		if y == rotated {
			continue
		}
		if supplied*y%p == 1 {
			t.Errorf("a wrong inverse %d must not compose to the identity", y)
		}
	}
	t.Log("the gate is one multiplication; no value is revealed beyond the relation")

	// Attribution is by pairing, not by structure: each downstream holds its own number, so
	// the failing pair names the node without any lcm arithmetic.
	type downstream struct {
		id       string
		supplied int
		returns  int
	}
	peers := []downstream{
		{"a", 17, inverseOf(17, p)},
		{"b", 23, inverseOf(23, p)},
		{"c", 29, 41}, // wrong on purpose
	}

	failing := ""
	for _, d := range peers {
		if d.supplied*d.returns%p != 1 {
			if failing != "" {
				t.Errorf("more than one pair failed: %s and %s", failing, d.id)
			}
			failing = d.id
		}
	}

	t.Logf("failing pair: %q", failing)
	if failing != "c" {
		t.Errorf("the failing pair must name the node, named %q", failing)
	}
	t.Log("attribution is free: the pairing is explicit, so no CRT structure is needed")
}

// TestInversionHasPeriodTwo is the aliasing, and it is structural rather than accidental:
// inversion is an involution, so any inversion-defined rotation repeats with period 2.
func TestInversionHasPeriodTwo(t *testing.T) {
	const p = 101
	x := 17

	// A node replaying x, x^-1, x, x^-1 ... passes the pair check on every turn.
	state := x
	passed := 0
	for turn := 0; turn < 10; turn++ {
		next := inverseOf(state, p)
		if state*next%p == 1 {
			passed++
		}
		state = next
	}

	t.Logf("alternating pair: %d/10 turns pass, state repeats with period 2", passed)
	if passed != 10 {
		t.Errorf("a replayed pair must pass every check, passed %d/10", passed)
	}

	// The period, stated directly: f(f(x)) = x for every x, so drift of 2 is invisible.
	for _, v := range []int{2, 3, 5, 17, 99} {
		if inverseOf(inverseOf(v, p), p) != v {
			t.Errorf("inversion must be an involution at %d", v)
		}
	}
	t.Log("period 2: the same blind spot as a parity helix, now built into the rotation")

	// And forcing the inversion is not work: the inverse is a function of the supplied value,
	// so anyone holding x produces the correct answer with no secret and no cost.
	forger := x
	if forger*inverseOf(forger, p)%p != 1 {
		t.Error("a forger holding x must succeed, because the inverse is computable from x")
	}
	t.Log("the gate proves the node received x; it does not prove the node spent anything")
}

// TestAbelianInversionWithAStepIsStillInvolution: adding a per-epoch factor does not help.
// In an abelian group, f(x) = g * x^-1 is also an involution, because the factors cancel.
func TestAbelianInversionWithAStepIsStillInvolution(t *testing.T) {
	const p = 101
	g := 7 // the per-epoch step

	step := func(x int) int { return g * inverseOf(x, p) % p }

	for _, x := range []int{3, 11, 42, 88} {
		once := step(x)
		twice := step(once)
		t.Logf("x=%d -> %d -> %d", x, once, twice)

		if twice != x {
			t.Errorf("the abelian step must be an involution at %d, got %d", x, twice)
		}
	}
	t.Log("any g * x^-1 is an involution: the epoch factor cancels, so the period is still 2")
}

// orderOf is the smallest k > 0 with p^k = identity.
func orderOf(p perm) int {
	state := p
	for k := 1; k < 8; k++ {
		if state == (perm{0, 1, 2}) {
			return k
		}
		state = state.compose(p)
	}
	return -1
}

// perm is a permutation of three elements, standing in for a non-abelian group.
type perm [3]int

func (p perm) compose(q perm) perm {
	var out perm
	for i := 0; i < 3; i++ {
		out[i] = p[q[i]]
	}
	return out
}

func (p perm) inverse() perm {
	var out perm
	for i := 0; i < 3; i++ {
		out[p[i]] = i
	}
	return out
}

// TestNonAbelianConjugationSetsThePeriod is the fix. In a non-abelian group, f(x) = t * x^-1
// has f^2(x) = t * x * t^-1, i.e. conjugation, whose period is the order of t. So the helix
// period comes from the conjugating element rather than being fixed at 2.
func TestNonAbelianConjugationSetsThePeriod(t *testing.T) {
	identity := perm{0, 1, 2}
	transposition := perm{1, 0, 2} // order 2
	threeCycle := perm{1, 2, 0}    // order 3

	// The period is not ord(tau). Since f^2 = conjugation by tau, the period is governed by
	// the conjugacy class: a transposition doubles (2 -> 4), a three-cycle does not (3 -> 3).
	// What matters is that it is allowed to exceed 2 at all.
	cases := []struct {
		name string
		tau  perm
		want int
	}{
		{"transposition", transposition, 4},
		{"three-cycle", threeCycle, 3},
	}

	x := perm{0, 2, 1} // a test element

	t.Logf("%-14s %-8s %-10s %-10s", "tau", "order", "period", "f^period=id")

	for _, c := range cases {
		step := func(v perm) perm { return c.tau.compose(v.inverse()) }

		// Iterate until the identity, and count.
		state := x
		period := 0
		for i := 0; i < 8; i++ {
			state = step(state)
			if state == x {
				period = i + 1
				break
			}
		}

		t.Logf("%-14s %-8d %-10d %-10v", c.name, orderOf(c.tau), period, period == c.want)

		if period != c.want {
			t.Errorf("%s: expected period %d, got %d", c.name, c.want, period)
		}
		if period <= 2 {
			t.Errorf("%s: a non-abelian group must allow a period above 2, got %d", c.name, period)
		}
	}

	// The algebra, stated: f^2 is conjugation, and conjugation has the order of tau.
	f := func(v perm) perm { return threeCycle.compose(v.inverse()) }
	if f(f(x)) != threeCycle.compose(x).compose(threeCycle.inverse()) {
		t.Error("f^2 must be conjugation, not the identity")
	}
	t.Log("f^2 = conjugation: the period is set by the conjugacy class, so 2 is not forced")

	// The corollary for a helix: to cover a drift of 30 with forced inversion, the group needs
	// an element of order at least 30 - or several coprime involutions, which is the residue
	// construction again in different clothing.
	if threeCycle.compose(threeCycle).compose(threeCycle) != identity {
		t.Error("fixture: the three-cycle must have order 3")
	}

	// The bound: f^2 = conjugation, so the period is at most twice the order of tau, and a
	// 30-wide blind spot needs an element whose conjugacy class has order 15 or more.
	if orderOf(threeCycle) != 3 || orderOf(transposition) != 2 {
		t.Error("fixture: the tau orders must be 3 and 2")
	}
	t.Log("so: forced inversion + period > 2 costs a non-abelian group with a large-order element")
}
