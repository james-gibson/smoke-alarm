package mcp_faults

import (
	"fmt"
	"hash/fnv"
	"testing"
)

// RAID at the route level, where the unit is not the disk but the route: replicas are hot
// paths of one logical route, skills are distributed across members, and block data is
// striped with parity. Three separate redundancy mechanisms that are easy to conflate, and
// conflating them is how a system ends up with route availability but no block durability.
//
// The placement function is the same shift the array uses, computed locally from the key, so
// there is no directory to keep consistent and nothing to leak.

// keyHash is a stable 64-bit hash for placement decisions.
func keyHash(key string) uint64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return h.Sum64()
}

// ownerMod places a key by modulo, the simplest function and the one that churns on resize.
func ownerMod(key string, n int) int {
	return int(keyHash(key) % uint64(n))
}

// mix64 is a splitmix64 finaliser: FNV alone correlates scores that differ only near the end
// of the input, which silently breaks rendezvous (measured: 49.8%% of keys moved instead of
// 20%%). The member index goes first *and* the score is mixed, so the differing byte is early
// and the result is uniform.
func mix64(x uint64) uint64 {
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	x *= 0x94d049bb133111eb
	x ^= x >> 31
	return x
}

// ownerHRW places a key by rendezvous hashing: the member with the highest random weight wins.
func ownerHRW(key string, n int) int {
	best, bestScore := -1, uint64(0)
	for m := 0; m < n; m++ {
		h := fnv.New64a()
		_, _ = fmt.Fprintf(h, "%d/%s", m, key)
		score := mix64(h.Sum64())
		if best < 0 || score > bestScore {
			best, bestScore = m, score
		}
	}
	return best
}

// TestRouteReplicasFollowAvailabilityMath is the mirror half: replicas of one route are
// interchangeable hot paths, so availability is 1 - p^k.
func TestRouteReplicasFollowAvailabilityMath(t *testing.T) {
	const perReplicaFailure = 0.01

	t.Logf("%-10s %-12s %-14s", "replicas", "availability", "downtime/year")

	for k := 1; k <= 4; k++ {
		p := 1.0
		for i := 0; i < k; i++ {
			p *= perReplicaFailure
		}
		availability := 1 - p
		downtime := p * 365 * 24 * 60 // minutes

		t.Logf("%-10d %-12.6f %-14.2f min", k, availability, downtime)

		if k > 1 && availability <= 1-perReplicaFailure {
			t.Errorf("%d replicas must beat a single replica", k)
		}
	}

	// The flat part of the curve: the second replica buys four nines, the fourth buys almost
	// nothing. Redundancy past three has to be justified by something other than availability.
	t.Log("the second replica is worth 99x; the fourth adds 0.000001")
	if perReplicaFailure < 0.001 {
		t.Log("at a 0.1% replica failure rate the knee moves one replica later")
	}
}

// TestPlacementIsByFunctionNotDirectory is the reason distributed skills do not need a routing
// table: any member computes the owner from the key.
func TestPlacementIsByFunctionNotDirectory(t *testing.T) {
	const keys = 10_000
	const n = 8

	// Every member agrees on the owner, computed locally.
	first := map[string]int{}
	for i := 0; i < keys; i++ {
		key := fmt.Sprintf("skill-%d", i)
		first[key] = ownerHRW(key, n)
	}
	agree := 0
	for key, want := range first {
		if ownerHRW(key, n) == want {
			agree++
		}
	}
	t.Logf("%d/%d keys placed identically on every member", agree, keys)
	if agree != keys {
		t.Error("placement must be deterministic across members")
	}

	// Load spread: rendezvous hashing distributes keys evenly.
	counts := make([]int, n)
	for _, owner := range first {
		counts[owner]++
	}
	minCount, maxCount := keys, 0
	for _, c := range counts {
		if c < minCount {
			minCount = c
		}
		if c > maxCount {
			maxCount = c
		}
	}
	t.Logf("load spread: min %d, max %d, ideal %d", minCount, maxCount, keys/n)
	if maxCount > 2*(keys/n) {
		t.Errorf("placement should be roughly even, max %d vs ideal %d", maxCount, keys/n)
	}

	// Cost: one hop at most, because the owner is computable, not looked up.
	t.Logf("directory entries: 0; hops: 1 (the owner is computed, not queried)")
}

