package smokealarm

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/config"
)

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve port: %v", err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port
}

func testConfig(t *testing.T, port int) Config {
	t.Helper()
	cfg, err := ParseConfig([]byte(fmt.Sprintf(`
version: "1"
service:
  name: "ocd-smoke-alarm"
  mode: "background"
  log_level: "warn"
health:
  enabled: true
  listen_addr: "127.0.0.1:%d"
  self_check: false
  endpoints:
    healthz: "/healthz"
    readyz: "/readyz"
    status: "/status"
targets: []
`, port)))
	if err != nil {
		t.Fatalf("ParseConfig: %v", err)
	}
	return cfg
}

func TestInstanceServesHealthAndStatus(t *testing.T) {
	cfg := testConfig(t, freePort(t))

	in, err := New(cfg, Options{Version: "test"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if startErr := in.Start(ctx); startErr != nil {
		t.Fatalf("Start: %v", startErr)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = in.Shutdown(shutdownCtx)
	}()

	if in.Addr() == "" {
		t.Fatal("expected a bound health address after Start")
	}

	client := &http.Client{Timeout: 2 * time.Second}
	var lastErr error
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, reqErr := client.Get(fmt.Sprintf("http://%s/healthz", in.Addr()))
		if reqErr != nil {
			lastErr = reqErr
		} else {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				lastErr = nil
				break
			}
			lastErr = fmt.Errorf("healthz status %d", resp.StatusCode)
		}
		time.Sleep(50 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("healthz never became ready: %v", lastErr)
	}

	resp, err := (&http.Client{Timeout: 2 * time.Second}).Get(in.StatusURL())
	if err != nil {
		t.Fatalf("GET /status: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected /status 200, got %d", resp.StatusCode)
	}
}

func TestInstanceRejectsDoubleStartAndDoubleShutdown(t *testing.T) {
	cfg := testConfig(t, freePort(t))
	in, err := New(cfg, Options{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if startErr := in.Start(ctx); startErr != nil {
		t.Fatalf("Start: %v", startErr)
	}
	if err := in.Start(ctx); err == nil {
		t.Fatal("expected a second Start to be rejected")
	}

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := in.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := in.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("second Shutdown should be a no-op, got %v", err)
	}
}

// discardLogger keeps wiring tests quiet.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// quietConfig is a minimal config with no health listener, for tests that only
// exercise New.
func quietConfig() Config {
	return Config{
		Version: "1",
		Service: config.ServiceConfig{Name: "ocd-smoke-alarm", Mode: "background", LogLevel: "warn"},
		Health:  config.HealthConfig{Enabled: false},
	}
}

// TestNewWiresKnownStateStore guards the regression this issue is about: the
// facade used to call engine.New(cfg) with no options, so the known-state store
// was never injected and known_state.enabled / runtime.baseline_file were
// silently inert. The store writes its baseline file on every update, so an
// appearing file is proof the option was passed.
func TestNewWiresKnownStateStore(t *testing.T) {
	baseline := filepath.Join(t.TempDir(), "known-good.json")

	// Hand-built rather than parsed, so this also covers New applying defaults.
	cfg := quietConfig()
	cfg.Health = config.HealthConfig{
		Enabled:    true,
		ListenAddr: fmt.Sprintf("127.0.0.1:%d", freePort(t)),
	}
	cfg.KnownState = config.KnownStateConfig{Enabled: true, Persist: true}
	cfg.Runtime = config.RuntimeConfig{BaselineFile: baseline}
	cfg.Targets = []config.TargetConfig{{
		ID:        "peer",
		Name:      "Peer",
		Enabled:   true,
		Protocol:  config.ProtocolHTTP,
		Transport: config.TransportHTTP,
		// A closed port: the probe fails fast and deterministically.
		Endpoint: fmt.Sprintf("http://127.0.0.1:%d/status", freePort(t)),
		Check:    config.TargetCheckConfig{Interval: "200ms", Timeout: "100ms", Retries: 1},
	}}

	in, err := New(cfg, Options{Logger: discardLogger()})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := in.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = in.Shutdown(shutdownCtx)
	}()

	deadline := time.Now().Add(10 * time.Second)
	for {
		if raw, readErr := os.ReadFile(baseline); readErr == nil && bytes.Contains(raw, []byte("peer")) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("baseline file was never written: known-state is not wired into the engine")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// TestNewReportsUnsupportedSubsystems covers the "fail loudly, do not ignore
// silently" requirement: an enabled-but-unimplemented subsystem is named, and a
// supported one is not falsely reported.
func TestNewReportsUnsupportedSubsystems(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Config)
		want   []string
	}{
		{
			name:   "nothing enabled",
			mutate: func(*Config) {},
			want:   nil,
		},
		{
			name: "supported switches are not reported",
			mutate: func(c *Config) {
				c.KnownState = config.KnownStateConfig{Enabled: true}
				c.Telemetry = config.TelemetryConfig{Enabled: true, Endpoint: "http://127.0.0.1:4318/v1/metrics"}
				c.Alerts = config.AlertsConfig{Sinks: config.AlertSinkConfig{Log: config.SinkToggleConfig{Enabled: true}}}
				c.Tuner = config.TunerConfig{Enabled: true, Advertise: true}
				c.Health.SelfCheck = true
			},
			want: nil,
		},
		{
			name:   "hosted only",
			mutate: func(c *Config) { c.Hosted = config.HostedConfig{Enabled: true} },
			want:   []string{"hosted validation server (hosted.enabled)"},
		},
		{
			name:   "foreground mode",
			mutate: func(c *Config) { c.Service.Mode = config.ModeForeground },
			want:   []string{"foreground TUI (service.mode)"},
		},
		{
			name: "everything the facade does not run",
			mutate: func(c *Config) {
				c.Hosted = config.HostedConfig{Enabled: true}
				c.Discovery = config.DiscoveryConfig{Enabled: true}
				c.DynamicConfig = config.DynamicConfigConfig{Enabled: true}
				c.Federation = config.FederationConfig{Enabled: true}
				c.Auth = config.GlobalAuthConfig{
					OAuth: config.GlobalOAuthConfig{MockRedirect: config.OAuthMockRedirectConfig{Enabled: true}},
				}
				c.RemoteAgent = config.RemoteAgentConfig{ManagedUpdates: true}
				c.Service.Mode = config.ModeForeground
			},
			want: []string{
				"discovery (discovery.enabled)",
				"dynamic config persistence (dynamic_config.enabled)",
				"federation (federation.enabled)",
				"foreground TUI (service.mode)",
				"hosted validation server (hosted.enabled)",
				"oauth mock redirect server (auth.oauth.mock_redirect.enabled)",
				"remote agent managed updates (remote_agent.managed_updates)",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := quietConfig()
			tc.mutate(&cfg)

			in, err := New(cfg, Options{Logger: discardLogger()})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			got := in.Unsupported()
			if len(got) != len(tc.want) {
				t.Fatalf("Unsupported() = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("Unsupported() = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

// TestNewStrictRejectsUnsupportedSubsystems checks the opt-in escalation from
// warning to error.
func TestNewStrictRejectsUnsupportedSubsystems(t *testing.T) {
	cfg := quietConfig()
	cfg.Hosted = config.HostedConfig{Enabled: true}
	cfg.Federation = config.FederationConfig{Enabled: true}

	if _, err := New(cfg, Options{Logger: discardLogger(), Strict: true}); err == nil {
		t.Fatal("expected Strict to reject a config with unimplemented subsystems")
	} else if !bytes.Contains([]byte(err.Error()), []byte("hosted validation server")) {
		t.Fatalf("error should name the subsystem, got: %v", err)
	}
}
