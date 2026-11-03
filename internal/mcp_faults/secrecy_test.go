package mcp_faults

import (
	"fmt"
	"testing"
)

// The code is open source, so nothing may rest on an attacker not knowing how it works.
// These tests exist to keep that honest: knowing the exact naming scheme, the exact
// chain rule and the exact schedule must confer no advantage. What is load-bearing is
// possession (a chain position that was issued, a stamp that was handed out) and the
// measure (an invariant that a fabricated claim cannot satisfy).

// TestKnownFormatGivesNoAdvantage builds a stamp with full knowledge of the format - the
// separator, the hash length, the tool name - and shows it is still refused. The format
// is public; the value is not derivable.
func TestKnownFormatGivesNoAdvantage(t *testing.T) {
	proxy := hashedProxy(t, "list-tools")

	// An attacker who has read the source knows exactly how a stamp is shaped:
	// 8 hex characters, an underscore, then the tool name. Every forgery below is
	// therefore in the correct format and differs only in the value.
	flipped := "0"
	if proxy.Prefix[0] == '0' {
		flipped = "1"
	}
	nearMiss := flipped + proxy.Prefix[1:]

	forgeries := []string{
		fmt.Sprintf("deadbeef_%s", proxy.ToolName),
		fmt.Sprintf("00000000_%s", proxy.ToolName),
		nearMiss + proxy.ToolName, // one hex character away from correct
	}

	for _, forged := range forgeries {
		ok, reason := proxy.ValidateCall(forged)
		if ok {
			t.Errorf("a forged stamp in the correct format was accepted: %q", forged)
		}
		if reason == "" {
			t.Errorf("a forged stamp was refused without a reason: %q", forged)
		}
	}

	// And the real one, which the attacker cannot construct, resolves.
	if ok, reason := proxy.ValidateCall(proxy.PrefixedToolName()); !ok || reason != "" {
		t.Fatalf("the issued stamp should resolve, got (%v, %q)", ok, reason)
	}

	t.Logf("format is public; %d forgeries in the correct format were all refused", len(forgeries))
	t.Logf("what the attacker lacks is the issued value, not the scheme")
}

// TestPossessionIsTheBoundary is the chain half: a position the ingress issued can be
// followed, and one it did not issue cannot, whether or not the rule is public.
func TestPossessionIsTheBoundary(t *testing.T) {
	tumble := NewAlarmTumble("ingress-a", 1)

	// Public knowledge: the chain advances on acceptance, tolerance is one.
	if ok, _ := tumble.Offer(0, Isotoped); !ok {
		t.Fatal("the first alarm should be accepted")
	}
	issued := tumble.Chain()

	// Issued position: accepted.
	if ok, _ := tumble.Offer(issued-1, Isotoped); !ok {
		t.Error("a lagging position inside the tolerance should be accepted")
	}

	// Unissued position, computed from a public rule: refused as too-new.
	if ok, _ := tumble.Offer(tumble.Chain()+5, Isotoped); ok {
		t.Error("an unissued future position must not be accepted, however derivable")
	}

	// Far past position: refused as stale, and it costs the cluster nothing.
	before := tumble.Rotated()
	if ok, _ := tumble.Offer(0, Isotoped); ok {
		t.Error("a stale position must not be accepted")
	}
	if tumble.Rotated() != before {
		t.Error("a refusal must not advance the chain")
	}

	t.Logf("chain rule is public; issued=%d, accepted only what was possessed", issued)
}

// TestTheMeasureIsWhatCannotBeFaked is the multivariate half, in the only form this
// package can express: a claim that is internally consistent but does not satisfy the
// measure is caught by the measure, not by secrecy.
func TestTheMeasureIsWhatCannotBeFaked(t *testing.T) {
	// A deriver can produce a well-formed pair that still fails to close: value and
	// mirror are both plausible, and their sum is not zero.
	drifted := flag{id: "drifted", value: complex(42, 0), mirror: complex(-42, 0.01)}
	proper := flag{id: "proper", value: complex(42, 0), mirror: complex(-42, 0)}

	if proper.residual() > 1e-12 {
		t.Errorf("a proper inversion must close, residual %v", proper.residual())
	}
	if drifted.residual() <= 1e-12 {
		t.Errorf("a drifted inversion must not close, residual %v", drifted.residual())
	}

	// The measure is a number, not a check against a secret: publish it and it still
	// discriminates.
	t.Logf("residuals: proper %.1e, drifted %.1e", proper.residual(), drifted.residual())

	if Classify(true, proper.residual() <= 1e-12) != VerdictTrusted {
		t.Error("a closing inversion should be trusted")
	}
	if Classify(true, drifted.residual() <= 1e-12) != VerdictUntrusted {
		t.Error("a drifted inversion should be untrusted, and that verdict is public")
	}
}

// TestObscurityIsNotCounted documents the accounting: unguessability raises a price and
// is not a security claim. Two proxies with the same tool name have independent stamps,
// so the space is per instance and guessing costs a priced attempt.
func TestObscurityIsNotCounted(t *testing.T) {
	a := hashedProxy(t, "list-tools")
	b := hashedProxy(t, "list-tools")

	if a.Prefix == b.Prefix {
		t.Fatal("fixture: per-instance stamps should differ")
	}

	// Guessing is priced, and the price is what deters: a wrong prefix costs the fuzz
	// tier, and eviction follows a handful of attempts.
	guessesBeforeEviction := evictionThreshold / 8
	t.Logf("per-instance space: 32 bits; guesses before eviction: %d", guessesBeforeEviction)

	if guessesBeforeEviction < 1 {
		t.Error("the price must allow at least one attempt to be visible")
	}

	// The claim to make in an ADR, stated as a test: the security property is that a
	// wrong guess is *detected and priced*, not that the space is unknowable.
	if ok, _ := a.ValidateCall(b.PrefixedToolName()); ok {
		t.Error("a neighbour's stamp must be refused")
	}
	if m := a.GetInvalidCallMetrics(); m.Distance42i == 0 {
		t.Error("a wrong stamp must be priced, not merely refused")
	}
}
