package mcp_faults

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"
	"testing"
	"time"
)

// Can a crypto hash or key replace the residues? Partly, and the split is exact.
//
// A residue has a *public algebraic structure*: it can be inverted (r -> -r), summed, and
// its delta factorizes, which is what makes the liar nameable. A hash has none of that. It
// is not invertible, so "forward and reverse" are two unrelated images rather than two
// directions of one function, and hashing the residue destroys the divisibility that
// attribution depends on.
//
// What a key adds is different, and necessary: binding (the value cannot be changed without
// changing the tag), hiding (an observer cannot read the position), and epoch binding (a tag
// cannot be replayed). So the honest construction is *wrap, don't replace* - residues are
// the payload, a keyed hash is the envelope, and the opening is private to the verifier.
//
// The triangle below is the constraint: hiding, attribution and public verifiability cannot
// all three hold at once, and which two you get is a design decision, not an implementation
// detail.

// tag is the wire form of a residue: a keyed, epoch-bound commitment.
func tag(key []byte, residueValue, epoch int) string {
	mac := hmac.New(sha256.New, key)
	// Length-prefixed domain separation, for the same reason RouteRef.ID does it: no two
	// fields may concatenate into one encoding.
	_, _ = fmt.Fprintf(mac, "%d:%d|%d:%d", len("residue"), residueValue, len("epoch"), epoch)
	return hex.EncodeToString(mac.Sum(nil))
}

// TestHashHasNoForwardOrReverse: the two directions are independent images, not an inverse
// pair, so opposition has to be *constructed* by domain separation rather than read off the
// arithmetic.
func TestHashHasNoForwardOrReverse(t *testing.T) {
	key := []byte("node-a")

	// Two directions: the same index, labeled. Nothing relates them algebraically.
	asc := tag(key, residue(19, 5), 19)
	desc := tag(key, opposing(19, 5), 19)

	t.Logf("ascending  %s", asc[:16])
	t.Logf("descending %s", desc[:16])

	// The residue pair satisfies a public relation; the tag pair satisfies none.
	if (residue(19, 5)+opposing(19, 5))%5 != 0 {
		t.Error("the residues must sum to zero mod 5, that is the opposition")
	}
	if asc == desc {
		t.Error("the two directions must not collide")
	}

	// Turn one bit of the input and the tag is unrelated: equality is the only test, and
	// there is no notion of "close".
	near := tag(key, residue(19, 5)^1, 19)
	if near == asc {
		t.Fatal("a one-bit change must not preserve the tag")
	}

	same := 0
	for i := 0; i < len(asc); i++ {
		if asc[i] == near[i] {
			same++
		}
	}
	t.Logf("hex characters agreeing after a one-bit input change: %d/%d", same, len(asc))
	if same > len(asc)/2 {
		t.Errorf("the tag must avalanche, %d/%d characters agreed", same, len(asc))
	}

	t.Log("no inverse: the direction is a label, not an operation")
}

// TestHashingDestroysAttribution is the cost of moving the helix into the hash. The residue
// delta carries the lcm structure that names a liar; a tag carries nothing.
func TestHashingDestroysAttribution(t *testing.T) {
	moduli := []int{2, 3, 5}
	const index = 17
	key := []byte("node-a")

	// Residues: corrupting one path shifts the reconstruction by a multiple of the others'
	// lcm, so the delta names the path. Measured in helix_c_test.go; restated here.
	deltaFor := func(corrupt int) int {
		liar := newTuple(index, moduli)
		liar.residues[corrupt] = (liar.residues[corrupt] + 1) % moduli[corrupt]
		return normalizeDelta(liar.position()-index, lcmOf(moduli))
	}

	t.Logf("%-10s %-10s %-14s", "corrupt", "delta", "names via lcm")
	for corrupt := range moduli {
		d := deltaFor(corrupt)
		named := -1
		for i, m := range moduli {
			if d%m != 0 {
				named = i
			}
		}
		t.Logf("%-10d %-10d %-14d", corrupt, d, named)
		if named != corrupt {
			t.Errorf("residues must attribute path %d, named %d", corrupt, named)
		}
	}

	// Tags: the same corruption produces an unrelated tag. There is no delta to factor, so
	// attribution from the tuple alone is impossible.
	honest := tag(key, index%5, index)
	corrupt := tag(key, (index%5+1)%5, index)

	t.Logf("honest tag %s...", honest[:12])
	t.Logf("corrupt tag %s...", corrupt[:12])
	if honest == corrupt {
		t.Fatal("fixture: the corruption must change the tag")
	}

	// What is left is all-or-nothing: detect by equality against the expectation, attribute
	// by majority across at least three claimants.
	t.Log("a tag detects certainly and attributes never; residues do both, and reveal position")

	if deltaFor(0) == 0 {
		t.Error("a residue corruption must move the reconstruction")
	}
}

// TestKeyedCommitmentBindsOpeningAndEpoch is what the key is for, and neither property is
// available from a bare residue.
func TestKeyedCommitmentBindsOpeningAndEpoch(t *testing.T) {
	key := []byte("node-a")
	const index = 19
	r := index % 5

	current := tag(key, r, index)

	// Binding: the same opening in another epoch is a different tag, so it cannot be
	// replayed from an earlier turn.
	previous := tag(key, r, index-30)
	if previous == current {
		t.Error("a tag must bind the epoch, or a stale residue is replayable")
	}
	t.Logf("replay of an earlier epoch: %s != %s", previous[:12], current[:12])

	// Hiding: an observer holding the tag learns nothing about r without the key, and even
	// with the key the tag is not invertible - the verifier must be given the opening.
	wrongKey := tag([]byte("node-b"), r, index)
	if wrongKey == current {
		t.Error("a different key must produce a different tag")
	}

	// Verification is recomputation with the opening, which is why the opening travels on the
	// private channel to C and the tag travels on the public one.
	recomputed := tag(key, r, index)
	if !hmac.Equal([]byte(recomputed), []byte(current)) {
		t.Error("verification must be recomputation, not inversion")
	}

	// And a tampered opening fails, which is the whole point of binding.
	if tag(key, (r+1)%5, index) == current {
		t.Error("a tampered opening must not verify")
	}
	t.Logf("tag binds (value, epoch) and hides value; verification is recomputation")
}

