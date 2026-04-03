package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/semaphore"

	"github.com/james-gibson/smoke-alarm/internal/auth"
	"github.com/james-gibson/smoke-alarm/internal/config"
	"github.com/james-gibson/smoke-alarm/internal/knownstate"
	"github.com/james-gibson/smoke-alarm/internal/safety"
	"github.com/james-gibson/smoke-alarm/internal/targets"
	"github.com/james-gibson/smoke-alarm/internal/telemetry"
)

// Prober runs protocol-level checks for a target.
type Prober interface {
	Probe(ctx context.Context, target targets.Target, headers map[string]string) (targets.CheckResult, error)
}

// Notifier receives alert-worthy events (regressions/outages/escalations).
type Notifier interface {
	Notify(ctx context.Context, event AlertEvent) error
}

// AlertEvent is emitted when a check result should be elevated.
type AlertEvent struct {
	TargetID     string               `json:"target_id"`
	TargetName   string               `json:"target_name"`
	State        targets.HealthState  `json:"state"`
	Severity     targets.Severity     `json:"severity"`
	Regression   bool                 `json:"regression"`
	Message      string               `json:"message"`
	FailureClass targets.FailureClass `json:"failure_class"`
	CheckedAt    time.Time            `json:"checked_at"`
	Details      map[string]any       `json:"details,omitempty"`
}

// TargetRuntimeStatus is the current status view for a target.
type TargetRuntimeStatus struct {
	TargetID            string               `json:"target_id"`
	Name                string               `json:"name"`
	Endpoint            string               `json:"endpoint"`
	Protocol            targets.Protocol     `json:"protocol"`
	State               targets.HealthState  `json:"state"`
	Severity            targets.Severity     `json:"severity"`
	FailureClass        targets.FailureClass `json:"failure_class"`
	Message             string               `json:"message"`
	LastCheckedAt       time.Time            `json:"last_checked_at"`
	Latency             time.Duration        `json:"latency"`
	StatusCode          int                  `json:"status_code"`
	ConsecutiveFailures int                  `json:"consecutive_failures"`
	EverHealthy         bool                 `json:"ever_healthy"`
	Regression          bool                 `json:"regression"`
}

// Engine is the monitoring scheduler/executor with regression classification.
type Engine struct {
	cfg           config.Config
	authMgr       *auth.Manager
	store         *knownstate.Store
	prober        Prober
	safetyScanner *safety.Scanner
	notifiers     []Notifier
	telemetry     *telemetry.Exporter

	targets []targets.Target

	sem *semaphore.Weighted

	mu       sync.RWMutex
	statuses map[string]TargetRuntimeStatus
	events   []AlertEvent
	eventCap int

	startOnce sync.Once
	stopOnce  sync.Once
	wg        sync.WaitGroup
}

// Option customizes Engine creation.
type Option func(*Engine)

// WithStore sets known-state baseline persistence.
func WithStore(store *knownstate.Store) Option {
	return func(e *Engine) { e.store = store }
}

// WithProber sets custom protocol prober.
func WithProber(p Prober) Option {
	return func(e *Engine) {
		if p != nil {
			e.prober = p
		}
	}
}

// WithNotifier adds an alert sink.
func WithNotifier(n Notifier) Option {
	return func(e *Engine) {
		if n != nil {
			e.notifiers = append(e.notifiers, n)
		}
	}
}

// WithTelemetry enables metrics export.
func WithTelemetry(t *telemetry.Exporter) Option {
	return func(e *Engine) {
		e.telemetry = t
	}
}

// WithAuthManager overrides auth manager.
func WithAuthManager(m *auth.Manager) Option {
	return func(e *Engine) {
		if m != nil {
			e.authMgr = m
		}
	}
}

// WithSafetyScanner overrides the pre-protocol HURL safety scanner.
func WithSafetyScanner(scanner *safety.Scanner) Option {
	return func(e *Engine) {
		if scanner != nil {
			e.safetyScanner = scanner
		}
	}
}

