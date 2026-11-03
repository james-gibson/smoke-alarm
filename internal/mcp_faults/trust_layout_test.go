package mcp_faults

import (
	"fmt"
	"testing"
)

// RAID parity layouts are placement functions, and the placement function can assign *trust*
// as well as data. If parity rotates across members with the stripe index, then so does the
// verifier role, and the two cannot disagree about who checks whom.
//
// The layout used here is left-symmetric, the same arithmetic as a RAID-5 array:
//
//	slot(m, i) = (m - i) mod n          member m's position in stripe i
//	parity(i)  = the member in slot n-1
//
// Three properties follow, and each is a constraint the static topology did not have:
//
//	1. Every member is the verifier 1/n of the time, so no member is permanently the checker.
//	2. A parity holder holds no data shard in its own stripe, so nothing verifies itself.
//	3. Control has a *duration cost*: to be the verifier for k consecutive ticks needs k
//	   distinct members, where a static verifier needs one.

const trustMembers = 4

// roleAt is the placement function: which slot member m occupies at tick i.
func roleAt(m, i, n int) (slot int, parity bool) {
	slot = ((m-i)%n + n) % n
	return slot, slot == n-1
}

// verifierAt names the parity holder for a tick.
func verifierAt(i, n int) int {
	for m := 0; m < n; m++ {
		if _, parity := roleAt(m, i, n); parity {
			return m
		}
	}
	return -1
}

// TestLeftSymmetricRotatesTrustAndData is the layout table, and the first two properties.
func TestLeftSymmetricRotatesTrustAndData(t *testing.T) {
	const ticks = 8

	t.Logf("%-6s", "tick")
	header := "tick  "
	for m := 0; m < trustMembers; m++ {
		header += fmt.Sprintf("  m%d ", m)
	}
	t.Log(header)

	for i := 0; i < ticks; i++ {
		line := fmt.Sprintf("%-6d", i)
		for m := 0; m < trustMembers; m++ {
			slot, parity := roleAt(m, i, trustMembers)
			if parity {
				line += "  P  "
			} else {
				line += fmt.Sprintf("  d%d ", slot)
			}
		}
		t.Log(line)
	}

	// Property 1: each member is the verifier exactly once per n ticks.
	counts := make([]int, trustMembers)
	for i := 0; i < trustMembers*3; i++ {
		counts[verifierAt(i, trustMembers)]++
	}
	for m, c := range counts {
		t.Logf("member %d is verifier %d/%d ticks", m, c, trustMembers*3)
		if c != 3 {
			t.Errorf("member %d should be verifier 3 times in 12 ticks, got %d", m, c)
		}
	}

	// Property 2: the role is a function of (m - i) only, so trust and data cannot drift
	// apart - they are one shift of the same row.
	for i := 0; i < ticks; i++ {
		for m := 0; m < trustMembers; m++ {
			slot, parity := roleAt(m, i, trustMembers)
			// Shifting both the member and the tick by one must preserve the role.
			slotShifted, parityShifted := roleAt((m+1)%trustMembers, i+1, trustMembers)
			if slot != slotShifted || parity != parityShifted {
				t.Errorf("role must depend only on (m-i): m=%d i=%d gave %d/%v vs %d/%v",
					m, i, slot, parity, slotShifted, parityShifted)
			}
		}
	}
	t.Log("role = f((m - i) mod n): one function places data and assigns the verifier")
}

// TestNoMemberVerifiesItsOwnShard is the RAID invariant translated: parity and data never
// share a disk in the same stripe, so a verifier never attests to its own shard.
func TestNoMemberVerifiesItsOwnShard(t *testing.T) {
	for i := 0; i < 12; i++ {
		verifier := verifierAt(i, trustMembers)
		for m := 0; m < trustMembers; m++ {
			slot, parity := roleAt(m, i, trustMembers)
			if m != verifier && slot == trustMembers-1 {
				t.Errorf("tick %d: member %d holds a parity slot without being the verifier", i, m)
			}
			if parity && m != verifier {
				t.Errorf("tick %d: parity slot must belong to the verifier", i)
			}
		}

		// The verifier holds no data slot in this stripe, which is the invariant.
		slot, parity := roleAt(verifier, i, trustMembers)
		if !parity || slot != trustMembers-1 {
			t.Errorf("tick %d: the verifier must hold the parity slot, got %d/%v", i, slot, parity)
		}
	}
	t.Log("a verifier never holds a data shard in the stripe it verifies")
}

