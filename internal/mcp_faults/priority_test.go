package mcp_faults

import (
	"fmt"
	"testing"
)

// Routing profiles are not interchangeable. An ETag update is cheap, idempotent and
// retryable; a fire alarm has no acceptable refusal rate. Under a herd the two share
// one queue, and the discipline decides which of them survives saturation.

// tick is one service interval at mu items/sec.
type classLoad struct {
	name      string
	bulkPer   int // bulk arrivals per service tick
	critPer   int // critical arrivals every N ticks (0 = none)
	critEvery int
	ticks     int
	capacity  int
}

type classResult struct {
	critServed  int
	critDropped int
	critMaxWait int // ticks
	bulkServed  int
	bulkDropped int
	evicted     int
}

// runClassLoad floods the queue and reports per-class outcomes.
func runClassLoad(load classLoad, prioritized bool) classResult {
	q := NewClassedFIFO(load.capacity, prioritized)

	type arrival struct {
		at    int
		id    string
		class Class
	}
	queue := make([]arrival, 0, load.capacity*4)

	res := classResult{}
	waits := make([]int, 0, load.ticks)

	for tick := 0; tick < load.ticks; tick++ {
		// Service first, then arrivals, so a critical item is not refused behind a
		// burst that arrives in the same tick.
		if prioritized {
			if item, ok := q.Pop(); ok {
				for len(queue) > 0 && queue[0].id == item.ID {
					waits = append(waits, tick-queue[0].at)
					queue = queue[1:]
				}
				switch item.Class {
				case Critical:
					res.critServed++
				default:
					res.bulkServed++
				}
			}
		} else {
			// Plain FIFO: head of the same slice, no class awareness.
			if item, ok := q.plainPop(); ok {
				for len(queue) > 0 && queue[0].id == item.ID {
					waits = append(waits, tick-queue[0].at)
					queue = queue[1:]
				}
				switch item.Class {
				case Critical:
					res.critServed++
				default:
					res.bulkServed++
				}
			}
		}

		for i := 0; i < load.bulkPer; i++ {
			id := fmt.Sprintf("bulk-%d-%d", tick, i)
			if q.Push(id, Bulk) {
				queue = append(queue, arrival{at: tick, id: id, class: Bulk})
			} else {
				res.bulkDropped++
			}
		}

		if load.critEvery > 0 && tick%load.critEvery == 0 {
			for i := 0; i < load.critPer; i++ {
				id := fmt.Sprintf("crit-%d-%d", tick, i)
				if q.Push(id, Critical) {
					queue = append(queue, arrival{at: tick, id: id, class: Critical})
				} else {
					res.critDropped++
				}
			}
		}
	}

	res.evicted = q.Evicted()
	if len(waits) > 0 {
		maxWait := 0
		for _, w := range waits {
			if w > maxWait {
				maxWait = w
			}
		}
		res.critMaxWait = maxWait
	}
	return res
}

// plainPop serves the arrival order, which is what a class-blind queue does.
func (q *ClassedFIFO) plainPop() (ClassedItem, bool) {
	if len(q.items) == 0 {
		return ClassedItem{}, false
	}
	item := q.items[0]
	q.items = q.items[1:]
	q.served[item.Class]++
	return item, true
}

// TestHerdThatIgnoresTheFireIsNotHealthy is the point in one table: a class-blind
// queue refuses the fire at the same rate as the ETag traffic, so the dashboard's
// bulk SLO stays green while the critical class fails outright.
func TestHerdThatIgnoresTheFireIsNotHealthy(t *testing.T) {
	loads := []classLoad{
		{name: "gentle", bulkPer: 1, critPer: 1, critEvery: 10, ticks: 400, capacity: 16},
		{name: "herd", bulkPer: 4, critPer: 1, critEvery: 10, ticks: 400, capacity: 64},
		{name: "stampede", bulkPer: 9, critPer: 1, critEvery: 10, ticks: 400, capacity: 256},
	}

	t.Logf("%-10s %-9s %-8s %-10s %-9s %-10s", "load", "discipline", "crit ok", "crit drop", "bulk ok", "evicted")
	for _, load := range loads {
		for _, prioritized := range []bool{false, true} {
			res := runClassLoad(load, prioritized)
			discipline := "fifo"
			if prioritized {
				discipline = "priority"
			}
			critTotal := res.critServed + res.critDropped
			bulkTotal := res.bulkServed + res.bulkDropped
			critOK := 0.0
			bulkOK := 0.0
			if critTotal > 0 {
				critOK = 100 * float64(res.critServed) / float64(critTotal)
			}
			if bulkTotal > 0 {
				bulkOK = 100 * float64(res.bulkServed) / float64(bulkTotal)
			}
			t.Logf("%-10s %-9s %-8.1f %-10d %-9.1f %-10d",
				load.name, discipline, critOK, res.critDropped, bulkOK, res.evicted)
		}
	}

	// The specific claim: as the herd grows, a class-blind queue drives the critical
	// refusal rate up with it, and priority holds it at zero.
	for _, load := range loads {
		blind := runClassLoad(load, false)
		prio := runClassLoad(load, true)

		if prio.critDropped != 0 {
			t.Errorf("%s: priority refused %d critical items", load.name, prio.critDropped)
		}
		if load.bulkPer >= 4 && blind.critDropped == 0 {
			t.Errorf("%s: a class-blind queue should refuse critical items under %dx load, refused none",
				load.name, load.bulkPer)
		}
		if prio.critServed <= blind.critServed && load.bulkPer >= 4 {
			t.Errorf("%s: priority served %d critical vs blind %d", load.name, prio.critServed, blind.critServed)
		}
	}

	// And the masking: the aggregate looks acceptable because bulk dominates the
	// count, while the critical class is the one that failed.
	herd := loads[1]
	res := runClassLoad(herd, false)
	aggregate := 100 * float64(res.critServed+res.bulkServed) /
		float64(res.critServed+res.critDropped+res.bulkServed+res.bulkDropped)
	critical := 100 * float64(res.critServed) / float64(res.critServed+res.critDropped)
	t.Logf("class-blind under herd: aggregate success %.1f%%, critical success %.1f%%", aggregate, critical)

	if critical >= aggregate {
		t.Errorf("the critical class should look worse than the aggregate: %.1f vs %.1f", critical, aggregate)
	}
}

// TestPriorityCostsBulkHeadroom is the trade made explicit: protecting the fire
// pushes bulk traffic out, and the bulk refusal rate is what pays for it.
func TestPriorityCostsBulkHeadroom(t *testing.T) {
	load := classLoad{name: "herd", bulkPer: 4, critPer: 1, critEvery: 10, ticks: 400, capacity: 64}

	blind := runClassLoad(load, false)
	prio := runClassLoad(load, true)

	t.Logf("bulk refused: fifo %d, priority %d", blind.bulkDropped, prio.bulkDropped)
	t.Logf("critical refused: fifo %d, priority %d", blind.critDropped, prio.critDropped)
	t.Logf("bulk evicted for critical: %d", prio.evicted)

	// Priority is not free: either it refuses more bulk, or it evicts bulk to make
	// room. Both are the same payment, and neither is allowed to touch critical.
	if prio.bulkDropped == 0 && prio.evicted == 0 {
		t.Error("priority under flood must cost bulk something")
	}
	if prio.critDropped != 0 {
		t.Error("priority must never refuse a critical item while bulk exists")
	}
}
