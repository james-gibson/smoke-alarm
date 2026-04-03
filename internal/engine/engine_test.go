package engine

import (
	contextpkg "context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/config"
	"github.com/james-gibson/smoke-alarm/internal/targets"
)

func TestEngineStateTransitionTriggersAlert(t *testing.T) {
	h := newEngineHarness(t)
	defer h.stop(t)

	h.prober.Push(newCheckResult(targets.StateHealthy, targets.SeverityInfo, false, "initial healthy"))
	waitForStatus(t, h.engine, "t1", time.Second, func(st TargetRuntimeStatus) bool {
		return st.State == targets.StateHealthy
	})

	expectedMessage := "detected regression"
	h.prober.Push(newCheckResult(targets.StateRegression, targets.SeverityCritical, true, expectedMessage))

	ev := h.notifier.WaitForEvent(t, time.Second)
	if ev.TargetID != "t1" {
		t.Fatalf("unexpected alert target id: %s", ev.TargetID)
	}
	if ev.State != targets.StateRegression {
		t.Fatalf("expected regression state, got %s", ev.State)
	}
	if ev.Message != expectedMessage {
		t.Fatalf("expected message %q, got %q", expectedMessage, ev.Message)
	}

	waitForStatus(t, h.engine, "t1", time.Second, func(st TargetRuntimeStatus) bool {
		return st.State == targets.StateRegression
	})
}

func TestEngineEventHistoryRecordsLatest(t *testing.T) {
	h := newEngineHarness(t)
	defer h.stop(t)

	h.engine.mu.Lock()
	h.engine.eventCap = 8
	h.engine.mu.Unlock()

	h.prober.Push(newCheckResult(targets.StateHealthy, targets.SeverityInfo, false, "initial healthy"))
	waitForStatus(t, h.engine, "t1", time.Second, func(st TargetRuntimeStatus) bool {
		return st.State == targets.StateHealthy
	})

	firstMessage := "regression one"
	h.prober.Push(newCheckResult(targets.StateRegression, targets.SeverityCritical, true, firstMessage))
	firstEvent := h.notifier.WaitForEvent(t, time.Second)

	waitForStatus(t, h.engine, "t1", time.Second, func(st TargetRuntimeStatus) bool {
		return st.State == targets.StateRegression
	})

	h.prober.Push(newCheckResult(targets.StateHealthy, targets.SeverityInfo, false, "recovery"))
	waitForStatus(t, h.engine, "t1", time.Second, func(st TargetRuntimeStatus) bool {
		return st.State == targets.StateHealthy
	})

	secondMessage := "regression two"
	h.prober.Push(newCheckResult(targets.StateRegression, targets.SeverityCritical, true, secondMessage))
	secondEvent := h.notifier.WaitForEvent(t, time.Second)

	waitForStatus(t, h.engine, "t1", time.Second, func(st TargetRuntimeStatus) bool {
		return st.State == targets.StateRegression
	})

	h.engine.mu.RLock()
	defer h.engine.mu.RUnlock()

	if len(h.engine.events) == 0 {
		t.Fatalf("expected alert events to be recorded")
	}
	last := h.engine.events[len(h.engine.events)-1]
	if last.Message != secondEvent.Message {
		t.Fatalf("expected last event message %q, got %q", secondEvent.Message, last.Message)
	}

	var foundFirst, foundSecond bool
	for _, ev := range h.engine.events {
		if ev.Message == firstEvent.Message {
			foundFirst = true
		}
		if ev.Message == secondEvent.Message {
			foundSecond = true
		}
	}
	if !foundFirst || !foundSecond {
		t.Fatalf("expected both alert messages to be present in history")
	}
}

type engineHarness struct {
	engine   *Engine
	prober   *queueProber
	notifier *collectorNotifier
	cancel   contextpkg.CancelFunc
	errCh    chan error
}

func newEngineHarness(t *testing.T) *engineHarness {
	t.Helper()

	stateDir := t.TempDir()
	cfg := newTestConfig(t, stateDir)
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("config validation failed: %v", err)
	}

	prober := newQueueProber()
	notifier := newCollectorNotifier()

	eng, err := New(cfg, WithProber(prober), WithNotifier(notifier))
	if err != nil {
		t.Fatalf("engine.New failed: %v", err)
	}

	ctx, cancel := contextpkg.WithCancel(contextpkg.Background())
	errCh := make(chan error, 1)
	go func() {
		if err := eng.Start(ctx); err != nil && !errors.Is(err, contextpkg.Canceled) {
			errCh <- err
		} else {
			errCh <- nil
		}
	}()

	return &engineHarness{
		engine:   eng,
		prober:   prober,
		notifier: notifier,
		cancel:   cancel,
		errCh:    errCh,
	}
}

