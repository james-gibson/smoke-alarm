package mcp_faults

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// epochClock renders a clock seam for a scheduled proxy: the proxy's schedule is
// re-based onto the test clock so the rotation clock can be advanced without
// sleeping or depending on wall time.
func epochClock(t *testing.T, proxy *ToolProxy, start time.Time) *testClock {
	t.Helper()

	clock := &testClock{t: start}
	proxy.now = clock.Now
	proxy.Schedule.Origin = start
	proxy.Schedule.Base = proxy.Epoch
	proxy.Prefix = proxy.Schedule.PrefixAt(proxy.Epoch)

	if proxy.Prefix == "" {
		t.Fatalf("scheduled proxy has no prefix at epoch %d", proxy.Epoch)
	}
	return clock
}

// TestPrefixIsDerivedNotDrawn is the substitution at the center of the fix: the
// route is a function of (nonce, epoch), so a prefix names one epoch instead of
// belonging to the process for life.
func TestPrefixIsDerivedNotDrawn(t *testing.T) {
	now := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	schedule := NewRouteSchedule(now, ClusterRotationPeriod)

	if got, want := schedule.PrefixAt(0), schedule.PrefixAt(0); got != want {
		t.Fatalf("derivation is not deterministic: %q != %q", got, want)
	}
	if schedule.PrefixAt(0) == schedule.PrefixAt(1) {
		t.Fatal("consecutive epochs must not share a prefix")
	}

	// The width the old draw had is preserved: 8 hex chars between the markers.
	prefix := schedule.PrefixAt(0)
	if trimmed := strings.TrimSuffix(strings.TrimPrefix(prefix, "honeypot_"), "_"); len(trimmed) != 8 {
		t.Errorf("prefix %q no longer carries 32 bits", prefix)
	}

	// A different instance nonce derives a different route at the same epoch.
	other := NewRouteSchedule(now, ClusterRotationPeriod)
	if other.PrefixAt(0) == schedule.PrefixAt(0) {
		t.Error("two instances derived the same route")
	}
}

// TestScheduleIsComputableAhead is the property that lets rotation be unspoken:
// a client given the schedule computes the prefix for any epoch without asking,
// and agrees with the node that holds the same schedule.
func TestScheduleIsComputableAhead(t *testing.T) {
	start := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	proxy := NewToolProxy("list-tools", false)
	clock := epochClock(t, proxy, start)

	// The client's copy of the schedule: the nonce, the epoch it started at, the
	// origin and the period. Nothing else is exchanged.
	client := proxy.Schedule

	for epoch := int64(0); epoch <= 3; epoch++ {
		at := start.Add(time.Duration(epoch) * ClusterRotationPeriod)
		clock.t = at

		if got := proxy.Schedule.EpochAt(at); got != epoch {
			t.Fatalf("node reads epoch %d at %s, want %d", got, at, epoch)
		}
		if got := client.EpochAt(at); got != epoch {
			t.Fatalf("client reads epoch %d at %s, want %d", got, at, epoch)
		}

		// The node presents this prefix; the client derived it. They match without
		// a notification, a listing, or a log line.
		presented := proxy.PrefixedToolName()
		derived := client.PrefixAt(client.EpochAt(at)) + proxy.ToolName
		if presented != derived {
			t.Fatalf("epoch %d: node presents %q, client derives %q", epoch, presented, derived)
		}
		if ok, reason := proxy.ValidateCall(derived); !ok || reason != "" {
			t.Fatalf("epoch %d: client-derived name did not resolve (%q)", epoch, reason)
		}
	}
}

