package alerts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/engine"
)

var (
	// ErrNotifierUnsupported means the current OS does not support desktop notifications
	// in this notifier implementation.
	ErrNotifierUnsupported = errors.New("desktop notifier unsupported on this platform")
	// ErrNotifierUnavailable means the OS notification command is not available.
	ErrNotifierUnavailable = errors.New("desktop notifier command unavailable")
)

// NotifierGroup fans out events to multiple notifier backends.
type NotifierGroup struct {
	notifiers []engine.Notifier
}

// NewNotifierGroup creates a group notifier.
func NewNotifierGroup(notifiers ...engine.Notifier) *NotifierGroup {
	compact := make([]engine.Notifier, 0, len(notifiers))
	for _, n := range notifiers {
		if n != nil {
			compact = append(compact, n)
		}
	}
	return &NotifierGroup{notifiers: compact}
}

// Notify sends the event to all configured notifiers and joins any errors.
func (g *NotifierGroup) Notify(ctx context.Context, event engine.AlertEvent) error {
	var errs []error
	for _, n := range g.notifiers {
		if err := n.Notify(ctx, event); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}

// LogNotifier emits structured log alerts.
type LogNotifier struct {
	logger       *slog.Logger
	minSeverity  string
	dedupeWindow time.Duration

	mu   sync.Mutex
	last map[string]time.Time
}

// NewLogNotifier creates a log-based notifier.
func NewLogNotifier(logger *slog.Logger, minSeverity string, dedupeWindow time.Duration) *LogNotifier {
	if logger == nil {
		logger = slog.Default()
	}
	return &LogNotifier{
		logger:       logger,
		minSeverity:  strings.ToLower(strings.TrimSpace(minSeverity)),
		dedupeWindow: dedupeWindow,
		last:         make(map[string]time.Time),
	}
}

// Notify logs alert events with dedupe and severity filtering.
func (n *LogNotifier) Notify(_ context.Context, event engine.AlertEvent) error {
	if !severityAllowed(string(event.Severity), n.minSeverity) {
		return nil
	}
	if n.isDeduped(event) {
		return nil
	}

	details := ""
	if len(event.Details) > 0 {
		if b, err := json.Marshal(event.Details); err == nil {
			details = string(b)
		}
	}

	msg := sanitize(event.Message)
	attrs := []any{
		"target_id", event.TargetID,
		"target_name", event.TargetName,
		"state", string(event.State),
		"severity", string(event.Severity),
		"failure_class", string(event.FailureClass),
		"regression", event.Regression,
		"checked_at", event.CheckedAt.UTC().Format(time.RFC3339),
	}
	if details != "" {
		attrs = append(attrs, "details", details)
	}

	switch strings.ToLower(string(event.Severity)) {
	case "critical", "error":
		n.logger.Error(msg, attrs...)
	case "warn", "warning":
		n.logger.Warn(msg, attrs...)
	default:
		n.logger.Info(msg, attrs...)
	}
	return nil
}

func (n *LogNotifier) isDeduped(event engine.AlertEvent) bool {
	if n.dedupeWindow <= 0 {
		return false
	}
	key := dedupeKey(event)
	now := time.Now()

	n.mu.Lock()
	defer n.mu.Unlock()

	if ts, ok := n.last[key]; ok && now.Sub(ts) < n.dedupeWindow {
		return true
	}
	n.last[key] = now
	return false
}

// CommandRunner abstracts command execution (useful for tests).
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) error
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("%s %v failed: %w: %s", name, args, err, msg)
		}
		return fmt.Errorf("%s %v failed: %w", name, args, err)
	}
	return nil
}

// DesktopNotifier sends OS desktop notifications (macOS and Linux).
type DesktopNotifier struct {
	titlePrefix   string
	minSeverity   string
	dedupeWindow  time.Duration
	commandTimout time.Duration
	runner        CommandRunner

	mu   sync.Mutex
	last map[string]time.Time
}

