package mcp_faults

import (
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	mrand "math/rand"
	"strings"
	"testing"
	"time"
)

// testClock drives the grace window deterministically, without sleeping.
type testClock struct{ t time.Time }

func (c *testClock) Now() time.Time          { return c.t }
func (c *testClock) Advance(d time.Duration) { c.t = c.t.Add(d) }

// hashedProxy builds a proxy whose route name is shaped exactly like a real
// route: <8 hex>_<tool>. Production honeypots are meant to be indistinguishable
// from real routes, so nothing here may depend on a "honeypot_" marker.
func hashedProxy(t *testing.T, toolName string) *ToolProxy {
	t.Helper()
	return hashedProxyAt(t, toolName, nil)
}

// hashedProxyAt is hashedProxy with a pinned clock.
func hashedProxyAt(t *testing.T, toolName string, now func() time.Time) *ToolProxy {
	t.Helper()

	prefixBytes := make([]byte, 4)
	if _, err := crand.Read(prefixBytes); err != nil {
		t.Fatalf("prefix entropy: %v", err)
	}

	return &ToolProxy{
		ProxyID:        "proxy_" + toolName + "_" + hex.EncodeToString(prefixBytes),
		ToolName:       toolName,
		Prefix:         hex.EncodeToString(prefixBytes) + "_",
		InvalidCalls:   []InvalidToolCall{},
		HoneypotMode:   false,
		SuspicionLevel: 1,
		now:            now,
	}
}

// TestHashedRouteNamesAreIndistinguishable checks classification when the
// honeypot carries no honest marker, so a caller cannot tell a decoy route from
// a real one by its name.
func TestHashedRouteNamesAreIndistinguishable(t *testing.T) {
	proxy := hashedProxy(t, "list-tools")

	if strings.Contains(strings.ToLower(proxy.Prefix), "honeypot") {
		t.Fatalf("decoy prefix must not advertise itself: %q", proxy.Prefix)
	}
	if want := 8; len(strings.TrimSuffix(proxy.Prefix, "_")) != want {
		t.Errorf("prefix should carry %d hex chars to match a real route, got %q", want, proxy.Prefix)
	}

	// The correctly stamped route resolves, whatever the name looks like.
	if ok, reason := proxy.ValidateCall(proxy.PrefixedToolName()); !ok || reason != "" {
		t.Errorf("correctly stamped call must resolve cleanly, got (%v, %q)", ok, reason)
	}

	// A bare name is a bypass regardless of naming scheme.
	if ok, reason := proxy.ValidateCall("list-tools"); ok || reason != "direct-call-to-tool-bypasses-proxy" {
		t.Errorf("bare call must read as direct, got (%v, %q)", ok, reason)
	}

	// A wrong hash is indistinguishable from fuzz, by construction.
	other := hashedProxy(t, "list-tools")
	if ok, reason := proxy.ValidateCall(other.PrefixedToolName()); ok || reason != "fuzzed-or-wrong-prefix" {
		t.Errorf("wrong hashed prefix must read as fuzz, got (%v, %q)", ok, reason)
	}

	metrics := proxy.GetInvalidCallMetrics()
	if metrics.DirectCalls != 1 || metrics.FuzzAttempts != 1 {
		t.Errorf("expected 1 direct + 1 fuzz, got %d direct + %d fuzz", metrics.DirectCalls, metrics.FuzzAttempts)
	}
	// 24 for the bypass, 8 for the wrong hash - the two tiers a prod-shaped
	// attacker can actually occupy.
	if metrics.Distance42i != 32 {
		t.Errorf("expected 32 distance (24 direct + 8 fuzz), got %d", metrics.Distance42i)
	}
}

