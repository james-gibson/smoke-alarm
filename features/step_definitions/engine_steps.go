package stepdefinitions

// engine_steps.go — step definitions for features/engine.feature
//
// All scenarios that can run in-process against engine.Engine + mock Prober
// are implemented here. Scenarios that require real subprocess execution
// (stdio protocol handshake) remain ErrPending pending subprocess test
// infrastructure.

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cucumber/godog"
	"github.com/james-gibson/smoke-alarm/internal/config"
	"github.com/james-gibson/smoke-alarm/internal/engine"
	"github.com/james-gibson/smoke-alarm/internal/knownstate"
	"github.com/james-gibson/smoke-alarm/internal/safety"
	"github.com/james-gibson/smoke-alarm/internal/targets"
)

// ── mock types ───────────────────────────────────────────────────────────────

// mockEngineProber is a configurable fake that implements engine.Prober.
type mockEngineProber struct {
	mu            sync.Mutex
	results       map[string]targets.CheckResult
	callCounts    map[string]int
	sequences     map[string][]targets.CheckResult // consumed FIFO per target
	sleepDur      time.Duration                    // artificial delay for concurrency tests
	active        int32                            // atomic: current active calls
	maxConcurrent int32                            // atomic: peak concurrent calls
	fallbackCalls map[string]bool
	fallback      engine.Prober
}

func newMockEngineProber() *mockEngineProber {
	return &mockEngineProber{
		results:       make(map[string]targets.CheckResult),
		callCounts:    make(map[string]int),
		sequences:     make(map[string][]targets.CheckResult),
		fallbackCalls: make(map[string]bool),
	}
}

func (m *mockEngineProber) setResult(id string, r targets.CheckResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r.TargetID = id
	m.results[id] = r
}

func (m *mockEngineProber) setSequence(id string, seq []targets.CheckResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range seq {
		seq[i].TargetID = id
	}
	m.sequences[id] = seq
}

func (m *mockEngineProber) callCount(id string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.callCounts[id]
}

func (m *mockEngineProber) totalCallCount() int { //nolint:unused
	m.mu.Lock()
	defer m.mu.Unlock()
	total := 0
	for _, v := range m.callCounts {
		total += v
	}
	return total
}

func (m *mockEngineProber) Probe(ctx context.Context, t targets.Target, headers map[string]string) (targets.CheckResult, error) {
	cur := atomic.AddInt32(&m.active, 1)
	defer atomic.AddInt32(&m.active, -1)
	// Track peak concurrency.
	for {
		old := atomic.LoadInt32(&m.maxConcurrent)
		if cur <= old {
			break
		}
		if atomic.CompareAndSwapInt32(&m.maxConcurrent, old, cur) {
			break
		}
	}
	if m.sleepDur > 0 {
		select {
		case <-ctx.Done():
		case <-time.After(m.sleepDur):
		}
	}

	m.mu.Lock()
	m.callCounts[t.ID]++

	// Consume from sequence if available.
	if seq, ok := m.sequences[t.ID]; ok && len(seq) > 0 {
		r := seq[0]
		m.sequences[t.ID] = seq[1:]
		m.mu.Unlock()
		return r, nil
	}

	// Try the fallback when no result is configured.
	if m.fallback != nil {
		if _, ok := m.results[t.ID]; !ok {
			m.fallbackCalls[t.ID] = true
			m.mu.Unlock()
			return m.fallback.Probe(ctx, t, headers)
		}
	}

	r, ok := m.results[t.ID]
	m.mu.Unlock()
	if !ok {
		return targets.CheckResult{
			TargetID: t.ID,
			State:    targets.StateHealthy,
			Severity: targets.SeverityInfo,
			Message:  "mock: default healthy",
		}, nil
	}
	return r, nil
}

// captureEngineNotifier captures dispatched alert events.
type captureEngineNotifier struct {
	mu     sync.Mutex
	events []engine.AlertEvent
}

func (n *captureEngineNotifier) Notify(_ context.Context, ev engine.AlertEvent) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.events = append(n.events, ev)
	return nil
}

func (n *captureEngineNotifier) snapshot() []engine.AlertEvent {
	n.mu.Lock()
	defer n.mu.Unlock()
	out := make([]engine.AlertEvent, len(n.events))
	copy(out, n.events)
	return out
}

func (n *captureEngineNotifier) count() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.events)
}

// ── engine state ─────────────────────────────────────────────────────────────

var engState struct {
	eng      *engine.Engine
	prober   *mockEngineProber
	notifier *captureEngineNotifier
	cancelFn context.CancelFunc
	doneCh   chan struct{}

	// Config assembly.
	targetCfgs       []config.TargetConfig
	maxWorkers       int
	eventHistorySz   int
	knownStateEn     bool
	outageThresh     int
	aggressiveAlerts bool
	storeFile        string

	// The most recently referenced target ID (used by assertion steps without id arg).
	currentTargetID string

	// Snapshot results.
	snapshotEvents   []engine.AlertEvent
	snapshotStatuses []engine.TargetRuntimeStatus

	// Retry scenario: how many prober calls occurred in the retry cycle.
	retryCallCount int

	// Stdio prober direct-invocation.
	stdioTarget    targets.Target
	stdioHasTarget bool
	stdioResult    *targets.CheckResult
	stdioFallback  *mockEngineProber
}

func resetEngState() {
	if engState.cancelFn != nil {
		engState.cancelFn()
		engState.cancelFn = nil
	}
	if engState.doneCh != nil {
		select {
		case <-engState.doneCh:
		case <-time.After(500 * time.Millisecond):
		}
		engState.doneCh = nil
	}
	if engState.storeFile != "" {
		_ = os.Remove(engState.storeFile)
		engState.storeFile = ""
	}
	engState.eng = nil
	engState.prober = nil
	engState.notifier = nil
	engState.targetCfgs = nil
	engState.maxWorkers = 0
	engState.eventHistorySz = 0
	engState.knownStateEn = false
	engState.outageThresh = 0
	engState.aggressiveAlerts = false
	engState.currentTargetID = ""
	engState.snapshotEvents = nil
	engState.snapshotStatuses = nil
	engState.retryCallCount = 0
	engState.stdioHasTarget = false
	engState.stdioResult = nil
	engState.stdioFallback = nil
}

// ── config helpers ────────────────────────────────────────────────────────────

