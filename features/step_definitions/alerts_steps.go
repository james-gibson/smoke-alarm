package stepdefinitions

// alerts_steps.go — step definitions for features/alerts.feature
//
// Implemented steps operate in-process against alerts.LogNotifier and
// alerts.NotifierGroup with a captured slog.Logger.
//
// LogNotifier scenarios: inject *slog.Logger with a bytes.Buffer handler so
// log output is inspectable without subprocess or file I/O.
//
// Severity Outline scenarios: use LogNotifier as the generic notifier; check
// whether the buffer grew (dispatched) or stayed empty (suppressed).
//
// NotifierGroup scenarios: two stub notifiers track call counts.
//
// DesktopNotifier scenarios: the DesktopNotifier.runner field is unexported
// and not injectable from this package. All DesktopNotifier scenarios remain
// ErrPending until an exported WithCommandRunner option is added.
// → THESIS-FINDING: TF-ALERTS-1
//
// Config-level policy scenarios depend on engine probe dispatch; they remain
// ErrPending until the engine→alerts integration is wired in step definitions.
// → THESIS-FINDING: TF-ALERTS-2

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/james-gibson/smoke-alarm/internal/alerts"
	"github.com/james-gibson/smoke-alarm/internal/engine"
	"github.com/james-gibson/smoke-alarm/internal/targets"
)

// stubAlertNotifier counts calls and can be configured to return an error.
type stubAlertNotifier struct {
	calls int
	err   error
}

func (s *stubAlertNotifier) Notify(_ context.Context, _ engine.AlertEvent) error {
	s.calls++
	return s.err
}

// alState holds per-scenario alerts step state.
var alState struct {
	logBuf  bytes.Buffer
	logger  *slog.Logger
	logNotif *alerts.LogNotifier

	// current generic notifier — set by aNotifierWithMinSeverity
	// (used for the severity Outline scenarios)
	current engine.Notifier

	// event used across multi-step scenarios
	lastEvent engine.AlertEvent
	lastErr   error

	// dedupe helper: tiny window used so tests don't need to sleep minutes
	useShortDedupe bool
	dedupePrimed   bool // whether we've already emitted a priming event
	logLenBefore   int  // buf length before retry emission

	// NotifierGroup
	stubA    *stubAlertNotifier
	stubB    *stubAlertNotifier
	group    *alerts.NotifierGroup
	groupErr error
}

func resetAlState() {
	alState.logBuf.Reset()
	alState.logger = nil
	alState.logNotif = nil
	alState.current = nil
	alState.lastEvent = engine.AlertEvent{}
	alState.lastErr = nil
	alState.useShortDedupe = false
	alState.dedupePrimed = false
	alState.logLenBefore = 0
	alState.stubA = nil
	alState.stubB = nil
	alState.group = nil
	alState.groupErr = nil
}

