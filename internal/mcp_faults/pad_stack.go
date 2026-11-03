package mcp_faults

import "time"

// Pad is one layer's contribution, emitted as an event onto a queue. The layers
// stack: a request crossing ingress, then backup, then a context layer carries a
// stack of pads in that order, and the order is the information.
type Pad struct {
	Layer string // the layer that emitted it (ingress, backup, context, ...)
	Queue string // the queue the event was emitted onto
	Seq   int64  // position in the stack, assigned on push
	ID    string // the pad's own identity
}

// PadStack is a request's accumulated capability. It is deliberately not a set: two
// identical pads pushed twice are two entries, and A-then-B is not B-then-A. That
// is what "they combine like a stack" costs and what a join could not express.
type PadStack struct {
	pads []Pad
}

// Push adds a pad on top of the stack and returns it with its assigned position.
func (s *PadStack) Push(pad Pad) Pad {
	pad.Seq = int64(len(s.pads))
	s.pads = append(s.pads, pad)
	return pad
}

// Top returns the most recently pushed pad.
func (s *PadStack) Top() (Pad, bool) {
	if len(s.pads) == 0 {
		return Pad{}, false
	}
	return s.pads[len(s.pads)-1], true
}

// Len is how many pads the request currently carries.
func (s *PadStack) Len() int { return len(s.pads) }

// Pads returns the stack in push order.
func (s *PadStack) Pads() []Pad {
	out := make([]Pad, len(s.pads))
	copy(out, s.pads)
	return out
}

// Truncate keeps the bottom n pads. This is the flood case: pressure does not
// subtract pads from the middle, it stops the stack short, so every result is still
// a well-formed prefix of what a quiet run would have produced.
func (s *PadStack) Truncate(n int) PadStack {
	if n < 0 {
		n = 0
	}
	if n > len(s.pads) {
		n = len(s.pads)
	}
	kept := make([]Pad, n)
	copy(kept, s.pads[:n])
	return PadStack{pads: kept}
}

// Equal reports whether two stacks are the same sequence of pads.
func (s PadStack) Equal(other PadStack) bool {
	if len(s.pads) != len(other.pads) {
		return false
	}
	for i := range s.pads {
		if s.pads[i] != other.pads[i] {
			return false
		}
	}
	return true
}

// Pending is a request that has been accepted but not yet trusted. Acceptance is
// cheap and does not consult the slower method; trust is decided later, against it.
type Pending struct {
	Key        RouteKey // (address, claimant, epoch) - the triple, never the address alone
	Call       string
	Stack      PadStack
	N          int  // the argument the request asked about
	Answer     bool // what the fast path answered
	AcceptedAt time.Time
}

// PendingQueue is the committer's durable queue. Items are accepted on arrival and
// drained only when a slower method can decide whether to trust them.
type PendingQueue struct {
	items []Pending
}

// Enqueue accepts a request. It never refuses: acceptance is admitting the request
// into the flow, which is not the same decision as trusting it.
func (q *PendingQueue) Enqueue(p Pending) {
	q.items = append(q.items, p)
}

// Len is the depth of the durable queue.
func (q *PendingQueue) Len() int { return len(q.items) }

// Resolver is the slower method that decides trust for one queued item.
type Resolver func(p Pending) bool

// Verify drains the queue against the resolver, reporting how many items were
// trusted. Trust is where a verdict is earned; acceptance earned nothing.
func (q *PendingQueue) Verify(resolve Resolver) (trusted, untrusted int) {
	for _, item := range q.items {
		if resolve(item) {
			trusted++
		} else {
			untrusted++
		}
	}
	q.items = nil
	return trusted, untrusted
}

// Unresolved is how many accepted items are still waiting for a trust decision.
// They are unexamined, not pending innocence.
func (q *PendingQueue) Unresolved() int { return len(q.items) }
