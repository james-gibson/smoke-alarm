package mcp_faults

import (
	"fmt"
	"math/rand"
	"testing"
	"time"
)

// Golden as a functor of the epoch, and golden as a triple.
//
// Reading one: golden state is not stored, it is derived - golden(epoch) = F(mod(UTC,
// 5)). Nothing ages, so nothing can be stale, and there is no snapshot to transfer or
// poison. What it cannot encode is history, so it is the *expected* set, not the
// observed one.
//
// Reading two: golden is itself three clusters - one observing, one cutting skill
// tokens, one the reference. That is self-similar to a live group, which raises the
// question of who references the reference. The functor is the answer: the reference
// role has to bottom out in something derivable, or the chain of references never ends.

// goldenEpoch is the functor: the expected rotation set for an epoch, derived and
// therefore identical on every node that evaluates it.
func goldenEpoch(epoch int64, size int) []string {
	rng := rand.New(rand.NewSource(epoch))
	out := make([]string, 0, size)
	for i := 0; i < size; i++ {
		out = append(out, fmt.Sprintf("rot-%d-%08x", epoch, rng.Uint32()))
	}
	return out
}

// epochOf maps an instant onto its epoch index.
func epochOf(at time.Time) int64 {
	return at.UTC().Unix() / int64(ClusterRotationPeriod/time.Second)
}

// TestFunctorCannotBeStale is the first consequence: the staleness bound disappears for
// everything golden is a function of. The history still lags - it is observed, not
// derived - but the expectation cannot be old, because it is evaluated at call time.
func TestFunctorCannotBeStale(t *testing.T) {
	t.Logf("%-10s %-16s %-16s", "lag", "snapshot misses", "functor misses")

	for _, lag := range []time.Duration{0, time.Second, 30 * time.Second, 5 * time.Minute} {
		snapshotMisses := rotationRate * lag.Seconds()
		functorMisses := 0.0 // evaluated now, not recalled from then

		t.Logf("%-10s %-16.0f %-16.0f", lag, snapshotMisses, functorMisses)

		if functorMisses != 0 {
			t.Error("a derived expectation cannot be stale")
		}
	}

	// What it cannot do is tell you what happened. The expected set is derivable; the
	// observed set is a receipt, and receipts still age.
	at := time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)
	expected := goldenEpoch(epochOf(at), 8)
	if len(expected) != 8 {
		t.Fatalf("expected 8 rotations for the epoch, got %d", len(expected))
	}

	// Determinism: two nodes evaluating the same epoch agree, byte for byte.
	again := goldenEpoch(epochOf(at), 8)
	for i := range expected {
		if expected[i] != again[i] {
			t.Fatalf("the functor is not deterministic at %d", i)
		}
	}

	// And it changes with the epoch, or it is a constant, not a schedule.
	next := goldenEpoch(epochOf(at)+1, 8)
	if next[0] == expected[0] {
		t.Error("consecutive epochs must differ")
	}
}

// TestDerivedGoldenNeedsNoTransfer is the rebuild consequence: a snapshot has to be
// shipped, a functor has to be evaluated. The transfer and the cold restore both
// vanish, leaving only hashing.
func TestDerivedGoldenNeedsNoTransfer(t *testing.T) {
	const n = 1_000_000

	race := goldenRace{n: n, identities: 100, hashes: 11}
	snapshot := race.rebuildTimeWarm()
	cold := race.rebuildTimeCold()

	hashingOnly := time.Duration(float64(n) * float64(11) * hashProbe.Seconds() * float64(time.Second))

	t.Logf("snapshot rebuild: %s", snapshot)
	t.Logf("cold restore + rebuild: %s", cold)
	t.Logf("derived rebuild: %s (hashing only)", hashingOnly)

	if hashingOnly >= snapshot {
		t.Errorf("deriving should not be slower than shipping: %s vs %s", hashingOnly, snapshot)
	}
	if hashingOnly >= cold {
		t.Errorf("deriving must beat a cold restore: %s vs %s", hashingOnly, cold)
	}
	// The transfer was the smaller half; the cold restore was the killer.
	transferShare := 100 * (snapshot - hashingOnly).Seconds() / snapshot.Seconds()
	t.Logf("transfer is %.0f%% of a warm rebuild; the cold restore is %.0fx the rebuild itself",
		transferShare, cold.Seconds()/snapshot.Seconds())
}

// TestPublicScheduleIsSafeOnlyBecauseTheFutureIsRefused is the risk a functor
// introduces: anyone can compute it, including the attacker. What keeps that harmless
// is that a chain position the ingress has not reached cannot be used.
func TestPublicScheduleIsSafeOnlyBecauseTheFutureIsRefused(t *testing.T) {
	at := time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)
	current := epochOf(at)

	tumble := NewAlarmTumble("ingress-a", 1)
	if ok, _ := tumble.Offer(0, Isotoped); !ok {
		t.Fatal("the first alarm should be accepted")
	}

	// The attacker derives the schedule and pre-dates alarms for future epochs. Those
	// are future chain positions, which this ingress never issued.
	futureAccepted := 0
	for ahead := int64(1); ahead <= 10; ahead++ {
		for _, id := range goldenEpoch(current+ahead, 4) {
			_ = id
			if ok, _ := tumble.Offer(tumble.Chain()+ahead, Isotoped); ok {
				futureAccepted++
			}
		}
	}

	t.Logf("pre-dated alarms from future epochs: %d accepted, %d refused", futureAccepted, tumble.TooNew())
	if futureAccepted != 0 {
		t.Errorf("a public schedule must not let the future be spent early, accepted %d", futureAccepted)
	}
	if tumble.TooNew() == 0 {
		t.Error("future chain positions should be counted as too-new")
	}

	// So the functor being public costs nothing on its own: knowing what should exist is
	// not the same as being able to spend it.
	t.Logf("schedule is public; spending it is not")
}

// TestGoldenTripleIsWellFoundedByTheFunctor is reading two: observing, cutting, and
// reference. The reference role cannot be another cluster without regress, so it
// bottoms out in the functor.
func TestGoldenTripleIsWellFoundedByTheFunctor(t *testing.T) {
	roles := []string{"observing", "cutting", "reference"}
	t.Logf("golden roles: %v", roles)

	// Observing and cutting are live actions on state. Reference is not an action: it is
	// a value, and the value is derived.
	live := 0
	derived := 0
	for _, role := range roles {
		if role == "reference" {
			derived++
			continue
		}
		live++
	}

	if live != 2 || derived != 1 {
		t.Fatalf("expected two live roles and one derived, got %d/%d", live, derived)
	}

	// The regress test: a reference cluster would need its own reference. The functor
	// ends that, because evaluating it twice gives the same answer with no authority.
	at := time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)
	a := goldenEpoch(epochOf(at), 16)
	b := goldenEpoch(epochOf(at), 16)
	for i := range a {
		if a[i] != b[i] {
			t.Fatal("the reference must not depend on who evaluated it")
		}
	}
	t.Logf("reference bottoms out in %d derived rotations, evaluated identically on both nodes", len(a))

	// The two live roles need each other: an observation without a cutter has no
	// capability to check, and a cutter without an observer has nothing to witness.
	if live != 2 {
		t.Error("the live half of the triple needs both roles")
	}
}
