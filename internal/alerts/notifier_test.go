package alerts

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/engine"
)

type stubNotifier struct {
	err   error
	calls int
}

func (s *stubNotifier) Notify(context.Context, engine.AlertEvent) error {
	s.calls++
	return s.err
}

func TestNotifierGroupNotifyAggregatesErrors(t *testing.T) {
	t.Parallel()

	event := engine.AlertEvent{
		TargetID:   "t-1",
		TargetName: "Target One",
		State:      "outage",
		Severity:   "critical",
		CheckedAt:  time.Now(),
	}

	n1 := &stubNotifier{}
	n2Err := errors.New("backend failure")
	n2 := &stubNotifier{err: n2Err}

	group := NewNotifierGroup(n1, nil, n2)

	err := group.Notify(context.Background(), event)
	if err == nil {
		t.Fatalf("expected aggregated error, got nil")
	}
	if !errors.Is(err, n2Err) {
		t.Fatalf("expected error to wrap notifier error, got %v", err)
	}
	if n1.calls != 1 || n2.calls != 1 {
		t.Fatalf("expected each notifier to be invoked once, got n1=%d n2=%d", n1.calls, n2.calls)
	}
}

func TestLogNotifierSeverityFilteringAndDedupe(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
		if a.Key == slog.SourceKey {
			return slog.Attr{}
		}
		return a
	}}))

	ln := NewLogNotifier(logger, "warn", time.Hour)

	ctx := context.Background()
	eventLow := engine.AlertEvent{
		TargetID:     "low",
		Severity:     "info",
		Message:      "ignored",
		CheckedAt:    time.Now(),
		FailureClass: engine.AlertEvent{}.FailureClass,
	}
	if err := ln.Notify(ctx, eventLow); err != nil {
		t.Fatalf("unexpected error for filtered event: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected no log output for filtered severity")
	}

	event := engine.AlertEvent{
		TargetID:     "critical",
		TargetName:   "Critical Target",
		State:        "outage",
		Severity:     "critical",
		Message:      "Bearer secret should be redacted token=mytoken",
		FailureClass: "protocol",
		Regression:   true,
		CheckedAt:    time.Now(),
		Details:      map[string]any{"count": 1},
	}
	if err := ln.Notify(ctx, event); err != nil {
		t.Fatalf("unexpected error emitting log: %v", err)
	}
	output := buf.String()
	if !strings.Contains(output, "Bearer ****") || !strings.Contains(output, "token=****") || strings.Contains(output, "token=mytoken") {
		t.Fatalf("expected sanitized output, got %q", output)
	}

	before := buf.Len()
	if err := ln.Notify(ctx, event); err != nil {
		t.Fatalf("unexpected error on deduped event: %v", err)
	}
	if buf.Len() != before {
		t.Fatalf("expected deduped event to avoid additional log output")
	}
}

func TestSanitizeAndDedupeKey(t *testing.T) {
	t.Parallel()

	raw := "Bearer abc TOKEN=xyz refresh_token"
	want := "Bearer ****abc TOKEN=xyz refresh_token_redacted"
	if got := sanitize(raw); got != want {
		t.Fatalf("sanitize() = %q, want %q", got, want)
	}

	event := engine.AlertEvent{
		TargetID:     "A",
		State:        "healthy",
		FailureClass: "none",
		Severity:     "info",
		Message:      "Bearer abc",
	}
	key := dedupeKey(event)
	if !strings.Contains(key, "bearer ****") {
		t.Fatalf("expected dedupe key to contain sanitized message, got %q", key)
	}
}

func TestNotificationHelpers(t *testing.T) {
	t.Parallel()

	event := engine.AlertEvent{
		TargetID:     "t-123",
		TargetName:   "",
		State:        "outage",
		Severity:     "critical",
		Message:      "Something broke",
		Regression:   true,
		FailureClass: "protocol",
	}
	body := notificationBody(event)
	if !strings.Contains(body, "t-123 (OUTAGE)") || !strings.Contains(body, "regression") {
		t.Fatalf("unexpected notification body: %q", body)
	}

	event.TargetName = "Primary Service"
	if got := targetNameOrID(event); got != "Primary Service" {
		t.Fatalf("expected target name fallback, got %q", got)
	}

	if got := urgencyFromSeverity("critical"); got != "critical" {
		t.Fatalf("expected critical urgency, got %q", got)
	}
	if got := urgencyFromSeverity("warn"); got != "normal" {
		t.Fatalf("expected normal urgency for warn, got %q", got)
	}
	if got := urgencyFromSeverity("info"); got != "low" {
		t.Fatalf("expected low urgency for info, got %q", got)
	}
}

func TestSeverityAllowedOrdering(t *testing.T) {
	t.Parallel()

	cases := []struct {
		current string
		min     string
		want    bool
	}{
		{"critical", "warn", true},
		{"error", "critical", false},
		{"warn", "warn", true},
		{"info", "", true},
		{"debug", "info", false},
	}
	for _, tc := range cases {
		if got := severityAllowed(tc.current, tc.min); got != tc.want {
			t.Fatalf("severityAllowed(%q, %q) = %v, want %v", tc.current, tc.min, got, tc.want)
		}
	}
}

func TestDesktopNotifierDeduping(t *testing.T) {
	t.Parallel()

	n := NewDesktopNotifier("[Test]", "info", time.Hour)
	n.commandTimout = 10 * time.Millisecond

	event := engine.AlertEvent{
		TargetID:     "id-1",
		State:        "healthy",
		Severity:     "info",
		Message:      "first",
		FailureClass: "none",
	}

	if n.isDeduped(event) {
		t.Fatalf("first event should not be deduped")
	}
	if !n.isDeduped(event) {
		t.Fatalf("second identical event should be deduped")
	}

	// Ensure different message bypasses dedupe.
	event.Message = "second"
	if n.isDeduped(event) {
		t.Fatalf("different event payload should not be deduped")
	}
}
