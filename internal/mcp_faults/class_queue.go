package mcp_faults

// Class is a route's criticality profile. It belongs to the declared route - part
// of what the address promises - not to the queue, because one queue serves every
// class and only the policy decides who goes first.
type Class int

const (
	// Critical is life safety: a missed deadline is a failure, not a delay. It has
	// no acceptable refusal rate.
	Critical Class = iota
	// Interactive is a human waiting on the other end.
	Interactive
	// Bulk is cheap, idempotent, retryable work such as an ETag update.
	Bulk
)

// String names the class for logs.
func (c Class) String() string {
	switch c {
	case Critical:
		return "critical"
	case Interactive:
		return "interactive"
	default:
		return "bulk"
	}
}

// ClassedFIFO is a bounded queue that knows the class of what it holds.
//
// Under a flood the discipline is the whole story: a plain FIFO refuses a critical
// item exactly as readily as a bulk one, because both arrive at the same tail. This
// queue instead protects Critical by evicting the least important Bulk entry, and
// serves Critical first. The cost is Bulk starvation, which is a deliberate trade.
type ClassedFIFO struct {
	capacity int
	protect  bool // true: critical arrivals displace bulk; false: plain FIFO refusal
	items    []ClassedItem
	dropped  map[Class]int
	served   map[Class]int
	evicted  int
}

// ClassedItem is one queued entry.
type ClassedItem struct {
	ID    string
	Class Class
}

// NewClassedFIFO returns an empty queue with a fixed capacity. protectCritical
// selects the discipline: true protects critical arrivals by displacing bulk, false
// refuses whoever arrives at a full queue.
func NewClassedFIFO(capacity int, protectCritical bool) *ClassedFIFO {
	if capacity < 1 {
		capacity = 1
	}
	return &ClassedFIFO{
		capacity: capacity,
		protect:  protectCritical,
		dropped:  map[Class]int{},
		served:   map[Class]int{},
	}
}

// Push admits an item. When full, a Critical arrival displaces the lowest-priority
// Bulk entry if there is one; anything else is refused.
func (q *ClassedFIFO) Push(id string, class Class) bool {
	if len(q.items) < q.capacity {
		q.items = append(q.items, ClassedItem{ID: id, Class: class})
		return true
	}

	if class == Critical && q.protect {
		for i := len(q.items) - 1; i >= 0; i-- {
			if q.items[i].Class == Bulk {
				q.items = append(q.items[:i], q.items[i+1:]...)
				q.evicted++
				q.items = append(q.items, ClassedItem{ID: id, Class: class})
				return true
			}
		}
	}

	q.dropped[class]++
	return false
}

// Pop serves the most critical item available.
func (q *ClassedFIFO) Pop() (ClassedItem, bool) {
	if len(q.items) == 0 {
		return ClassedItem{}, false
	}

	best := 0
	for i, item := range q.items {
		if item.Class < q.items[best].Class {
			best = i
		}
	}

	item := q.items[best]
	q.items = append(q.items[:best], q.items[best+1:]...)
	q.served[item.Class]++
	return item, true
}

// Len is the current depth.
func (q *ClassedFIFO) Len() int { return len(q.items) }

// Dropped is how many items of a class were refused.
func (q *ClassedFIFO) Dropped(class Class) int { return q.dropped[class] }

// Served is how many items of a class were served.
func (q *ClassedFIFO) Served(class Class) int { return q.served[class] }

// Evicted is how many Bulk entries were displaced for Critical ones.
func (q *ClassedFIFO) Evicted() int { return q.evicted }
