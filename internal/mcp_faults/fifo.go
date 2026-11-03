package mcp_faults

// DropPolicy is what a full FIFO does with the next item, mirroring a unix pipe
// whose reader cannot keep up: either the write fails (newest is refused) or the
// oldest entry is discarded to make room.
type DropPolicy int

const (
	// DropNewest refuses the arriving item, like a blocking pipe's write failing.
	DropNewest DropPolicy = iota
	// DropOldest evicts the head, like a ring buffer overwriting history.
	DropOldest
)

// FIFO is a bounded first-in-first-out queue with a fixed capacity and one drop
// policy. It records the high-water mark and which identities ever occupied a slot,
// because "was it seen at all" is a different question from "was it served".
type FIFO struct {
	capacity int
	policy   DropPolicy
	items    []string
	dropped  int
	pushed   int
	served   int
	highMark int
	observed map[string]bool
}

// NewFIFO returns a queue with the given capacity and policy.
func NewFIFO(capacity int, policy DropPolicy) *FIFO {
	if capacity < 1 {
		capacity = 1
	}
	return &FIFO{
		capacity: capacity,
		policy:   policy,
		observed: map[string]bool{},
	}
}

// Push attempts to enqueue id, reporting whether it entered the queue. An accepted
// item is recorded as observed: occupying a slot is the only way to be seen.
func (f *FIFO) Push(id string) bool {
	f.pushed++

	if len(f.items) < f.capacity {
		f.items = append(f.items, id)
		f.observed[id] = true
		f.noteHighMark()
		return true
	}

	f.dropped++
	switch f.policy {
	case DropOldest:
		f.items = f.items[1:]
		f.items = append(f.items, id)
		f.observed[id] = true
		f.noteHighMark()
		return true
	default:
		return false
	}
}

// Pop removes the head item.
func (f *FIFO) Pop() (string, bool) {
	if len(f.items) == 0 {
		return "", false
	}
	id := f.items[0]
	f.items = f.items[1:]
	f.served++
	return id, true
}

// Len is the current depth.
func (f *FIFO) Len() int { return len(f.items) }

// Capacity is the fixed bound.
func (f *FIFO) Capacity() int { return f.capacity }

// HighWater is the deepest the queue ever got; it can never exceed Capacity.
func (f *FIFO) HighWater() int { return f.highMark }

// Dropped is how many pushes were refused or evicted.
func (f *FIFO) Dropped() int { return f.dropped }

// Observed reports whether id ever occupied a slot.
func (f *FIFO) Observed(id string) bool { return f.observed[id] }

// Contains reports whether id is queued right now.
func (f *FIFO) Contains(id string) bool {
	for _, item := range f.items {
		if item == id {
			return true
		}
	}
	return false
}

// Position is id's zero-based index in the queue, or -1 when absent.
func (f *FIFO) Position(id string) int {
	for i, item := range f.items {
		if item == id {
			return i
		}
	}
	return -1
}

func (f *FIFO) noteHighMark() {
	if len(f.items) > f.highMark {
		f.highMark = len(f.items)
	}
}
