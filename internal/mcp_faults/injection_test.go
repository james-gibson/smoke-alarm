package mcp_faults

import (
	"testing"
	"time"
)

// TestFaultRandSeedWindows pins the seed rule: the seed is a function of UTC
// truncated to FaultSeedWindow, so a run is reproducible from its log timestamp
// and runs started in the same epoch make the same draws.
func TestFaultRandSeedWindows(t *testing.T) {
	base := time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)
	t.Setenv(FaultSeedEnv, "") // exercise the tumble, not a pin

	if got, want := faultRandSeed(base, "Transient Failures"), faultRandSeed(base.Add(4*time.Second), "Transient Failures"); got != want {
		t.Errorf("seed must be stable within a %s epoch: %d != %d", FaultSeedWindow, got, want)
	}

	if faultRandSeed(base, "Transient Failures") == faultRandSeed(base.Add(FaultSeedWindow), "Transient Failures") {
		t.Error("seed must change at an epoch boundary")
	}

	// Truncation is on UTC, not on the caller's zone: the same instant must
	// seed identically however the clock is expressed.
	west := time.FixedZone("UTC-5", -5*60*60)
	if a, b := faultRandSeed(base, "Chaotic/Hostile"), faultRandSeed(base.In(west), "Chaotic/Hostile"); a != b {
		t.Errorf("seed must be zone independent: %d != %d", a, b)
	}

	// Profiles in one epoch must not share a stream, or a clean early stage
	// would force a clean later stage.
	if faultRandSeed(base, "Transient Failures") == faultRandSeed(base, "Chaotic/Hostile") {
		t.Error("profiles in the same epoch must draw independent streams")
	}
}

// TestFaultRandSeedForce pins the static route: FaultSeedEnv fixes the epoch, so
// a run can be forced off the tumble and replayed exactly. The pin is gated on
// testing.Testing(), which is false in a shipped binary — the guard itself is
// stdlib, so it cannot be exercised from inside a test binary.
func TestFaultRandSeedForce(t *testing.T) {
	base := time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)

	loose := faultRandSeed(base, "Transient Failures")

	t.Setenv(FaultSeedEnv, "42")
	forced := faultRandSeed(base, "Transient Failures")

	if forced == loose {
		t.Fatal("pinned seed must override the tumbled epoch")
	}
	if later := faultRandSeed(base.Add(37*time.Second), "Transient Failures"); later != forced {
		t.Errorf("pinned seed must not drift with the clock: %d != %d", later, forced)
	}
	if other := faultRandSeed(base, "Chaotic/Hostile"); other == forced {
		t.Error("a pinned epoch must still give each profile its own stream")
	}

	// An unparseable pin falls back to the tumble rather than silently pinning
	// every profile to one seed.
	t.Setenv(FaultSeedEnv, "not-a-number")
	if got := faultRandSeed(base, "Transient Failures"); got != loose {
		t.Errorf("invalid pin must fall back to the epoch seed: %d != %d", got, loose)
	}
}

// TestParseSeedPin covers the pin parser directly, since the testing.Testing()
// gate above it returns true for any code running under `go test`.
func TestParseSeedPin(t *testing.T) {
	cases := []struct {
		raw   string
		want  int64
		valid bool
	}{
		{"", 0, false},
		{"not-a-number", 0, false},
		{"42", 42, true},
		{"-7", -7, true},
		{"1792549125", 1792549125, true},
	}
	for _, c := range cases {
		got, ok := parseSeedPin(c.raw)
		if ok != c.valid || got != c.want {
			t.Errorf("parseSeedPin(%q) = (%d, %v), want (%d, %v)", c.raw, got, ok, c.want, c.valid)
		}
	}
}
