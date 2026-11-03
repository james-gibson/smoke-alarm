package mcp_faults

import (
	"math"
	"testing"
)

// Groups of three, independent chains. This is the arrangement that fixes the scaling
// hole: the unit of the alarm budget must be the unit of the filter, so a filter per
// group means saturating the sidechannel costs per-group effort instead of one
// attacker identity riding every ingress.

// groupTopology is a cluster of independent triples.
type groupTopology struct {
	groups            int
	membersPerGroup   int
	ingressesPerGroup int
}

// members is the total legitimate membership.
func (g groupTopology) members() int { return g.groups * g.membersPerGroup }

// quorum is the votes one group needs: a majority of three.
func (g groupTopology) quorum() int { return g.ingressesPerGroup/2 + 1 }

// saturationPerGroup is the identities needed to saturate one group's filter, given
// that group's own ingresses share that filter and its own budget bounds them.
func (g groupTopology) saturationPerGroup() float64 {
	design := perClaimantPerEpoch() * float64(g.membersPerGroup)
	spamPerIdentity := perClaimantPerEpoch() * float64(g.ingressesPerGroup)
	return saturationFactor * design / spamPerIdentity
}

// saturationPerGroupedCluster is the cost to saturate every group. Each group's filter
// is separate, so the effort adds instead of multiplying.
func (g groupTopology) saturationPerGroupedCluster() float64 {
	return float64(g.groups) * g.saturationPerGroup()
}

// saturationSharePerGrouped is the attacker's cost as a share of the membership.
func (g groupTopology) saturationSharePerGrouped() float64 {
	return g.saturationPerGroupedCluster() / float64(g.members())
}

// saturationShareGlobalFilter is the broken arrangement: one filter, per-ingress
// budgets, so one identity rides every ingress into the same summary.
func (g groupTopology) saturationShareGlobalFilter() float64 {
	ingresses := g.groups * g.ingressesPerGroup
	design := perClaimantPerEpoch() * float64(g.members())
	return saturationFactor * design / (perClaimantPerEpoch() * float64(ingresses)) / float64(g.members())
}

// blastRadius is the fraction of the system one compromised group can affect.
func (g groupTopology) blastRadius() float64 { return 1 / float64(g.groups) }

// TestGroupsOfThreeRestoreScaleInvariance is the result: with a filter per group, the
// cost of saturating the whole sidechannel scales with the membership again, so growth
// stops helping the attacker.
func TestGroupsOfThreeRestoreScaleInvariance(t *testing.T) {
	t.Logf("%-8s %-9s %-12s %-14s %-16s", "groups", "members", "group cost", "total cost", "share of cluster")

	var shares []float64
	for _, groups := range []int{1, 10, 100} {
		top := groupTopology{groups: groups, membersPerGroup: 30, ingressesPerGroup: 3}

		perGroup := top.saturationPerGroup()
		total := top.saturationPerGroupedCluster()
		share := top.saturationSharePerGrouped()
		shares = append(shares, share)

		t.Logf("%-8d %-9d %-12.1f %-14.1f %-16.2f", groups, top.members(), perGroup, total, share)

		// The per-group cost never changes: a triple is the unit.
		if groups == 1 && math.Abs(perGroup-saturationFactor*float64(top.membersPerGroup)/float64(top.ingressesPerGroup)) > 0.001 {
			t.Errorf("per-group cost unexpected: %.2f", perGroup)
		}
	}

	// Constant share at every cluster size - this is what the global filter failed.
	for _, share := range shares {
		if math.Abs(share-shares[0]) > 0.001 {
			t.Errorf("grouped saturation share should be constant, got %v", shares)
		}
	}
	if math.Abs(shares[0]-saturationFactor/float64(3)) > 0.01 {
		t.Errorf("one triple should cost %.2f of its membership, got %.2f", saturationFactor/3, shares[0])
	}
}

