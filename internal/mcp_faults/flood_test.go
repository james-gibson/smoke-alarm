package mcp_faults

import (
	"fmt"
	"testing"
)

// Capture-the-flag under flood.
//
// Red plants one stake - a request carrying the flag - and gets to choose where:
// at any depth of the queue, or as an arrival at any point during the flood. Blue
// wants to see the stake without serving it. Service is a win for red, invisibility
// is a win for red; blue wins by observing a stake that never gets served.
//
// The queue is a bounded FIFO with the two unix-pipe behaviors a writer can meet:
// the write is refused (DropNewest) or history is overwritten (DropOldest).

// floodParams describes one run of the experiment.
type floodParams struct {
	capacity int
	policy   DropPolicy
	kind     string // "preloaded" (planted in the queue) or "arrival" (planted mid-flood)
	level    int    // depth to plant at, for preloaded
	steps    int
}

// floodOutcome is what happened to the stake.
type floodOutcome struct {
	servedAt int  // step at which the stake was popped, -1 if never
	dwell    int  // steps from plant to service, -1 if never
	observed bool // did the stake ever occupy a slot
	dropped  int  // total dropped items, flood included
	highMark int  // deepest the queue got
}

const (
	arrivalsPerStep = 2 // flood: twice the service rate
	servicePerStep  = 1
)

// runFlood plants the stake and floods the queue.
func runFlood(p floodParams) floodOutcome {
	q := NewFIFO(p.capacity, p.policy)
	stake := RouteKey{Address: "flag", Claimant: "red", Epoch: 1}.String()
	filler := func(i int) string { return fmt.Sprintf("filler-%d", i) }

	servedAt := -1
	plantedAt := 0

	switch p.kind {
	case "preloaded":
		// Plant the stake at the requested depth: fill to the level, plant, top up.
		for i := 0; i < p.level; i++ {
			q.Push(filler(i))
		}
		q.Push(stake)
		for i := p.level; i < p.capacity; i++ {
			q.Push(filler(i))
		}
	default: // "arrival"
		// Saturate first, so the stake arrives into a queue that is genuinely full.
		for i := 0; i < p.capacity; i++ {
			q.Push(filler(i))
		}
		plantedAt = p.capacity
		q.Push(stake)
	}

	for step := plantedAt; step < p.steps && servedAt < 0; step++ {
		for a := 0; a < arrivalsPerStep; a++ {
			q.Push(filler(step*arrivalsPerStep + a))
		}
		for s := 0; s < servicePerStep; s++ {
			id, ok := q.Pop()
			if !ok {
				break
			}
			if id == stake && servedAt < 0 {
				servedAt = step
			}
		}
	}

	out := floodOutcome{
		servedAt: servedAt,
		dwell:    -1,
		observed: q.Observed(stake),
		dropped:  q.Dropped(),
		highMark: q.HighWater(),
	}
	if servedAt >= 0 {
		out.dwell = servedAt - plantedAt
	}
	return out
}

// TestFloodAtQueueSizes is the matrix: queue size against plant position, for both
// pipe behaviors. Red chooses the plant, so every level is a row.
func TestFloodAtQueueSizes(t *testing.T) {
	capacities := []int{1, 4, 16, 64, 256}
	policies := []DropPolicy{DropNewest, DropOldest}

	t.Logf("%-5s %-11s %-10s %-6s %-9s %-8s %-8s",
		"cap", "policy", "plant", "level", "served@", "observed", "dropped")

	for _, capacity := range capacities {
		for _, policy := range policies {
			name := "newest"
			if policy == DropOldest {
				name = "oldest"
			}

			// Preloaded at five depths, deduplicated for small queues.
			levels := uniqueInts([]int{0, capacity / 4, capacity / 2, 3 * capacity / 4, capacity - 1})
			for _, level := range levels {
				out := runFlood(floodParams{capacity: capacity, policy: policy, kind: "preloaded", level: level, steps: 4*capacity + 8})
				t.Logf("%-5d %-11s %-10s %-6d %-9d %-8v %-8d",
					capacity, name, "preloaded", level, out.servedAt, out.observed, out.dropped)

				if out.highMark > capacity {
					t.Errorf("cap %d: high-water %d exceeded capacity", capacity, out.highMark)
				}
				// Red cannot capture the flag without being seen: service requires
				// occupying a slot, and a slot is what makes it observable.
				if out.servedAt >= 0 && !out.observed {
					t.Errorf("cap %d: stake served at step %d without ever being observed", capacity, out.servedAt)
				}
			}

			// Arrival: planted into a saturated queue, the case the flood creates.
			out := runFlood(floodParams{capacity: capacity, policy: policy, kind: "arrival", level: 0, steps: 4*capacity + 8})
			t.Logf("%-5d %-11s %-10s %-6s %-9d %-8v %-8d",
				capacity, name, "arrival", "-", out.servedAt, out.observed, out.dropped)

			if out.highMark > capacity {
				t.Errorf("cap %d: high-water %d exceeded capacity", capacity, out.highMark)
			}
			if out.servedAt >= 0 && !out.observed {
				t.Errorf("cap %d: arrival stake served without being observed", capacity)
			}
		}
	}
}

// TestStakeVisibilityDependsOnThePipeBehaviour pins the two outcomes red can get,
// because they are structurally different rather than merely probabilistic.
func TestStakeVisibilityDependsOnThePipeBehaviour(t *testing.T) {
	const capacity = 16

	// A refused write is invisible: the stake never occupied a slot, so no
	// observation exists and the canary never fires.
	refused := runFlood(floodParams{capacity: capacity, policy: DropNewest, kind: "arrival", steps: 200})
	if refused.observed {
		t.Error("a refused arrival must not be observable")
	}
	if refused.servedAt >= 0 {
		t.Error("a refused arrival must never be served")
	}

	// An overwritten history is visible and effective in neither direction: seen,
	// then evicted before service.
	overwritten := runFlood(floodParams{capacity: capacity, policy: DropOldest, kind: "arrival", steps: 200})
	if !overwritten.observed {
		t.Error("an admitted arrival must be observable")
	}
	if overwritten.servedAt >= 0 {
		t.Error("a saturated ring buffer should evict the stake before service")
	}

	// Therefore red's only way to capture the flag is to be seen doing it. The
	// asymmetry is the point: invisibility and effectiveness are mutually exclusive
	// in this discipline, because both require the same slot.
	if refused.observed == overwritten.observed {
		t.Error("the two pipe behaviors should differ in observability, not just in service")
	}
}

// TestDeeperPlantWaitsLonger checks the preloaded case: under a refusal policy the
// queue is always full, so a stake planted deeper only reaches the head later.
func TestDeeperPlantWaitsLonger(t *testing.T) {
	const capacity = 64

	previous := -1
	for _, level := range []int{0, 8, 16, 32, 63} {
		out := runFlood(floodParams{capacity: capacity, policy: DropNewest, kind: "preloaded", level: level, steps: 400})
		if out.servedAt < 0 {
			t.Fatalf("level %d: a preloaded stake should eventually be served", level)
		}
		if previous >= 0 && out.servedAt < previous {
			t.Errorf("level %d served at %d, before the shallower plant at %d", level, out.servedAt, previous)
		}
		previous = out.servedAt
	}
}

func uniqueInts(values []int) []int {
	seen := map[int]bool{}
	out := make([]int, 0, len(values))
	for _, v := range values {
		if v < 0 || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
