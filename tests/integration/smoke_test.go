package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/config"
	"github.com/james-gibson/smoke-alarm/internal/discovery"
	"github.com/james-gibson/smoke-alarm/internal/engine"
	"github.com/james-gibson/smoke-alarm/internal/health"
	"github.com/james-gibson/smoke-alarm/internal/knownstate"
	"github.com/james-gibson/smoke-alarm/internal/meta"
	"github.com/james-gibson/smoke-alarm/internal/ops"
	"github.com/james-gibson/smoke-alarm/internal/targets"
)

func TestHealthServer_LivenessReadinessAndStatus(t *testing.T) {
	t.Parallel()

	addr := freeAddr(t)
	srv := health.NewServer(health.Options{
		ServiceName: "itest",
		Version:     "test",
		ListenAddr:  addr,
	})

	srv.SetComponent("engine", false, "warming")
	srv.SetReady(false, "engine warming up")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.Start(ctx)
	}()

	base := "http://" + addr
	waitForHTTPStatus(t, base+"/healthz", http.StatusOK, 2*time.Second)
	waitForHTTPStatus(t, base+"/readyz", http.StatusServiceUnavailable, 2*time.Second)

	srv.SetComponent("engine", true, "running")
	srv.SetReady(true, "")

	srv.UpsertTargetStatus(health.TargetStatus{
		ID:         "target-a",
		Protocol:   "mcp",
		Endpoint:   "http://example.local/mcp",
		State:      "healthy",
		Severity:   "info",
		Message:    "ok",
		Regression: false,
		CheckedAt:  time.Now().UTC(),
		LatencyMS:  12,
	})

	waitForHTTPStatus(t, base+"/readyz", http.StatusOK, 2*time.Second)

	var statusResp health.StatusResponse
	getJSONWithRetry(t, base+"/status", &statusResp, 2*time.Second)

	if !statusResp.Live {
		t.Fatalf("expected live=true")
	}
	if !statusResp.Ready {
		t.Fatalf("expected ready=true")
	}
	if len(statusResp.Targets) != 1 {
		t.Fatalf("expected 1 target in status, got %d", len(statusResp.Targets))
	}
	if statusResp.Targets[0].ID != "target-a" {
		t.Fatalf("unexpected target id: %s", statusResp.Targets[0].ID)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("health server error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("health server did not shut down in time")
	}
}

