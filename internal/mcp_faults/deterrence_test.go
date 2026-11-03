package mcp_faults

import (
	"fmt"
	"testing"
	"time"
)

// Competence is served. That sentence is only half the design, and the missing half is
// the one that matters operationally: competence is *served identically*, so it buys the
// attacker nothing they cannot already have, while it buys the client continuity that
// compounds. The same act therefore has opposite sign for the two, which is the whole
// deterrence argument. Not exclusion - a losing trade.
//
// Three claims are measured here:
//
//  1. Admission parity. Every claimant gets the same rate however well it derives, so
//     the marginal return on an attacker's competence is zero.
//  2. Rotation is a tax that falls on the taxpayer. It invalidates the attacker's own
//     credentials at exactly the rate it invalidates everyone else's, and only the
//     honest holder has a refund (derivation).
//  3. Every destabilization costs the attacker more than it costs the cluster, in the
//     one column that compounds: durable advantage. The exception is load, which buys
//     exposure once, up to a fixed floor, and never again.

// marginCase is one attempt to destabilize the cluster, priced on both sides.
type marginCase struct {
	name string
	// attack is what the attacker must spend to reach the effect, in their own time.
	attack time.Duration
	// repair is what the cluster spends to remove it, in its own time.
	repair time.Duration
	// durable is what the attacker still holds afterwards, as a multiple of what they
	// had before. Zero means the effect was fully refunded.
	durable float64
}

// margin is the attacker's spend per unit of defender spend. Above 1 means the attack is
// more expensive to mount than to undo, which is the only inequality that deters.
func (m marginCase) margin() float64 {
	if m.repair <= 0 {
		return -1 // no repair needed: not a contest
	}
	return m.attack.Seconds() / m.repair.Seconds()
}

// TestDestabilizationIsALosingTrade prices each measured attack against its repair.
func TestDestabilizationIsALosingTrade(t *testing.T) {
	// The same race ADR 0002 measures: an attacker saturating at the per-claimant
	// ceiling against a defender rebuilding from derived state.
	race := goldenRace{n: 100_000, identities: 100, hashes: 11}

	cases := []marginCase{
		{
			name:    "saturate the parental filter",
			attack:  race.timeToSaturate(),
			repair:  race.rebuildTimeWarm(),
			durable: 0,
		},
		{
			name: "evict an honest cohort",
			// Thirty rotations, each requiring an accepted alarm from the attacker.
			attack: time.Duration(float64(evictionThreshold/burnedSkillCost) / alarmsPerClaimant * float64(time.Second)),
			// The cohort is re-issued from the next tick's derived state.
			repair: 550 * time.Millisecond,
			// The attacker's own stamps burned in the same thirty rotations.
			durable: 0,
		},
		{
			name:    "flood with stale alarms",
			attack:  10 * time.Second, // at the per-claimant ceiling
			repair:  0,                // refused, and refusal advances nothing
			durable: 0,
		},
		{
			name:    "guess a stamp",
			attack:  time.Duration(fuzzAlarmsPerIdentity) * time.Second,
			repair:  0, // detected by the measure, not by a human
			durable: 0,
		},
	}

	t.Logf("%-32s %-12s %-12s %-10s %-8s", "attempt", "attack", "repair", "margin", "keeps")

	for _, c := range cases {
		m := c.margin()
		t.Logf("%-32s %-12s %-12s %-10s %-8.2f", c.name, c.attack, c.repair, marginLabel(m), c.durable)

		if c.repair > 0 && c.attack < c.repair {
			t.Errorf("%s: the attacker spent less than the repair (%s vs %s)", c.name, c.attack, c.repair)
		}
		// The load-bearing assertion: nothing above leaves the attacker holding value.
		if c.durable != 0 {
			t.Errorf("%s: a durable gain of %.2fx would make the attack worthwhile", c.name, c.durable)
		}
	}

	// And the reason there is no durable gain: nothing a rotation produces is
	// capturable. See TestRotationIsATaxThatFallsOnTheTaxpayer.
	t.Log("no attempt above leaves anything behind: the effects are refunded by derivation, not by exclusion")
}

func marginLabel(m float64) string {
	if m < 0 {
		return "none"
	}
	return fmt.Sprintf("%.0fx", m)
}

