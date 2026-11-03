package mcp_faults

import (
	"math"
	"testing"
	"time"
)

// SLA arithmetic for the thundering herd, with the failover node moving in at
// ingress saturation.
//
// Two paths are compared:
//
//   serving   - admit and answer, capacity mu per node, no slow method
//   correct   - admit, queue, and answer only once the slow method has decided
//               trust, so capacity is 1/verifyLatency per node
//
// The result that matters is that the fleet multiplier for correctness is mu*v,
// independent of the herd size and of how fast the failover node arrives.

type slaModel struct {
	mu            float64       // requests/sec served by one node
	verifyLatency time.Duration // the slow method's latency per item
	failoverDelay time.Duration // detection + promotion for the backup node
}

// nodesForFast is how many serving nodes a load needs to stay stable.
func (m slaModel) nodesForFast(lambda float64) int {
	return int(math.Ceil(lambda / m.mu))
}

// nodesForCorrect is how many nodes a load needs when every item must clear the
// slow method before it counts.
func (m slaModel) nodesForCorrect(lambda float64) int {
	perNode := 1 / m.verifyLatency.Seconds()
	return int(math.Ceil(lambda / perNode))
}

// herdTime is time to last of n simultaneous retries. Without a join, serversAfter
// nodes serve from the start. With a join, the backup only arrives after the
// failover delay, so the ingress alone carries everything until then.
func (m slaModel) herdTime(n, serversAfter int, joins bool) time.Duration {
	seconds := func(v float64) time.Duration { return time.Duration(v * float64(time.Second)) }

	if !joins {
		return seconds(float64(n) / (float64(serversAfter) * m.mu))
	}

	preJoin := float64(serversAfter-1) * m.mu
	elapsed := m.failoverDelay.Seconds()
	servedBefore := math.Min(float64(n), preJoin*elapsed)
	postJoin := float64(serversAfter) * m.mu

	return seconds(elapsed + (float64(n)-servedBefore)/postJoin)
}

// correctTime is time to last when nothing may be answered before the slow method
// clears it. The slow stage, not the serving stage, sets the capacity.
func (m slaModel) correctTime(n, servers int) time.Duration {
	perNode := 1 / m.verifyLatency.Seconds()
	return time.Duration((float64(n)/(perNode*float64(servers)) + m.failoverDelay.Seconds()) * float64(time.Second))
}

// switchoverRefusals is traffic the ingress must hold while the backup is promoted.
// If the buffer is smaller than the herd arriving in that window, the difference is
// refused.
func (m slaModel) switchoverRefusals(lambda float64, buffer int) int {
	arriving := lambda * m.failoverDelay.Seconds()
	if arriving <= float64(buffer) {
		return 0
	}
	return int(arriving - float64(buffer))
}

// stable reports whether the queue drains at all.
func stable(lambda, capacity float64) bool { return capacity > lambda }

// TestCorrectnessFleetMultiplierIsMuV is the headline: answering correctly costs
// mu*v times the fleet, whatever the load and however fast the failover is.
func TestCorrectnessFleetMultiplierIsMuV(t *testing.T) {
	model := slaModel{mu: 100, verifyLatency: 50 * time.Millisecond, failoverDelay: time.Second}

	multiplier := model.mu * model.verifyLatency.Seconds()
	if multiplier != 5 {
		t.Fatalf("fixture: expected a 5x multiplier, got %v", multiplier)
	}

	for _, lambda := range []float64{150, 400, 1000} {
		fast := model.nodesForFast(lambda)
		correct := model.nodesForCorrect(lambda)
		ratio := float64(correct) / float64(fast)
		t.Logf("lambda=%4.0f/s: serving needs %2d nodes, correct needs %3d, ratio %.1f",
			lambda, fast, correct, ratio)

		if math.Abs(ratio-multiplier) > 1.0 {
			t.Errorf("lambda=%0.f: ratio %.1f should follow mu*v = %v", lambda, ratio, multiplier)
		}
	}

	// The multiplier is a property of the pair (mu, v), not of the herd: at loads
	// large enough that ceiling effects vanish, both counts scale with lambda.
	if got := model.nodesForCorrect(1000); got != 50 {
		t.Errorf("1000/s should need 50 verifying nodes at v=50ms, got %d", got)
	}
	if got := model.nodesForFast(1000); got != 10 {
		t.Errorf("1000/s should need 10 serving nodes at mu=100/s, got %d", got)
	}
}