// New builds an engine from validated configuration.
func New(cfg config.Config, opts ...Option) (*Engine, error) {
	compiled, err := compileTargets(cfg)
	if err != nil {
		return nil, err
	}

	maxWorkers := cfg.Service.MaxWorkers
	if maxWorkers < 1 {
		maxWorkers = 1
	}

	eventCap := cfg.Runtime.EventHistorySize
	if eventCap <= 0 {
		eventCap = 500
	}

	e := &Engine{
		cfg:           cfg,
		authMgr:       auth.NewManager(),
		prober:        NewStdioProber(),
		safetyScanner: safety.NewScanner(),
		targets:       compiled,
		sem:           semaphore.NewWeighted(int64(maxWorkers)),
		statuses:      make(map[string]TargetRuntimeStatus, len(compiled)),
		events:        make([]AlertEvent, 0, eventCap),
		eventCap:      eventCap,
		notifiers:     nil,
	}

	for _, opt := range opts {
		opt(e)
	}
	return e, nil
}

// Start runs monitor loops until ctx is canceled.
func (e *Engine) Start(ctx context.Context) error {
	var startErr error
	e.startOnce.Do(func() {
		if e.store != nil {
			if err := e.store.Load(ctx); err != nil {
				startErr = fmt.Errorf("load known-state: %w", err)
				return
			}
		}

		for _, t := range e.targets {
			targetCopy := t
			e.wg.Add(1)
			go func() {
				defer e.wg.Done()
				e.runTargetLoop(ctx, targetCopy)
			}()
		}
	})

	if startErr != nil {
		return startErr
	}

	<-ctx.Done()
	e.stopOnce.Do(func() {
		e.wg.Wait()
		if e.store != nil {
			_ = e.store.Save(context.Background())
		}
	})
	return nil
}

func (e *Engine) runTargetLoop(ctx context.Context, t targets.Target) {
	// Immediate first check.
	e.executeCheck(ctx, t)

	ticker := time.NewTicker(t.Check.Interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			e.executeCheck(ctx, t)
		}
	}
}

func (e *Engine) executeCheck(ctx context.Context, t targets.Target) {
	// Bound concurrent active probes.
	if err := e.sem.Acquire(ctx, 1); err != nil {
		return
	}
	defer e.sem.Release(1)

	var final targets.CheckResult
	var runErr error

	retries := t.Check.Retries
	if retries < 0 {
		retries = 0
	}

	// Pre-protocol stop-gap safety stage: registered HURL tests.
	// If any registered HURL test fails, block deeper protocol probing.
	if blocked, safetyResult := e.runSafetyStage(ctx, t); blocked {
		final = safetyResult
		final = e.classifyWithPolicy(ctx, t, final, nil)
		e.storeStatus(t, final)
		e.maybeAlert(ctx, t, final)
		return
	}

	for attempt := 0; attempt <= retries; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, t.Check.Timeout)
		start := time.Now()

		headers, hErr := e.buildHeaders(attemptCtx, t.Auth)
		if hErr != nil {
			cancel()
			final = targets.CheckResult{
				TargetID:     t.ID,
				Protocol:     t.Protocol,
				State:        targets.StateUnhealthy,
				Severity:     targets.SeverityWarn,
				FailureClass: targets.FailureAuth,
				Message:      redactAuthErr(hErr),
				CheckedAt:    time.Now(),
				Attempt:      attempt,
				Latency:      time.Since(start),
			}
			runErr = hErr
		} else {
			result, pErr := e.prober.Probe(attemptCtx, t, headers)
			cancel()
			if pErr != nil {
				final = targets.CheckResult{
					TargetID:     t.ID,
					Protocol:     t.Protocol,
					State:        targets.StateUnhealthy,
					Severity:     targets.SeverityWarn,
					FailureClass: classifyProbeError(pErr),
					Message:      pErr.Error(),
					CheckedAt:    time.Now(),
					Attempt:      attempt,
					Latency:      time.Since(start),
				}
				runErr = pErr
			} else {
				final = result
				final.Attempt = attempt
				if final.CheckedAt.IsZero() {
					final.CheckedAt = time.Now()
				}
				if final.TargetID == "" {
					final.TargetID = t.ID
				}
				if final.Protocol == "" {
					final.Protocol = t.Protocol
				}
				runErr = nil
			}
		}

		// Retry only failures.
		if !final.IsFailure() {
			break
		}
		if attempt < retries {
			continue
		}
	}

	final = e.classifyWithPolicy(ctx, t, final, runErr)
	e.storeStatus(t, final)
	e.maybeAlert(ctx, t, final)
}