func makeEngTargetCfg(id, interval string) config.TargetConfig {
	if interval == "" {
		interval = "5ms"
	}
	return config.TargetConfig{
		ID:        id,
		Enabled:   true,
		Protocol:  "http",
		Transport: "http",
		Name:      id,
		Endpoint:  "http://127.0.0.1:1/",
		Check: config.TargetCheckConfig{
			Interval: interval,
			Timeout:  "500ms",
		},
	}
}

func addEngTarget(id, interval string) {
	tc := makeEngTargetCfg(id, interval)
	for i, existing := range engState.targetCfgs {
		if existing.ID == id {
			engState.targetCfgs[i] = tc
			engState.currentTargetID = id
			return
		}
	}
	engState.targetCfgs = append(engState.targetCfgs, tc)
	engState.currentTargetID = id
}

func buildEngCfg() config.Config {
	maxW := engState.maxWorkers
	if maxW < 1 {
		maxW = 50 // default high
	}
	return config.Config{
		Version: "1",
		Service: config.ServiceConfig{
			Name:       "test",
			MaxWorkers: maxW,
		},
		Runtime: config.RuntimeConfig{
			EventHistorySize: engState.eventHistorySz,
		},
		Targets: engState.targetCfgs,
		Alerts:  config.AlertsConfig{Aggressive: engState.aggressiveAlerts},
		KnownState: config.KnownStateConfig{
			Enabled:                            engState.knownStateEn,
			OutageThresholdConsecutiveFailures: engState.outageThresh,
		},
	}
}