// TestCorrectnessIsFreeBelowServiceTime is the crossover: when the slow method is
// faster than serving an item, queueing it costs nothing in capacity.
func TestCorrectnessIsFreeBelowServiceTime(t *testing.T) {
	const mu = 100 // 10ms per item

	t.Logf("%-10s %-8s %-10s", "verify(v)", "mu*v", "cost")
	for _, v := range []time.Duration{time.Millisecond, 5 * time.Millisecond, 10 * time.Millisecond, 50 * time.Millisecond, 250 * time.Millisecond} {
		model := slaModel{mu: mu, verifyLatency: v}
		multiplier := model.mu * v.Seconds()

		verdict := "free or cheaper"
		if multiplier > 1 {
			verdict = "fleet multiplier"
		}
		t.Logf("%-10s %-8.1f %-10s", v, multiplier, verdict)

		if v <= time.Duration(1e9/mu)*time.Nanosecond && multiplier > 1 {
			t.Errorf("v=%s below the service time should not cost capacity (mu*v=%v)", v, multiplier)
		}
	}
}

// TestHerdSLAWithFailoverAtSaturation gives the serving numbers, with and without
// the backup moving in.
func TestHerdSLAWithFailoverAtSaturation(t *testing.T) {
	model := slaModel{mu: 100, verifyLatency: 50 * time.Millisecond, failoverDelay: time.Second}
	const herd = 1000

	withoutFailover := model.herdTime(herd, 1, false)
	withFailover := model.herdTime(herd, 2, true) // one ingress, one backup
	fourNodes := model.herdTime(herd, 4, false)
	correct := model.correctTime(herd, 2)

	t.Logf("herd of %d, mu=%0.0f/s, failover after %s:", herd, model.mu, model.failoverDelay)
	t.Logf("  one node, no failover:  %s", withoutFailover)
	t.Logf("  one node + backup:      %s", withFailover)
	t.Logf("  four nodes from start:  %s", fourNodes)
	t.Logf("  correct (v=50ms, 2 nodes): %s", correct)
	t.Logf("  correctness penalty:    %.1fx", correct.Seconds()/withFailover.Seconds())

	if withFailover >= withoutFailover {
		t.Errorf("failover must help: %s vs %s", withFailover, withoutFailover)
	}
	// The backup halves the remaining time; the switchover is pure overhead.
	ideal := time.Duration((float64(herd) / (2 * model.mu)) * float64(time.Second))
	if withFailover < ideal {
		t.Errorf("failover cannot beat the two-node service time: %s < %s", withFailover, ideal)
	}
	if correct <= withFailover {
		t.Errorf("correctness should cost time at v=50ms: %s vs %s", correct, withFailover)
	}
}

// TestSwitchoverBufferIsTheSwitchoverLoad quantifies what the ingress must hold
// while the backup is promoted.
func TestSwitchoverBufferIsTheSwitchoverLoad(t *testing.T) {
	model := slaModel{mu: 100, verifyLatency: 50 * time.Millisecond, failoverDelay: time.Second}

	t.Logf("%-10s %-12s %-14s", "lambda", "arrival in 1s", "refusals @64")
	for _, lambda := range []float64{150, 400, 1000} {
		refusals := model.switchoverRefusals(lambda, 64)
		t.Logf("%-10.0f %-12.0f %-14d", lambda, lambda*model.failoverDelay.Seconds(), refusals)
		if refusals < 0 {
			t.Errorf("refusals cannot be negative at lambda=%0.f", lambda)
		}
	}

	// A buffer at or above the switchover load refuses nothing.
	if model.switchoverRefusals(150, 150) != 0 {
		t.Error("a buffer sized to the switchover load should refuse nothing")
	}
	if model.switchoverRefusals(400, 64) == 0 {
		t.Error("a 64-slot buffer cannot hold 400 arrivals in a second")
	}
}

// TestStabilityNeedsHeadroom is the precondition for any of the above: with the
// herd at or above capacity, delay is unbounded and the SLA is not the question.
func TestStabilityNeedsHeadroom(t *testing.T) {
	model := slaModel{mu: 100, verifyLatency: 50 * time.Millisecond}

	for _, c := range []struct {
		servers int
		lambda  float64
	}{{1, 100}, {1, 150}, {2, 150}, {2, 200}, {4, 400}} {
		capacity := float64(c.servers) * model.mu
		if stable(c.lambda, capacity) {
			delay := 1 / (capacity - c.lambda)
			t.Logf("%d nodes vs %0.0f/s: stable, queueing delay %0.3fs", c.servers, c.lambda, delay)
			continue
		}
		t.Logf("%d nodes vs %0.0f/s: unstable, queue grows without bound", c.servers, c.lambda)
	}

	if stable(150, 100) {
		t.Error("one node cannot be stable at 150/s")
	}
	if !stable(150, 200) {
		t.Error("two nodes should be stable at 150/s")
	}
}