func TestEngine_RegressionAfterHealthy(t *testing.T) {
	t.Parallel()

	var code atomic.Int64
	code.Store(200)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(int(code.Load()))
		_, _ = w.Write([]byte(`ok`))
	}))
	defer srv.Close()

	cfg := baseConfigForEndpoint(t, srv.URL, func(c *config.Config) {
		c.KnownState.Enabled = true
		c.KnownState.Persist = false
		c.KnownState.SustainSuccessBeforeMarkHealthy = 1
		c.KnownState.OutageThresholdConsecutiveFailures = 3
		c.Alerts.Aggressive = true
	})

	store := knownstate.NewStore(
		filepath.Join(t.TempDir(), "known-good.json"),
		knownstate.WithAutoPersist(false),
		knownstate.WithSustainSuccess(cfg.KnownState.SustainSuccessBeforeMarkHealthy),
	)
	eng, err := engine.New(cfg, engine.WithStore(store))
	if err != nil {
		t.Fatalf("engine.New failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- eng.Start(ctx)
	}()

	st := waitForTargetStatus(t, eng, "t1", 3*time.Second, func(s engine.TargetRuntimeStatus) bool {
		return s.State == targets.StateHealthy
	})
	if st.State != targets.StateHealthy {
		t.Fatalf("expected healthy before regression, got %s", st.State)
	}

	code.Store(500)

	st = waitForTargetStatus(t, eng, "t1", 3*time.Second, func(s engine.TargetRuntimeStatus) bool {
		return s.State == targets.StateRegression && s.Regression
	})

	if st.State != targets.StateRegression {
		t.Fatalf("expected regression state, got %s", st.State)
	}
	if !st.Regression {
		t.Fatalf("expected regression=true")
	}
	if st.Severity != targets.SeverityCritical {
		t.Fatalf("expected critical severity, got %s", st.Severity)
	}
	if !strings.Contains(strings.ToLower(st.Message), "regression") {
		t.Fatalf("expected regression message, got %q", st.Message)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("engine returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("engine did not stop after cancel")
	}
}

func TestEngine_AuthFailureClassification(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`ok`))
	}))
	defer srv.Close()

	cfg := baseConfigForEndpoint(t, srv.URL, func(c *config.Config) {
		c.KnownState.Enabled = false
		c.Targets[0].Auth.Type = "bearer"
		c.Targets[0].Auth.Header = "Authorization"
		c.Targets[0].Auth.SecretRef = "env://OCD_SMOKE_ALARM_ITEST_MISSING_SECRET"
	})

	eng, err := engine.New(cfg)
	if err != nil {
		t.Fatalf("engine.New failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- eng.Start(ctx)
	}()

	st := waitForTargetStatus(t, eng, "t1", 3*time.Second, func(s engine.TargetRuntimeStatus) bool {
		return s.FailureClass == targets.FailureAuth
	})

	if st.FailureClass != targets.FailureAuth {
		t.Fatalf("expected auth failure class, got %s", st.FailureClass)
	}
	if st.State != targets.StateUnhealthy {
		t.Fatalf("expected unhealthy state, got %s", st.State)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("engine returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("engine did not stop after cancel")
	}
}

func TestEngine_OutageEscalationForNeverHealthyTarget(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`boom`))
	}))
	defer srv.Close()

	cfg := baseConfigForEndpoint(t, srv.URL, func(c *config.Config) {
		c.KnownState.Enabled = true
		c.KnownState.Persist = false
		c.KnownState.SustainSuccessBeforeMarkHealthy = 1
		c.KnownState.OutageThresholdConsecutiveFailures = 1
		c.Alerts.Aggressive = false
	})

	store := knownstate.NewStore(
		filepath.Join(t.TempDir(), "known-good.json"),
		knownstate.WithAutoPersist(false),
		knownstate.WithSustainSuccess(cfg.KnownState.SustainSuccessBeforeMarkHealthy),
	)
	eng, err := engine.New(cfg, engine.WithStore(store))
	if err != nil {
		t.Fatalf("engine.New failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- eng.Start(ctx)
	}()

	st := waitForTargetStatus(t, eng, "t1", 3*time.Second, func(s engine.TargetRuntimeStatus) bool {
		return s.State == targets.StateOutage
	})

	if st.State != targets.StateOutage {
		t.Fatalf("expected outage state, got %s", st.State)
	}
	if st.Severity != targets.SeverityCritical {
		t.Fatalf("expected critical severity, got %s", st.Severity)
	}
	if st.Regression {
		t.Fatalf("expected regression=false for never healthy target")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("engine returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("engine did not stop after cancel")
	}
}

func TestLifecycleController_ExecuteSuccessAndCommit(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	lockPath := filepath.Join(tmp, "update.lock")
	journalPath := filepath.Join(tmp, "update.journal.log")
	currentPath := filepath.Join(tmp, "version.current")
	previousPath := filepath.Join(tmp, "version.previous")

	if err := os.WriteFile(currentPath, []byte("1.0.0\n"), 0o644); err != nil {
		t.Fatalf("write current version: %v", err)
	}

	healthSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`ok`))
	}))
	defer healthSrv.Close()

	runner := &fakeRunner{}
	controller := ops.NewLifecycleController(ops.Plan{
		LockFilePath:        lockPath,
		JournalPath:         journalPath,
		CurrentVersionPath:  currentPath,
		PreviousVersionPath: previousPath,
		StopCommand:         "stop-cmd",
		StartCommand:        "start-cmd",
		VerifyCommand:       "verify-cmd",
		HealthURL:           healthSrv.URL,
		ReadyURL:            healthSrv.URL,
		RequireReady:        true,
		PollInterval:        20 * time.Millisecond,
		GracefulStopTimeout: 1 * time.Second,
		StartTimeout:        1 * time.Second,
		VerifyTimeout:       1 * time.Second,
	})
	controller.SetRunner(runner)

	res, err := controller.Execute(context.Background(), func(context.Context) (ops.DeployResult, error) {
		return ops.DeployResult{
			NewVersion: "1.1.0",
		}, nil
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !res.Committed {
		t.Fatalf("expected committed=true")
	}
	if res.RolledBack {
		t.Fatalf("expected rolledBack=false")
	}
	if res.NewVersion != "1.1.0" {
		t.Fatalf("unexpected new version: %s", res.NewVersion)
	}

	gotCurrent := mustReadTrimmed(t, currentPath)
	gotPrevious := mustReadTrimmed(t, previousPath)
	if gotCurrent != "1.1.0" {
		t.Fatalf("current version mismatch: %s", gotCurrent)
	}
	if gotPrevious != "1.0.0" {
		t.Fatalf("previous version mismatch: %s", gotPrevious)
	}

	runner.assertContainsInOrder(t, []string{"stop-cmd", "start-cmd", "verify-cmd"})

	journalBytes, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatalf("read journal: %v", err)
	}
	if strings.TrimSpace(string(journalBytes)) == "" {
		t.Fatalf("expected journal entries")
	}
}

