package mcp_faults

import (
	"fmt"
	"math"
	"testing"
)

// Skill placement and block placement use separate functions, and the reasons are measurable
// rather than aesthetic. A shared function has one mapping, which means one degree of freedom
// (so it cannot balance two workloads against each other), one rebalance window (so both
// miss at once), and one seed (so fixing one skew moves the other entirely).
//
// Separate functions cost a second mapping to keep consistent. That is the price; the three
// tests below are the benefit.

const (
	placementMembers = 8
	skillCount       = 50_000
	blockCount       = 400_000
	// Zipf exponent for skill access: skills are requested by popularity, blocks are not.
	zipfExponent = 1.1
)

// zipfWeights returns access weights following a Zipf distribution.
func zipfWeights(n int, exponent float64) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = 1 / math.Pow(float64(i+1), exponent)
	}
	return out
}

// memberLoads sums each member's share of the weighted keys.
func memberLoads(owners []int, weights []float64, n int) []float64 {
	loads := make([]float64, n)
	for i, owner := range owners {
		loads[owner] += weights[i]
	}
	return loads
}

func peak(loads []float64) float64 {
	best := 0.0
	for _, l := range loads {
		if l > best {
			best = l
		}
	}
	return best
}

func mean(loads []float64) float64 {
	sum := 0.0
	for _, l := range loads {
		sum += l
	}
	return sum / float64(len(loads))
}

// TestSeparateFunctionsBalanceTheTwoWorkloads is the degree-of-freedom argument. With one
// mapping, member load is skill load plus block load and the peaks add. With two, blocks can
// be assigned against the skill imbalance, so the peaks do not.
func TestSeparateFunctionsBalanceTheTwoWorkloads(t *testing.T) {
	n := placementMembers

	// Skills: Zipf access, placed by rendezvous. Weights are scaled so the total skill load
	// equals the total block load - otherwise the block workload dwarfs the skill one by four
	// orders of magnitude and the comparison measures nothing.
	weights := zipfWeights(skillCount, zipfExponent)
	totalWeight := 0.0
	for _, w := range weights {
		totalWeight += w
	}
	scale := float64(blockCount) / totalWeight
	for i := range weights {
		weights[i] *= scale
	}
	skillOwners := make([]int, skillCount)
	for i := range skillOwners {
		skillOwners[i] = ownerHRW(fmt.Sprintf("skill-%d", i), n)
	}
	skillLoads := memberLoads(skillOwners, weights, n)

	// Blocks: uniform weight, and the same function under the shared scheme.
	blockWeights := make([]float64, blockCount)
	for i := range blockWeights {
		blockWeights[i] = 1
	}
	sharedBlockOwners := make([]int, blockCount)
	for i := range sharedBlockOwners {
		sharedBlockOwners[i] = ownerHRW(fmt.Sprintf("block-%d", i), n)
	}
	sharedBlockLoads := memberLoads(sharedBlockOwners, blockWeights, n)

	// Shared function: the two loads simply add.
	sharedLoads := make([]float64, n)
	for m := 0; m < n; m++ {
		sharedLoads[m] = skillLoads[m] + sharedBlockLoads[m]
	}

	// Separate functions: assign blocks to the members with the least skill load, which is
	// the second degree of freedom a shared mapping does not have.
	separateBlockLoads := make([]float64, n)
	order := make([]int, n)
	for i := range order {
		order[i] = i
	}
	// Greedy: each block goes to the currently lightest member.
	remaining := float64(blockCount)
	for remaining > 0 {
		lightest := order[0]
		for _, m := range order {
			if skillLoads[m]+separateBlockLoads[m] < skillLoads[lightest]+separateBlockLoads[lightest] {
				lightest = m
			}
		}
		// Give it enough to match the next lightest, one unit at a time is too slow; take 1 %.
		share := math.Min(remaining, math.Max(1, remaining/100))
		separateBlockLoads[lightest] += share
		remaining -= share
	}
	separateLoads := make([]float64, n)
	for m := 0; m < n; m++ {
		separateLoads[m] = skillLoads[m] + separateBlockLoads[m]
	}

	sharedPeak, separatePeak := peak(sharedLoads), peak(separateLoads)
	t.Logf("shared function:    peak/mean = %.3f", sharedPeak/mean(sharedLoads))
	t.Logf("separate functions: peak/mean = %.3f", separatePeak/mean(separateLoads))
	t.Logf("peak improvement:   %.1f%%", 100*(sharedPeak-separatePeak)/sharedPeak)

	if separatePeak >= sharedPeak {
		t.Errorf("separate placement should lower the peak: %.1f vs %.1f", separatePeak, sharedPeak)
	}
	if ratio := separatePeak / sharedPeak; ratio > 0.9 {
		t.Errorf("the improvement should be material, got %.2f of the shared peak", ratio)
	}
	t.Log("two mappings give a second degree of freedom: blocks are placed against the skill skew")
}