// TestOneTimeSkillsIgnoredToVariation checks that redeeming a one-time skill
// resolves, and that *not* redeeming one leaves no trace whatsoever.
//
// A one-time skill is a pre-stamped capability used exactly once. In production
// some fraction is never redeemed: the agent solved the task another way, the
// call was unnecessary, the skill expired unused. That variation is not
// evidence, so it must not move distance or suspicion at any rate - including
// "every skill ignored", which must be indistinguishable from a perfect agent.
func TestOneTimeSkillsIgnoredToVariation(t *testing.T) {
	const (
		toolName = "list-tools"
		issued   = 200
	)

	for _, ignoreRate := range []float64{0, 0.1, 0.25, 0.5, 0.75, 0.9, 1.0} {
		t.Run(fmt.Sprintf("ignored=%.0f%%", ignoreRate*100), func(t *testing.T) {
			proxy := hashedProxy(t, toolName)
			rng := mrand.New(mrand.NewSource(20261022))

			redeemed := 0
			for i := 0; i < issued; i++ {
				if rng.Float64() < ignoreRate {
					continue // never presented, so nothing should be recorded
				}
				redeemed++
				if ok, reason := proxy.ValidateCall(proxy.PrefixedToolName()); !ok || reason != "" {
					t.Fatalf("redeemed one-time skill %d judged suspicious: %q", i, reason)
				}
			}

			// Ignoring a skill is silence, not suspicion.
			if m := proxy.GetInvalidCallMetrics(); m.TotalInvalid != 0 || m.Distance42i != 0 {
				t.Fatalf("%d ignored of %d issued moved the metrics: %d invalid, %d distance",
					issued-redeemed, issued, m.TotalInvalid, m.Distance42i)
			}

			// Distance must depend on what was done, not on how many skills went
			// unused: one bypass scores the same at every variation degree.
			proxy.ValidateCall(toolName)
			if m := proxy.GetInvalidCallMetrics(); m.Distance42i != 24 {
				t.Errorf("one bypass after %d redemptions scored %d, want 24", redeemed, m.Distance42i)
			}
		})
	}
}

// TestOneTimeSkillReuseIsScored pins the gap rather than a behavior worth
// keeping: a stamped route resolves on every use today, so a replayed one-time
// skill is indistinguishable from a second legitimate use. Rotation bounds the
// window (see #52) but does not detect the replay; when replay accounting lands
// (james-gibson/smoke-alarm#73) this test should fail, and that failure is the
// signal to invert it.
func TestOneTimeSkillReuseIsScored(t *testing.T) {
	proxy := hashedProxy(t, "list-tools")

	for use := 1; use <= 3; use++ {
		if ok, reason := proxy.ValidateCall(proxy.PrefixedToolName()); !ok || reason != "" {
			t.Fatalf("use %d of a stamped route did not resolve (%q); if replay accounting now exists, invert this expectation", use, reason)
		}
	}
}

// TestDeepRewriteCostsLessThanFuzz is the multi-layer statement: a call rewritten
// at every hop still carries the capability, so the cost grows slowly instead of
// jumping to the fuzz tier the moment a layer touches the name.
func TestDeepRewriteCostsLessThanFuzz(t *testing.T) {
	const layers = 5

	proxy := hashedProxy(t, "list-tools")

	for layer := 1; layer <= layers; layer++ {
		proxy.IssueSkill(fmt.Sprintf("hop-%d", layer))

		// Each hop rewrites the suffix but forwards the prefix untouched.
		ok, reason := proxy.ValidateCall(proxy.Prefix + fmt.Sprintf("via-hop-%d", layer))
		if ok {
			t.Fatalf("layer %d: a rewritten name is not the route", layer)
		}
		if reason != "skill-route-mismatch" {
			t.Fatalf("layer %d: rewritten-but-attributable call read as %q", layer, reason)
		}
	}

	metrics := proxy.GetInvalidCallMetrics()
	if metrics.FuzzAttempts != 0 {
		t.Errorf("%d layers of rewriting must not be fuzz, got %d", layers, metrics.FuzzAttempts)
	}
	if want := layers * burnedSkillCost; metrics.Distance42i != want {
		t.Errorf("%d layers should cost %d, got %d", layers, want, metrics.Distance42i)
	}
	if metrics.Distance42i >= layers*8 {
		t.Errorf("deep rewriting must stay below the fuzz tier: %d >= %d", metrics.Distance42i, layers*8)
	}
}