// NewDesktopNotifier creates a desktop notifier.
func NewDesktopNotifier(titlePrefix, minSeverity string, dedupeWindow time.Duration) *DesktopNotifier {
	prefix := strings.TrimSpace(titlePrefix)
	if prefix == "" {
		prefix = "[Smoke Alarm]"
	}
	return &DesktopNotifier{
		titlePrefix:   prefix,
		minSeverity:   strings.ToLower(strings.TrimSpace(minSeverity)),
		dedupeWindow:  dedupeWindow,
		commandTimout: 3 * time.Second,
		runner:        execRunner{},
		last:          make(map[string]time.Time),
	}
}

// Notify sends a desktop notification if supported by OS and filtered policy.
func (n *DesktopNotifier) Notify(ctx context.Context, event engine.AlertEvent) error {
	if !severityAllowed(string(event.Severity), n.minSeverity) {
		return nil
	}
	if n.isDeduped(event) {
		return nil
	}

	title := fmt.Sprintf("%s %s", n.titlePrefix, strings.ToUpper(string(event.Severity)))
	if event.Regression {
		title += " REGRESSION"
	}
	body := notificationBody(event)

	timeout := n.commandTimout
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("osascript"); err != nil {
			return fmt.Errorf("%w: osascript not found", ErrNotifierUnavailable)
		}
		script := fmt.Sprintf(`display notification "%s" with title "%s"`, escapeAppleScript(body), escapeAppleScript(title))
		return n.runner.Run(runCtx, "osascript", "-e", script)

	case "linux":
		if _, err := exec.LookPath("notify-send"); err != nil {
			return fmt.Errorf("%w: notify-send not found", ErrNotifierUnavailable)
		}
		urgency := urgencyFromSeverity(string(event.Severity))
		return n.runner.Run(runCtx, "notify-send", "--urgency="+urgency, title, body)

	default:
		return ErrNotifierUnsupported
	}
}

func (n *DesktopNotifier) isDeduped(event engine.AlertEvent) bool {
	if n.dedupeWindow <= 0 {
		return false
	}
	key := dedupeKey(event)
	now := time.Now()

	n.mu.Lock()
	defer n.mu.Unlock()

	if ts, ok := n.last[key]; ok && now.Sub(ts) < n.dedupeWindow {
		return true
	}
	n.last[key] = now
	return false
}

func notificationBody(event engine.AlertEvent) string {
	msg := sanitize(event.Message)
	if msg == "" {
		msg = "No additional message"
	}
	base := fmt.Sprintf("%s (%s): %s", targetNameOrID(event), strings.ToUpper(string(event.State)), msg)
	if event.Regression {
		base += " [faulty smoke test regression]"
	}
	return base
}

// targetNameOrID returns target name fallback for notifications.
func targetNameOrID(e engine.AlertEvent) string {
	if strings.TrimSpace(e.TargetName) != "" {
		return e.TargetName
	}
	return e.TargetID
}

func urgencyFromSeverity(sev string) string {
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "critical", "error":
		return "critical"
	case "warn", "warning":
		return "normal"
	default:
		return "low"
	}
}

func severityAllowed(current, min string) bool {
	if strings.TrimSpace(min) == "" {
		return true
	}
	return severityRank(current) >= severityRank(min)
}

func severityRank(v string) int {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "critical":
		return 4
	case "error":
		return 3
	case "warn", "warning":
		return 2
	case "info":
		return 1
	default:
		return 0
	}
}

func dedupeKey(event engine.AlertEvent) string {
	return strings.ToLower(strings.Join([]string{
		event.TargetID,
		string(event.State),
		string(event.FailureClass),
		string(event.Severity),
		sanitize(event.Message),
	}, "|"))
}

func sanitize(s string) string {
	v := strings.TrimSpace(s)
	if v == "" {
		return v
	}
	// Basic secret redaction hooks.
	replacer := strings.NewReplacer(
		"Bearer ", "Bearer ****",
		"bearer ", "bearer ****",
		"token=", "token=****",
		"access_token", "access_token_redacted",
		"refresh_token", "refresh_token_redacted",
	)
	return replacer.Replace(v)
}

func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}