func (e *Engine) buildHeaders(ctx context.Context, ac targets.AuthConfig) (map[string]string, error) {
	if e.authMgr == nil {
		return nil, errors.New("auth manager not configured")
	}
	mat, err := e.authMgr.BuildHeaders(ctx, ac)
	if err != nil {
		return nil, err
	}
	return mat.Headers, nil
}

func (e *Engine) classifyWithPolicy(ctx context.Context, t targets.Target, result targets.CheckResult, probeErr error) targets.CheckResult {
	if result.CheckedAt.IsZero() {
		result.CheckedAt = time.Now()
	}
	if result.Severity == "" {
		result.Severity = severityFromState(e.cfg, result.State)
	}
	if result.FailureClass == "" {
		if probeErr != nil {
			result.FailureClass = classifyProbeError(probeErr)
		} else {
			result.FailureClass = targets.FailureNone
		}
	}
	if strings.TrimSpace(result.Message) == "" {
		result.Message = defaultMessageForState(result.State)
	}

	// Known-state + regression semantics.
	if e.store != nil && e.cfg.KnownState.Enabled {
		ksStatus := toKnownStateStatus(result.State)
		up, err := e.store.Update(ctx, knownstate.UpdateInput{
			TargetID:  t.ID,
			Status:    ksStatus,
			ErrorText: result.Message,
			CheckedAt: result.CheckedAt,
		})
		if err == nil {
			result.PreviouslyOK = up.Previous.EverHealthy
			if up.IsRegression || (up.Current.EverHealthy && knownstate.IsFailure(ksStatus)) {
				result.State = targets.StateRegression
				result.Regression = true
				result.Severity = targets.SeverityCritical
				result.Message = "faulty smoke test regression: previously healthy target now failing"
			}

			// Outage threshold after regression logic.
			//
			// Policy:
			// - Suppress bootstrap outages for auth/config failures before baseline healthy.
			// - Allow pre-baseline outages for transport/protocol classes (network/timeout/tls/protocol/rate-limited/unknown).
			if knownstate.IsFailure(ksStatus) &&
				up.Current.ConsecutiveFailures >= e.cfg.KnownState.OutageThresholdConsecutiveFailures &&
				result.State != targets.StateRegression &&
				(up.Current.EverHealthy ||
					(result.FailureClass != targets.FailureAuth &&
						result.FailureClass != targets.FailureConfig)) {
				result.State = targets.StateOutage
				result.Severity = targets.SeverityCritical
				result.Message = fmt.Sprintf("outage: %d consecutive failures", up.Current.ConsecutiveFailures)
			}

			if up.HadPrevious && up.Previous.CurrentStatus != up.Current.CurrentStatus {
				result.Transition = &targets.Transition{
					From:   fromKnown(up.Previous.CurrentStatus),
					To:     fromKnown(up.Current.CurrentStatus),
					At:     result.CheckedAt,
					Reason: result.Message,
				}
			}
		}
	}

	// Aggressive policy: any failure after previously healthy should elevate.
	if e.cfg.Alerts.Aggressive && result.PreviouslyOK && result.IsFailure() {
		if result.Severity != targets.SeverityCritical {
			result.Severity = targets.SeverityCritical
		}
		if !result.Regression && result.State != targets.StateRegression {
			result.State = targets.StateOutage
		}
	}

	return result
}