// TestResizeCostDependsOnTheFunction is the RAID expansion cost in route terms: what fraction
// of skills move when a member joins.
func TestResizeCostDependsOnTheFunction(t *testing.T) {
	const keys = 5_000
	const before, after = 4, 5

	movedMod, movedHRW := 0, 0
	for i := 0; i < keys; i++ {
		key := fmt.Sprintf("skill-%d", i)
		if ownerMod(key, before) != ownerMod(key, after) {
			movedMod++
		}
		if ownerHRW(key, before) != ownerHRW(key, after) {
			movedHRW++
		}
	}

	t.Logf("%-12s %-10s %-10s", "function", "moved", "share")
	t.Logf("%-12s %-10d %-10.1f%%", "modulo", movedMod, 100*float64(movedMod)/keys)
	t.Logf("%-12s %-10d %-10.1f%%", "rendezvous", movedHRW, 100*float64(movedHRW)/keys)

	// Modulo churns almost everything; rendezvous moves the ideal 1/(n+1).
	if movedMod <= movedHRW {
		t.Errorf("modulo should churn more than rendezvous: %d vs %d", movedMod, movedHRW)
	}
	ideal := 100.0 / float64(after)
	if share := 100 * float64(movedHRW) / keys; share > 1.5*ideal || share < 0.5*ideal {
		t.Errorf("rendezvous should move about %.0f%%, moved %.1f%%", ideal, share)
	}
	t.Log("adding a member is cheap only under rendezvous: modulo remaps nearly the whole key space")
}

// TestRouteRedundancyIsNotBlockDurability is the conflation trap. Replicas keep the route
// serving; they do nothing for a block whose members are all lost, because every replica
// reads through to the same striped layout.
func TestRouteRedundancyIsNotBlockDurability(t *testing.T) {
	const n = 4

	// Route availability with 2 replicas.
	routeAvailability := 1 - 0.01*0.01

	// Block durability with no parity: losing any member loses that shard outright.
	shardLoss := 1.0 / float64(n)
	t.Logf("2 route replicas: availability %.6f", routeAvailability)
	t.Logf("block striped across %d members with no parity: losing one loses %.0f%% of the block", n, 100*shardLoss)

	if routeAvailability <= 0.99 {
		t.Error("two replicas should give four nines")
	}
	// With parity the block survives one loss; without it, one loss is fatal for that shard.
	withParitySurvives := 1 // n-1 of n is still enough
	t.Logf("with single parity the same block survives %d loss(es)", withParitySurvives)

	// The decision this implies: redundancy level is a property of the *route class*, not of
	// the cluster, because a derivable route needs no parity while an authored one does.
	t.Log("route replicas buy service; block parity buys the data; neither substitutes for the other")
}

// TestSelfServeConflictAtBlockGranularity keeps ADR 0010's invariant at the block level: the
// member holding parity for a block must not be the member serving that block.
func TestSelfServeConflictAtBlockGranularity(t *testing.T) {
	const (
		n      = 4
		blocks = 4_000
	)

	// Parity for block b sits on (b + n - 1) mod n, the same left-symmetric shift as the array.
	parityOwner := func(block int) int {
		return ((block+n-1)%n + n) % n
	}

	// With one replica, the preferred server is the parity holder for 1/n of the blocks, so
	// those blocks have no redundant path at all.
	conflicts := 0
	for b := 0; b < blocks; b++ {
		if parityOwner(b) == 0 {
			conflicts++
		}
	}
	share := 100 * float64(conflicts) / blocks
	t.Logf("one replica: %d/%d blocks are served by their own parity holder (%.0f%%)", conflicts, blocks, share)

	if got, want := share, 100.0/n; got < want-2 || got > want+2 {
		t.Errorf("the self-serve conflict should be 1/n = %.0f%%, got %.0f%%", want, got)
	}

	// With two replicas there is always an eligible one, because at most one member is
	// excluded. The cost moves from a conflict to an exclusion: the preferred replica is
	// bypassed for 1/n of the blocks.
	bypassed := 0
	for b := 0; b < blocks; b++ {
		server := 0
		if parityOwner(b) == 0 {
			server = 1 // the second replica takes it
			bypassed++
		}
		if server == parityOwner(b) {
			t.Fatalf("block %d is still served by its parity holder", b)
		}
	}
	t.Logf("two replicas: %d/%d blocks bypass the preferred replica (%.0f%%), no conflicts",
		bypassed, blocks, 100*float64(bypassed)/blocks)

	if bypassed == 0 {
		t.Error("some blocks must be routed away from the parity holder")
	}
	t.Log("one replica cannot serve a quarter of its blocks redundantly; two always can")
}