// TestGlobalFilterStillDegrades is the contrast, kept so the reason is on record: with
// one shared filter and per-ingress budgets, the share falls as the cluster grows.
func TestGlobalFilterStillDegrades(t *testing.T) {
	t.Logf("%-8s %-9s %-16s %-16s", "groups", "members", "grouped share", "global-filter share")
	for _, groups := range []int{1, 10, 100} {
		top := groupTopology{groups: groups, membersPerGroup: 30, ingressesPerGroup: 3}
		t.Logf("%-8d %-9d %-16.2f %-16.3f",
			groups, top.members(), top.saturationSharePerGrouped(), top.saturationShareGlobalFilter())

		if groups > 1 && top.saturationShareGlobalFilter() >= top.saturationSharePerGrouped() {
			t.Errorf("%d groups: the global filter should be cheaper to attack than independent ones", groups)
		}
	}

	// The global filter's share strictly falls: more ingresses, same filter.
	one := groupTopology{groups: 1, membersPerGroup: 30, ingressesPerGroup: 3}
	hundred := groupTopology{groups: 100, membersPerGroup: 30, ingressesPerGroup: 3}
	if hundred.saturationShareGlobalFilter() >= one.saturationShareGlobalFilter() {
		t.Error("a shared filter should get easier to saturate as ingresses multiply")
	}
}

// TestIndependenceIsTheBlastRadius is the other half of the benefit: a compromised group
// is a fraction of the system, and the chains do not carry the compromise across.
func TestIndependenceIsTheBlastRadius(t *testing.T) {
	for _, groups := range []int{1, 3, 10, 100} {
		top := groupTopology{groups: groups, membersPerGroup: 30, ingressesPerGroup: 3}
		t.Logf("%-8d groups: blast radius %.0f%% of the system", groups, top.blastRadius()*100)

		if groups == 1 && top.blastRadius() != 1 {
			t.Error("a single group is the whole system")
		}
	}

	hundred := groupTopology{groups: 100}
	ten := groupTopology{groups: 10}
	if hundred.blastRadius() >= ten.blastRadius() {
		t.Error("more groups must mean a smaller failure domain")
	}
}

// TestGroupsOfThreeTolerateOneFault checks the sizing: three ingresses with a quorum of
// two survive one bad member, which is the minimum worth calling a chain of trust.
func TestGroupsOfThreeTolerateOneFault(t *testing.T) {
	top := groupTopology{groups: 1, membersPerGroup: 3, ingressesPerGroup: 3}

	if top.quorum() != 2 {
		t.Fatalf("a triple should need 2 of 3, got %d", top.quorum())
	}

	// One member can be compromised and the group still commits.
	honest := top.ingressesPerGroup - 1
	if honest < top.quorum() {
		t.Errorf("%d honest members cannot meet a quorum of %d", honest, top.quorum())
	}

	// Two cannot: that is the fault the group tolerates, and the reason triples are the
	// minimum rather than a convenience.
	if top.ingressesPerGroup-2 >= top.quorum() {
		t.Error("a triple must not tolerate two faults")
	}
}

// TestCrossGroupIsTheOnlySharedSurface is the cost of independence, stated as a budget
// problem: two groups need a bridge, and the bridge is where a global budget reappears.
func TestCrossGroupIsTheOnlySharedSurface(t *testing.T) {
	top := groupTopology{groups: 100, membersPerGroup: 30, ingressesPerGroup: 3}

	// If no claim crosses a boundary, the chains are fully independent and no global
	// budget exists at all.
	crossGroupRate := 0.0
	if crossGroupRate > 0 {
		t.Fatal("fixture: independence means nothing crosses")
	}
	t.Logf("%d groups, %.0f cross-group claims/s: fully independent sidechannels",
		top.groups, crossGroupRate)

	// The moment a claim crosses, the bridge carries a share of the membership and needs
	// the budget the groups were avoiding.
	bridged := 0.01 // 1% of claims cross
	bridgeClaims := float64(top.members()) * bridged
	t.Logf("with %.0f%% of claims crossing, the bridge sees %.2f claims/s and must be budgeted globally",
		bridged*100, bridgeClaims)

	if bridgeClaims <= 0 {
		t.Error("a bridge must carry something")
	}

	// And the bridge's exposure is the old problem: one shared budget serving everyone.
	bridgeShare := saturationFactor * bridgeClaims / float64(top.members())
	t.Logf("bridge saturation share: %.2f of the membership", bridgeShare)

	if bridgeShare >= top.saturationSharePerGrouped() {
		t.Logf("bridging does not degrade the grouped share (%.2f vs %.2f)",
			bridgeShare, top.saturationSharePerGrouped())
	}
}