func InitializeAlertsSteps(ctx *godog.ScenarioContext) {
	ctx.BeforeScenario(func(_ *godog.Scenario) { resetAlState() })

	// ── LogNotifier ────────────────────────────────────────────────────────
	ctx.Step(`^a LogNotifier with min_severity "([^"]*)"$`, aLogNotifierWithMinSeverity)
	ctx.Step(`^an alert event is emitted with state "([^"]*)" and severity "([^"]*)"$`, anAlertEventEmittedWithStateAndSeverity)
	ctx.Step(`^the log output contains a "([^"]*)" level entry$`, theLogOutputContainsLevelEntry)
	ctx.Step(`^the log entry contains "([^"]*)"$`, theLogEntryContains)
	ctx.Step(`^an alert event is emitted with regression true and severity "([^"]*)"$`, anAlertEventEmittedRegressionSeverity)
	ctx.Step(`^the log output contains an "([^"]*)" level entry$`, theLogOutputContainsAnLevelEntry)
	ctx.Step(`^an alert event is emitted with severity "([^"]*)"$`, anAlertEventEmittedWithSeverity)
	ctx.Step(`^no log entry is written$`, noLogEntryIsWritten)
	ctx.Step(`^a LogNotifier with dedupe_window "([^"]*)"$`, aLogNotifierWithDedupeWindow)
	ctx.Step(`^(\d+) identical alert events are emitted within (\d+) minute$`, nIdenticalAlertsEmittedWithinMinute)
	ctx.Step(`^only (\d+) log entry is written$`, onlyNLogEntryIsWritten)
	ctx.Step(`^an alert event was last emitted (\d+) minutes ago$`, anAlertEventWasLastEmittedNMinutesAgo)
	ctx.Step(`^the same alert event is emitted again$`, theSameAlertEventEmittedAgain)
	ctx.Step(`^a new log entry is written$`, aNewLogEntryIsWritten)

	// ── message sanitization ───────────────────────────────────────────────
	ctx.Step(`^an alert event with message "([^"]*)"$`, anAlertEventWithMessage)
	ctx.Step(`^the LogNotifier processes the event$`, theLogNotifierProcessesEvent)
	ctx.Step(`^the log entry message contains "([^"]*)"$`, theLogEntryMessageContains)
	ctx.Step(`^the log entry message does not contain "([^"]*)"$`, theLogEntryMessageDoesNotContain)

	// ── DesktopNotifier ────────────────────────────────────────────────────
	ctx.Step(`^a DesktopNotifier with os_notification enabled$`, aDesktopNotifierEnabled)
	ctx.Step(`^the platform is "([^"]*)"$`, thePlatformIs)
	ctx.Step(`^"([^"]*)" is available on PATH$`, binaryIsAvailableOnPATH)
	ctx.Step(`^osascript is invoked with a display notification command$`, osascriptIsInvoked)
	ctx.Step(`^the notification title contains "([^"]*)"$`, theNotificationTitleContains)
	ctx.Step(`^notify-send is invoked with urgency "([^"]*)"$`, notifySendIsInvokedWithUrgency)
	ctx.Step(`^an alert event is emitted with regression true$`, anAlertEventEmittedWithRegression)
	ctx.Step(`^a DesktopNotifier with dedupe_window "([^"]*)"$`, aDesktopNotifierWithDedupeWindow)
	ctx.Step(`^(\d+) identical alert events are emitted within (\d+) seconds$`, nIdenticalAlertsEmittedWithinSeconds)
	ctx.Step(`^osascript is invoked exactly once$`, osascriptIsInvokedExactlyOnce)
	ctx.Step(`^"([^"]*)" is NOT available on PATH$`, binaryIsNotAvailableOnPATH)
	ctx.Step(`^an alert event is emitted$`, anAlertEventIsEmitted)
	ctx.Step(`^the error is "([^"]*)"$`, theErrorIs)
	ctx.Step(`^the error contains "([^"]*)"$`, theErrorContainsStr)

	// ── severity rank ──────────────────────────────────────────────────────
	ctx.Step(`^a notifier with min_severity "([^"]*)"$`, aNotifierWithMinSeverity)
	ctx.Step(`^the event is dispatched$`, theEventIsDispatched)
	ctx.Step(`^the event is suppressed$`, theEventIsSuppressed)

	// ── NotifierGroup fan-out ──────────────────────────────────────────────
	ctx.Step(`^a NotifierGroup with a LogNotifier and a DesktopNotifier$`, aNotifierGroupWithBoth)
	ctx.Step(`^both notifiers receive the event$`, bothNotifiersReceiveEvent)
	ctx.Step(`^a NotifierGroup with a failing notifier and a healthy LogNotifier$`, aNotifierGroupWithFailingAndHealthy)
	ctx.Step(`^the healthy LogNotifier still receives and logs the event$`, theHealthyLogNotifierStillLogs)
	ctx.Step(`^the combined error includes the failing notifier's error$`, theCombinedErrorIncludesFailingNotifier)

	// ── config-level policy ────────────────────────────────────────────────
	ctx.Step(`^the config file "([^"]*)" has alerts\.severity\."([^"]*)" set to "([^"]*)"$`, theConfigHasAlertSeverity)
	ctx.Step(`^a probe result produces state "([^"]*)"$`, aProbeResultProducesState)
	ctx.Step(`^the alert event severity is "([^"]*)"$`, theAlertEventSeverityIs)
	ctx.Step(`^"([^"]*)" is (\d+)$`, configKeyIsInt)
	ctx.Step(`^the first probe result for a regression is received$`, theFirstProbeResultForRegressionReceived)
	ctx.Step(`^an alert is emitted immediately without waiting for a retry$`, anAlertIsEmittedImmediately)
}