func (e *Engine) storeStatus(t targets.Target, result targets.CheckResult) {
	e.mu.Lock()
	defer e.mu.Unlock()

	st := TargetRuntimeStatus{
		TargetID:      t.ID,
		Name:          t.Name,
		Endpoint:      t.Endpoint,
		Protocol:      t.Protocol,
		State:         result.State,
		Severity:      result.Severity,
		FailureClass:  result.FailureClass,
		Message:       result.Message,
		LastCheckedAt: result.CheckedAt,
		Latency:       result.Latency,
		StatusCode:    result.StatusCode,
		Regression:    result.Regression,
	}

	if e.store != nil {
		if ks, ok := e.store.Get(t.ID); ok {
			st.ConsecutiveFailures = ks.ConsecutiveFailures
			st.EverHealthy = ks.EverHealthy
		}
	}

	e.statuses[t.ID] = st

	if e.telemetry != nil {
		e.telemetry.RecordTargetState(context.Background(), t.ID, string(result.State))
		if result.Latency > 0 {
			e.telemetry.RecordCheckLatency(context.Background(), t.ID, result.Latency.Milliseconds())
		}
		if result.State == targets.StateUnhealthy || result.State == targets.StateOutage {
			e.telemetry.RecordCheckFailure(context.Background(), t.ID, string(result.FailureClass))
		}
	}
}

func (e *Engine) maybeAlert(ctx context.Context, t targets.Target, result targets.CheckResult) {
	// Alert conditions:
	// - regression always
	// - outage always
	// - critical severity always
	if !(result.Regression || result.State == targets.StateOutage || result.Severity == targets.SeverityCritical) {
		return
	}

	event := AlertEvent{
		TargetID:     t.ID,
		TargetName:   t.Name,
		State:        result.State,
		Severity:     result.Severity,
		Regression:   result.Regression,
		Message:      result.Message,
		FailureClass: result.FailureClass,
		CheckedAt:    result.CheckedAt,
		Details: map[string]any{
			"endpoint":   t.Endpoint,
			"protocol":   t.Protocol,
			"attempt":    result.Attempt,
			"statusCode": result.StatusCode,
			"latency":    result.Latency.String(),
		},
	}

	e.mu.Lock()
	if len(e.events) < e.eventCap {
		e.events = append(e.events, event)
	} else {
		copy(e.events, e.events[1:])
		e.events[len(e.events)-1] = event
	}
	e.mu.Unlock()

	for _, n := range e.notifiers {
		_ = n.Notify(ctx, event)
	}
}

// SnapshotStatuses returns sorted target status snapshot for health/status APIs.
func (e *Engine) SnapshotStatuses() []TargetRuntimeStatus {
	e.mu.RLock()
	defer e.mu.RUnlock()

	out := make([]TargetRuntimeStatus, 0, len(e.statuses))
	for _, st := range e.statuses {
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TargetID < out[j].TargetID })
	return out
}

// SnapshotEvents returns a copy of recent alert events in chronological order.
func (e *Engine) SnapshotEvents() []AlertEvent {
	e.mu.RLock()
	defer e.mu.RUnlock()

	out := make([]AlertEvent, len(e.events))
	copy(out, e.events)
	return out
}

// IsReady returns true once at least one status exists for all enabled targets.
func (e *Engine) IsReady() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.statuses) >= len(e.targets) && len(e.targets) > 0
}

// -------------------- Default Prober --------------------

// HTTPProber performs lightweight endpoint checks.
type HTTPProber struct {
	client *http.Client
}