// TestLeakedRouteExpires is the issue's headline: a prefix observed once is a
// route for one epoch, not for the life of the process.
func TestLeakedRouteExpires(t *testing.T) {
	start := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	proxy := NewToolProxy("list-tools", false)
	clock := epochClock(t, proxy, start)

	leaked := proxy.PrefixedToolName() // observed by a hop that read the body
	if ok, _ := proxy.ValidateCall(leaked); !ok {
		t.Fatal("the prefix must work in the epoch it was issued in")
	}

	// Move past the epoch boundary and the notification budget after it.
	clock.Advance(ClusterRotationPeriod + DefaultGraceWindow + time.Millisecond)

	ok, reason := proxy.ValidateCall(leaked)
	if ok {
		t.Fatal("a leaked prefix must not survive its epoch")
	}
	if reason != "stale-route-epoch" {
		t.Fatalf("expired prefix read as %q", reason)
	}

	if proxy.Epoch != 1 {
		t.Errorf("route did not rotate: epoch %d", proxy.Epoch)
	}

	// The replay is priced as rotation, not as attack: the eight-and-a-half times
	// an enumerator's guess would cost.
	metrics := proxy.GetInvalidCallMetrics()
	if metrics.StaleRouteCalls != 1 || metrics.FuzzAttempts != 0 {
		t.Errorf("expired prefix must not be fuzz: stale=%d fuzz=%d", metrics.StaleRouteCalls, metrics.FuzzAttempts)
	}
	if metrics.Distance42i != burnedSkillCost {
		t.Errorf("expired prefix cost %d, want %d", metrics.Distance42i, burnedSkillCost)
	}
	if metrics.Distance42i >= 8 {
		t.Errorf("expiry must cost less than a guess: %d >= 8", metrics.Distance42i)
	}

	// And the call records which epoch it was replayed from, which is what makes
	// the log line a refresh problem rather than an unknown probe.
	last := proxy.InvalidCalls[len(proxy.InvalidCalls)-1]
	if last.Epoch != 1 || last.CallEpoch != 0 {
		t.Errorf("epochs not recorded: call epoch %d, route epoch %d (want 0 and 1)", last.CallEpoch, last.Epoch)
	}
	if last.Source != "stale" {
		t.Errorf("source %q, want stale", last.Source)
	}
}

// TestStaleAndEnumerationAreDifferentPrices is the second half of the fix: a
// prefix we retired and a prefix we never issued must not read alike.
func TestStaleAndEnumerationAreDifferentPrices(t *testing.T) {
	start := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)

	stale := NewToolProxy("list-tools", false)
	staleClock := epochClock(t, stale, start)
	retired := stale.PrefixedToolName()
	staleClock.Advance(ClusterRotationPeriod + DefaultGraceWindow + time.Millisecond)

	guessed := NewToolProxy("list-tools", false)
	guessClock := epochClock(t, guessed, start)
	guessClock.Advance(ClusterRotationPeriod + DefaultGraceWindow + time.Millisecond)

	_, staleReason := stale.ValidateCall(retired)
	_, guessReason := guessed.ValidateCall("honeypot_deadbeef_tool")

	if staleReason == guessReason {
		t.Fatalf("stale and enumeration share a reason: %q", staleReason)
	}

	staleMetrics := stale.GetInvalidCallMetrics()
	guessMetrics := guessed.GetInvalidCallMetrics()
	if staleMetrics.Distance42i >= guessMetrics.Distance42i {
		t.Errorf("staleness (%d) must cost less than enumeration (%d)",
			staleMetrics.Distance42i, guessMetrics.Distance42i)
	}
	if guessMetrics.FuzzAttempts != 1 {
		t.Errorf("the guess must be fuzz, got %d", guessMetrics.FuzzAttempts)
	}
	if staleMetrics.FuzzAttempts != 0 {
		t.Errorf("the stale client must not be fuzz, got %d", staleMetrics.FuzzAttempts)
	}
}

// TestStaleInsideGraceResolves pins the notification budget: one epoch behind,
// inside grace, is a client that has not heard yet, and it still works.
func TestStaleInsideGraceResolves(t *testing.T) {
	start := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	proxy := NewToolProxy("list-tools", false)
	clock := epochClock(t, proxy, start)

	leaked := proxy.PrefixedToolName()
	clock.Advance(ClusterRotationPeriod + DefaultGraceWindow/2)

	if ok, reason := proxy.ValidateCall(leaked); !ok {
		t.Fatalf("a client inside the notification window was refused: %q", reason)
	}
	if metrics := proxy.GetInvalidCallMetrics(); metrics.TotalInvalid != 0 {
		t.Errorf("grace should record nothing, recorded %d calls", metrics.TotalInvalid)
	}
}

// TestLazyRotationPricesFromTheBoundary checks the tempting mistake: dating the
// retirement at the observed call instead of at the epoch boundary would give a
// leaked prefix one free call after every boundary, because the retire and the
// call would always happen together.
func TestLazyRotationPricesFromTheBoundary(t *testing.T) {
	start := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	proxy := NewToolProxy("list-tools", false)
	clock := epochClock(t, proxy, start)

	leaked := proxy.PrefixedToolName()

	// No traffic at all for several epochs; the first call after the lapse is the
	// one that notices.
	clock.Advance(3 * ClusterRotationPeriod)

	ok, reason := proxy.ValidateCall(leaked)
	if ok {
		t.Fatal("a prefix three epochs old was accepted")
	}
	if reason != "stale-route-epoch" {
		t.Fatalf("read as %q", reason)
	}

	retired := proxy.Retired[len(proxy.Retired)-1]
	if want := start.Add(ClusterRotationPeriod); !retired.RetiredAt.Equal(want) {
		t.Errorf("retirement dated %s, want the boundary %s", retired.RetiredAt, want)
	}
}

