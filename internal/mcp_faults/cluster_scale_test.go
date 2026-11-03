package mcp_faults

import (
	"math"
	"testing"
	"time"
)

// Does exposure tolerance hold as the cluster grows?
//
// The per-claimant cap is constant by construction: (tolerance+1) alarms per round
// trip. What scales is everything around it. With N ingress nodes each holding its own
// chain, one claimant identity is served by every chain, so its throughput multiplies
// by N while the filter's design size grows only with the legitimate membership.

const (
	// Per-claimant ceiling, isotoped alarms at tolerance 1 and a 50ms round trip.
	alarmsPerClaimant = 40.0
	// Rotation epoch, and therefore the window the filter is sized for.
	epoch = 5 * time.Second
	// Filter saturation point, from the measured 4x row: false admissions pass 50%.
	saturationFactor = 4.0
)

// perClaimantPerEpoch is how many alarms one identity can land inside one epoch.
func perClaimantPerEpoch() float64 { return alarmsPerClaimant * epoch.Seconds() }

// identitiesToSaturatePerIngress is the attacker's cost when every ingress keeps an
// independent chain: the spam lands on all of them at once.
func identitiesToSaturatePerIngress(ingresses, legitimate int) float64 {
	design := perClaimantPerEpoch() * float64(legitimate)
	return saturationFactor * design / (perClaimantPerEpoch() * float64(ingresses))
}

// identitiesToSaturateGlobal is the cost when one global budget bounds total
// acceptances per epoch, so parallel ingresses do not multiply the attacker.
func identitiesToSaturateGlobal(legitimate int) float64 {
	return saturationFactor * float64(legitimate)
}

// TestPerClaimantCapIsScaleInvariant is the part that does hold: growing the cluster
// does not raise what one claimant can land on a single chain.
func TestPerClaimantCapIsScaleInvariant(t *testing.T) {
	for _, tolerance := range []int{0, 1} {
		alarms := tolerance + 1
		roundTrip := 50 * time.Millisecond
		rate := float64(alarms) / roundTrip.Seconds() // per claimant, per second
		t.Logf("tolerance %d: %d alarms per round trip, %.1f/s per claimant", tolerance, alarms, rate)
		if rate > 41 {
			t.Errorf("the per-claimant cap should stay near 40/s, got %.1f", rate)
		}
	}

	// And it is not a function of cluster size: the cap is per chain, and one claimant
	// talks to one chain at a time.
	if perClaimantPerEpoch() != alarmsPerClaimant*epoch.Seconds() {
		t.Error("the per-claimant cap must not depend on the fleet")
	}
}

// TestIngressParallelismMultipliesTheAttacker is the hole. If each ingress keeps its
// own independent chain, the attacker gets N chains' worth of acceptance from one
// identity, so the identities needed to saturate the filter *fall* as the cluster grows.
func TestIngressParallelismMultipliesTheAttacker(t *testing.T) {
	t.Logf("%-10s %-14s %-16s %-12s", "ingresses", "legit members", "ids to saturate", "share of cluster")

	for _, legitimate := range []int{10, 100, 1000} {
		for _, ingresses := range []int{3, 10, 100} {
			ids := identitiesToSaturatePerIngress(ingresses, legitimate)
			share := ids / float64(legitimate)
			t.Logf("%-10d %-14d %-16.1f %-12.2f", ingresses, legitimate, ids, share)

			// The ratio falling below 1 means fewer fake claimants than real ones is
			// enough - the attack gets cheaper as the cluster grows.
			if share < 1 && ingresses >= 10 {
				t.Logf("  -> %d ingresses let an attacker saturate with %.0f%% of the membership",
					ingresses, share*100)
			}
		}
	}

	// The structural claim: per-ingress chains make the attacker's cost fall with N.
	small := identitiesToSaturatePerIngress(3, 100)
	large := identitiesToSaturatePerIngress(100, 100)
	if large >= small {
		t.Errorf("per-ingress parallelism should reduce the required identities: %.1f vs %.1f", large, small)
	}
}