func TestLifecycleController_RollbackOnVerifyFailure(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	lockPath := filepath.Join(tmp, "update.lock")
	journalPath := filepath.Join(tmp, "update.journal.log")
	currentPath := filepath.Join(tmp, "version.current")
	previousPath := filepath.Join(tmp, "version.previous")

	if err := os.WriteFile(currentPath, []byte("2.0.0\n"), 0o644); err != nil {
		t.Fatalf("write current version: %v", err)
	}

	runner := &fakeRunner{
		fail: map[string]error{
			"verify-cmd": errors.New("verification failed"),
		},
	}
	controller := ops.NewLifecycleController(ops.Plan{
		LockFilePath:        lockPath,
		JournalPath:         journalPath,
		CurrentVersionPath:  currentPath,
		PreviousVersionPath: previousPath,
		StopCommand:         "stop-cmd",
		StartCommand:        "start-cmd",
		VerifyCommand:       "verify-cmd",
		RequireReady:        false,
		PollInterval:        20 * time.Millisecond,
		GracefulStopTimeout: 1 * time.Second,
		StartTimeout:        1 * time.Second,
		VerifyTimeout:       1 * time.Second,
	})
	controller.SetRunner(runner)

	var rollbackCalled atomic.Bool

	res, err := controller.Execute(context.Background(), func(context.Context) (ops.DeployResult, error) {
		return ops.DeployResult{
			NewVersion: "2.1.0",
			Rollback: func(context.Context) error {
				rollbackCalled.Store(true)
				return nil
			},
		}, nil
	})

	if err == nil {
		t.Fatalf("expected execute to fail on verify")
	}
	if !res.RolledBack {
		t.Fatalf("expected rolledBack=true")
	}
	if !rollbackCalled.Load() {
		t.Fatalf("expected deploy rollback hook to be called")
	}
	if res.Committed {
		t.Fatalf("expected committed=false")
	}
	if res.FailureReason == "" {
		t.Fatalf("expected non-empty failure reason")
	}
}

