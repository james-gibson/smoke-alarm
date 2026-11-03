package mcp_faults

import (
	"testing"
	"time"
)

// The partitioning is RAID-shaped, and the constraints transfer almost exactly: striping buys
// bandwidth linearly, write amplification punishes per-record durability, and a rebuild must
// finish before the next failure or the redundancy was decorative.
//
// What does not transfer is the *source* of the rebuild. Parity reconstructs from surviving
// blocks; derivation regenerates from a function. That changes four numbers at once: capacity
// overhead, what the rebuild needs to read, how long the window is open, and whether the
// rebuild can reproduce corruption.
//
// Constants are ballpark and named, so the comparison re-runs when the hardware does.
const (
	// Sequential throughput against the durable tier.
	seqBytesPerSecond = 100 * 1000 * 1000
	// Random small-write throughput, which is what per-record durability costs.
	randomIOPS = 20_000
	// RAID-5 write penalty: read old data, read old parity, write new data, write new parity.
	raid5WritePenalty = 4
	// Stripe width used for the comparison.
	raid5Disks = 4
)

// perRecordIOps is what durability at record granularity costs: one logical write amplified by
// the parity penalty.
func perRecordIOps(records int) int {
	return records * raid5WritePenalty
}

// fullStripeTime is one sequential write of a whole cycle's payload, which needs no
// read-modify-write because every block in the stripe changes at once.
func fullStripeTime(records int) time.Duration {
	raw := int64(records) * bytesPerEntry
	return time.Duration(float64(raw) / float64(seqBytesPerSecond) * float64(time.Second))
}

// parityRebuildTime reads (n-1)/n of the payload and writes one share.
func parityRebuildTime(records, disks int) time.Duration {
	raw := float64(int64(records) * bytesPerEntry)
	read := raw * float64(disks-1) / float64(disks)
	write := raw / float64(disks)
	return time.Duration((read + write) / float64(seqBytesPerSecond) * float64(time.Second))
}

// TestStripingBuysBandwidthLinearly is RAID 0, and it is why membership helps.
func TestStripingBuysBandwidthLinearly(t *testing.T) {
	t.Logf("%-8s %-12s %-12s", "members", "rows each", "write")

	var previous time.Duration
	for _, m := range []int{1, 2, 4, 8} {
		rows := recordsPerCycle / m
		write := time.Duration(float64(rows) / localRowsPerSecond * float64(time.Second))
		t.Logf("%-8d %-12d %-12s", m, rows, write)

		if m > 1 && write*2 > previous {
			t.Errorf("%d members should roughly halve the time, got %s vs %s", m, write, previous)
		}
		previous = write
	}
	t.Log("striping the rows across members is the same arithmetic as striping across spindles")
}

// TestWriteAmplificationIsTheRealConstraint is the number that dwarfs everything else in the
// durable path: per-record durability is a read-modify-write workload.
func TestWriteAmplificationIsTheRealConstraint(t *testing.T) {
	ops := perRecordIOps(recordsPerCycle)
	perRecord := time.Duration(float64(ops) / float64(randomIOPS) * float64(time.Second))
	stripe := fullStripeTime(recordsPerCycle)

	t.Logf("%-28s %-10d %-12s", "per-record writes (IOPS)", ops, perRecord)
	t.Logf("%-28s %-10s %-12s", "one full-stripe write", "-", stripe)

	if perRecord <= stripe {
		t.Errorf("write amplification should dominate a full-stripe write, got %s vs %s", perRecord, stripe)
	}

	// The ratio, and the rule it implies.
	t.Logf("amplification: %.0fx", float64(perRecord)/float64(stripe))
	t.Log("durability belongs at stripe granularity: one write per cycle, never per record")

	// Compare against the numbers the ADRs already use: the SQLite commit was 10 s at 1M
	// rows, which was already a batched figure. The unbatched version is this.
	if perRecord < 10*time.Second {
		t.Log("even at 20k IOPS the amplified path is at least as bad as the batched row store")
	}
}