// TestGroupCommitmentKeepsTheSumCheck is the middle ground: a commitment whose exponent is
// the residue keeps *additive* structure under hiding, so the opposition test survives while
// the value stays opaque. Divisibility attribution does not survive.
//
// The constraint is easy to miss and fatal: the modulus must divide the group's order.
// Working modulo a prime p, the multiplicative group has order p-1, so a helix of period 30
// needs a group whose order is a multiple of 30. Here Z_31* with generator 3. Get it wrong
// and g^r * g^-r is g^m, not the identity, and every sum check fails.
func TestGroupCommitmentKeepsTheSumCheck(t *testing.T) {
	const (
		p     = 31 // the multiplicative group has order 30
		g     = 3  // a generator of Z_31*
		order = p - 1
		m     = 5 // residue modulus; must divide the order
	)
	index := 3

	if order%m != 0 {
		t.Fatalf("the modulus must divide the group order: %d does not divide %d", m, order)
	}

	mod := big.NewInt(p)
	pow := func(exp int) *big.Int {
		e := ((exp % order) + order) % order
		return new(big.Int).Exp(big.NewInt(g), big.NewInt(int64(e)), mod)
	}
	mul := func(a, b *big.Int) *big.Int {
		return new(big.Int).Mod(new(big.Int).Mul(a, b), mod)
	}
	one := big.NewInt(1)

	r := residue(index, m)
	cAsc := pow(r)
	// The descending path commits to (order - r), so the two exponents sum to the order.
	cDesc := pow(order - r)

	if mul(cAsc, cDesc).Cmp(one) != 0 {
		t.Errorf("opposing commitments must multiply to the identity, got %s", mul(cAsc, cDesc))
	}
	if pow(order).Cmp(one) != 0 {
		t.Errorf("the generator must have order %d", order)
	}

	// A common drift of d moves the ascending exponent by +d and the descending one by -d,
	// so the sum stays the identity and the difference factor is g^(2d) - visible without
	// either residue being revealed.
	for _, d := range []int{0, 1, 2, 3} {
		sum := mul(pow(r+d), pow(order-r-d))
		if sum.Cmp(one) != 0 {
			t.Errorf("drift %d: the sum must stay invariant, got %s", d, sum)
		}

		// g^(2(r+d)) / g^(2r) = g^(2d).
		gained := mul(pow(2*(r+d)), pow(-2*r))
		if gained.Cmp(pow(2*d)) != 0 {
			t.Errorf("drift %d: the difference factor should be g^%d, got %s", d, 2*d, gained)
		}
	}

	// What is lost: divisibility. A group element carries no factor structure a verifier can
	// test, so "the delta names the liar" does not survive - only equality of elements does.
	if pow(6).Cmp(pow(1)) == 0 {
		t.Error("fixture: distinct exponents must give distinct elements")
	}
	t.Log("group commitment: sum and difference checks survive hiding; divisibility does not")
	t.Logf("period %d must divide the group order %d; a 210-period helix needs an order divisible by 210", m, order)

	_ = index
}

// BenchmarkCommitCosts prices the three wire forms, so the choice is not a matter of taste.
func BenchmarkResidue(b *testing.B) {
	for i := 0; i < b.N; i++ {
		if residue(i, 5) < 0 {
			b.Fatal("residue")
		}
	}
}

func BenchmarkHMACSHA256(b *testing.B) {
	key := []byte("node-a")
	for i := 0; i < b.N; i++ {
		if tag(key, i%5, i) == "" {
			b.Fatal("tag")
		}
	}
}

func BenchmarkGroupCommit(b *testing.B) {
	p := new(big.Int).Sub(new(big.Int).Lsh(big.NewInt(1), 61), big.NewInt(1))
	g := big.NewInt(37)
	for i := 0; i < b.N; i++ {
		_ = new(big.Int).Exp(g, big.NewInt(int64(i%5)), p)
	}
}

// TestCommitCostsAreIrrelevantAtThisEpoch turns the benchmark into a planning number: the
// helix runs at the rotation rate, so even a microsecond per residue is free at 5 s epochs.
func TestCommitCostsAreIrrelevantAtThisEpoch(t *testing.T) {
	// Conservative constants rather than measured ones, so the test does not depend on the
	// machine: a keyed hash is sub-microsecond, a 61-bit exponentiation is a few.
	const (
		hashCost    = 1 * 1000 // nanoseconds, generous
		modexpCost  = 10 * 1000
		residuesPer = 3
		checksPer   = 2
	)

	perTurn := time.Duration(residuesPer*checksPer) * modexpCost
	turnsPerSecond := 1 / ClusterRotationPeriod.Seconds()

	t.Logf("worst case per turn: %s; turns per second: %.3f; duty %.6f%%",
		perTurn, turnsPerSecond, 100*perTurn.Seconds()*turnsPerSecond)

	if duty := perTurn.Seconds() * turnsPerSecond; duty > 0.001 {
		t.Errorf("the helix must stay under 0.1%% of a core at this epoch, got %.6f%%", duty*100)
	}
	t.Logf("a hashed wire form costs %d ns more per residue, which this epoch cannot notice", hashCost)
}