// TestBurnedSkillRouteIsUnfuzzed checks the attribution rule: a stamp we issued
// is never fuzz, however stale rotation has made it.
func TestBurnedSkillRouteIsUnfuzzed(t *testing.T) {
	clock := &testClock{t: time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)}
	proxy := hashedProxyAt(t, "list-tools", clock.Now)

	stamp := proxy.IssueSkill("reader")
	proxy.Rotate()
	clock.Advance(DefaultGraceWindow + time.Millisecond) // notification has landed

	ok, reason := proxy.ValidateCall(stamp)
	if ok {
		t.Fatal("a burned stamp must not resolve: rotation moved the route")
	}
	if reason != "burned-skill-route" {
		t.Errorf("burned stamp must read as burned, got %q", reason)
	}

	metrics := proxy.GetInvalidCallMetrics()
	if metrics.FuzzAttempts != 0 {
		t.Errorf("attributable call counted as fuzz (%d)", metrics.FuzzAttempts)
	}
	if metrics.SkillRouteCalls != 1 {
		t.Errorf("expected 1 attributable call, got %d", metrics.SkillRouteCalls)
	}
	if metrics.Distance42i != burnedSkillCost {
		t.Errorf("burned stamp should cost %d, got %d", burnedSkillCost, metrics.Distance42i)
	}
	if metrics.Distance42i >= 8 {
		t.Errorf("rotation must cost less than a fuzz attempt: %d >= 8", metrics.Distance42i)
	}
}

// TestRotationBurnsSkillsButBarelyMovesScore separates the two events: rotating
// burns every skill at once, and burning is not a call, so it moves no score.
func TestRotationBurnsSkillsButBarelyMovesScore(t *testing.T) {
	const issued = 50

	clock := &testClock{t: time.Date(2026, 10, 22, 3, 27, 30, 0, time.UTC)}
	proxy := hashedProxyAt(t, "list-tools", clock.Now)

	stamps := make([]string, 0, issued)
	for i := 0; i < issued; i++ {
		stamps = append(stamps, proxy.IssueSkill(fmt.Sprintf("skill-%d", i)))
	}

	before := proxy.GetInvalidCallMetrics().Distance42i
	burned := proxy.Rotate()
	if burned != issued {
		t.Fatalf("rotation should burn %d skills, burned %d", issued, burned)
	}
	if got := proxy.GetInvalidCallMetrics().Distance42i; got != before {
		t.Errorf("burning %d skills must not move the score: %d != %d", issued, got, before)
	}

	// One client caught mid-rotation, past grace: a rotation's worth of staleness
	// is worth less than half a fuzz attempt.
	clock.Advance(DefaultGraceWindow + time.Millisecond)
	proxy.ValidateCall(stamps[0])
	delta := proxy.GetInvalidCallMetrics().Distance42i - before
	if delta != burnedSkillCost {
		t.Errorf("a burned redemption should add %d, got %d", burnedSkillCost, delta)
	}
	if delta*2 >= 8 {
		t.Errorf("rotation must stay well below the fuzz tier: %d vs 8", delta)
	}
}

// TestMangledStampIsFuzz draws the line the attribution rule needs: deep enough
// mutation makes a call unattributable, and only then is it fuzzing.
func TestMangledStampIsFuzz(t *testing.T) {
	proxy := hashedProxy(t, "list-tools")
	old := proxy.IssueSkill("reader")
	proxy.Rotate()                   // burns "reader" and moves the route
	proxy.IssueSkill("reader-fresh") // a skill on the new route

	// Suffix rewritten in transit: the capability is still present, so the call
	// is ours, not fuzz.
	ok, reason := proxy.ValidateCall(proxy.Prefix + "other-tool")
	if ok {
		t.Fatal("a rebuilt name is not the route")
	}
	if reason != "skill-route-mismatch" {
		t.Errorf("intact capability must stay attributable, got %q", reason)
	}

	// Prefix corrupted in transit: the capability is gone, so it is fuzz.
	mangled := "x" + old[1:]
	if _, _, attributed := proxy.attribute(mangled); attributed {
		t.Fatalf("mangled stamp %q must not attribute", mangled)
	}
	ok, reason = proxy.ValidateCall(mangled)
	if ok {
		t.Fatal("mangled stamp must not resolve")
	}
	if reason != "fuzzed-or-wrong-prefix" {
		t.Errorf("unattributable call must read as fuzz, got %q", reason)
	}

	metrics := proxy.GetInvalidCallMetrics()
	if metrics.FuzzAttempts != 1 {
		t.Errorf("expected exactly the mangled call in the fuzz bucket, got %d", metrics.FuzzAttempts)
	}
	if want := burnedSkillCost + 8; metrics.Distance42i != want {
		t.Errorf("expected %d (attributable %d + fuzz 8), got %d", want, burnedSkillCost, metrics.Distance42i)
	}
}
