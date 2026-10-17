package alerts

import (
	"log/slog"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/config"
	"github.com/james-gibson/smoke-alarm/internal/engine"
)

// NotifierFromConfig builds the notifier implied by cfg's alert sinks, or nil
// when no sink is enabled.
//
// This is the single definition of "which sinks does this config ask for". It is
// shared by the CLI (cmd/ocd-smoke-alarm) and the embeddable facade
// (pkg/smokealarm) so the two cannot drift — an embedder's `alerts:` block means
// the same thing as the CLI's.
func NotifierFromConfig(cfg config.Config, logger *slog.Logger) engine.Notifier {
	sinks := make([]engine.Notifier, 0, 2)
	dedupe := dedupeWindow(cfg.Alerts.DedupeWindow)

	// A log sink needs somewhere to log to; without a logger the sink is a
	// no-op, so omit it rather than attach a dead notifier.
	if cfg.Alerts.Sinks.Log.Enabled && logger != nil {
		sinks = append(sinks, NewLogNotifier(logger, "info", dedupe))
	}
	if cfg.Alerts.Sinks.OSNotification.Enabled {
		sinks = append(sinks, NewDesktopNotifier(
			cfg.Alerts.Sinks.OSNotification.TitlePrefix,
			"warn",
			dedupe,
		))
	}
	if len(sinks) == 0 {
		return nil
	}
	return NewNotifierGroup(sinks...)
}

// dedupeWindow parses a configured dedupe window, defaulting to two minutes.
func dedupeWindow(raw string) time.Duration {
	const def = 2 * time.Minute
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return def
	}
	return d
}