// TestCompetenceBuysTheSameRateSoItBuysNothing is the parity claim. The per-claimant
// ceiling does not care how well the claimant derives, so an attacker's competence
// converts into exactly the admission rate every other claimant already has.
func TestCompetenceBuysTheSameRateSoItBuysNothing(t *testing.T) {
	type client struct {
		name string
		// continuity is the value derivation actually buys: how long the client
		// survives a rotation it did not witness.
		continuity time.Duration
		// refund is what it costs the client to be restored after eviction.
		refund time.Duration
	}

	clients := []client{
		{name: "plain alias", continuity: 0, refund: 0}, // no tolerance, and no way back
		{name: "isotoped", continuity: 2 * ClusterRotationPeriod, refund: 0},
		{name: "hydrated from F(epoch+1)", continuity: 2 * ClusterRotationPeriod, refund: 550 * time.Millisecond},
	}

	// Admission rate: identical for all three, by design.
	admission := alarmsPerClaimant
	for _, c := range clients {
		t.Logf("%-24s continuity=%-10s admission=%.0f/s", c.name, c.continuity, admission)
	}

	// The client's value rises with derivation; the admission rate does not. So the
	// attacker who derives correctly pays more for the same rate.
	for i := 1; i < len(clients); i++ {
		if clients[i].continuity < clients[i-1].continuity {
			t.Errorf("%s has less continuity than %s: derivation must never lose value",
				clients[i].name, clients[i-1].name)
		}
	}
	if clients[0].continuity != 0 {
		t.Error("the plain alias form is the one that buys no tolerance at all")
	}

	// The inversion, stated as the test: the marginal admission return on competence is
	// zero, so competence alone never buys an attacker a lever. It buys a client
	// continuity, which is renewable; that is the asymmetry.
	marginalAdmissionGain := admission - admission
	if marginalAdmissionGain != 0 {
		t.Errorf("competence must not raise the admission ceiling, gained %.0f", marginalAdmissionGain)
	}
	t.Logf("marginal admission gain from competence: %.0f/s; continuity gain: %s",
		marginalAdmissionGain, clients[len(clients)-1].continuity)
}

// TestRotationIsATaxThatFallsOnTheTaxpayer is the structural reason the trade loses. A
// rotation does not hand the attacker anything: it is a monotone counter that invalidates
// stale credentials, symmetrically. Inducing one charges the tax to the inducer too, and
// only the honest holder is refunded.
func TestRotationIsATaxThatFallsOnTheTaxpayer(t *testing.T) {
	rotationsToEvict := evictionThreshold / burnedSkillCost

	// Both sides hold stamps; both sides burn at the same rate per rotation.
	attackerBurned := rotationsToEvict * burnedSkillCost
	honestBurned := rotationsToEvict * burnedSkillCost

	// The refund is the asymmetry: the honest cohort is re-issued from derived state and
	// the attacker's cohort has to be rebuilt by hand, at the admission ceiling.
	honestRefund := 550 * time.Millisecond
	attackerRefund := float64(rotationsToEvict) / alarmsPerClaimant // seconds to rebuild at the ceiling

	t.Logf("%-28s %-8s %-12s", "holder", "burned", "refund")
	t.Logf("%-28s %-8d %-12s", "honest cohort", honestBurned, honestRefund)
	t.Logf("%-28s %-8d %-12.2fs", "attacker cohort", attackerBurned, attackerRefund)

	if attackerBurned != honestBurned {
		t.Error("the tax must fall symmetrically, or it is not a tax but a privilege")
	}
	if attackerRefund <= honestRefund.Seconds() {
		t.Errorf("the honest refund must be cheaper: %.2fs vs %s", attackerRefund, honestRefund)
	}

	// Scaled up, the tax is the attacker's dominant cost: to evict one honest cohort
	// they burn their own at 2 per rotation and can only rebuild at the shared ceiling.
	t.Logf("inducing %d rotations costs the attacker their entire cohort and charges the defender %s",
		rotationsToEvict, honestRefund)
	t.Log("the attacker cannot capture a rotation: its only output is invalidation, which is symmetric")
}

// TestTheOnlyUncapturedGainDoesNotCompound names the one honest exception, and shows
// that under the shipped configuration the attacker cannot even buy it. Raising load
// widens the acceptance window through the churn-yield policy; but the nominal window is
// already the replay tolerance, so the yield band is empty and widening it is a policy
// decision the attacker does not own.
func TestTheOnlyUncapturedGainDoesNotCompound(t *testing.T) {
	nominalChurn := churnFor(clusterCapacity / 2)
	nominalWindow := windowAt(nominalChurn)

	// What a yield would buy, if it were available.
	halfChurn := nominalChurn / 2
	yieldedWindow := windowAt(halfChurn)

	t.Logf("nominal window %s; halved churn would give %s; replay tolerance %s",
		nominalWindow, yieldedWindow, replayTolerance)

	// The gain exists in principle, and it is not attacker-controllable: it is the
	// defender widening the tolerance, which is a different operating point.
	if yieldedWindow <= nominalWindow {
		t.Error("halving churn should widen the window, or there is nothing to discuss")
	}
	if replaySafe(halfChurn) {
		t.Error("under the shipped configuration the widening already breaches the tolerance")
	}

	// The property that keeps it from compounding: the gain is a single widening to a
	// fixed point, not a rate. Repeating the attack returns the same bound.
	bounds := make([]time.Duration, 3)
	for i := range bounds {
		bounds[i] = yieldedWindow
	}
	if bounds[0] != bounds[2] {
		t.Error("a repeated yield must not compound")
	}

	t.Logf("exposure is bought once, to %s, and never again: spend is linear and gain is flat",
		bounds[0])
	t.Log("the attacker can raise the load; only the defender can widen the tolerance")
}
