package mcp_faults

import (
	"fmt"
	"math"
	"testing"
	"time"
)

// TestRotationFilterAdmitsEverythingLegitimate is the property the hot path depends
// on: a rotation that was published is never refused. False negatives would refuse
// the fire, which is the one error this design must not make.
func TestRotationFilterAdmitsEverythingLegitimate(t *testing.T) {
	f := NewRotationFilter(1000, 10)

	published := make([]string, 0, 1000)
	for i := 0; i < 1000; i++ {
		id := fmt.Sprintf("rotation-%d", i)
		published = append(published, id)
		f.Add(id)
	}

	for _, id := range published {
		if !f.Present(id) {
			t.Fatalf("published rotation %q was refused; the filter must not lie that way", id)
		}
	}
	if f.Hashes() < 1 {
		t.Fatal("a filter needs at least one probe")
	}
}

// TestRotationFilterFalseAdmissionMeasured checks the theory against the filter, since
// the false-admission rate is the security number now that per-item verification is
// skipped.
func TestRotationFilterFalseAdmissionMeasured(t *testing.T) {
	const (
		inserted = 2000
		probes   = 20000
	)

	t.Logf("%-8s %-7s %-10s %-10s", "bits/item", "hashes", "theory", "measured")
	for _, bitsPerItem := range []int{4, 8, 10, 12, 16, 24} {
		f := NewRotationFilter(inserted, bitsPerItem)
		for i := 0; i < inserted; i++ {
			f.Add(fmt.Sprintf("rotation-%d", i))
		}

		admitted := 0
		for i := 0; i < probes; i++ {
			if f.Present(fmt.Sprintf("impostor-%d", i)) {
				admitted++
			}
		}

		measured := float64(admitted) / probes
		theory := f.TheoryFalsePositive()
		t.Logf("%-8d %-7d %-10.4f %-10.4f", bitsPerItem, f.Hashes(), theory, measured)

		if theory > 0.5 {
			continue // a tiny filter is not a claim worth checking tightly
		}
		if measured > theory*3+0.005 {
			t.Errorf("%d bits/item: measured %.4f far above theory %.4f", bitsPerItem, measured, theory)
		}
		if measured == 0 && theory > 0.01 {
			t.Errorf("%d bits/item: nothing admitted but theory says %.4f", bitsPerItem, theory)
		}
	}
}

// TestSidechannelReplacesTheVerificationBottleneck is the SLA consequence. Per-item
// verification costs v; a filter probe costs a few hashes. That is the entire source
// of the mu*v fleet multiplier, so a shared filter returns the correct path to the
// serving path's capacity.
func TestSidechannelReplacesTheVerificationBottleneck(t *testing.T) {
	const (
		lambda      = 400.0
		mu          = 100.0
		verifyTime  = 50 * time.Millisecond
		probeCost   = time.Microsecond
		filterItems = 1000
	)

	servingNodes := int(math.Ceil(lambda / mu))

	perItem := slaModel{mu: mu, verifyLatency: verifyTime}
	correctNodesPerItem := perItem.nodesForCorrect(lambda)

	// A shared filter is a per-query cost, not a per-item review.
	filter := NewRotationFilter(filterItems, 16)
	probesPerNode := 1 / probeCost.Seconds()
	correctNodesFilter := int(math.Ceil(lambda / probesPerNode))

	t.Logf("serving nodes for %.0f/s: %d", lambda, servingNodes)
	t.Logf("correct nodes, per-item v=50ms: %d", correctNodesPerItem)
	t.Logf("correct nodes, filter probes (%s each, %d hashes): %d",
		probeCost, filter.Hashes(), correctNodesFilter)

	if correctNodesFilter > servingNodes {
		t.Errorf("a filter should not need more nodes than serving: %d > %d", correctNodesFilter, servingNodes)
	}
	if correctNodesPerItem <= servingNodes {
		t.Errorf("fixture: per-item verification should be the bottleneck, %d vs %d", correctNodesPerItem, servingNodes)
	}

	// The multiplier collapses from mu*v to about one.
	perItemMultiplier := float64(correctNodesPerItem) / float64(servingNodes)
	filterMultiplier := float64(correctNodesFilter) / float64(servingNodes)
	t.Logf("fleet multiplier: per-item %.1fx, sidechannel %.2fx", perItemMultiplier, filterMultiplier)

	if filterMultiplier >= perItemMultiplier/10 {
		t.Errorf("the filter should remove most of the multiplier: %.2f vs %.1f", filterMultiplier, perItemMultiplier)
	}
}

// TestBitsPerItemFollowsTheEpochBudget is the security arithmetic. The per-query rate
// compounds over an epoch, so the budget is set per epoch and the bits follow.
func TestBitsPerItemFollowsTheEpochBudget(t *testing.T) {
	const budget = 0.01 // 1% chance of admitting one impostor per epoch

	t.Logf("%-10s %-12s %-12s %-14s", "queries", "per-query p", "bits/item", "measured")
	for _, queries := range []int{100, 1000, 10000, 100000} {
		bits := BitsForFalseAdmission(queries, budget)
		perQuery := budget / float64(queries)

		// Build a filter at that budget and measure the per-query rate it delivers.
		f := NewRotationFilter(2000, bits)
		for i := 0; i < 2000; i++ {
			f.Add(fmt.Sprintf("rotation-%d", i))
		}
		admitted := 0
		for i := 0; i < 50000; i++ {
			if f.Present(fmt.Sprintf("impostor-%d", i)) {
				admitted++
			}
		}
		measured := float64(admitted) / 50000

		t.Logf("%-10d %-12.2e %-12d %-14.2e", queries, perQuery, bits, measured)

		// The epoch budget is the compounding of the per-query rate, so the measured
		// rate must stay below the per-query target.
		if measured > perQuery*2+1e-6 {
			t.Errorf("queries=%d: measured %.2e exceeds the per-query target %.2e", queries, measured, perQuery)
		}
	}

	// Monotonic: more queries in an epoch means more bits per item.
	previous := 0
	for _, queries := range []int{100, 1000, 10000, 100000} {
		bits := BitsForFalseAdmission(queries, budget)
		if bits < previous {
			t.Errorf("queries=%d needs %d bits, fewer than the %d before", queries, bits, previous)
		}
		previous = bits
	}
}

// TestStaleFilterRefusesTheNewEpoch is the failure mode a shared summary introduces.
// A filter has no false negatives only for what was inserted, so an epoch's rotations
// published into the next epoch's filter are simply absent - and absence is a refusal.
func TestStaleFilterRefusesTheNewEpoch(t *testing.T) {
	stale := NewRotationFilter(100, 10)
	for i := 0; i < 100; i++ {
		stale.Add(fmt.Sprintf("epoch-1-rotation-%d", i))
	}

	refused := 0
	for i := 0; i < 1000; i++ {
		if !stale.Present(fmt.Sprintf("epoch-2-rotation-%d", i)) {
			refused++
		}
	}
	// Essentially the whole new epoch is refused; the handful that are not are the
	// filter's false admissions, which is the same rate as any other impostor.
	if refused < 900 {
		t.Errorf("a stale filter should refuse nearly the whole new epoch, refused %d of 1000", refused)
	}
	t.Logf("stale filter: %d of 1000 new-epoch rotations refused, %d admitted as false positives",
		refused, 1000-refused)

	// Which is why grace exists: the window in which two filters are both acceptable.
	if DefaultGraceWindow <= 0 {
		t.Error("grace must be positive for two filters to overlap")
	}
}
