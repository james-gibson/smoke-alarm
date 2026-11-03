package mcp_faults

import (
	"math"
	"testing"
	"time"
)

// Role shift at saturation: C demotes to B's role, and C's backups step in.
//
// Two capacities are not interchangeable:
//
//	serving  - mu per node, answers requests
//	stamping - sigma = 1/verifyLatency per node, decides trust
//
// A node moved from stamping to serving buys the wrong capacity. The tables below
// price that, because the demotion decision is usually taken for the herd SLA and
// paid for by the correct one.

// roles is a fleet composition.
type roles struct {
	serving  int
	stamping int
}

// herdSLA is time to last when serving capacity changes mid-run.
func (m slaModel) herdSLA(n int, before, after roles) time.Duration {
	preJoin := float64(before.serving) * m.mu
	elapsed := m.failoverDelay.Seconds()
	servedBefore := math.Min(float64(n), preJoin*elapsed)
	postJoin := float64(after.serving) * m.mu

	return time.Duration((elapsed + (float64(n)-servedBefore)/postJoin) * float64(time.Second))
}

// correctSLA is time to last when every item must clear the slow method. Only the
// stamping count moves this: serving capacity cannot clear a backlog of untrusted
// items.
func (m slaModel) correctSLA(n int, after roles) time.Duration {
	sigma := 1 / m.verifyLatency.Seconds()
	return time.Duration((m.failoverDelay.Seconds() + float64(n)/(sigma*float64(after.stamping))) * float64(time.Second))
}

// backlogAt is the untrusted queue depth after t seconds at load lambda. A single
// stamping node is the bottleneck, so the backlog grows at lambda - sigma.
func (m slaModel) backlogAt(lambda float64, after roles, t time.Duration) float64 {
	sigma := 1 / m.verifyLatency.Seconds()
	net := lambda - sigma*float64(after.stamping)
	if net <= 0 {
		return 0
	}
	return net * t.Seconds()
}

// TestDemotionBuysTheWrongCapacity prices the role shift.
func TestDemotionBuysTheWrongCapacity(t *testing.T) {
	model := slaModel{mu: 100, verifyLatency: 50 * time.Millisecond, failoverDelay: time.Second}
	const herd = 1000

	// A is ingress, B is its backup. C stamps.
	ingressOnly := roles{serving: 1, stamping: 1}
	withBackup := roles{serving: 2, stamping: 1}
	demoted := roles{serving: 3, stamping: 1}     // C moves to B's role, one backup stamps
	demotedPlus := roles{serving: 3, stamping: 2} // C's backups are plural and stamp
	stamperAdded := roles{serving: 2, stamping: 2}

	scenarios := []struct {
		name   string
		before roles
		after  roles
	}{
		{"A alone", ingressOnly, ingressOnly},
		{"A+B backup", withBackup, withBackup},
		{"C demotes (1 stamper left)", withBackup, demoted},
		{"C demotes + 2 stampers", withBackup, demotedPlus},
		{"no demotion, +1 stamper", withBackup, stamperAdded},
	}

	t.Logf("%-30s %-9s %-9s %-9s", "scenario", "herd", "correct", "stampers")
	for _, s := range scenarios {
		herdSLA := model.herdSLA(herd, s.before, s.after)
		correctSLA := model.correctSLA(herd, s.after)
		t.Logf("%-30s %-9s %-9s %-9d", s.name, herdSLA, correctSLA, s.after.stamping)

		if s.after.stamping == 1 && correctSLA < 50*time.Second {
			t.Errorf("%s: one stamper cannot clear 1000 items faster than 50s, got %s", s.name, correctSLA)
		}
		if s.after.stamping == 2 && correctSLA > 30*time.Second {
			t.Errorf("%s: two stampers should halve the correct SLA, got %s", s.name, correctSLA)
		}
	}

	// The structural claims: each SLA follows exactly one count.
	if model.correctSLA(herd, demoted) != model.correctSLA(herd, withBackup) {
		t.Error("adding serving nodes must not change the correct SLA")
	}
	if model.herdSLA(herd, withBackup, demotedPlus) != model.herdSLA(herd, withBackup, demoted) {
		t.Error("adding stampers must not change the herd SLA")
	}

	// And the comparison that decides the policy: a stamping node is worth more
	// than a demotion, in the metric that governs correctness.
	demotionGain := model.herdSLA(herd, withBackup, withBackup) - model.herdSLA(herd, withBackup, demoted)
	stamperGain := model.correctSLA(herd, withBackup) - model.correctSLA(herd, stamperAdded)
	t.Logf("demotion buys %s of herd SLA; one stamper buys %s of correct SLA", demotionGain, stamperGain)

	if stamperGain <= demotionGain {
		t.Errorf("a stamper should buy more than a demotion: %s vs %s", stamperGain, demotionGain)
	}
}

// TestDemotionOrphansTheBacklog is the risk that is not on the SLA chart: when C
// steps down, everything it accepted but had not resolved becomes unexamined unless
// its backup adopts the queue.
func TestDemotionOrphansTheBacklog(t *testing.T) {
	model := slaModel{mu: 100, verifyLatency: 50 * time.Millisecond, failoverDelay: time.Second}

	t.Logf("%-10s %-9s %-12s", "lambda", "stampers", "backlog/s")
	for _, lambda := range []float64{150, 400, 1000} {
		pending := model.backlogAt(lambda, roles{serving: 3, stamping: 1}, time.Second)
		t.Logf("%-10.0f %-9d %-12.0f", lambda, 1, pending)
	}

	// At 400/s against a 20/s stamper the queue grows by 380 items per second, so a
	// one-second handover leaves 380 accepted-but-unresolved items behind.
	pending := model.backlogAt(400, roles{serving: 3, stamping: 1}, time.Second)
	if pending != 380 {
		t.Errorf("expected a 380-item backlog after one second at 400/s, got %v", pending)
	}

	// Those items are unexamined, which is a third state and not a clean bill.
	for i := 0; i < int(pending); i++ {
		if got := Classify(false, false); got != VerdictUnexamined {
			t.Fatalf("orphaned item should be unexamined, got %q", got)
		}
	}

	// With capacity at or above load there is no backlog to orphan.
	if got := model.backlogAt(150, roles{serving: 3, stamping: 8}, time.Second); got != 0 {
		t.Errorf("8 stampers at 150/s should have no backlog, got %v", got)
	}
}