func TestMetaGenerator_GenerateValidateWriteRoundTrip(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()

	gen := meta.NewGenerator(config.MetaConfigConfig{
		Enabled:           true,
		OutputDir:         tmp,
		Formats:           []string{"yaml", "json"},
		IncludeConfidence: true,
		IncludeProvenance: true,
		Placeholders: config.MetaPlaceholders{
			Token:        "${TOKEN}",
			ClientSecret: "${CLIENT_SECRET}",
			Endpoint:     "${ENDPOINT}",
		},
	})

	recs := []discovery.Record{
		{
			Source:     "env",
			Confidence: 0.87,
			Target: targets.Target{
				ID:        "discovered-acp",
				Enabled:   true,
				Protocol:  targets.ProtocolACP,
				Name:      "Discovered ACP",
				Endpoint:  "wss://agent.example.test/acp",
				Transport: targets.TransportWebSocket,
				Auth: targets.AuthConfig{
					Type:      targets.AuthOAuth,
					ClientID:  "itest-client",
					TokenURL:  "https://auth.example.test/token",
					SecretRef: "",
					Scopes:    []string{"acp.read"},
				},
				Check: targets.CheckPolicy{
					Interval: 30 * time.Second,
					Timeout:  5 * time.Second,
					Retries:  1,
				},
				Expected: targets.ExpectedBehavior{
					HealthyStatusCodes: []int{101},
					MinCapabilities:    []string{"session/setup", "prompt/turn"},
				},
			},
		},
	}

	doc := gen.GenerateFromDiscovery(recs)
	if err := meta.ValidateDocument(doc); err != nil {
		t.Fatalf("ValidateDocument failed: %v", err)
	}

	paths, err := gen.Write(context.Background(), doc)
	if err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("expected 2 output files, got %d", len(paths))
	}

	var jsonPath string
	for _, p := range paths {
		b, readErr := os.ReadFile(p)
		if readErr != nil {
			t.Fatalf("read output file %s: %v", p, readErr)
		}
		if strings.TrimSpace(string(b)) == "" {
			t.Fatalf("output file %s is empty", p)
		}
		if strings.HasSuffix(p, ".json") {
			jsonPath = p
		}
	}

	if jsonPath == "" {
		t.Fatalf("expected one json output file")
	}

	var roundTrip meta.Document
	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read json output: %v", err)
	}
	if err := json.Unmarshal(raw, &roundTrip); err != nil {
		t.Fatalf("json round-trip unmarshal failed: %v", err)
	}
	if len(roundTrip.Entries) != 1 {
		t.Fatalf("expected 1 entry after round trip, got %d", len(roundTrip.Entries))
	}
	if roundTrip.Entries[0].ID != "discovered-acp" {
		t.Fatalf("unexpected entry id: %s", roundTrip.Entries[0].ID)
	}
	if roundTrip.Entries[0].Confidence == nil {
		t.Fatalf("expected confidence annotation")
	}
	if roundTrip.Entries[0].Provenance == "" {
		t.Fatalf("expected provenance annotation")
	}
}

// ---------- helpers ----------

func baseConfigForEndpoint(t *testing.T, endpoint string, mutate func(*config.Config)) config.Config {
	t.Helper()

	tmp := t.TempDir()
	cfg := config.Config{
		Version: "1",
		Service: config.ServiceConfig{
			Name:         "itest-smoke-alarm",
			Environment:  "test",
			Mode:         config.ModeBackground,
			LogLevel:     "info",
			PollInterval: "40ms",
			Timeout:      "300ms",
			MaxWorkers:   2,
		},
		Health: config.HealthConfig{
			Enabled:    false,
			ListenAddr: "disabled",
		},
		Runtime: config.RuntimeConfig{
			LockFile:                filepath.Join(tmp, "runtime.lock"),
			StateDir:                filepath.Join(tmp, "state"),
			BaselineFile:            filepath.Join(tmp, "state", "known-good.json"),
			EventHistorySize:        128,
			GracefulShutdownTimeout: "2s",
		},
		Alerts: config.AlertsConfig{
			Aggressive:                    true,
			NotifyOnRegressionImmediately: true,
			RetryBeforeEscalation:         0,
			DedupeWindow:                  "1s",
			Cooldown:                      "1s",
			Severity: config.SeverityConfig{
				Healthy:    "info",
				Degraded:   "warn",
				Regression: "critical",
				Outage:     "critical",
			},
			Sinks: config.AlertSinkConfig{
				Log:            config.SinkToggleConfig{Enabled: false},
				OSNotification: config.OSNotificationSinkConfig{Enabled: false},
			},
		},
		KnownState: config.KnownStateConfig{
			Enabled:                            true,
			Persist:                            false,
			SustainSuccessBeforeMarkHealthy:    1,
			ClassifyNewFailuresAfterHealthyAs:  "regression",
			OutageThresholdConsecutiveFailures: 2,
		},
		Targets: []config.TargetConfig{
			{
				ID:        "t1",
				Enabled:   true,
				Protocol:  "http",
				Name:      "integration-target",
				Endpoint:  endpoint,
				Transport: "http",
				Expected: config.ExpectedConfig{
					HealthyStatusCodes: []int{200},
				},
				Auth: config.TargetAuthConfig{
					Type: "none",
				},
				Check: config.TargetCheckConfig{
					Interval: "40ms",
					Timeout:  "300ms",
					Retries:  0,
				},
			},
		},
	}

	cfg.ApplyDefaults()
	if mutate != nil {
		mutate(&cfg)
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("config validate failed: %v", err)
	}
	return cfg
}