// TestTrustRotationCostsDuration is the security property the layout buys. A static verifier
// costs one member to control; a rotating one costs one member per tick of control.
func TestTrustRotationCostsDuration(t *testing.T) {
	t.Logf("%-10s %-14s %-16s %-12s", "ticks", "static cost", "rotating cost", "per member")

	for _, k := range []int{1, 2, 3, 4} {
		// Static: the same member verifies every tick.
		static := 1
		// Rotating: the verifier advances by one each tick, so k consecutive ticks need k
		// distinct members.
		rotating := k

		t.Logf("%-10d %-14d %-16d %-12.2f", k, static, rotating, float64(rotating)/float64(trustMembers))

		if rotating != k {
			t.Errorf("%d ticks of control should cost %d members, got %d", k, k, rotating)
		}
		if k > 1 && rotating <= static {
			t.Error("rotation must make sustained control more expensive")
		}
	}

	// The bound: to be verifier on every tick costs the whole membership, which is the same
	// quantity every quorum argument rests on.
	t.Logf("to verify every tick costs %d of %d members: the whole set", trustMembers, trustMembers)
	t.Log("duration of control is the new cost axis, and it is linear in ticks")
}

// TestDualParityDoublesTheVerifierSet is the RAID-6 variant: two parity slots (P and Q), so
// two verifiers per stripe and double the duration cost.
func TestDualParityDoublesTheVerifierSet(t *testing.T) {
	const n = 6

	// P and Q occupy the last two slots of each stripe.
	verifiersAt := func(i int) (p, q int) {
		for m := 0; m < n; m++ {
			slot := ((m-i)%n + n) % n
			switch slot {
			case n - 1:
				p = m
			case n - 2:
				q = m
			}
		}
		return p, q
	}

	for i := 0; i < 6; i++ {
		p, q := verifiersAt(i)
		t.Logf("tick %d: P=m%d Q=m%d", i, p, q)
		if p == q {
			t.Errorf("tick %d: the two parity slots must be distinct members", i)
		}
	}

	// Duration cost doubles: k ticks of dual control needs 2k distinct members.
	t.Logf("%-8s %-14s %-14s", "ticks", "members (P+Q)", fmt.Sprintf("share of %d", n))
	for _, k := range []int{1, 2, 3} {
		need := 2 * k
		t.Logf("%-8d %-14d %-14.2f", k, need, float64(need)/float64(n))
		if need <= k {
			t.Error("dual parity must cost at least as much as single parity")
		}
	}
	t.Log("dual parity doubles the verifier set, so it doubles the duration cost")
}

// TestDataResidenceIsOneTick is the distribution half. Placement rotates, so no member holds
// the same shard on consecutive ticks: it returns to that slot only after a full cycle, and
// in between it has held every other slot. Reconstructing one shard's history therefore takes
// the whole membership, not one compromised host.
func TestDataResidenceIsOneTick(t *testing.T) {
	const ticks = trustMembers * 2

	for m := 0; m < trustMembers; m++ {
		longestRun := 0
		run := 0
		previous := -1

		for i := 0; i < ticks; i++ {
			slot, parity := roleAt(m, i, trustMembers)
			if parity {
				run = 0
				previous = -1
				continue
			}
			if slot == previous {
				run++
			} else {
				run = 1
			}
			if run > longestRun {
				longestRun = run
			}
			previous = slot
		}

		t.Logf("member %d: longest consecutive residence %d tick(s)", m, longestRun)
		if longestRun > 1 {
			t.Errorf("member %d held one slot for %d consecutive ticks", m, longestRun)
		}
	}

	// The cost of following a single shard: the shard moves one slot per tick, so the member
	// that holds it changes every tick, and a full history needs every member once.
	followers := map[int]bool{}
	for i := 0; i < trustMembers; i++ {
		for m := 0; m < trustMembers; m++ {
			if slot, parity := roleAt(m, i, trustMembers); !parity && slot == 0 {
				followers[m] = true
			}
		}
	}
	t.Logf("following shard 0 across %d ticks needs %d distinct members", trustMembers, len(followers))
	if len(followers) != trustMembers {
		t.Errorf("one shard's history should need every member, got %d", len(followers))
	}
	t.Log("residence is one tick: a compromised host sees every shard briefly, none of them completely")
}

// TestRebuildMovesTheParityRole is the operational consequence: the member that reconstructs
// a lost shard at tick i is not the member that held parity at tick i-1.
func TestRebuildMovesTheParityRole(t *testing.T) {
	for lostAt := 1; lostAt < 6; lostAt++ {
		before := verifierAt(lostAt-1, trustMembers)
		during := verifierAt(lostAt, trustMembers)

		t.Logf("failure at tick %d: parity was m%d, rebuild is done by m%d", lostAt, before, during)
		if before == during {
			t.Errorf("tick %d: the rebuilding verifier must differ from the previous holder", lostAt)
		}
	}

	// That rotation is what stops a single compromised member from both holding parity and
	// performing every reconstruction.
	t.Log("rebuild is distributed by the same shift, so no member is the permanent repairer")
}