// TestRetiredWindowIsBounded checks the cost of the ledger: only a bounded number
// of closed epochs stay attributable. Past that a prefix is no longer a capability
// we vouch for, so it reads as a guess rather than as a stale client.
func TestRetiredWindowIsBounded(t *testing.T) {
	start := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	proxy := NewToolProxy("list-tools", false)
	epochClock(t, proxy, start)

	ancient := proxy.PrefixedToolName()
	for i := 0; i < RetiredEpochWindow+2; i++ {
		proxy.Rotate()
	}

	if got := len(proxy.Retired); got != RetiredEpochWindow {
		t.Fatalf("ledger grew to %d, want %d", got, RetiredEpochWindow)
	}

	ok, reason := proxy.ValidateCall(ancient)
	if ok {
		t.Fatal("a prefix from outside the ledger was accepted")
	}
	if reason != "fuzzed-or-wrong-prefix" {
		t.Errorf("prefix outside the ledger read as %q, want the enumeration tier", reason)
	}
}

// TestAheadClientIsSkewNotEnumeration covers the other sign of the same problem:
// an agent whose clock runs fast derives the next epoch before we reach it. If
// that read as fuzz, the safe map would punish accurate clocks.
func TestAheadClientIsSkewNotEnumeration(t *testing.T) {
	start := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	proxy := NewToolProxy("list-tools", false)
	clock := epochClock(t, proxy, start)

	ahead := proxy.Schedule.PrefixAt(proxy.Epoch+1) + proxy.ToolName
	clock.Advance(time.Millisecond)

	ok, reason := proxy.ValidateCall(ahead)
	if ok {
		t.Fatal("a future epoch's prefix is not the current route")
	}
	if reason != "ahead-route-epoch" {
		t.Fatalf("clock skew read as %q", reason)
	}

	metrics := proxy.GetInvalidCallMetrics()
	if metrics.FuzzAttempts != 0 {
		t.Errorf("skew counted as fuzz: %d", metrics.FuzzAttempts)
	}
	if metrics.Distance42i != burnedSkillCost {
		t.Errorf("skew cost %d, want %d", metrics.Distance42i, burnedSkillCost)
	}

	// Beyond the tolerance it is no longer a clock: a prefix for an epoch two
	// rotations out is not one any honest schedule produces yet.
	clock.Advance(time.Millisecond)
	far := proxy.Schedule.PrefixAt(proxy.Epoch+2) + proxy.ToolName
	if _, reason := proxy.ValidateCall(far); reason != "fuzzed-or-wrong-prefix" {
		t.Errorf("a prefix two epochs ahead read as %q", reason)
	}
}

// TestClockDrivenRotationBurnsStamps checks that a rotation the clock performed
// is the same event as an explicit one: stamps issued under the closed epoch are
// burned, and a replay of one is attributable rather than clean.
func TestClockDrivenRotationBurnsStamps(t *testing.T) {
	start := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	proxy := NewToolProxy("list-tools", false)
	clock := epochClock(t, proxy, start)

	stamp := proxy.IssueSkill("reader")
	clock.Advance(ClusterRotationPeriod + DefaultGraceWindow + time.Millisecond)

	ok, reason := proxy.ValidateCall(stamp)
	if ok {
		t.Fatal("a stamp from a closed epoch was accepted")
	}
	if reason != "burned-skill-route" {
		t.Fatalf("stamp read as %q, want the burned tier", reason)
	}
	if metrics := proxy.GetInvalidCallMetrics(); metrics.FuzzAttempts != 0 {
		t.Errorf("an issued stamp was counted as fuzz: %d", metrics.FuzzAttempts)
	}
}