func waitForTargetStatus(
	t *testing.T,
	eng *engine.Engine,
	targetID string,
	timeout time.Duration,
	predicate func(engine.TargetRuntimeStatus) bool,
) engine.TargetRuntimeStatus {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, st := range eng.SnapshotStatuses() {
			if st.TargetID == targetID && predicate(st) {
				return st
			}
		}
		time.Sleep(20 * time.Millisecond)
	}
	var dump strings.Builder
	for _, st := range eng.SnapshotStatuses() {
		fmt.Fprintf(&dump, "%s=%s(%s) ", st.TargetID, st.State, st.Message)
	}
	t.Fatalf("timeout waiting for status on %s; got: %s", targetID, strings.TrimSpace(dump.String()))
	return engine.TargetRuntimeStatus{}
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to allocate free port: %v", err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

func waitForHTTPStatus(t *testing.T, url string, want int, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 500 * time.Millisecond}

	var lastErr error
	var lastCode int
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err != nil {
			lastErr = err
			time.Sleep(30 * time.Millisecond)
			continue
		}
		lastCode = resp.StatusCode
		_ = resp.Body.Close()
		if resp.StatusCode == want {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}

	if lastErr != nil {
		t.Fatalf("timeout waiting for %s status=%d: last error=%v", url, want, lastErr)
	}
	t.Fatalf("timeout waiting for %s status=%d: last code=%d", url, want, lastCode)
}

func getJSONWithRetry(t *testing.T, url string, dst any, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 500 * time.Millisecond}

	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := client.Get(url)
		if err != nil {
			lastErr = err
			time.Sleep(30 * time.Millisecond)
			continue
		}
		func() {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				lastErr = fmt.Errorf("status %d", resp.StatusCode)
				return
			}
			if decErr := json.NewDecoder(resp.Body).Decode(dst); decErr != nil {
				lastErr = decErr
				return
			}
			lastErr = nil
		}()
		if lastErr == nil {
			return
		}
		time.Sleep(30 * time.Millisecond)
	}
	t.Fatalf("failed to GET+decode %s within timeout: %v", url, lastErr)
}

func mustReadTrimmed(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return strings.TrimSpace(string(b))
}

type fakeRunner struct {
	mu       sync.Mutex
	commands []string
	fail     map[string]error
}

func (r *fakeRunner) Run(_ context.Context, command string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands = append(r.commands, command)
	if r.fail != nil {
		if err, ok := r.fail[command]; ok {
			return "", err
		}
	}
	return "ok", nil
}

func (r *fakeRunner) assertContainsInOrder(t *testing.T, expected []string) {
	t.Helper()

	r.mu.Lock()
	defer r.mu.Unlock()

	if len(expected) == 0 {
		return
	}
	pos := 0
	for _, got := range r.commands {
		if got == expected[pos] {
			pos++
			if pos == len(expected) {
				return
			}
		}
	}
	t.Fatalf("expected command sequence %v in %v", expected, r.commands)
}
