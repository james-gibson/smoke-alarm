package alerts

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/config"
)

// TestNotifierFromConfig covers the sink selection shared by the CLI and
// pkg/smokealarm. It used to live in cmd/ocd-smoke-alarm, where the embeddable
// facade could not reach it.
func TestNotifierFromConfig(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	logEnabled := config.SinkToggleConfig{Enabled: true}
	desktopEnabled := config.OSNotificationSinkConfig{Enabled: true, TitlePrefix: "[Smoke Alarm]"}

	tests := []struct {
		name   string
		sinks  config.AlertSinkConfig
		logger *slog.Logger
		want   bool // a notifier should be built
	}{
		{name: "no sinks enabled", logger: logger, want: false},
		{name: "log sink", sinks: config.AlertSinkConfig{Log: logEnabled}, logger: logger, want: true},
		{name: "log sink without a logger is inert", sinks: config.AlertSinkConfig{Log: logEnabled}, logger: nil, want: false},
		{name: "desktop sink", sinks: config.AlertSinkConfig{OSNotification: desktopEnabled}, logger: logger, want: true},
		{
			name:   "both sinks",
			sinks:  config.AlertSinkConfig{Log: logEnabled, OSNotification: desktopEnabled},
			logger: logger,
			want:   true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := config.Config{Alerts: config.AlertsConfig{Sinks: tc.sinks}}
			got := NotifierFromConfig(cfg, tc.logger)
			if tc.want && got == nil {
				t.Fatal("expected a notifier, got nil")
			}
			if !tc.want && got != nil {
				t.Fatalf("expected no notifier, got %T", got)
			}
		})
	}
}

func TestDedupeWindow(t *testing.T) {
	tests := []struct {
		raw  string
		want time.Duration
	}{
		{raw: "", want: 2 * time.Minute},
		{raw: "not-a-duration", want: 2 * time.Minute},
		{raw: "-5s", want: 2 * time.Minute},
		{raw: "0s", want: 2 * time.Minute},
		{raw: "30s", want: 30 * time.Second},
		{raw: "90m", want: 90 * time.Minute},
	}

	for _, tc := range tests {
		if got := dedupeWindow(tc.raw); got != tc.want {
			t.Errorf("dedupeWindow(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}