// TestOutOfBandRotationKeepsOneTimeline checks that an explicit rotation does not
// fight the schedule: after rotating on instruction, the node does not get dragged
// forward again the moment it looks at the clock, and the epoch it opened is the
// one it presents.
func TestOutOfBandRotationKeepsOneTimeline(t *testing.T) {
	start := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	proxy := NewToolProxy("list-tools", false)
	clock := epochClock(t, proxy, start)

	proxy.Rotate()
	presented := proxy.PrefixedToolName()

	// Half an epoch later the schedule must still describe this epoch, not the
	// one the original origin would have predicted.
	clock.Advance(ClusterRotationPeriod / 2)

	if got := proxy.PrefixedToolName(); got != presented {
		t.Fatalf("presented %q, then %q half an epoch later", presented, got)
	}
	if got := proxy.Schedule.EpochAt(clock.Now()); got != proxy.Epoch {
		t.Errorf("schedule says epoch %d, route is at %d", got, proxy.Epoch)
	}
	if ok, reason := proxy.ValidateCall(presented); !ok {
		t.Errorf("the presented route does not validate: %q", reason)
	}
}

// TestConcurrentRotationHappensExactlyOnce is the invariant the lazy clock adds:
// many callers can notice the same closed epoch at once, and the route must move
// once - not once per caller, and not into a torn state where the prefix belongs
// to one epoch and the ledger to another.
func TestConcurrentRotationHappensExactlyOnce(t *testing.T) {
	start := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	proxy := NewToolProxy("list-tools", false)
	clock := epochClock(t, proxy, start)

	// Ten epochs of idleness, then a burst of calls that all notice at once.
	const idleEpochs = 10
	clock.Advance(idleEpochs * ClusterRotationPeriod)

	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			proxy.PrefixedToolName()
			proxy.ValidateCall("list-tools")
			proxy.GetInvalidCallMetrics()
		}()
	}
	wg.Wait()

	if proxy.Epoch != idleEpochs {
		t.Errorf("epoch %d, want %d", proxy.Epoch, idleEpochs)
	}
	if proxy.Rotations != 1 {
		t.Errorf("rotated %d times for one closed epoch", proxy.Rotations)
	}
	if got := len(proxy.Retired); got != 1 {
		t.Errorf("retired %d prefixes for one rotation, want 1", got)
	}
	if want := proxy.Schedule.PrefixAt(idleEpochs); proxy.Prefix != want {
		t.Errorf("prefix %q does not match the epoch it claims (%q)", proxy.Prefix, want)
	}

	// One rotation retires the epoch-0 prefix, not whichever prefix each caller
	// happened to read.
	if retired := proxy.Retired[0]; retired.Epoch != 0 {
		t.Errorf("retired epoch %d, want 0", retired.Epoch)
	}
}

// TestKeyCarriesTheEpoch checks the route key: the address alone is shared by
// design, so the epoch is what makes a stale receipt distinguishable from a
// forged one.
func TestKeyCarriesTheEpoch(t *testing.T) {
	start := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	proxy := NewToolProxy("list-tools", false)
	clock := epochClock(t, proxy, start)

	before := proxy.Key()
	clock.Advance(ClusterRotationPeriod)
	after := proxy.Key()

	if before.Matches(after) {
		t.Fatal("the route key survived a rotation")
	}
	if before.Claimant != after.Claimant {
		t.Error("same instance must keep its claimant")
	}
	if before.Epoch+1 != after.Epoch {
		t.Errorf("epoch did not advance by one: %d -> %d", before.Epoch, after.Epoch)
	}
	if !strings.Contains(after.String(), fmt.Sprintf("#%d", after.Epoch)) {
		t.Errorf("key %q does not render its epoch", after.String())
	}
}

// TestMetricsCarryTheEpoch checks that the epoch reaches the operator-facing
// summary, so a stale-route reading can be placed on the rotation timeline.
func TestMetricsCarryTheEpoch(t *testing.T) {
	start := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC)
	proxy := NewToolProxy("list-tools", false)
	clock := epochClock(t, proxy, start)

	clock.Advance(2 * ClusterRotationPeriod)

	// Traffic is what advances a lazy clock, and metrics are a record of traffic.
	if ok, reason := proxy.ValidateCall(proxy.PrefixedToolName()); !ok {
		t.Fatalf("current route did not resolve: %q", reason)
	}
	metrics := proxy.GetInvalidCallMetrics()

	if metrics.Epoch != proxy.Epoch || metrics.Epoch != 2 {
		t.Errorf("metrics report epoch %d, route is at %d", metrics.Epoch, proxy.Epoch)
	}
	if metrics.Rotations != 1 {
		t.Errorf("expected one rotation from epoch 0 to 2, got %d", metrics.Rotations)
	}
	if !strings.Contains(metrics.String(), "epoch=2") {
		t.Errorf("summary does not carry the epoch: %s", metrics.String())
	}
}