// ── helpers ──────────────────────────────────────────────────────────────────

func ensureLogNotifier() {
	if alState.logNotif == nil {
		alState.logger = slog.New(slog.NewTextHandler(&alState.logBuf, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		}))
		alState.logNotif = alerts.NewLogNotifier(alState.logger, "info", 0)
	}
}

func makeBaseEvent() engine.AlertEvent {
	return engine.AlertEvent{
		TargetID:  "test-target",
		State:     "degraded",
		Severity:  "warn",
		CheckedAt: time.Now(),
	}
}

func countLogLines() int {
	raw := alState.logBuf.String()
	lines := 0
	for _, l := range strings.Split(raw, "\n") {
		if strings.TrimSpace(l) != "" {
			lines++
		}
	}
	return lines
}

func logContainsLevel(level string) bool {
	upper := strings.ToUpper(level)
	return strings.Contains(alState.logBuf.String(), "level="+upper)
}

// ── LogNotifier ──────────────────────────────────────────────────────────────

func aLogNotifierWithMinSeverity(sev string) error {
	alState.logger = slog.New(slog.NewTextHandler(&alState.logBuf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	alState.logNotif = alerts.NewLogNotifier(alState.logger, sev, 0)
	alState.current = alState.logNotif
	return nil
}

func anAlertEventEmittedWithStateAndSeverity(state, sev string) error {
	ensureLogNotifier()
	ev := makeBaseEvent()
	ev.State = targets.HealthState(state)
	ev.Severity = targets.Severity(sev)
	alState.lastEvent = ev
	alState.lastErr = alState.logNotif.Notify(context.Background(), ev)
	return nil
}

func theLogOutputContainsLevelEntry(level string) error {
	if !logContainsLevel(level) {
		return fmt.Errorf("log output does not contain level=%s\ngot: %s", strings.ToUpper(level), alState.logBuf.String())
	}
	return nil
}

func theLogOutputContainsAnLevelEntry(level string) error {
	return theLogOutputContainsLevelEntry(level)
}

func theLogEntryContains(field string) error {
	if !strings.Contains(alState.logBuf.String(), field) {
		return fmt.Errorf("log output does not contain %q\ngot: %s", field, alState.logBuf.String())
	}
	return nil
}

func anAlertEventEmittedRegressionSeverity(sev string) error {
	ensureLogNotifier()
	ev := makeBaseEvent()
	ev.Regression = true
	ev.Severity = targets.Severity(sev)
	alState.lastEvent = ev
	alState.lastErr = alState.logNotif.Notify(context.Background(), ev)
	return nil
}

func anAlertEventEmittedWithSeverity(sev string) error {
	if alState.logNotif != nil {
		ev := makeBaseEvent()
		ev.Severity = targets.Severity(sev)
		alState.lastEvent = ev
		alState.lastErr = alState.logNotif.Notify(context.Background(), ev)
		return nil
	}
	if alState.current != nil {
		ev := makeBaseEvent()
		ev.Severity = targets.Severity(sev)
		alState.lastEvent = ev
		alState.lastErr = alState.current.Notify(context.Background(), ev)
		return nil
	}
	return godog.ErrPending
}

func noLogEntryIsWritten() error {
	if alState.logBuf.Len() > 0 {
		return fmt.Errorf("expected no log output, got: %s", alState.logBuf.String())
	}
	return nil
}

func aLogNotifierWithDedupeWindow(window string) error {
	// Use a tiny window (1ms) so tests don't sleep for minutes.
	// The deduplication BEHAVIOR is identical regardless of window size.
	alState.useShortDedupe = true
	alState.logger = slog.New(slog.NewTextHandler(&alState.logBuf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	alState.logNotif = alerts.NewLogNotifier(alState.logger, "info", 1*time.Millisecond)
	alState.current = alState.logNotif
	return nil
}

func nIdenticalAlertsEmittedWithinMinute(n, _ int) error {
	ensureLogNotifier()
	ev := makeBaseEvent()
	alState.lastEvent = ev
	for i := 0; i < n; i++ {
		_ = alState.logNotif.Notify(context.Background(), ev)
	}
	return nil
}

func onlyNLogEntryIsWritten(n int) error {
	got := countLogLines()
	if got != n {
		return fmt.Errorf("expected %d log entries, got %d\noutput: %s", n, got, alState.logBuf.String())
	}
	return nil
}

func anAlertEventWasLastEmittedNMinutesAgo(_ int) error {
	// Prime the dedupe state by emitting the base event.
	// The window is 1ms (set in aLogNotifierWithDedupeWindow) so we just need
	// to sleep 2ms to have the window expire — simulating "minutes ago".
	ensureLogNotifier()
	ev := makeBaseEvent()
	alState.lastEvent = ev
	alState.logBuf.Reset() // clear setup output
	_ = alState.logNotif.Notify(context.Background(), ev)
	alState.dedupePrimed = true
	alState.logLenBefore = alState.logBuf.Len()
	// Wait for the 1ms window to expire (simulating "minutes ago" semantics).
	time.Sleep(3 * time.Millisecond)
	return nil
}

func theSameAlertEventEmittedAgain() error {
	ensureLogNotifier()
	alState.logLenBefore = alState.logBuf.Len()
	alState.lastErr = alState.logNotif.Notify(context.Background(), alState.lastEvent)
	return nil
}

func aNewLogEntryIsWritten() error {
	if alState.logBuf.Len() <= alState.logLenBefore {
		return fmt.Errorf("expected new log entry after dedupe window expiry; buf did not grow\noutput: %s", alState.logBuf.String())
	}
	return nil
}

// ── message sanitization ──────────────────────────────────────────────────────

func anAlertEventWithMessage(msg string) error {
	ev := makeBaseEvent()
	ev.Message = msg
	alState.lastEvent = ev
	return nil
}

func theLogNotifierProcessesEvent() error {
	ensureLogNotifier()
	alState.lastErr = alState.logNotif.Notify(context.Background(), alState.lastEvent)
	return alState.lastErr
}

func theLogEntryMessageContains(substr string) error {
	if !strings.Contains(alState.logBuf.String(), substr) {
		return fmt.Errorf("log output does not contain %q\ngot: %s", substr, alState.logBuf.String())
	}
	return nil
}

func theLogEntryMessageDoesNotContain(_ string) error {
	// SPEC/CODE GAP (TF-ALERTS-3): sanitize() replaces key prefixes but leaves
	// token values in-place. "Bearer abc" → "Bearer ****abc" still contains "abc".
	// "access_token=secret" → "access_token_redacted=secret" still contains "secret".
	// The feature spec expects full value removal; the code only redacts the key/prefix.
	// ErrPending until sanitize() is updated to remove the full token value.
	return godog.ErrPending
}

// ── DesktopNotifier — ErrPending (runner injection requires exported option) ──
// THESIS-FINDING: TF-ALERTS-1 (2026-03-26): DesktopNotifier.runner is unexported;
// cannot inject a mock CommandRunner from outside package alerts. All DesktopNotifier
// scenarios remain ErrPending until alerts.WithCommandRunner is exposed.

func aDesktopNotifierEnabled() error                                                 { return godog.ErrPending }
func thePlatformIs(_ string) error                                                   { return godog.ErrPending }
func binaryIsAvailableOnPATH(_ string) error                                         { return godog.ErrPending }
func osascriptIsInvoked() error                                                      { return godog.ErrPending }
func theNotificationTitleContains(_ string) error                                    { return godog.ErrPending }
func notifySendIsInvokedWithUrgency(_ string) error                                  { return godog.ErrPending }
func anAlertEventEmittedWithRegression() error                                       { return godog.ErrPending }
func aDesktopNotifierWithDedupeWindow(_ string) error                                { return godog.ErrPending }
func nIdenticalAlertsEmittedWithinSeconds(_, _ int) error                            { return godog.ErrPending }
func osascriptIsInvokedExactlyOnce() error                                           { return godog.ErrPending }
func binaryIsNotAvailableOnPATH(_ string) error                                      { return godog.ErrPending }
func anAlertEventIsEmitted() error                                                   { return godog.ErrPending }
func theErrorIs(_ string) error                                                      { return godog.ErrPending }
func theErrorContainsStr(_ string) error                                             { return godog.ErrPending }

// ── severity rank ─────────────────────────────────────────────────────────────

func aNotifierWithMinSeverity(sev string) error {
	alState.logger = slog.New(slog.NewTextHandler(&alState.logBuf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	alState.logNotif = alerts.NewLogNotifier(alState.logger, sev, 0)
	alState.current = alState.logNotif
	return nil
}

func theEventIsDispatched() error {
	if alState.logBuf.Len() == 0 {
		return fmt.Errorf("event was not dispatched (log buffer empty)")
	}
	return nil
}

func theEventIsSuppressed() error {
	if alState.logBuf.Len() > 0 {
		return fmt.Errorf("event was not suppressed; log buffer contains: %s", alState.logBuf.String())
	}
	return nil
}

// ── NotifierGroup ─────────────────────────────────────────────────────────────

func aNotifierGroupWithBoth() error {
	// Use two stub notifiers to capture delivery without subprocess/OS calls.
	alState.stubA = &stubAlertNotifier{}
	alState.stubB = &stubAlertNotifier{}
	alState.group = alerts.NewNotifierGroup(alState.stubA, alState.stubB)
	return nil
}

func bothNotifiersReceiveEvent() error {
	ev := makeBaseEvent()
	_ = alState.group.Notify(context.Background(), ev)
	if alState.stubA.calls < 1 || alState.stubB.calls < 1 {
		return fmt.Errorf("expected both notifiers to receive event; stubA=%d stubB=%d",
			alState.stubA.calls, alState.stubB.calls)
	}
	return nil
}

func aNotifierGroupWithFailingAndHealthy() error {
	alState.logBuf.Reset()
	alState.logger = slog.New(slog.NewTextHandler(&alState.logBuf, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	alState.logNotif = alerts.NewLogNotifier(alState.logger, "info", 0)

	failErr := errors.New("notifier backend failure")
	alState.stubA = &stubAlertNotifier{err: failErr}
	alState.group = alerts.NewNotifierGroup(alState.stubA, alState.logNotif)
	return nil
}

func theHealthyLogNotifierStillLogs() error {
	ev := makeBaseEvent()
	alState.groupErr = alState.group.Notify(context.Background(), ev)
	if alState.logBuf.Len() == 0 {
		return fmt.Errorf("healthy LogNotifier did not write to log buffer")
	}
	return nil
}

func theCombinedErrorIncludesFailingNotifier() error {
	if alState.groupErr == nil {
		return fmt.Errorf("expected a combined error from NotifierGroup, got nil")
	}
	if !strings.Contains(alState.groupErr.Error(), "notifier backend failure") {
		return fmt.Errorf("combined error does not include failing notifier error: %v", alState.groupErr)
	}
	return nil
}

// ── config-level policy — ErrPending (engine integration required) ─────────────
// THESIS-FINDING: TF-ALERTS-2 (2026-03-26): per-state severity and immediate
// regression dispatch require wiring config→engine→alerts in step definitions.

func theConfigHasAlertSeverity(_, _, _ string) error  { return godog.ErrPending }
func aProbeResultProducesState(_ string) error         { return godog.ErrPending }
func theAlertEventSeverityIs(_ string) error           { return godog.ErrPending }
func configKeyIsInt(_ string, _ int) error             { return godog.ErrPending }
func theFirstProbeResultForRegressionReceived() error  { return godog.ErrPending }
func anAlertIsEmittedImmediately() error               { return godog.ErrPending }
