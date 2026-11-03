package mcp_faults

import (
	"math/rand"
	"testing"
	"time"
)

func pad(layer, queue, id string) Pad { return Pad{Layer: layer, Queue: queue, ID: id} }

// TestStackOrderAndRepetitionMatter pins what a stack costs: order is information,
// and two identical pads are two entries. A commutative join could express neither.
func TestStackOrderAndRepetitionMatter(t *testing.T) {
	var ab, ba, aTwice PadStack
	ab.Push(pad("ingress", "q1", "read"))
	ab.Push(pad("context", "q2", "context"))

	ba.Push(pad("context", "q2", "context"))
	ba.Push(pad("ingress", "q1", "read"))

	aTwice.Push(pad("ingress", "q1", "read"))
	aTwice.Push(pad("ingress", "q1", "read"))

	if ab.Equal(ba) {
		t.Error("order must change the stack: ingress-then-context != context-then-ingress")
	}
	if ab.Len() == aTwice.Len() && ab.Equal(aTwice) {
		t.Error("repetition must add depth: read twice is not read once")
	}
	if aTwice.Len() != 2 {
		t.Errorf("two pushes should give depth 2, got %d", aTwice.Len())
	}

	// Positions are assigned in push order, which is the provenance a join loses.
	for i, p := range ab.Pads() {
		if p.Seq != int64(i) {
			t.Errorf("pad %d has Seq %d", i, p.Seq)
		}
	}
	if top, ok := ab.Top(); !ok || top.Layer != "context" {
		t.Errorf("top of the stack should be the last layer, got %+v", top)
	}
}

// TestFloodTruncatesToAPrefix is the "flood of traffic? less pads" case: pressure
// stops the stack short rather than removing pads from the middle, so the result is
// still a well-formed prefix and never malformed.
func TestFloodTruncatesToAPrefix(t *testing.T) {
	var quiet PadStack
	for _, p := range []Pad{
		pad("ingress", "q1", "read"),
		pad("ingress", "q1", "invoke"),
		pad("context", "q2", "context"),
		pad("context", "q2", "plan"),
		pad("context", "q2", "budget"),
	} {
		quiet.Push(p)
	}

	flooded := quiet.Truncate(2)

	if flooded.Len() != 2 {
		t.Fatalf("truncation should keep 2 pads, got %d", flooded.Len())
	}
	// Every kept pad is the same pad at the same position as the quiet run.
	for i, p := range flooded.Pads() {
		if p != quiet.Pads()[i] {
			t.Errorf("prefix differs at %d: %+v vs %+v", i, p, quiet.Pads()[i])
		}
	}
	// Weaker, not invalid: the flooded stack is a stack.
	if _, ok := flooded.Top(); !ok {
		t.Error("a truncated stack is still a stack")
	}
	// Truncation beyond the depth is a no-op rather than an error.
	if over := quiet.Truncate(99); !over.Equal(quiet) {
		t.Error("truncating past the depth must not change the stack")
	}
	if empty := quiet.Truncate(0); empty.Len() != 0 {
		t.Error("truncating to 0 leaves an empty but valid stack")
	}
}

// TestAcceptIsNotTrust is the separation the model requires: accepting a request is
// admitting it into the flow, and decides nothing about whether to trust it. Trust
// is earned later, against a slower method, and that is when a verdict exists.
func TestAcceptIsNotTrust(t *testing.T) {
	clock := &testClock{t: time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)}

	var queue PendingQueue
	for n := 1; n <= 300; n++ {
		var stack PadStack
		stack.Push(pad("ingress", "q1", "read"))
		if n%3 == 0 {
			stack = stack.Truncate(0) // a flooded subset arrives with no pads at all
		}
		queue.Enqueue(Pending{
			Call:       "isOdd",
			Stack:      stack,
			N:          7,
			Answer:     isOdd(7),
			AcceptedAt: clock.Now(),
		})
	}

	// Acceptance is unconditional: no refusals, no verdicts, no distance.
	if queue.Len() != 300 {
		t.Fatalf("every request should be accepted, queue depth %d", queue.Len())
	}

	// Trust is decided later, by the slower method, and only then is anything scored.
	slow := func(p Pending) bool { return p.Answer == isOdd(p.N) } // the independent check
	trusted, untrusted := queue.Verify(slow)

	if trusted != 300 || untrusted != 0 {
		t.Errorf("a correct slow method should trust all 300: %d/%d", trusted, untrusted)
	}
	if queue.Len() != 0 {
		t.Error("verification drains the durable queue")
	}

	// And a compromised slow method is only visible at trust time, not at acceptance.
	biased := &biasedResolver{p: 0.10, rng: rand.New(rand.NewSource(7))}
	var second PendingQueue
	for n := 0; n < 1000; n++ {
		second.Enqueue(Pending{
			Call:       "isOdd",
			Stack:      PadStack{},
			N:          7,
			Answer:     biased.IsOdd(7),
			AcceptedAt: clock.Now(),
		})
	}
	if second.Len() != 1000 {
		t.Fatal("acceptance must not depend on the verifier's honesty")
	}
	trusted, untrusted = second.Verify(func(p Pending) bool { return p.Answer == isOdd(p.N) })
	t.Logf("biased slow method: %d trusted, %d untrusted out of 1000", trusted, untrusted)

	if untrusted == 0 {
		t.Error("a biased slow method should eventually refuse something")
	}
	// Acceptance stays 100% regardless, which is the whole point of queueing.
	if trusted+untrusted != 1000 {
		t.Errorf("every accepted item must be resolved: %d", trusted+untrusted)
	}
}