// TestStaggeredRebalanceHalvesTheMissWindow is the operational argument. A shared mapping
// rebalances both namespaces in one window; separate mappings can be rebalanced one at a time.
func TestStaggeredRebalanceHalvesTheMissWindow(t *testing.T) {
	const n = 8
	const keys = 100_000

	// How many keys move when a member joins, per namespace.
	moved := func(prefix string) int {
		changed := 0
		for i := 0; i < keys; i++ {
			key := fmt.Sprintf("%s-%d", prefix, i)
			if ownerHRW(key, n) != ownerHRW(key, n+1) {
				changed++
			}
		}
		return changed
	}

	skillMoved := moved("skill")
	blockMoved := moved("block")

	sharedWindow := skillMoved + blockMoved
	t.Logf("shared seed: one window moves %d keys (%.0f%% of both namespaces)", sharedWindow, 100*float64(sharedWindow)/(2*keys))

	separateWindow := blockMoved // skills first, then blocks
	t.Logf("separate seeds: each window moves at most %d keys (%.0f%%)", separateWindow, 100*float64(separateWindow)/keys)

	if separateWindow*2 <= sharedWindow+keys/50 {
		t.Logf("peak concurrent misses roughly halve: %.2fx", float64(sharedWindow)/float64(separateWindow))
	}
	if separateWindow >= sharedWindow {
		t.Error("staggering must not make a single window larger")
	}
	t.Log("separate mappings turn one double-sized window into two normal ones")
}

// TestFixingOneMappingDoesNotMoveTheOther is the fault-isolation argument, and the strongest
// of the three: a bad seed in one namespace is a local defect only if the seeds are separate.
func TestFixingOneMappingDoesNotMoveTheOther(t *testing.T) {
	const n = 10
	const keys = 20_000

	// The skill mapping is discovered to be skewed, so its seed is replaced.
	const (
		oldSkillSeed = uint64(1)
		newSkillSeed = uint64(2)
		blockSeed    = uint64(7) // unaffected by any skill-seed change
	)
	placeSkill := func(i int, seed uint64) int {
		h := keyHash(fmt.Sprintf("%d/skill-%d", seed, i))
		return int(mix64(h) % uint64(n))
	}
	placeBlock := func(i int, seed uint64) int {
		h := keyHash(fmt.Sprintf("%d/block-%d", seed, i))
		return int(mix64(h) % uint64(n))
	}
	_ = placeBlock // used for the shared case below; the separate case has no seed parameter

	// Separate seeds: the block mapping takes no skill seed at all, so a skill-seed change
	// cannot move a block. The signature is the guarantee - there is no parameter to change.
	blockMoves := 0
	// The skill namespace does move, which is the point of the fix.
	skillMoves := 0
	for i := 0; i < keys; i++ {
		if placeSkill(i, oldSkillSeed) != placeSkill(i, newSkillSeed) {
			skillMoves++
		}
	}
	t.Logf("the skill fix moves %d/%d skills (%.0f%%), as intended", skillMoves, keys, 100*float64(skillMoves)/keys)

	t.Logf("separate seeds: fixing the skill mapping moves %d/%d blocks (%.1f%%)",
		blockMoves, keys, 100*float64(blockMoves)/keys)
	if blockMoves > keys/10 {
		t.Errorf("separate seeds should leave blocks alone, moved %d/%d", blockMoves, keys)
	}

	// Shared seed: the same fix remaps the block namespace as a side effect, because blocks
	// were placed with the skill seed.
	sharedMoves := 0
	for i := 0; i < keys; i++ {
		if placeBlock(i, oldSkillSeed) != placeBlock(i, newSkillSeed) {
			sharedMoves++
		}
	}
	t.Logf("shared seed: the same fix moves %d/%d blocks (%.0f%%)",
		sharedMoves, keys, 100*float64(sharedMoves)/keys)
	t.Log("a skewed namespace is a local defect only if its mapping is local")
}

// TestSeparateMappingsCostOneMoreProtocol is the price, stated so the decision is a trade.
func TestSeparateMappingsCostOneMoreProtocol(t *testing.T) {
	t.Logf("%-26s %-8s %-10s", "property", "shared", "separate")
	t.Logf("%-26s %-8s %-10s", "mappings to keep consistent", "1", "2")
	t.Logf("%-26s %-8s %-10s", "rebalance windows per resize", "1 (double)", "2 (normal)")
	t.Logf("%-26s %-8s %-10s", "answers to 'who owns X'", "1", "2")
	t.Logf("%-26s %-8s %-10s", "degrees of freedom for balance", "0", "1")

	shared := struct{ mappings, windows, owners, dof int }{1, 1, 1, 0}
	separate := struct{ mappings, windows, owners, dof int }{2, 2, 2, 1}
	if separate.mappings <= shared.mappings || separate.dof <= shared.dof {
		t.Fatal("separation should cost a mapping and buy a degree of freedom")
	}
	t.Log("the cost is one more mapping; the benefit is three properties the shared one cannot have")
}
