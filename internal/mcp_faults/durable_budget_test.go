package mcp_faults

import (
	"testing"
	"time"
)

// A golden cluster tracking a million routes per cycle, with a durable path: process the
// records, duplicate them through SQLite, ship them to object storage. The question is
// whether that pipeline fits the tick - and the answer splits the design in two.
//
// The numbers are assumptions where they are not already constants, and every one is named
// so the budget can be re-run when hardware changes.
const (
	recordsPerCycle = 1_000_000
	// Per-record cost of materializing a record from the derivation.
	derivePerRecord = 100 * time.Nanosecond
	// Batched SQLite insert rate with WAL and one transaction per cycle.
	sqliteRowsPerSecond = 100_000
	// Object store throughput, per copy.
	s3BytesPerSecond = 100 * 1000 * 1000 // 100 MB/s per copy
	// Two copies for durability.
	duplicateCopies = 2
	// The filter is the durable artifact of a derived state.
	filterBitsPerItem = 26
)

// stage is one step of the pipeline, with its cost at the given record count.
type stage struct {
	name string
	cost time.Duration
}

// materializedPath writes every record to SQLite and then ships the rows.
func materializedPath(records int) []stage {
	raw := int64(records) * bytesPerEntry
	return []stage{
		{"derive", time.Duration(records) * derivePerRecord},
		{"hash into filter", time.Duration(records) * 11 * hashProbe},
		{"sqlite commit", time.Duration(float64(records)/sqliteRowsPerSecond*float64(time.Second)) * 1},
		{"s3 ship (2 copies)", time.Duration(float64(raw) / float64(s3BytesPerSecond) * float64(duplicateCopies) * float64(time.Second))},
	}
}

// derivedPath ships the manifest and the filter; the records are recomputed on read.
func derivedPath(records int) []stage {
	filterBytes := int64(records) * filterBitsPerItem / 8
	return []stage{
		{"manifest to sqlite", 100 * time.Microsecond},
		{"filter to s3 (2 copies)", time.Duration(float64(filterBytes) / float64(s3BytesPerSecond) * float64(duplicateCopies) * float64(time.Second))},
	}
}

func total(stages []stage) time.Duration {
	var sum time.Duration
	for _, s := range stages {
		sum += s.cost
	}
	return sum
}

func report(t *testing.T, label string, stages []stage) time.Duration {
	t.Logf("%s:", label)
	for _, s := range stages {
		t.Logf("  %-22s %s", s.name, s.cost)
	}
	sum := total(stages)
	t.Logf("  %-22s %s", "total", sum)
	return sum
}

// TestDerivedPathFitsTheTickTwiceOver is the design: the durable path carries the
// derivation and the filter, never the million records.
func TestDerivedPathFitsTheTickTwiceOver(t *testing.T) {
	sum := report(t, "derived path, 1M routes", derivedPath(recordsPerCycle))

	if 2*sum > ClusterRotationPeriod {
		t.Fatalf("the derived path must fit twice over: %s", 2*sum)
	}

	margin := float64(ClusterRotationPeriod) / float64(2*sum)
	t.Logf("twice over: %s, margin %.0fx", 2*sum, margin)
	t.Logf("the filter is %d bytes at %d bits/item", recordsPerCycle*filterBitsPerItem/8, filterBitsPerItem)

	if margin < 10 {
		t.Errorf("the derived path should have an order of magnitude of headroom, got %.0fx", margin)
	}
}

// TestMaterializedPathBreaksTheTick is the finding: writing a million rows per cycle does not
// fit, and no amount of care elsewhere recovers it.
func TestMaterializedPathBreaksTheTick(t *testing.T) {
	sum := report(t, "materialized path, 1M routes", materializedPath(recordsPerCycle))

	ticks := float64(sum) / float64(ClusterRotationPeriod)
	t.Logf("materialized: %s = %.2f ticks; twice-over needs %s", sum, ticks, 2*sum)

	if sum <= ClusterRotationPeriod {
		t.Fatalf("this path should not fit a tick at 1M rows; if it does, the constants changed")
	}
	if 2*sum <= ClusterRotationPeriod {
		t.Error("twice-over must fail on the materialized path, that is the point")
	}

	// The store's requirement, stated directly: to write the records twice within a tick the
	// store has to sustain 400k rows/s, which is above a batched SQLite transaction.
	needed := float64(recordsPerCycle) / (float64(ClusterRotationPeriod) / 2 / float64(time.Second))
	t.Logf("to fit twice over, the store must sustain %.0f rows/s", needed)
	if needed <= sqliteRowsPerSecond {
		t.Errorf("fixture: 1M rows twice in 5 s should exceed %.0f rows/s, got %.0f", float64(sqliteRowsPerSecond), needed)
	}

	// And the dominant term is not the network: SQLite is 10 s of the 11.3 s.
	perStore := float64(recordsPerCycle) / sqliteRowsPerSecond
	t.Logf("sqlite alone: %.1f s of the %.1f s total", perStore, sum.Seconds())
}

// TestColdRestoreFromObjectStore is the asymmetric half: the same million rows cost seconds
// to ship and ten seconds to load, so a cold restore misses at least two ticks even when the
// object store is fast.
func TestColdRestoreFromObjectStore(t *testing.T) {
	raw := int64(recordsPerCycle) * bytesPerEntry
	ship := time.Duration(float64(raw) / float64(s3BytesPerSecond) * float64(time.Second))
	load := time.Duration(float64(recordsPerCycle) / sqliteRowsPerSecond * float64(time.Second))
	cold := ship + load

	t.Logf("object store fetch %s + sqlite load %s = %s (%.2f ticks)",
		ship, load, cold, float64(cold)/float64(ClusterRotationPeriod))

	if cold <= ClusterRotationPeriod {
		t.Errorf("a cold restore of 1M rows should not fit one tick, got %s", cold)
	}
	// It is still far better than the tape-class restore the ADRs budget for, which is why
	// both numbers exist and neither is wrong.
	if cold > coldRestore {
		t.Errorf("object-store restore should beat the cold-restore worst case (%s)", coldRestore)
	}

	// The derived path is what makes this asymmetry acceptable: the live cluster never waits
	// for the durable one, so a two-tick restore is a warmer problem, not an outage.
	t.Logf("live repair %s vs durable restore %s: %.0fx", repairFor(recordsPerCycle, 11), cold,
		float64(cold)/float64(repairFor(recordsPerCycle, 11)))
}

// TestDurablePathNeverBlocksRepair is the invariant behind the split, and the reason the
// numbers can be this lopsided without being a risk.
func TestDurablePathNeverBlocksRepair(t *testing.T) {
	repair := repairFor(recordsPerCycle, 11)
	durable := total(derivedPath(recordsPerCycle))

	// The derivation is idempotent, so a missed durable write costs a snapshot, not a state.
	if durable > ClusterRotationPeriod/2 {
		t.Errorf("the durable path should leave most of the tick to the live path, got %s", durable)
	}
	t.Logf("live repair %s, durable write %s, both inside a %s tick with room for detection",
		repair, durable, ClusterRotationPeriod)

	// The rule that follows: the durable store lags by design, so nothing on the repair path
	// may read from it.
	t.Log("repair reads the derivation; the object store is for cold starts and audit only")
}