func (h *engineHarness) stop(t *testing.T) {
	t.Helper()
	h.cancel()
	h.prober.Close()
	select {
	case err := <-h.errCh:
		if err != nil {
			t.Fatalf("engine returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("engine did not stop in time")
	}
}

type queueProber struct {
	ch        chan targets.CheckResult
	closeOnce sync.Once
}

func newQueueProber() *queueProber {
	return &queueProber{
		ch: make(chan targets.CheckResult, 16),
	}
}

func (p *queueProber) Push(res targets.CheckResult) {
	p.ch <- res
}

func (p *queueProber) Close() {
	p.closeOnce.Do(func() { close(p.ch) })
}

func (p *queueProber) Probe(ctx contextpkg.Context, target targets.Target, headers map[string]string) (targets.CheckResult, error) {
	select {
	case res, ok := <-p.ch:
		if !ok {
			return targets.CheckResult{}, contextpkg.Canceled
		}
		res.TargetID = target.ID
		res.Protocol = target.Protocol
		if res.CheckedAt.IsZero() {
			res.CheckedAt = time.Now()
		}
		return res, nil
	case <-ctx.Done():
		return targets.CheckResult{}, ctx.Err()
	}
}

type collectorNotifier struct {
	mu      sync.Mutex
	history []AlertEvent
	ch      chan AlertEvent
}

func newCollectorNotifier() *collectorNotifier {
	return &collectorNotifier{
		history: make([]AlertEvent, 0, 16),
		ch:      make(chan AlertEvent, 16),
	}
}

func (n *collectorNotifier) Notify(_ contextpkg.Context, event AlertEvent) error {
	n.mu.Lock()
	n.history = append(n.history, event)
	n.mu.Unlock()

	select {
	case n.ch <- event:
	default:
	}
	return nil
}

func (n *collectorNotifier) WaitForEvent(t *testing.T, timeout time.Duration) AlertEvent {
	t.Helper()
	select {
	case ev := <-n.ch:
		return ev
	case <-time.After(timeout):
		t.Fatalf("timed out waiting for alert event")
		return AlertEvent{}
	}
}

func newTestConfig(t *testing.T, stateDir string) config.Config {
	t.Helper()

	lockFile := filepath.Join(stateDir, "engine.lock")
	baselineFile := filepath.Join(stateDir, "known-good.json")

	return config.Config{
		Version: "1",
		Service: config.ServiceConfig{
			Name:         "unit-test",
			Mode:         config.ModeHeadless,
			LogLevel:     config.LogInfo,
			PollInterval: "25ms",
			Timeout:      "250ms",
			MaxWorkers:   1,
		},
		Runtime: config.RuntimeConfig{
			LockFile:                lockFile,
			StateDir:                stateDir,
			BaselineFile:            baselineFile,
			EventHistorySize:        64,
			GracefulShutdownTimeout: "200ms",
		},
		Health: config.HealthConfig{
			Enabled: false,
		},
		Discovery: config.DiscoveryConfig{
			Enabled: false,
		},
		Alerts: config.AlertsConfig{
			Aggressive:   true,
			DedupeWindow: "100ms",
			Cooldown:     "100ms",
			Severity: config.SeverityConfig{
				Healthy:    config.SeverityInfo,
				Degraded:   config.SeverityWarn,
				Regression: config.SeverityCritical,
				Outage:     config.SeverityCritical,
			},
			Sinks: config.AlertSinkConfig{
				Log: config.SinkToggleConfig{Enabled: true},
			},
		},
		Auth: config.GlobalAuthConfig{
			Keystore: config.GlobalKeystoreConfig{Enabled: false},
			Redaction: config.RedactionConfig{
				Enabled: true,
				Mask:    "****",
			},
		},
		Targets: []config.TargetConfig{
			{
				ID:        "t1",
				Enabled:   true,
				Protocol:  string(targets.ProtocolMCP),
				Name:      "Test Target",
				Endpoint:  "https://example.com/mcp",
				Transport: string(targets.TransportHTTP),
				Expected: config.ExpectedConfig{
					HealthyStatusCodes: []int{200},
				},
				Auth: config.TargetAuthConfig{
					Type: string(targets.AuthNone),
				},
				Check: config.TargetCheckConfig{
					Interval:        "30ms",
					Timeout:         "150ms",
					Retries:         0,
					RequiredMethods: []string{"initialize"},
				},
			},
		},
		KnownState: config.KnownStateConfig{
			Enabled: false,
		},
		MetaConfig: config.MetaConfigConfig{
			Enabled: false,
		},
		DynamicConfig: config.DynamicConfigConfig{
			Enabled: false,
		},
		Telemetry: config.TelemetryConfig{
			Enabled: false,
		},
		Federation: config.FederationConfig{
			Enabled: false,
		},
		RemoteAgent: config.RemoteAgentConfig{
			ManagedUpdates: false,
		},
		Hosted: config.HostedConfig{
			Enabled: false,
		},
	}
}

func newCheckResult(state targets.HealthState, severity targets.Severity, regression bool, message string) targets.CheckResult {
	result := targets.CheckResult{
		State:      state,
		Severity:   severity,
		Regression: regression,
		Message:    message,
	}
	if state == targets.StateHealthy {
		result.FailureClass = targets.FailureNone
	} else {
		result.FailureClass = targets.FailureProtocol
	}
	return result
}

func waitForStatus(t *testing.T, eng *Engine, targetID string, timeout time.Duration, predicate func(TargetRuntimeStatus) bool) TargetRuntimeStatus {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		statuses := eng.SnapshotStatuses()
		for _, st := range statuses {
			if st.TargetID == targetID && predicate(st) {
				return st
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("condition not met for target %s within %s", targetID, timeout)
	return TargetRuntimeStatus{}
}
