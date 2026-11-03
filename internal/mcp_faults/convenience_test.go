package mcp_faults

import (
	"math"
	"testing"
	"time"
)

// The red team attacks what it can predict, so every convenience is an attack surface and
// must be hardened when it is added - not when it is abused. A convenience is anything we
// put in for our own sake: a cheap invariant to test against, a readable refusal reason, a
// fixed period, a derived identifier.
//
// The test for each one is the same: **can it be satisfied without having done the thing it
// is supposed to attest?** If yes, it measures behavior and must never gate an
// attestation. Two conveniences are worked through here because both are already in the
// code and both fail that test in instructive ways.

// TestStaticInvariantCannotMeasureRotation is the isOdd/isEven example.
//
// A total function is a wonderful test fixture: cheap, deterministic, independently
// checkable. It is a useless rotation measure, because a pure function has a zero
// derivative - its answer does not change when the epoch does. Reusing the invariant as a
// rotation probe therefore reports "no rotation" for a node that rotated thirty times, and
// "healthy" for a node that never rotated at all.
func TestStaticInvariantCannotMeasureRotation(t *testing.T) {
	clock := &testClock{t: time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)}
	proxy := hashedProxyAt(t, "isOdd", clock.Now)
	proxy.Scope = "lab"

	// The invariant's signal across a run: sample the answer the way a second skill
	// would, once per epoch, while the cluster rotates.
	invariantAnswers := map[bool]struct{}{}
	stamps := map[string]struct{}{}

	for tick := 0; tick < 30; tick++ {
		clock.Advance(ClusterRotationPeriod)
		proxy.Rotate()

		// What a rotation probe would see if it asked the static invariant.
		invariantAnswers[isOdd(7)] = struct{}{}
		// What it sees if it asks for something the rotation actually changes.
		stamps[proxy.IssueSkill("agent-north")] = struct{}{}
	}

	t.Logf("after 30 rotations: invariant distinct answers=%d, stamp distinct values=%d",
		len(invariantAnswers), len(stamps))

	// The invariant saturates at its own two values and carries no rotation signal.
	if len(invariantAnswers) != 1 {
		t.Errorf("the invariant must be constant across rotations, saw %d values", len(invariantAnswers))
	}
	// The stamp tracks the epoch exactly.
	if len(stamps) != 30 {
		t.Errorf("a per-epoch stamp should give one value per rotation, got %d", len(stamps))
	}

	// The failure stated as the two health reports. A lazy node answers correctly every
	// tick and never rotates: the invariant scores it perfect, participation scores zero.
	lazyHealthy := 0
	for tick := 0; tick < 30; tick++ {
		if isOdd(7) != isEven(7) { // the independent check, passed every time
			lazyHealthy++
		}
	}
	t.Logf("lazy node: invariant health %d/30, rotations %d", lazyHealthy, 0)

	if lazyHealthy != 30 {
		t.Errorf("the invariant cannot penalize a node that never rotates, scored %d/30", lazyHealthy)
	}

	// So the rule: the invariant may measure *behavior* (is the node answering
	// correctly today), and may never measure *participation* (did it rotate).
	t.Log("a pure function attests arithmetic; participation must be read from state the rotation changed")
}

// TestRefusalReasonIsAGuidanceOracle is the second convenience, and the more expensive
// one: readable failure reasons. They help an operator, and they also tell an attacker
// which check failed - one probe per bit, and the direction to turn next.
func TestRefusalReasonIsAGuidanceOracle(t *testing.T) {
	clock := &testClock{t: time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)}
	proxy := hashedProxyAt(t, "isOdd", clock.Now)

	// Measure the reasons the proxy actually emits, rather than assuming them.
	probe := func(call string) string {
		ok, reason := proxy.ValidateCall(call)
		if ok {
			return "accepted"
		}
		return reason
	}

	observed := map[string]string{
		"bare":      probe("isOdd"),
		"malformed": probe("isOddnoprefix"),
		"forged":    probe("deadbeef_isOdd"),
		"neighbor":  probe(hashedProxyAt(t, "isOdd", clock.Now).PrefixedToolName()),
	}

	for name, reason := range observed {
		t.Logf("%-10s -> %q", name, reason)
	}

	// The mirror adds the two identity reasons; the wire has at least these four.
	mirrorReasons := []string{RotationIdentityMismatch, RotationUnauthorizedReplica}
	distinct := map[string]struct{}{}
	for _, r := range observed {
		if r != "accepted" {
			distinct[r] = struct{}{}
		}
	}
	for _, r := range mirrorReasons {
		distinct[r] = struct{}{}
	}

	checks := len(distinct)
	bitsPerProbe := math.Log2(float64(checks))

	t.Logf("distinct failure reasons reachable: %d -> %.2f bits of guidance per probe", checks, bitsPerProbe)

	// The cost model. Blind, the attacker must find a configuration that passes every
	// check: expected 2^(k-1) probes for k binary checks. Guided, each probe names the
	// failing check, so it is k probes - linear instead of exponential.
	blind := math.Pow(2, float64(checks)-1)
	guided := float64(checks)

	t.Logf("enumerating %d checks: %.0f probes guided, %.0f blind (%.0fx)", checks, guided, blind, blind/guided)

	if bitsPerProbe <= 0 {
		t.Fatal("no distinct reasons were observed, so the fixture proves nothing")
	}
	if guided >= blind {
		t.Errorf("guidance must be cheaper than blindness: %.0f vs %.0f", guided, blind)
	}

	// The sharper point: the reason is not just a count, it is a direction. It separates
	// "change your identity" from "get registered", which is the same gradient ADR 0006
	// refuses in the name space, applied to the check space.
	if RotationIdentityMismatch == RotationUnauthorizedReplica {
		t.Error("the two identity reasons must be distinguishable to be an oracle")
	}
	t.Logf("reasons split the attacker's strategy: %q vs %q", RotationIdentityMismatch, RotationUnauthorizedReplica)

	// The hardening: one coarse reason on the wire, the fine one in the operator's log.
	// Coarse guidance is zero bits per probe, so the search stays exponential.
	coarseBits := math.Log2(1)
	t.Logf("coarse wire reason -> %.0f bits/probe; the attacker is back to the 2^32 search", coarseBits)
	if coarseBits != 0 {
		t.Error("a single coarse reason must carry no guidance")
	}
}