// NewHTTPProber returns the default probe implementation.
func NewHTTPProber() *HTTPProber {
	return &HTTPProber{
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Probe checks endpoint reachability/health semantics.
// For HTTP(S), it performs either simple status checks or JSON-RPC handshake checks
// for MCP/ACP targets depending on handshake profile.
// For WS(S), it validates TCP reachability with low overhead.
func (p *HTTPProber) Probe(ctx context.Context, target targets.Target, headers map[string]string) (targets.CheckResult, error) {
	u, err := url.Parse(target.Endpoint)
	if err != nil {
		return targets.CheckResult{}, fmt.Errorf("invalid endpoint: %w", err)
	}

	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if target.Transport == targets.TransportSSE {
			return p.probeSSE(ctx, target, headers)
		}
		if shouldRunHTTPHandshake(target) {
			return p.probeJSONRPCHandshakeHTTP(ctx, target, headers)
		}
		return p.probeSimpleHTTP(ctx, target, headers)

	case "ws", "wss":
		start := time.Now()
		addr := hostPortWithDefault(u)
		dialer := net.Dialer{}
		conn, err := dialer.DialContext(ctx, "tcp", addr)
		lat := time.Since(start)
		if err != nil {
			return targets.CheckResult{}, err
		}
		_ = conn.Close()

		return targets.CheckResult{
			TargetID:     target.ID,
			Protocol:     target.Protocol,
			State:        targets.StateHealthy,
			Severity:     targets.SeverityInfo,
			FailureClass: targets.FailureNone,
			Message:      "reachable",
			Latency:      lat,
			CheckedAt:    time.Now(),
		}, nil

	default:
		return targets.CheckResult{}, fmt.Errorf("unsupported endpoint scheme %q", u.Scheme)
	}
}

func (p *HTTPProber) probeSSE(ctx context.Context, target targets.Target, headers map[string]string) (targets.CheckResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.Endpoint, nil)
	if err != nil {
		return targets.CheckResult{}, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("Accept", "text/event-stream")

	start := time.Now()
	resp, err := p.client.Do(req)
	lat := time.Since(start)
	if err != nil {
		return targets.CheckResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	ok := statusMatches(resp.StatusCode, target.Expected.HealthyStatusCodes)
	state := targets.StateHealthy
	sev := targets.SeverityInfo
	fc := targets.FailureNone
	msg := "sse stream reachable"

	ct := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if ct != "" && !strings.Contains(ct, "text/event-stream") {
		ok = false
		state = targets.StateUnhealthy
		sev = targets.SeverityWarn
		fc = targets.FailureProtocol
		msg = fmt.Sprintf("unexpected content-type %q for sse endpoint", ct)
	}

	if !ok && fc == targets.FailureNone {
		state = targets.StateUnhealthy
		sev = targets.SeverityWarn
		fc = targets.FailureProtocol
		msg = fmt.Sprintf("unexpected status code %d", resp.StatusCode)
	}

	return targets.CheckResult{
		TargetID:     target.ID,
		Protocol:     target.Protocol,
		State:        state,
		Severity:     sev,
		FailureClass: fc,
		Message:      msg,
		StatusCode:   resp.StatusCode,
		Latency:      lat,
		CheckedAt:    time.Now(),
		Details: map[string]any{
			"transport":    "sse",
			"content_type": ct,
		},
	}, nil
}

func (p *HTTPProber) probeSimpleHTTP(ctx context.Context, target targets.Target, headers map[string]string) (targets.CheckResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.Endpoint, nil)
	if err != nil {
		return targets.CheckResult{}, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	start := time.Now()
	resp, err := p.client.Do(req)
	lat := time.Since(start)
	if err != nil {
		return targets.CheckResult{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	ok := statusMatches(resp.StatusCode, target.Expected.HealthyStatusCodes)
	state := targets.StateHealthy
	sev := targets.SeverityInfo
	fc := targets.FailureNone
	msg := "healthy"

	if !ok {
		state = targets.StateUnhealthy
		sev = targets.SeverityWarn
		fc = targets.FailureProtocol
		msg = fmt.Sprintf("unexpected status code %d", resp.StatusCode)
	}

	return targets.CheckResult{
		TargetID:     target.ID,
		Protocol:     target.Protocol,
		State:        state,
		Severity:     sev,
		FailureClass: fc,
		Message:      msg,
		StatusCode:   resp.StatusCode,
		Latency:      lat,
		CheckedAt:    time.Now(),
	}, nil
}

func (p *HTTPProber) probeJSONRPCHandshakeHTTP(ctx context.Context, target targets.Target, headers map[string]string) (targets.CheckResult, error) {
	methods := httpHandshakeMethods(target)
	start := time.Now()
	exercised := make([]string, 0, len(methods))

	for i, method := range methods {
		reqBody, err := json.Marshal(rpcRequest{
			JSONRPC: "2.0",
			ID:      1 + i,
			Method:  method,
			Params:  httpHandshakeParams(target.Protocol, method),
		})
		if err != nil {
			return targets.CheckResult{}, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.Endpoint, bytes.NewReader(reqBody))
		if err != nil {
			return targets.CheckResult{}, err
		}
		req.Header.Set("Content-Type", "application/json")
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		resp, err := p.client.Do(req)
		if err != nil {
			return targets.CheckResult{}, err
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_ = resp.Body.Close()
			return targets.CheckResult{
				TargetID:     target.ID,
				Protocol:     target.Protocol,
				State:        targets.StateUnhealthy,
				Severity:     targets.SeverityWarn,
				FailureClass: targets.FailureProtocol,
				Message:      fmt.Sprintf("handshake method %q failed with status %d", method, resp.StatusCode),
				StatusCode:   resp.StatusCode,
				Latency:      time.Since(start),
				CheckedAt:    time.Now(),
			}, nil
		}

		var rpcResp rpcResponse
		decErr := json.NewDecoder(resp.Body).Decode(&rpcResp)
		_ = resp.Body.Close()
		if decErr != nil {
			return targets.CheckResult{
				TargetID:     target.ID,
				Protocol:     target.Protocol,
				State:        targets.StateUnhealthy,
				Severity:     targets.SeverityWarn,
				FailureClass: targets.FailureProtocol,
				Message:      fmt.Sprintf("handshake method %q returned invalid JSON-RPC body: %v", method, decErr),
				Latency:      time.Since(start),
				CheckedAt:    time.Now(),
			}, nil
		}
		if rpcResp.Error != nil {
			return targets.CheckResult{
				TargetID:     target.ID,
				Protocol:     target.Protocol,
				State:        targets.StateUnhealthy,
				Severity:     targets.SeverityWarn,
				FailureClass: targets.FailureProtocol,
				Message:      fmt.Sprintf("handshake method %q rejected: %s", method, rpcResp.Error.Error()),
				Latency:      time.Since(start),
				CheckedAt:    time.Now(),
			}, nil
		}

		exercised = append(exercised, method)
	}

	return targets.CheckResult{
		TargetID:     target.ID,
		Protocol:     target.Protocol,
		State:        targets.StateHealthy,
		Severity:     targets.SeverityInfo,
		FailureClass: targets.FailureNone,
		Message:      fmt.Sprintf("http(s) handshake passed: %s", strings.Join(exercised, ", ")),
		Latency:      time.Since(start),
		CheckedAt:    time.Now(),
		Details: map[string]any{
			"transport":         "http",
			"profile":           target.Check.HandshakeProfile,
			"exercised_methods": exercised,
		},
	}, nil
}

func shouldRunHTTPHandshake(target targets.Target) bool {
	// Handshake is only applicable to MCP/ACP over non-SSE HTTP paths.
	if target.Protocol != targets.ProtocolMCP && target.Protocol != targets.ProtocolACP {
		return false
	}
	if target.Transport == targets.TransportSSE {
		return false
	}
	profile := strings.ToLower(strings.TrimSpace(target.Check.HandshakeProfile))
	return profile != "none"
}

func httpHandshakeMethods(target targets.Target) []string {
	if len(target.Check.RequiredMethods) > 0 {
		return normalizeHandshakeMethods(target.Check.RequiredMethods)
	}

	profile := strings.ToLower(strings.TrimSpace(target.Check.HandshakeProfile))
	if profile == "" {
		profile = "base"
	}

	switch profile {
	case "strict":
		if target.Protocol == targets.ProtocolACP {
			return []string{"initialize", "session/setup", "prompt/turn"}
		}
		return []string{"initialize", "tools/list", "resources/list"}
	default:
		return []string{"initialize"}
	}
}

func normalizeHandshakeMethods(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, m := range in {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		if _, ok := seen[m]; ok {
			continue
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}
	return out
}

func httpHandshakeParams(proto targets.Protocol, method string) any {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "initialize":
		params := map[string]any{
			"clientInfo": map[string]any{
				"name":    "ocd-smoke-alarm",
				"version": "0.1.0",
			},
			"capabilities": map[string]any{},
		}
		if proto == targets.ProtocolMCP {
			params["protocolVersion"] = "2024-11-05"
		}
		return params
	case "session/setup":
		return map[string]any{
			"clientInfo": map[string]any{
				"name":    "ocd-smoke-alarm",
				"version": "0.1.0",
			},
			"capabilities": map[string]any{},
		}
	case "prompt/turn":
		return map[string]any{
			"prompt": "smoke-alarm handshake validation",
			"input":  "health-check",
		}
	default:
		return map[string]any{}
	}
}

func statusMatches(code int, expected []int) bool {
	if len(expected) == 0 {
		return code >= 200 && code < 300
	}
	for _, c := range expected {
		if c == code {
			return true
		}
	}
	return false
}

func hostPortWithDefault(u *url.URL) string {
	host := u.Hostname()
	port := u.Port()
	if port != "" {
		return net.JoinHostPort(host, port)
	}
	switch strings.ToLower(u.Scheme) {
	case "wss", "https":
		return net.JoinHostPort(host, "443")
	default:
		return net.JoinHostPort(host, "80")
	}
}

// -------------------- Helpers --------------------

func compileTargets(cfg config.Config) ([]targets.Target, error) {
	out := make([]targets.Target, 0, len(cfg.Targets))
	for _, t := range cfg.Targets {
		if !t.Enabled {
			continue
		}
		interval, err := time.ParseDuration(t.Check.Interval)
		if err != nil {
			return nil, fmt.Errorf("target %q interval invalid: %w", t.ID, err)
		}
		timeout, err := time.ParseDuration(t.Check.Timeout)
		if err != nil {
			return nil, fmt.Errorf("target %q timeout invalid: %w", t.ID, err)
		}

		compiled := targets.Target{
			ID:        t.ID,
			Enabled:   t.Enabled,
			Protocol:  targets.Protocol(strings.ToLower(t.Protocol)),
			Name:      t.Name,
			Endpoint:  t.Endpoint,
			Transport: targets.Transport(strings.ToLower(t.Transport)),
			Type:      targets.TargetType(strings.ToLower(t.Type)),
			Expected: targets.ExpectedBehavior{
				HealthyStatusCodes: t.Expected.HealthyStatusCodes,
				MinCapabilities:    t.Expected.MinCapabilities,
				KnownAgentCountMin: t.Expected.KnownAgentCountMin,
				ExpectedVersion:    t.Expected.ExpectedVersion,
			},
			Auth: targets.AuthConfig{
				Type:        targets.AuthType(strings.ToLower(t.Auth.Type)),
				Header:      t.Auth.Header,
				KeyName:     t.Auth.KeyName,
				SecretRef:   t.Auth.SecretRef,
				ClientID:    t.Auth.ClientID,
				TokenURL:    t.Auth.TokenURL,
				RedirectURL: t.Auth.RedirectURL,
				CallbackID:  t.Auth.CallbackID,
				Scopes:      t.Auth.Scopes,
			},
			Stdio: targets.StdioCommand{
				Command: t.Stdio.Command,
				Args:    t.Stdio.Args,
				Env:     t.Stdio.Env,
				Cwd:     t.Stdio.Cwd,
			},
			Check: targets.CheckPolicy{
				Interval:         interval,
				Timeout:          timeout,
				Retries:          t.Check.Retries,
				HandshakeProfile: t.Check.HandshakeProfile,
				RequiredMethods:  append([]string(nil), t.Check.RequiredMethods...),
				HURLTests:        mapHURLTests(t.Check.HURLTests),
			},
			Headers: t.Headers,
		}
		if err := compiled.Validate(); err != nil {
			return nil, fmt.Errorf("target %q invalid: %w", t.ID, err)
		}
		out = append(out, compiled)
	}
	return out, nil
}

func mapHURLTests(in []config.HURLTestConfig) []targets.HURLTest {
	if len(in) == 0 {
		return nil
	}
	out := make([]targets.HURLTest, 0, len(in))
	for _, t := range in {
		out = append(out, targets.HURLTest{
			Name:     t.Name,
			File:     t.File,
			Endpoint: t.Endpoint,
			Method:   t.Method,
			Headers:  t.Headers,
			Body:     t.Body,
		})
	}
	return out
}

func (e *Engine) runSafetyStage(ctx context.Context, t targets.Target) (bool, targets.CheckResult) {
	if e.safetyScanner == nil || len(t.Check.HURLTests) == 0 {
		return false, targets.CheckResult{}
	}

	_ = e.safetyScanner.RegisterTarget(t)
	report := e.safetyScanner.RunTarget(ctx, t)
	if !report.AnyTestsSeen || !report.HasBlocking {
		return false, targets.CheckResult{}
	}

	firstFailure := "pre-protocol HURL safety checks failed"
	failureClass := targets.FailureProtocol
	for _, rr := range report.Results {
		if rr.Outcome == safety.OutcomeFail {
			if strings.TrimSpace(rr.Message) != "" {
				firstFailure = rr.Message
			}
			if rr.FailureClass != "" && rr.FailureClass != targets.FailureNone {
				failureClass = rr.FailureClass
			}
			break
		}
	}

	return true, targets.CheckResult{
		TargetID:     t.ID,
		Protocol:     t.Protocol,
		State:        targets.StateUnhealthy,
		Severity:     targets.SeverityWarn,
		FailureClass: failureClass,
		Message:      firstFailure,
		CheckedAt:    time.Now(),
		Details: map[string]any{
			"stage":   safety.Stage,
			"passed":  report.Passed,
			"failed":  report.Failed,
			"skipped": report.Skipped,
		},
	}
}

func classifyProbeError(err error) targets.FailureClass {
	if err == nil {
		return targets.FailureNone
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return targets.FailureTimeout
	}
	var nerr net.Error
	if errors.As(err, &nerr) {
		if nerr.Timeout() {
			return targets.FailureTimeout
		}
		return targets.FailureNetwork
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "tls"), strings.Contains(msg, "x509"):
		return targets.FailureTLS
	case strings.Contains(msg, "unauthorized"), strings.Contains(msg, "forbidden"), strings.Contains(msg, "token"), strings.Contains(msg, "oauth"):
		return targets.FailureAuth
	case strings.Contains(msg, "rate"):
		return targets.FailureRateLimited
	default:
		return targets.FailureUnknown
	}
}

func toKnownStateStatus(s targets.HealthState) knownstate.Status {
	switch s {
	case targets.StateHealthy:
		return knownstate.StatusHealthy
	case targets.StateDegraded:
		return knownstate.StatusDegraded
	case targets.StateOutage:
		return knownstate.StatusOutage
	case targets.StateRegression, targets.StateUnhealthy:
		return knownstate.StatusFailed
	default:
		return knownstate.StatusUnknown
	}
}

func fromKnown(s knownstate.Status) targets.HealthState {
	switch s {
	case knownstate.StatusHealthy:
		return targets.StateHealthy
	case knownstate.StatusDegraded:
		return targets.StateDegraded
	case knownstate.StatusOutage:
		return targets.StateOutage
	case knownstate.StatusFailed:
		return targets.StateUnhealthy
	default:
		return targets.StateUnknown
	}
}

func severityFromState(cfg config.Config, s targets.HealthState) targets.Severity {
	switch s {
	case targets.StateHealthy:
		return toSeverity(cfg.Alerts.Severity.Healthy, targets.SeverityInfo)
	case targets.StateDegraded:
		return toSeverity(cfg.Alerts.Severity.Degraded, targets.SeverityWarn)
	case targets.StateRegression:
		return toSeverity(cfg.Alerts.Severity.Regression, targets.SeverityCritical)
	case targets.StateOutage, targets.StateUnhealthy:
		return toSeverity(cfg.Alerts.Severity.Outage, targets.SeverityCritical)
	default:
		return targets.SeverityWarn
	}
}

func toSeverity(raw string, fallback targets.Severity) targets.Severity {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case string(targets.SeverityInfo):
		return targets.SeverityInfo
	case string(targets.SeverityWarn):
		return targets.SeverityWarn
	case string(targets.SeverityCritical):
		return targets.SeverityCritical
	default:
		return fallback
	}
}

func defaultMessageForState(s targets.HealthState) string {
	switch s {
	case targets.StateHealthy:
		return "healthy"
	case targets.StateDegraded:
		return "degraded"
	case targets.StateOutage:
		return "outage"
	case targets.StateRegression:
		return "regression detected"
	case targets.StateUnhealthy:
		return "unhealthy"
	default:
		return "unknown"
	}
}

func redactAuthErr(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	// Minimal redaction hook: prevent accidental Bearer/token echoing.
	msg = strings.ReplaceAll(msg, "Bearer ", "Bearer ****")
	msg = strings.ReplaceAll(msg, "bearer ", "bearer ****")
	msg = strings.ReplaceAll(msg, "token=", "token=****")
	return msg
}