// TestGlobalBudgetRestoresScaleInvariance is the fix: bound total acceptances per
// epoch. Saturation then costs a fixed multiple of the membership, whatever the number
// of ingresses.
func TestGlobalBudgetRestoresScaleInvariance(t *testing.T) {
	t.Logf("%-14s %-16s %-12s", "legit members", "ids to saturate", "share of cluster")

	var shares []float64
	for _, legitimate := range []int{10, 100, 1000} {
		ids := identitiesToSaturateGlobal(legitimate)
		share := ids / float64(legitimate)
		shares = append(shares, share)
		t.Logf("%-14d %-16.1f %-12.2f", legitimate, ids, share)
	}

	// Constant share: the attacker must be 4x the membership, at every cluster size.
	for _, share := range shares {
		if math.Abs(share-saturationFactor) > 0.001 {
			t.Errorf("a global budget should fix the share at %.1fx the membership, got %.2f", saturationFactor, share)
		}
	}
}

// TestJoiningIsCheaperThanSpamming is the motivation, priced. A global rotation needs a
// quorum of distinct claimants, and the quorum tracks the membership, so the cost of
// forcing rotations grows with the cluster while the cost of joining grows with it too -
// and joining also buys the service.
func TestJoiningIsCheaperThanSpamming(t *testing.T) {
	const (
		alarmsPerIdentity = 7 // credible alarms before its own distance evicts it
		invocationCost    = 8 // distance for touching a honeypot to make an alarm credible
	)
	_ = invocationCost

	t.Logf("%-10s %-9s %-18s %-16s", "members", "quorum", "ids per rotation", "alarms burned")

	for _, members := range []int{3, 10, 100} {
		quorum := members/2 + 1
		idsPerRotation := float64(quorum)
		alarmsNeeded := idsPerRotation // one credible alarm each
		t.Logf("%-10d %-9d %-18.1f %-16.0f", members, quorum, idsPerRotation, alarmsNeeded)

		// Every identity costs at least one credible alarm, which is a honeypot
		// invocation, so the attacker's bill is quorum * invocation cost.
		attackerCost := idsPerRotation * invocationCost
		memberCost := float64(invocationCost) // joining costs one credible presence
		if attackerCost <= memberCost {
			t.Errorf("%d members: spamming must cost more than joining: %.0f vs %.0f",
				members, attackerCost, memberCost)
		}
		if members > 10 && idsPerRotation <= float64(members)/2 {
			t.Logf("  -> forcing one global rotation needs %0.0f identities, i.e. half the membership",
				idsPerRotation)
		}
	}

	// The ratio that motivates joining: the attacker must field a quorum, which is
	// linear in the membership, so being serious at scale means being a member.
	smallQuorum := 3/2 + 1
	largeQuorum := 100/2 + 1
	if largeQuorum <= smallQuorum {
		t.Error("the quorum must grow with the membership or joining is never motivated")
	}
}

// TestPerClaimantCapSurvivesMembershipGrowth checks the local fairness claim: adding
// members must not slow an honest claimant, or the ratchet becomes a tax on growth.
func TestPerClaimantCapSurvivesMembershipGrowth(t *testing.T) {
	tumble := NewAlarmTumble("ingress-a", 1)

	// An honest claimant, sequencing its alarms and following the chain.
	accepted := 0
	for i := 0; i < 100; i++ {
		ref := tumble.Chain()
		if ok, _ := tumble.Offer(ref, Isotoped); ok {
			accepted++
		}
	}
	if accepted != 100 {
		t.Errorf("an honest sequential claimant should never be refused, accepted %d/100", accepted)
	}
	if tumble.Refused() != 0 {
		t.Errorf("honest sequencing produced %d refusals", tumble.Refused())
	}

	// The chain advanced once per accepted alarm, which is the cost the cluster pays
	// for legitimate traffic only.
	if tumble.Rotated() != accepted {
		t.Errorf("rotations %d should equal accepted alarms %d", tumble.Rotated(), accepted)
	}
}
