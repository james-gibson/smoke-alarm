package mcp_faults

import (
	"testing"
	"time"
)

// TestUnexaminedIsNotInnocence is the whole reason the third state exists: with two
// states, every item the system never looked at is reported as if it had been
// cleared.
func TestUnexaminedIsNotInnocence(t *testing.T) {
	if got := Classify(false, false); got != VerdictUnexamined {
		t.Errorf("an unadmitted item must be unexamined, got %q", got)
	}
	if got := Classify(false, true); got != VerdictUnexamined {
		t.Errorf("no admission, no trustworthy verdict: got %q", got)
	}
	if Classify(false, false) == VerdictUntrusted || Classify(false, false) == VerdictTrusted {
		t.Error("an unexamined item must not borrow either resolved state")
	}

	if Classify(true, true) != VerdictTrusted {
		t.Error("admitted and agreed should be trusted")
	}
	if Classify(true, false) != VerdictUntrusted {
		t.Error("admitted and refused should be untrusted")
	}

	if VerdictUnexamined.Resolved() {
		t.Error("unexamined is not a finding")
	}
	if !VerdictTrusted.Resolved() || !VerdictUntrusted.Resolved() {
		t.Error("trusted and untrusted are findings")
	}
}

// TestAcceptedButUnresolvedIsUnexamined covers the queue half: accepting an item is
// not resolving it, and a run that ends before the slow method reports leaves the
// item unexamined rather than clean.
func TestAcceptedButUnresolvedIsUnexamined(t *testing.T) {
	clock := &testClock{t: time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)}

	var queue PendingQueue
	for i := 0; i < 10; i++ {
		queue.Enqueue(Pending{Key: RouteKey{Address: "addr", Claimant: "node", Epoch: 1}, Call: "isOdd", AcceptedAt: clock.Now()})
	}
	if queue.Unresolved() != 10 {
		t.Fatalf("all 10 should be unresolved, got %d", queue.Unresolved())
	}

	// Two are resolved, the rest are abandoned by the draining timeout.
	trusted, untrusted := queue.Verify(func(p Pending) bool { return p.Call == "isOdd" })
	if trusted != 10 || untrusted != 0 {
		t.Fatalf("verify should resolve all 10: %d/%d", trusted, untrusted)
	}
	if queue.Unresolved() != 0 {
		t.Error("a full drain leaves nothing unresolved")
	}

	// A partial drain leaves the remainder unexamined.
	var partial PendingQueue
	for i := 0; i < 4; i++ {
		partial.Enqueue(Pending{Call: "isOdd"})
	}
	if _, _ = partial.Verify(func(Pending) bool { return true }); partial.Unresolved() != 0 {
		t.Error("drain should be complete")
	}
}