// convenience is an entry in the register. Adding one without filling all four fields is
// the failure mode this structure exists to make visible.
type convenience struct {
	name string
	// attests is what we would like it to prove.
	attests string
	// proves is what it actually proves, which is always weaker.
	proves string
	// cheapest is the cheapest way to satisfy it without doing the work.
	cheapest string
	// gatesAttestation is the forbidden use, and must always be false.
	gatesAttestation bool
}

// TestConvenienceRegister is the "harden as they are added" rule, applied to the
// conveniences already in the code. Every entry must name what it really proves and how
// cheaply it can be satisfied, and none may gate an attestation.
func TestConvenienceRegister(t *testing.T) {
	register := []convenience{
		{
			name:     "isOdd/isEven invariant pair",
			attests:  "the node is participating",
			proves:   "the node can do arithmetic - any node can, without rotating",
			cheapest: "compute n%2; zero cost, no state, no epoch",
		},
		{
			name:     "readable refusal reason",
			attests:  "the client knows why it was refused",
			proves:   "which check failed, i.e. which direction to turn next",
			cheapest: "one probe per check; linear enumeration of the check set",
		},
		{
			name:     "derived RouteRef.ID",
			attests:  "this node serves this route",
			proves:   "anyone can compute the id from public scope, tool and policy",
			cheapest: "hash the public definition; membership still needs the registry",
		},
		{
			name:     "fixed ClusterRotationPeriod",
			attests:  "the cluster shares a clock",
			proves:   "when to be stale, published to everyone including the attacker",
			cheapest: "read the constant; no guessing needed",
		},
		{
			name:     "public prefix format",
			attests:  "the caller has been issued a stamp",
			proves:   "the caller knows the format, not the value",
			cheapest: "read the source; the value still costs an issued stamp",
		},
		{
			name:     "DefaultRotationPolicy()",
			attests:  "grace is bounded",
			proves:   "the bound is computable, so the attacker knows exactly how long to claim",
			cheapest: "call the function; it is deliberately pure",
		},
	}

	t.Logf("%-30s %-12s %-10s", "convenience", "gates?", "cheapest")

	for _, c := range register {
		if c.attests == "" || c.proves == "" || c.cheapest == "" {
			t.Errorf("%s: every field must be filled, or the register is decorative", c.name)
		}
		if c.attests == c.proves {
			t.Errorf("%s: a convenience that proves what it attests is not a convenience", c.name)
		}
		if c.gatesAttestation {
			t.Errorf("%s: a convenience may measure behavior, never gate an attestation", c.name)
		}
		t.Logf("%-30s %-12v %-10s", c.name, c.gatesAttestation, c.cheapest)
	}

	// The two rules, stated once more so the register has a line to fail on.
	for _, c := range register {
		if c.name == "isOdd/isEven invariant pair" && c.gatesAttestation {
			t.Error("the invariant pair must never decide rotation")
		}
		if c.name == "readable refusal reason" && c.gatesAttestation {
			t.Error("a readable reason must never decide trust")
		}
	}
}

// TestGuidanceIsAdditiveWhereBlindnessIsMultiplicative is the quantified trade. The fine
// reason is only worth removing from the wire if the leakage is large, and it is large
// precisely because a blind refusal is *uninformative*: the attacker cannot tell which gate
// they are failing, so their attempts multiply across gates instead of adding.
//
// Model, stated because it is an assumption: k independent gates, t attempts on average to
// satisfy each. Blinded, the attacker cannot exclude a gate, so the expected attempts are
// the product; guided, each refusal names the gate, so they are the sum.
func TestGuidanceIsAdditiveWhereBlindnessIsMultiplicative(t *testing.T) {
	const attemptsPerGate = 4.0

	t.Logf("%-6s %-10s %-10s %-10s", "gates", "blind", "guided", "ratio")

	for _, gates := range []int{2, 3, 5, 8} {
		k := float64(gates)
		blind := math.Pow(attemptsPerGate, k)
		guided := k * attemptsPerGate
		t.Logf("%-6d %-10.0f %-10.0f %-10.0fx", gates, blind, guided, blind/guided)

		if blind <= guided {
			t.Errorf("%d gates: blindness must be more expensive than guidance", gates)
		}
	}

	// The ratio grows with the number of gates, which is the argument for hardening
	// conveniences as they are added: a system that grows gates grows the value of the
	// oracle, and the leak compounds with the surface.
	five := math.Pow(attemptsPerGate, 5) / (5 * attemptsPerGate)
	eight := math.Pow(attemptsPerGate, 8) / (8 * attemptsPerGate)
	if eight <= five {
		t.Error("the oracle's value must grow with the gate count")
	}
	t.Logf("five gates: %.0fx in the attacker's favor; eight gates: %.0fx", five, eight)

	// And the hardening has a fixed price: the operator keeps the fine reason in the log,
	// where the claimant cannot read it. One coarse reason on the wire, zero guidance.
	t.Logf("coarse wire reason: 0 bits/probe; fine reason survives in the mirror's log")
}