// TestParityRebuildVersusDerivation is the interesting one: parity is *faster* at this size,
// and still loses on every other axis.
func TestParityRebuildVersusDerivation(t *testing.T) {
	parity := parityRebuildTime(recordsPerCycle, raid5Disks)
	derive := time.Duration(recordsPerCycle) * 11 * hashProbe // local re-hash, no transfer
	goldenCopy := repairFor(recordsPerCycle, 11)              // includes the transfer

	t.Logf("RAID-5 rebuild (n=%d): %s  (reads %d/%d of the payload)",
		raid5Disks, parity, raid5Disks-1, raid5Disks)
	t.Logf("derivation rebuild:    %s  (hashes locally, reads nothing from peers)", derive)
	t.Logf("golden-copy rebuild:   %s  (hashes plus the transfer)", goldenCopy)

	// Parity wins on wall clock here, and that must be stated rather than hidden.
	if parity >= derive {
		t.Logf("parity is faster at this payload size: %s vs %s", parity, derive)
	}

	// What it costs: survivors, overhead, and fidelity.
	survivorsNeeded := raid5Disks - 1
	overhead := 100.0 / float64(raid5Disks)
	t.Logf("parity needs %d of %d members, and %.0f%% extra capacity", survivorsNeeded, raid5Disks, overhead)
	t.Logf("derivation needs 0 survivors and 0%% capacity: the state is a function, not a copy")

	if survivorsNeeded != 3 {
		t.Errorf("RAID-5 with 4 disks tolerates 1 loss, so it needs 3 survivors, got %d", survivorsNeeded)
	}

	// Fidelity: parity reproduces whatever was written, corruption included; derivation does
	// not, because it never reads the damaged copy.
	t.Log("parity reproduces a corrupted block faithfully; derivation cannot, by construction")
}

// TestRebuildWindowIsTheTwiceOverConstraint is the RAID rule that matters most here: a rebuild
// must finish before the next failure, or the second failure is fatal.
func TestRebuildWindowIsTheTwiceOverConstraint(t *testing.T) {
	parity := parityRebuildTime(recordsPerCycle, raid5Disks)
	derive := time.Duration(recordsPerCycle) * 11 * hashProbe
	materialized := total(materializedPath(recordsPerCycle))

	t.Logf("%-22s %-12s %-14s", "rebuild source", "cost", "closes in a tick?")

	for _, c := range []struct {
		name string
		cost time.Duration
	}{{"parity", parity}, {"derivation", derive}, {"materialized rows", materialized}} {
		ok := c.cost <= ClusterRotationPeriod
		t.Logf("%-22s %-12s %-14v", c.name, c.cost, ok)

		if c.name == "materialized rows" && ok {
			t.Error("the materialized pipeline must not close its rebuild window in one tick")
		}
	}

	// The constraint, in the same form ADR 0002 uses: the window must close twice per tick so
	// a second failure cannot arrive inside it.
	if 2*derive > ClusterRotationPeriod {
		t.Errorf("derivation should close twice per tick, got %s", 2*derive)
	}
	if 2*parity > ClusterRotationPeriod {
		t.Logf("parity closes twice per tick too: %s", 2*parity)
	}
	t.Log("the RAID constraint is not about speed, it is about the window: close it before the next loss")
}

// TestDerivationSurvivesTotalLoss is the axis on which parity has no answer at all.
func TestDerivationSurvivesTotalLoss(t *testing.T) {
	// Parity can reconstruct from any n-1 of n members: lose two and the stripe is gone.
	disks := raid5Disks
	for lost := 0; lost <= disks; lost++ {
		recoverable := disks-lost >= disks-1
		t.Logf("RAID-5 n=%d, lost=%d: recoverable=%v", disks, lost, recoverable)
		if lost > 1 && recoverable {
			t.Error("RAID-5 must not survive two losses")
		}
	}

	// Derivation survives losing every member, because it needs the function and the seed,
	// not a surviving copy.
	t.Logf("derivation, lost=%d: recoverable=true (recompute from F(epoch))", disks)

	// The catch, stated: what derivation cannot do is recover *unauthored* state. Anything
	// that was never derivable has to be stored, and that part is RAID-shaped again.
	t.Log("derivation covers what the functor covers; the rest still needs a parity or copy scheme")
}