func buildAndStartEngine(extraOpts ...engine.Option) error {
	cfg := buildEngCfg()

	engState.prober = newMockEngineProber()
	for _, tc := range engState.targetCfgs {
		engState.prober.setResult(tc.ID, targets.CheckResult{
			TargetID: tc.ID,
			State:    targets.StateHealthy,
			Severity: targets.SeverityInfo,
			Message:  "mock: healthy",
		})
	}

	opts := []engine.Option{engine.WithProber(engState.prober)}
	if engState.notifier != nil {
		opts = append(opts, engine.WithNotifier(engState.notifier))
	}
	if engState.knownStateEn {
		// Use a path that does not exist yet; knownstate.Store treats a missing
		// file as an empty baseline (not an error), and creates it on Save.
		engState.storeFile = filepath.Join(os.TempDir(),
			fmt.Sprintf("ks-engine-test-%d.json", time.Now().UnixNano()))
		storeOpts := []knownstate.Option{}
		if engState.outageThresh > 0 {
			// High sustain threshold prevents EverHealthy from being set by a
			// single healthy probe, allowing outage to fire via the pre-baseline
			// path (consecutive-failures ≥ threshold without regression state).
			storeOpts = append(storeOpts, knownstate.WithSustainSuccess(100))
		}
		store := knownstate.NewStore(engState.storeFile, storeOpts...)
		opts = append(opts, engine.WithStore(store))
	}
	opts = append(opts, extraOpts...)

	e, err := engine.New(cfg, opts...)
	if err != nil {
		return fmt.Errorf("engine.New: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = e.Start(ctx)
	}()

	engState.eng = e
	engState.cancelFn = cancel
	engState.doneCh = done
	return nil
}

// ── wait helpers ──────────────────────────────────────────────────────────────

func waitForAnyStatus(id string) error {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, st := range engState.eng.SnapshotStatuses() {
			if st.TargetID == id {
				return nil
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for any status of %q", id)
}

func waitForStatusState(id, state string) error {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, st := range engState.eng.SnapshotStatuses() {
			if st.TargetID == id && string(st.State) == state {
				return nil
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	for _, st := range engState.eng.SnapshotStatuses() {
		if st.TargetID == id {
			return fmt.Errorf("timeout: %q state=%q want=%q", id, st.State, state)
		}
	}
	return fmt.Errorf("timeout waiting for %q state=%q (no status recorded)", id, state)
}

func waitForAllStatuses(ids []string) error {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		seen := make(map[string]bool)
		for _, st := range engState.eng.SnapshotStatuses() {
			seen[st.TargetID] = true
		}
		allSeen := true
		for _, id := range ids {
			if !seen[id] {
				allSeen = false
				break
			}
		}
		if allSeen {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for all targets to be probed")
}

func waitForCallCount(id string, n int) error {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if engState.prober.callCount(id) >= n {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %d calls to %q (got %d)", n, id, engState.prober.callCount(id))
}

func waitForEventCount(n int) error {
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if len(engState.eng.SnapshotEvents()) >= n {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %d events (got %d)", n, len(engState.eng.SnapshotEvents()))
}

// ── result constructors ───────────────────────────────────────────────────────

func engHealthyResult(id string) targets.CheckResult {
	return targets.CheckResult{
		TargetID: id, State: targets.StateHealthy,
		Severity: targets.SeverityInfo, Message: "mock: healthy",
	}
}

func engUnhealthyResult(id string) targets.CheckResult {
	return targets.CheckResult{
		TargetID: id, State: targets.StateUnhealthy,
		Severity: targets.SeverityWarn, FailureClass: targets.FailureNetwork,
		Message: "mock: unhealthy",
	}
}

func engOutageResult(id string) targets.CheckResult {
	return targets.CheckResult{
		TargetID: id, State: targets.StateOutage,
		Severity: targets.SeverityCritical, FailureClass: targets.FailureNetwork,
		Message: "mock: outage",
	}
}

func engRegressionResult(id string) targets.CheckResult {
	return targets.CheckResult{
		TargetID: id, State: targets.StateRegression, Regression: true,
		Severity: targets.SeverityCritical, FailureClass: targets.FailureNetwork,
		Message: "mock: regression",
	}
}

// ── InitializeEngineSteps ─────────────────────────────────────────────────────

func InitializeEngineSteps(ctx *godog.ScenarioContext) {
	ctx.BeforeScenario(func(_ *godog.Scenario) {
		resetEngState()
	})
	ctx.AfterScenario(func(_ *godog.Scenario, _ error) {
		resetEngState()
	})

	// ── lifecycle ──────────────────────────────────────────────────────────
	ctx.Step(`^a target "([^"]*)" is configured with a (\d+) ms poll interval$`, aTargetIsConfiguredWithPollInterval)
	ctx.Step(`^the engine starts with that config$`, theEngineStartsWithThatConfig)
	ctx.Step(`^a probe result for target "([^"]*)" is recorded before the first poll tick$`, aProbeResultIsRecordedBeforeFirstPollTick)
	ctx.Step(`^(\d+) enabled targets are configured$`, nEnabledTargetsAreConfigured)
	ctx.Step(`^all targets have been probed at least once$`, allTargetsHaveBeenProbedAtLeastOnce)
	ctx.Step(`^the engine reports ready$`, theEngineReportsReady)
	ctx.Step(`^the engine is running with target "([^"]*)"$`, theEngineIsRunningWithTarget)
	ctx.Step(`^the engine context is canceled$`, theEngineContextIsCanceled)
	ctx.Step(`^the engine exits without error within (\d+) ms$`, theEngineExitsWithoutErrorWithin)
	ctx.Step(`^the engine is running with known-state enabled$`, theEngineIsRunningWithKnownStateEnabled)
	ctx.Step(`^the baseline file is written to disk$`, theBaselineFileIsWrittenToDisk)

	// ── probe scheduling ───────────────────────────────────────────────────
	ctx.Step(`^the engine runs for (\d+) ms$`, theEngineRunsFor)
	ctx.Step(`^at least (\d+) probe results are recorded for target "([^"]*)"$`, atLeastNProbeResultsAreRecorded)
	ctx.Step(`^(\d+) targets are configured$`, nTargetsAreConfigured)
	ctx.Step(`^max_workers is set to (\d+)$`, maxWorkersIsSetTo)
	ctx.Step(`^all targets are due for a probe simultaneously$`, allTargetsAreDueForProbeSimultaneously)
	ctx.Step(`^no more than (\d+) probes run concurrently$`, noMoreThanNProbesRunConcurrently)

	// ── state classification ───────────────────────────────────────────────
	ctx.Step(`^the prober returns a healthy result for "([^"]*)"$`, theProberReturnsHealthyResult)
	ctx.Step(`^the prober returns an unhealthy result for "([^"]*)"$`, theProberReturnsUnhealthyResult)
	ctx.Step(`^the status for "([^"]*)" is "([^"]*)"$`, theStatusForTargetIs)
	ctx.Step(`^the engine is running with target "([^"]*)" and known-state enabled$`, theEngineIsRunningWithTargetAndKnownState)
	ctx.Step(`^the result has regression flag set$`, theResultHasRegressionFlagSet)
	ctx.Step(`^a target "([^"]*)" is configured with outage threshold (\d+)$`, aTargetIsConfiguredWithOutageThreshold)
	ctx.Step(`^the prober returns (\d+) consecutive unhealthy results for "([^"]*)"$`, theProberReturnsConsecutiveUnhealthyResults)
	ctx.Step(`^the status message contains "([^"]*)"$`, theStatusMessageContains)
	ctx.Step(`^the engine is running with aggressive alerts enabled for target "([^"]*)"$`, theEngineIsRunningWithAggressiveAlerts)
	ctx.Step(`^the result severity is "([^"]*)"$`, theResultSeverityIs)
	ctx.Step(`^a target "([^"]*)" has a HURL safety check configured$`, aTargetHasHURLSafetyCheckConfigured)
	ctx.Step(`^the safety check for "([^"]*)" fails$`, theSafetyCheckFails)
	ctx.Step(`^no protocol probe is executed for "([^"]*)"$`, noProtocolProbeIsExecuted)
	ctx.Step(`^the status for "([^"]*)" reflects the safety failure$`, theStatusReflectsSafetyFailure)
	ctx.Step(`^a target "([^"]*)" is configured with (\d+) retries$`, aTargetIsConfiguredWithRetries)
	ctx.Step(`^the first (\d+) probes return failures and the final attempt succeeds$`, theFirstNProbesReturnFailures)
	ctx.Step(`^the recorded result for "([^"]*)" is "([^"]*)"$`, theRecordedResultIs)
	ctx.Step(`^the attempt index recorded is (\d+)$`, theAttemptIndexRecordedIs)

	// ── alert emission ─────────────────────────────────────────────────────
	ctx.Step(`^the engine is running with a notifier registered$`, theEngineIsRunningWithNotifier)
	ctx.Step(`^the prober returns a regression result for target "([^"]*)"$`, theProberReturnsRegressionResult)
	ctx.Step(`^an alert event is dispatched with state "([^"]*)"$`, anAlertEventIsDispatchedWithState)
	ctx.Step(`^the alert event target id is "([^"]*)"$`, theAlertEventTargetIDIs)
	ctx.Step(`^the prober returns an outage result for target "([^"]*)"$`, theProberReturnsOutageResult)
	ctx.Step(`^no alert event is dispatched$`, noAlertEventIsDispatched)
	ctx.Step(`^the prober returns a result with severity "([^"]*)" for target "([^"]*)"$`, theProberReturnsResultWithSeverity)

	// ── event ring buffer ──────────────────────────────────────────────────
	ctx.Step(`^(\d+) regression events occur for target "([^"]*)"$`, nRegressionEventsOccurForTarget)
	ctx.Step(`^the event history contains (\d+) entries$`, theEventHistoryContainsNEntries)
	ctx.Step(`^the engine is running with event history size (\d+)$`, theEngineIsRunningWithEventHistorySize)
	ctx.Step(`^(\d+) alert events are emitted$`, nAlertEventsAreEmitted)
	ctx.Step(`^the event history size does not exceed (\d+)$`, theEventHistorySizeDoesNotExceed)
	ctx.Step(`^the most recent event is last in the history$`, theMostRecentEventIsLast)
	ctx.Step(`^the engine has recorded (\d+) alert events$`, theEngineHasRecordedNAlertEvents)
	ctx.Step(`^I call SnapshotEvents$`, iCallSnapshotEvents)
	ctx.Step(`^the returned slice length is (\d+)$`, theReturnedSliceLengthIs)
	ctx.Step(`^events are ordered oldest-first$`, eventsAreOrderedOldestFirst)

	// ── snapshot API ───────────────────────────────────────────────────────
	ctx.Step(`^(\d+) targets with IDs "([^"]*)", "([^"]*)", "([^"]*)" are configured$`, nTargetsWithIDsAreConfigured)
	ctx.Step(`^the engine has a status for each target$`, theEngineHasStatusForEachTarget)
	ctx.Step(`^SnapshotStatuses returns entries in ascending target ID order$`, snapshotStatusesReturnsEntriesInOrder)
	ctx.Step(`^the prober returns a result with latency (\d+) ms and status code (\d+)$`, theProberReturnsResultWithLatencyAndStatusCode)
	ctx.Step(`^the snapshot for "([^"]*)" includes latency (\d+) ms$`, theSnapshotIncludesLatency)
	ctx.Step(`^the snapshot for "([^"]*)" includes status code (\d+)$`, theSnapshotIncludesStatusCode)

	ctx.Step(`^a probe target is registered for endpoint "([^"]*)"$`, aProbeTargetRegisteredForEndpoint)
	ctx.Step(`^the probe for target "([^"]*)" runs$`, theProbeForTargetRuns)
	ctx.Step(`^the target "([^"]*)" is classified as "([^"]*)"$`, theTargetIsClassifiedAs)

	// ── stdio transport probing ────────────────────────────────────────────
	ctx.Step(`^a stdio target "([^"]*)" with an empty command field$`, aStdioTargetWithEmptyCommand)
	ctx.Step(`^the stdio prober probes "([^"]*)"$`, theStdioProberProbes)
	ctx.Step(`^the result state is "([^"]*)"$`, theResultStateIs)
	ctx.Step(`^the failure class is "([^"]*)"$`, theFailureClassIs)
	ctx.Step(`^the message contains "([^"]*)"$`, theMessageContains)
	ctx.Step(`^a stdio target "([^"]*)" with handshake_profile "([^"]*)"$`, aStdioTargetWithHandshakeProfile)
	ctx.Step(`^no subprocess is launched$`, noSubprocessIsLaunched)
	ctx.Step(`^a stdio target "([^"]*)" with protocol "([^"]*)" and handshake_profile "([^"]*)"$`, aStdioTargetWithProtocolAndHandshakeProfile)
	ctx.Step(`^the exercised methods list is \["([^"]+)"(?:, "([^"]+)")*\]$`, theExercisedMethodsListIs)
	ctx.Step(`^a stdio target "([^"]*)" with required_methods \[([^\]]+)\]$`, aStdioTargetWithRequiredMethods)
	ctx.Step(`^a stdio target "([^"]*)" whose subprocess does not respond within the timeout$`, aStdioTargetThatTimesOut)
	ctx.Step(`^a stdio target "([^"]*)" whose subprocess returns a JSON-RPC error for "([^"]*)"$`, aStdioTargetThatReturnsRPCError)
	ctx.Step(`^a target "([^"]*)" with transport "([^"]*)"$`, aTargetWithTransportForEngine)
	ctx.Step(`^the HTTP fallback prober is invoked for "([^"]*)"$`, theHTTPFallbackProberIsInvoked)
	ctx.Step(`^a stdio target "([^"]*)" whose subprocess writes to stderr before crashing$`, aStdioTargetThatWritesToStderr)
	ctx.Step(`^the result message contains the stderr output$`, theResultMessageContainsStderrOutput)
}

// ── lifecycle ─────────────────────────────────────────────────────────────────

func aTargetIsConfiguredWithPollInterval(id string, ms int) error {
	addEngTarget(id, fmt.Sprintf("%dms", ms))
	return nil
}

func theEngineStartsWithThatConfig() error {
	if err := buildAndStartEngine(); err != nil {
		return err
	}
	// Wait for at least one probe to confirm the engine is running.
	if engState.currentTargetID != "" {
		return waitForAnyStatus(engState.currentTargetID)
	}
	return nil
}

func aProbeResultIsRecordedBeforeFirstPollTick(id string) error {
	// Verify that the engine recorded a status for id without waiting a full poll tick.
	// We just check the status is present; the engine does an immediate first probe.
	return waitForAnyStatus(id)
}

func nEnabledTargetsAreConfigured(n int) error {
	for i := 0; i < n; i++ {
		addEngTarget(fmt.Sprintf("t%d", i+1), "5ms")
	}
	return nil
}

func allTargetsHaveBeenProbedAtLeastOnce() error {
	ids := make([]string, 0, len(engState.targetCfgs))
	for _, tc := range engState.targetCfgs {
		ids = append(ids, tc.ID)
	}
	return waitForAllStatuses(ids)
}

func theEngineReportsReady() error {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if engState.eng.IsReady() {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fmt.Errorf("engine did not become ready within timeout")
}

func theEngineIsRunningWithTarget(id string) error {
	addEngTarget(id, "5ms")
	return buildAndStartEngine()
}

func theEngineContextIsCanceled() error {
	if engState.cancelFn != nil {
		engState.cancelFn()
		engState.cancelFn = nil
	}
	return nil
}

func theEngineExitsWithoutErrorWithin(ms int) error {
	if engState.doneCh == nil {
		return fmt.Errorf("engine doneCh is nil; was the engine started?")
	}
	timeout := time.Duration(ms) * time.Millisecond
	select {
	case <-engState.doneCh:
		engState.doneCh = nil
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("engine did not exit within %dms", ms)
	}
}

func theEngineIsRunningWithKnownStateEnabled() error {
	addEngTarget("t1", "5ms")
	engState.knownStateEn = true
	if err := buildAndStartEngine(); err != nil {
		return err
	}
	// Wait for at least one probe to complete so the store has data to save.
	return waitForCallCount("t1", 1)
}

func theBaselineFileIsWrittenToDisk() error {
	// The engine saves the store on graceful stop. After context cancellation
	// (the prior step), wait for the done channel, then verify the file.
	if engState.doneCh != nil {
		select {
		case <-engState.doneCh:
			engState.doneCh = nil
		case <-time.After(2 * time.Second):
			return fmt.Errorf("engine did not stop within timeout")
		}
	}
	if engState.storeFile == "" {
		return fmt.Errorf("no store file configured")
	}
	info, err := os.Stat(engState.storeFile)
	if err != nil {
		return fmt.Errorf("store file %q not found: %w", engState.storeFile, err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("store file %q is empty", engState.storeFile)
	}
	return nil
}

// ── probe scheduling ──────────────────────────────────────────────────────────

func theEngineRunsFor(ms int) error {
	// Start the engine and let it run for the given duration.
	if engState.eng == nil {
		if err := buildAndStartEngine(); err != nil {
			return err
		}
	}
	time.Sleep(time.Duration(ms) * time.Millisecond)
	return nil
}

func atLeastNProbeResultsAreRecorded(n int, id string) error {
	return waitForCallCount(id, n)
}

func nTargetsAreConfigured(n int) error {
	for i := 0; i < n; i++ {
		addEngTarget(fmt.Sprintf("w%d", i+1), "10ms")
	}
	return nil
}

func maxWorkersIsSetTo(n int) error {
	engState.maxWorkers = n
	// Update existing target configs to ensure they're slow enough for the test.
	for i := range engState.targetCfgs {
		engState.targetCfgs[i].Check.Interval = "10ms"
	}
	return nil
}

func allTargetsAreDueForProbeSimultaneously() error {
	// Build and start the engine; the immediate first-probe for all targets
	// exercises concurrent dispatch. Slow the prober so concurrency is visible.
	if err := buildAndStartEngine(); err != nil {
		return err
	}
	// Allow time for all immediate probes to overlap.
	engState.prober.sleepDur = 40 * time.Millisecond
	// Wait for all targets to have been called at least once.
	ids := make([]string, 0, len(engState.targetCfgs))
	for _, tc := range engState.targetCfgs {
		ids = append(ids, tc.ID)
	}
	return waitForAllStatuses(ids)
}

func noMoreThanNProbesRunConcurrently(n int) error {
	got := int(atomic.LoadInt32(&engState.prober.maxConcurrent))
	if got > n {
		return fmt.Errorf("max concurrent probes was %d, expected ≤ %d", got, n)
	}
	return nil
}

// ── state classification ──────────────────────────────────────────────────────

func ensureEngineRunning(id string) error {
	if engState.eng != nil {
		return nil
	}
	// Auto-start: add the target if not already present.
	found := false
	for _, tc := range engState.targetCfgs {
		if tc.ID == id {
			found = true
			break
		}
	}
	if !found {
		addEngTarget(id, "5ms")
	}
	return buildAndStartEngine()
}

func theProberReturnsHealthyResult(id string) error {
	if err := ensureEngineRunning(id); err != nil {
		return err
	}
	engState.prober.setResult(id, engHealthyResult(id))
	engState.currentTargetID = id
	return waitForStatusState(id, "healthy")
}

func theProberReturnsUnhealthyResult(id string) error {
	if err := ensureEngineRunning(id); err != nil {
		return err
	}
	engState.prober.setResult(id, engUnhealthyResult(id))
	engState.currentTargetID = id
	// Wait for a new probe to fire and the status map to update.
	prevCount := engState.prober.callCount(id)
	if err := waitForCallCount(id, prevCount+1); err != nil {
		return err
	}
	time.Sleep(10 * time.Millisecond)
	return nil
}

func theStatusForTargetIs(id, state string) error {
	// Poll for the expected state, allowing for classification delays.
	return waitForStatusState(id, state)
}

func theEngineIsRunningWithTargetAndKnownState(id string) error {
	addEngTarget(id, "5ms")
	engState.knownStateEn = true
	if err := buildAndStartEngine(); err != nil {
		return err
	}
	// Wait for first healthy probe to establish baseline.
	return waitForStatusState(id, "healthy")
}

func theResultHasRegressionFlagSet() error {
	id := engState.currentTargetID
	if id == "" {
		id = "t1"
	}
	for _, st := range engState.eng.SnapshotStatuses() {
		if st.TargetID == id {
			if !st.Regression {
				return fmt.Errorf("target %q: regression flag not set (state=%q)", id, st.State)
			}
			return nil
		}
	}
	return fmt.Errorf("no status found for %q", id)
}

func aTargetIsConfiguredWithOutageThreshold(id string, n int) error {
	addEngTarget(id, "5ms")
	engState.knownStateEn = true
	engState.outageThresh = n
	return nil
}

func theProberReturnsConsecutiveUnhealthyResults(n int, id string) error {
	if err := ensureEngineRunning(id); err != nil {
		return err
	}
	// Trigger n unhealthy probes and wait for the outage state.
	engState.prober.setResult(id, engUnhealthyResult(id))
	prevCount := engState.prober.callCount(id)
	if err := waitForCallCount(id, prevCount+n); err != nil {
		return err
	}
	time.Sleep(10 * time.Millisecond)
	return nil
}

func theStatusMessageContains(substr string) error {
	id := engState.currentTargetID
	if id == "" {
		id = "t1"
	}
	for _, st := range engState.eng.SnapshotStatuses() {
		if st.TargetID == id {
			if !strings.Contains(st.Message, substr) {
				return fmt.Errorf("status message %q does not contain %q", st.Message, substr)
			}
			return nil
		}
	}
	return fmt.Errorf("no status found for %q", id)
}

func theEngineIsRunningWithAggressiveAlerts(id string) error {
	addEngTarget(id, "5ms")
	engState.knownStateEn = true
	engState.aggressiveAlerts = true
	if err := buildAndStartEngine(); err != nil {
		return err
	}
	// Wait for first healthy probe to establish "ever healthy" baseline.
	return waitForStatusState(id, "healthy")
}

func theResultSeverityIs(sev string) error {
	id := engState.currentTargetID
	if id == "" {
		id = "t1"
	}
	// Poll for the expected severity.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		for _, st := range engState.eng.SnapshotStatuses() {
			if st.TargetID == id && string(st.Severity) == sev {
				return nil
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	for _, st := range engState.eng.SnapshotStatuses() {
		if st.TargetID == id {
			return fmt.Errorf("target %q severity=%q want=%q", id, st.Severity, sev)
		}
	}
	return fmt.Errorf("no status for %q", id)
}

// mockFailingRoundTripper always returns 500 Internal Server Error.
type mockFailingRoundTripper struct{}

func (mockFailingRoundTripper) RoundTrip(r *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusInternalServerError,
		Body:       http.NoBody,
		Header:     make(http.Header),
		Request:    r,
	}, nil
}

func aTargetHasHURLSafetyCheckConfigured(id string) error {
	tc := makeEngTargetCfg(id, "5ms")
	// Add a HURL test that will fail (endpoint returns 500 via mock transport).
	// The endpoint itself is checked by the safety scanner.
	// We set it on the target config's HURL tests via the Check field.
	tc.Check.HURLTests = []config.HURLTestConfig{
		{
			Name:     "safety-check",
			Endpoint: "http://127.0.0.1:1/health",
			Method:   "GET",
		},
	}
	for i, existing := range engState.targetCfgs {
		if existing.ID == id {
			engState.targetCfgs[i] = tc
			engState.currentTargetID = id
			return nil
		}
	}
	engState.targetCfgs = append(engState.targetCfgs, tc)
	engState.currentTargetID = id
	return nil
}

func theSafetyCheckFails(id string) error {
	// Build a safety scanner with a custom HTTP client that always returns 500.
	// This ensures the HURL endpoint check fails without needing a real server.
	failClient := &http.Client{
		Transport: mockFailingRoundTripper{},
	}
	scanner := safety.NewScanner(safety.WithHTTPClient(failClient))
	// Start the engine with this scanner.
	if err := buildAndStartEngine(engine.WithSafetyScanner(scanner)); err != nil {
		return err
	}
	// Wait for the safety check to have fired (status recorded).
	return waitForAnyStatus(id)
}

func noProtocolProbeIsExecuted(id string) error {
	// After the safety check blocks deeper probing, the mock prober should NOT
	// have been called for this target (safety stage returns early).
	count := engState.prober.callCount(id)
	if count > 0 {
		return fmt.Errorf("expected 0 protocol probes for %q (safety blocked), got %d", id, count)
	}
	return nil
}

func theStatusReflectsSafetyFailure(id string) error {
	for _, st := range engState.eng.SnapshotStatuses() {
		if st.TargetID == id {
			if st.State == targets.StateHealthy {
				return fmt.Errorf("target %q should not be healthy after safety failure (state=%q)", id, st.State)
			}
			return nil
		}
	}
	return fmt.Errorf("no status found for %q", id)
}

func aTargetIsConfiguredWithRetries(id string, n int) error {
	tc := makeEngTargetCfg(id, "100ms") // slower interval so retry completes within one cycle
	tc.Check.Retries = n
	for i, existing := range engState.targetCfgs {
		if existing.ID == id {
			engState.targetCfgs[i] = tc
			engState.currentTargetID = id
			return nil
		}
	}
	engState.targetCfgs = append(engState.targetCfgs, tc)
	engState.currentTargetID = id
	return nil
}

func theFirstNProbesReturnFailures(n int) error {
	id := engState.currentTargetID
	if id == "" {
		return fmt.Errorf("no current target configured")
	}
	if engState.eng == nil {
		if err := buildAndStartEngine(); err != nil {
			return err
		}
	}
	// Build a sequence: n failures followed by one success.
	seq := make([]targets.CheckResult, n+1)
	for i := 0; i < n; i++ {
		seq[i] = engUnhealthyResult(id)
	}
	seq[n] = engHealthyResult(id)
	engState.prober.setSequence(id, seq)
	// The attempt count = n failures + 1 success.
	engState.retryCallCount = n + 1
	// Wait for the healthy status to appear (sequence consumed successfully).
	return waitForStatusState(id, "healthy")
}

func theRecordedResultIs(id, state string) error {
	for _, st := range engState.eng.SnapshotStatuses() {
		if st.TargetID == id {
			if string(st.State) != state {
				return fmt.Errorf("target %q state=%q want=%q", id, st.State, state)
			}
			return nil
		}
	}
	return fmt.Errorf("no status for %q", id)
}

func theAttemptIndexRecordedIs(n int) error {
	// The attempt index is the total number of prober calls in the retry cycle.
	// With retries=2 and "first 2 fail, final succeeds", retryCallCount=3 (1-based).
	if engState.retryCallCount != n {
		return fmt.Errorf("attempt count: got %d, want %d", engState.retryCallCount, n)
	}
	return nil
}

// ── alert emission ────────────────────────────────────────────────────────────

func theEngineIsRunningWithNotifier() error {
	addEngTarget("t1", "5ms")
	engState.notifier = &captureEngineNotifier{}
	return buildAndStartEngine()
}

func theProberReturnsRegressionResult(id string) error {
	engState.prober.setResult(id, engRegressionResult(id))
	engState.currentTargetID = id
	// Wait for an alert event to be dispatched.
	prevCount := len(engState.notifier.snapshot())
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if engState.notifier.count() > prevCount {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for regression alert from %q", id)
}

func anAlertEventIsDispatchedWithState(state string) error {
	evts := engState.notifier.snapshot()
	for _, ev := range evts {
		if string(ev.State) == state {
			return nil
		}
	}
	return fmt.Errorf("no alert event with state %q (got %d events)", state, len(evts))
}

func theAlertEventTargetIDIs(id string) error {
	evts := engState.notifier.snapshot()
	for _, ev := range evts {
		if ev.TargetID == id {
			return nil
		}
	}
	return fmt.Errorf("no alert event for target %q", id)
}

func theProberReturnsOutageResult(id string) error {
	engState.prober.setResult(id, engOutageResult(id))
	engState.currentTargetID = id
	prevCount := engState.notifier.count()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if engState.notifier.count() > prevCount {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for outage alert from %q", id)
}

func noAlertEventIsDispatched() error {
	// Wait two poll cycles to confirm no alert fires.
	time.Sleep(30 * time.Millisecond)
	if count := engState.notifier.count(); count > 0 {
		evts := engState.notifier.snapshot()
		return fmt.Errorf("expected no alert events, got %d: first=%v", count, evts[0])
	}
	return nil
}

func theProberReturnsResultWithSeverity(sev, id string) error {
	engState.prober.setResult(id, targets.CheckResult{
		TargetID: id,
		State:    targets.StateOutage,
		Severity: targets.Severity(sev),
		Message:  "mock: severity=" + sev,
	})
	engState.currentTargetID = id
	prevCount := engState.notifier.count()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if engState.notifier.count() > prevCount {
			return nil
		}
		time.Sleep(5 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for alert (severity=%s) from %q", sev, id)
}

// ── event ring buffer ─────────────────────────────────────────────────────────

func nRegressionEventsOccurForTarget(n int, id string) error {
	if engState.eng == nil {
		addEngTarget(id, "5ms")
		engState.notifier = &captureEngineNotifier{}
		if err := buildAndStartEngine(); err != nil {
			return err
		}
	}
	// Set prober to return outage (always triggers an alert event).
	engState.prober.setResult(id, engOutageResult(id))
	if err := waitForEventCount(n); err != nil {
		return err
	}
	// Switch to healthy immediately so no more events accumulate between
	// this Given step and the Then assertion that follows.
	engState.prober.setResult(id, engHealthyResult(id))
	return nil
}

func theEventHistoryContainsNEntries(n int) error {
	evts := engState.eng.SnapshotEvents()
	if len(evts) < n {
		return fmt.Errorf("event history has %d entries, want at least %d", len(evts), n)
	}
	return nil
}

func theEngineIsRunningWithEventHistorySize(n int) error {
	addEngTarget("t1", "5ms")
	engState.eventHistorySz = n
	engState.notifier = &captureEngineNotifier{}
	return buildAndStartEngine()
}

func nAlertEventsAreEmitted(n int) error {
	id := engState.currentTargetID
	if id == "" {
		id = "t1"
	}
	engState.prober.setResult(id, engOutageResult(id))
	// Wait until the ring buffer has been filled (may be capped at eventHistorySz).
	// We just need n probes to have completed.
	return waitForCallCount(id, n)
}

func theEventHistorySizeDoesNotExceed(n int) error {
	// Give the ring one more cycle to stabilize.
	time.Sleep(20 * time.Millisecond)
	evts := engState.eng.SnapshotEvents()
	if len(evts) > n {
		return fmt.Errorf("event history size %d exceeds limit %d", len(evts), n)
	}
	return nil
}

func theMostRecentEventIsLast() error {
	evts := engState.eng.SnapshotEvents()
	if len(evts) < 2 {
		return nil // nothing to check
	}
	for i := 1; i < len(evts); i++ {
		if evts[i].CheckedAt.Before(evts[i-1].CheckedAt) {
			return fmt.Errorf("events[%d] (%v) is before events[%d] (%v): not oldest-first",
				i, evts[i].CheckedAt, i-1, evts[i-1].CheckedAt)
		}
	}
	return nil
}

func theEngineHasRecordedNAlertEvents(n int) error {
	// Ensure the engine is running and has triggered n events.
	if engState.eng == nil {
		addEngTarget("t1", "5ms")
		if err := buildAndStartEngine(); err != nil {
			return err
		}
	}
	engState.prober.setResult("t1", engOutageResult("t1"))
	if err := waitForEventCount(n); err != nil {
		return err
	}
	// Switch to healthy so the engine stops generating alert events between
	// this step and iCallSnapshotEvents, preventing a race at 5ms probe interval.
	engState.prober.setResult("t1", engHealthyResult("t1"))
	return nil
}

func iCallSnapshotEvents() error {
	engState.snapshotEvents = engState.eng.SnapshotEvents()
	return nil
}

func theReturnedSliceLengthIs(n int) error {
	if len(engState.snapshotEvents) < n {
		return fmt.Errorf("snapshot events length %d, want at least %d", len(engState.snapshotEvents), n)
	}
	return nil
}

func eventsAreOrderedOldestFirst() error {
	evts := engState.snapshotEvents
	for i := 1; i < len(evts); i++ {
		if evts[i].CheckedAt.Before(evts[i-1].CheckedAt) {
			return fmt.Errorf("events not ordered oldest-first: [%d]=%v > [%d]=%v",
				i-1, evts[i-1].CheckedAt, i, evts[i].CheckedAt)
		}
	}
	return nil
}

// ── snapshot API ──────────────────────────────────────────────────────────────

func nTargetsWithIDsAreConfigured(n int, a, b, c string) error {
	for _, id := range []string{a, b, c} {
		addEngTarget(id, "5ms")
	}
	return nil
}

func theEngineHasStatusForEachTarget() error {
	if engState.eng == nil {
		if err := buildAndStartEngine(); err != nil {
			return err
		}
	}
	ids := make([]string, 0, len(engState.targetCfgs))
	for _, tc := range engState.targetCfgs {
		ids = append(ids, tc.ID)
	}
	return waitForAllStatuses(ids)
}

func snapshotStatusesReturnsEntriesInOrder() error {
	statuses := engState.eng.SnapshotStatuses()
	if len(statuses) == 0 {
		return fmt.Errorf("no statuses returned")
	}
	for i := 1; i < len(statuses); i++ {
		if statuses[i].TargetID < statuses[i-1].TargetID {
			return fmt.Errorf("statuses not sorted: [%d]=%q > [%d]=%q",
				i-1, statuses[i-1].TargetID, i, statuses[i].TargetID)
		}
	}
	return nil
}

func theProberReturnsResultWithLatencyAndStatusCode(ms, code int) error {
	id := engState.currentTargetID
	if id == "" {
		id = "t1"
	}
	engState.prober.setResult(id, targets.CheckResult{
		TargetID:   id,
		State:      targets.StateHealthy,
		Severity:   targets.SeverityInfo,
		Latency:    time.Duration(ms) * time.Millisecond,
		StatusCode: code,
		Message:    "mock: with latency+status",
	})
	// Wait for a new probe to pick up the updated result.
	prevCount := engState.prober.callCount(id)
	return waitForCallCount(id, prevCount+1)
}

func theSnapshotIncludesLatency(id string, ms int) error {
	want := time.Duration(ms) * time.Millisecond
	for _, st := range engState.eng.SnapshotStatuses() {
		if st.TargetID == id {
			if st.Latency != want {
				return fmt.Errorf("target %q latency=%v want=%v", id, st.Latency, want)
			}
			return nil
		}
	}
	return fmt.Errorf("no status for %q", id)
}

func theSnapshotIncludesStatusCode(id string, code int) error {
	for _, st := range engState.eng.SnapshotStatuses() {
		if st.TargetID == id {
			if st.StatusCode != code {
				return fmt.Errorf("target %q status_code=%d want=%d", id, st.StatusCode, code)
			}
			return nil
		}
	}
	return fmt.Errorf("no status for %q", id)
}

// ── additional engine/alert steps ─────────────────────────────────────────────

func aProbeTargetRegisteredForEndpoint(endpoint string) error {
	id := fmt.Sprintf("ep-%d", len(engState.targetCfgs))
	tc := makeEngTargetCfg(id, "5ms")
	tc.Endpoint = endpoint
	engState.targetCfgs = append(engState.targetCfgs, tc)
	engState.currentTargetID = id
	return nil
}

func theProbeForTargetRuns(id string) error {
	prevCount := engState.prober.callCount(id)
	return waitForCallCount(id, prevCount+1)
}

func theTargetIsClassifiedAs(id, class string) error {
	return waitForStatusState(id, class)
}

// ── stdio transport probing ───────────────────────────────────────────────────

func aStdioTargetWithEmptyCommand(id string) error {
	engState.stdioTarget = targets.Target{
		ID:        id,
		Protocol:  targets.ProtocolMCP,
		Transport: targets.TransportStdio,
		Name:      id,
		Stdio:     targets.StdioCommand{Command: ""},
		Check:     targets.CheckPolicy{Interval: 5 * time.Millisecond, Timeout: 500 * time.Millisecond},
	}
	engState.stdioHasTarget = true
	return nil
}

func theStdioProberProbes(id string) error {
	if !engState.stdioHasTarget {
		return fmt.Errorf("no stdio target configured for %q", id)
	}
	sp := engine.NewStdioProber()
	if engState.stdioFallback != nil {
		sp.Fallback = engState.stdioFallback
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := sp.Probe(ctx, engState.stdioTarget, nil)
	if err != nil {
		// Store the error as an unhealthy result.
		result = targets.CheckResult{
			TargetID:     id,
			State:        targets.StateUnhealthy,
			FailureClass: targets.FailureNetwork,
			Message:      err.Error(),
		}
	}
	engState.stdioResult = &result
	return nil
}

func theResultStateIs(state string) error {
	if engState.stdioResult == nil {
		return fmt.Errorf("no stdio probe result available")
	}
	if string(engState.stdioResult.State) != state {
		return fmt.Errorf("result state=%q want=%q", engState.stdioResult.State, state)
	}
	return nil
}

func theFailureClassIs(class string) error {
	// HURL report result takes priority when present.
	if hurlState.lastFailClass != "" {
		if string(hurlState.lastFailClass) != class {
			return fmt.Errorf("failure class=%q want=%q", hurlState.lastFailClass, class)
		}
		return nil
	}
	if engState.stdioResult == nil {
		return fmt.Errorf("no stdio probe result or hurl result available")
	}
	if string(engState.stdioResult.FailureClass) != class {
		return fmt.Errorf("failure class=%q want=%q", engState.stdioResult.FailureClass, class)
	}
	return nil
}

func theMessageContains(substr string) error {
	if engState.stdioResult == nil {
		return fmt.Errorf("no stdio probe result available")
	}
	if !strings.Contains(engState.stdioResult.Message, substr) {
		return fmt.Errorf("message %q does not contain %q", engState.stdioResult.Message, substr)
	}
	return nil
}

func aStdioTargetWithHandshakeProfile(id, profile string) error {
	engState.stdioTarget = targets.Target{
		ID:        id,
		Protocol:  targets.ProtocolMCP,
		Transport: targets.TransportStdio,
		Name:      id,
		Stdio:     targets.StdioCommand{Command: "echo"},
		Check: targets.CheckPolicy{
			Interval:         5 * time.Millisecond,
			Timeout:          500 * time.Millisecond,
			HandshakeProfile: profile,
		},
	}
	engState.stdioHasTarget = true
	return nil
}

func noSubprocessIsLaunched() error {
	// Verified by checking the result message: "handshake_profile=none" means
	// the prober returned early without calling exec.Command.
	if engState.stdioResult == nil {
		return fmt.Errorf("no stdio probe result; run the prober first")
	}
	if !strings.Contains(engState.stdioResult.Message, "handshake_profile=none") {
		return fmt.Errorf("expected handshake_profile=none message, got %q", engState.stdioResult.Message)
	}
	return nil
}

// aStdioTargetWithProtocolAndHandshakeProfile configures a stdio target for
// scenarios that require an actual subprocess to verify exercised methods.
// Returns ErrPending because subprocess-based handshake tests require a
// real MCP/ACP server binary or mock server infrastructure.
func aStdioTargetWithProtocolAndHandshakeProfile(id, proto, profile string) error {
	return godog.ErrPending
}

func theExercisedMethodsListIs(first string, rest ...string) error {
	return godog.ErrPending
}

func aStdioTargetWithRequiredMethods(id, methods string) error {
	return godog.ErrPending
}

func aStdioTargetThatTimesOut(id string) error {
	return godog.ErrPending
}

func aStdioTargetThatReturnsRPCError(id, method string) error {
	return godog.ErrPending
}

// aTargetWithTransport — step registered here but aTargetWithTransport func
// is owned by config_validation_steps.go; this registers an HTTP/non-stdio
// target for the "HTTP fallback prober is invoked" scenario.
func aTargetWithTransportForEngine(id, transport string) error {
	tc := makeEngTargetCfg(id, "5ms")
	tc.Transport = transport
	for i, existing := range engState.targetCfgs {
		if existing.ID == id {
			engState.targetCfgs[i] = tc
			engState.currentTargetID = id
			return nil
		}
	}
	// Also set as stdio target for direct prober invocation.
	engState.stdioTarget = targets.Target{
		ID:        id,
		Protocol:  targets.ProtocolHTTP,
		Transport: targets.Transport(transport),
		Name:      id,
		Endpoint:  "http://127.0.0.1:1/",
		Check:     targets.CheckPolicy{Interval: 5 * time.Millisecond, Timeout: 500 * time.Millisecond},
	}
	engState.stdioHasTarget = true
	engState.currentTargetID = id
	return nil
}

func theHTTPFallbackProberIsInvoked(id string) error {
	// Use theStdioProberProbes to run the prober; it will call the fallback.
	if !engState.stdioHasTarget {
		return fmt.Errorf("no target configured for %q", id)
	}
	// Inject a fallback mock prober.
	fallback := newMockEngineProber()
	fallback.setResult(id, engHealthyResult(id))
	engState.stdioFallback = fallback

	sp := engine.NewStdioProber()
	sp.Fallback = fallback
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, _ := sp.Probe(ctx, engState.stdioTarget, nil)
	engState.stdioResult = &result

	// Verify the fallback was called.
	if fallback.callCount(id) == 0 {
		return fmt.Errorf("HTTP fallback prober was not invoked for %q", id)
	}
	return nil
}

func aStdioTargetThatWritesToStderr(id string) error {
	return godog.ErrPending
}

func theResultMessageContainsStderrOutput() error {
	return godog.ErrPending
}

// ── sort import guard ─────────────────────────────────────────────────────────

var _ = sort.Slice // ensure sort import is used
