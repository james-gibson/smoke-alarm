package mcp_faults

import (
	"testing"
	"time"
)

// Membership is what makes the durable path fit. A single shared row store cannot absorb a
// million records per cycle inside a tick, but the cluster does not have to: it tracks each
// member, so each member writes its own slice to its own local store, uncontended, and the
// shared store receives manifests and the filter.
//
// Two effects, and they compound:
//
//	partition  - rows/M per writer, so latency falls linearly with the membership
//	locality   - a single-writer local store has no cross-node contention, so it is faster
//	             than the shared one by the factor the shared store spends on coordination
//
// Total work is conserved - the cluster still writes a million rows - but the critical path
// is the slowest single writer, not the sum.
const (
	// Batched, shared row store: what a contended multi-writer path sustains.
	sharedRowsPerSecond = 100_000
	// Single-writer local store: WAL, no coordination, no network.
	localRowsPerSecond = 300_000
)

// shard is one member's share of one cycle.
type shard struct {
	members   int
	rowsEach  int
	shared    time.Duration // all rows through the shared store
	local     time.Duration // each member's slice, in parallel, on local disk
	wallClock time.Duration // the slowest member, which is the critical path
}

func shardFor(members int) shard {
	rowsEach := recordsPerCycle / members
	shared := time.Duration(float64(recordsPerCycle) / sharedRowsPerSecond * float64(time.Second))
	local := time.Duration(float64(rowsEach) / localRowsPerSecond * float64(time.Second))
	return shard{members: members, rowsEach: rowsEach, shared: shared, local: local, wallClock: local}
}

// TestMembershipPartitionsTheDurablePath is the table: how many members it takes before a
// fully materialized write fits the tick twice over.
func TestMembershipPartitionsTheDurablePath(t *testing.T) {
	t.Logf("%-8s %-10s %-12s %-12s %-10s", "members", "rows each", "local write", "twice over?", "vs shared")

	for _, m := range []int{1, 2, 4, 8, 16} {
		s := shardFor(m)
		twice := 2*s.wallClock <= ClusterRotationPeriod
		t.Logf("%-8d %-10d %-12s %-12v %-10.1fx", m, s.rowsEach, s.wallClock, twice, float64(s.shared)/float64(s.wallClock))

		if m > 1 && s.wallClock >= shardFor(m/2).wallClock {
			t.Errorf("%d members should be faster than %d", m, m/2)
		}
	}

	// The minimum membership for twice-over, at both store rates.
	minAtLocal := 0
	for m := 1; m <= 64; m++ {
		if 2*shardFor(m).wallClock <= ClusterRotationPeriod {
			minAtLocal = m
			break
		}
	}
	t.Logf("twice-over first holds at %d members on local stores", minAtLocal)

	minIfShared := 0
	for m := 1; m <= 64; m++ {
		if 2*time.Duration(float64(recordsPerCycle/m)/sharedRowsPerSecond*float64(time.Second)) <= ClusterRotationPeriod {
			minIfShared = m
			break
		}
	}
	t.Logf("the same, if every member wrote through the shared store: %d members", minIfShared)
	t.Logf("uncontended local writes %d rows/s vs %d rows/s shared: a %.0fx locality factor",
		localRowsPerSecond, sharedRowsPerSecond, float64(localRowsPerSecond)/float64(sharedRowsPerSecond))

	// At 300k rows/s a half-share is 1.67 s, so two members suffice; through the shared store
	// it takes four, because half of a million rows at 100k rows/s is 5 s.
	if minAtLocal != 2 {
		t.Errorf("two members at 300k rows/s should suffice, got %d", minAtLocal)
	}
	if minIfShared <= minAtLocal {
		t.Error("locality must reduce the membership needed, or the perk is not a perk")
	}
}

// TestMembershipTurnsElevenSecondsIntoOne is the headline comparison against the shared
// pipeline in durable_budget_test.go.
func TestMembershipTurnsElevenSecondsIntoOne(t *testing.T) {
	shared := total(materializedPath(recordsPerCycle))
	member := shardFor(4)

	// Each member also ships its own slice in parallel, so the object store cost is per
	// slice rather than per cluster.
	sliceBytes := int64(member.rowsEach) * bytesPerEntry
	ship := time.Duration(float64(sliceBytes) / float64(s3BytesPerSecond) * float64(duplicateCopies) * float64(time.Second))
	withShip := member.wallClock + ship

	t.Logf("shared path:        %s (%.2f ticks)", shared, float64(shared)/float64(ClusterRotationPeriod))
	t.Logf("4 member-local:      %s write + %s ship = %s (%.2f ticks)",
		member.wallClock, ship, withShip, float64(withShip)/float64(ClusterRotationPeriod))
	t.Logf("improvement:        %.1fx", float64(shared)/float64(withShip))

	if withShip > ClusterRotationPeriod {
		t.Errorf("the member-local path should fit a tick, got %s", withShip)
	}
	if 2*withShip > ClusterRotationPeriod {
		t.Errorf("and it should fit twice over, got %s", 2*withShip)
	}

	// Total bytes shipped are unchanged: partitioning buys latency, not bandwidth.
	totalShipped := time.Duration(float64(int64(recordsPerCycle)*bytesPerEntry) / float64(s3BytesPerSecond) * float64(duplicateCopies) * float64(time.Second))
	t.Logf("cluster bytes: %s either way - the gain is parallelism, not less work", totalShipped)
}

// TestTierDecidesWhichStoreYouGet is [ADR 0001] in operational terms: joining buys the local
// write path, and being tracked is what makes the slice addressable.
func TestTierDecidesWhichStoreYouGet(t *testing.T) {
	member := shardFor(4)
	memberShip := time.Duration(float64(int64(member.rowsEach)*bytesPerEntry)/float64(s3BytesPerSecond)*
		float64(duplicateCopies)*float64(time.Second)) + member.wallClock

	// A non-member does not get a slice: its records go through the shared path, or are not
	// tracked at all.
	nonMember := total(materializedPath(recordsPerCycle))

	t.Logf("member (1/4 slice): %-8s tracked, addressable, local disk", memberShip)
	t.Logf("non-member:         %-8s shared path, no local slot", nonMember)

	if memberShip >= nonMember {
		t.Error("membership must be worth something measurable")
	}

	// And the perk is bounded by membership, not by asking: the store cost scales with the
	// number of tracked members, which is the same quantity every quorum argument rests on.
	t.Logf("local slots are bounded by membership: %d shards, not on demand", member.members)
	t.Log("being tracked is the price of the fast path; the alarm remains for everyone else")
}
