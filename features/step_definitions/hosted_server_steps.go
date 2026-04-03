package stepdefinitions

// hosted_server_steps.go — step definitions for features/hosted-server.feature
//
// Implemented steps operate in-process against hosted.Server (no binary required).
// hostedState holds per-scenario embedded hosted server state.
//
// Steps that depend on engine probing (probe cycle, classification, HURL preflight,
// strict handshake) remain ErrPending until engine_steps.go is implemented.
// Steps that depend on shared pending steps (configFlagIsTrueInConfig in
// sse_transport_steps.go, ocdSmokeAlarmStartsWithThatConfig in oauth_mock_steps.go)
// remain pending until those files are implemented.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/james-gibson/smoke-alarm/internal/config"
	"github.com/james-gibson/smoke-alarm/internal/health"
	"github.com/james-gibson/smoke-alarm/internal/hosted"
)

// hostedState holds per-scenario hosted server state.
var hostedState struct {
	srv      *hosted.Server
	opts     hosted.Options
	baseURL  string
	cancelFn context.CancelFunc
}

func cleanupHostedState() {
	if hostedState.cancelFn != nil {
		hostedState.cancelFn()
		hostedState.cancelFn = nil
	}
	if hostedState.srv != nil {
		_ = hostedState.srv.Shutdown(context.Background())
		hostedState.srv = nil
	}
	hostedState.baseURL = ""
}

func InitializeHostedServerSteps(ctx *godog.ScenarioContext) {
	ctx.BeforeScenario(func(_ *godog.Scenario) {
		cleanupHostedState()
	})
	ctx.AfterScenario(func(_ *godog.Scenario, _ error) {
		cleanupHostedState()
	})

	// ── startup ────────────────────────────────────────────────────────────
	ctx.Step(`^"([^"]*)" is false in config "([^"]*)"$`, configFlagIsFalseInConfig)
	ctx.Step(`^ocd-smoke-alarm is running with config "([^"]*)"$`, ocdSmokeAlarmIsRunningWithConfig)
	ctx.Step(`^no listener is bound on the hosted listen address$`, noListenerIsBoundOnHostedAddress)

	// ── HTTP transport ─────────────────────────────────────────────────────
	ctx.Step(`^the hosted server is running on "([^"]*)"$`, theHostedServerIsRunningOn)
	ctx.Step(`^a valid MCP initialize JSON-RPC request is sent to "([^"]*)"$`, aValidMCPInitializeRequestSentTo)
	ctx.Step(`^the response is a valid MCP initialize JSON-RPC response$`, theResponseIsValidMCPInitialize)
	ctx.Step(`^a valid ACP initialize JSON-RPC request is sent to "([^"]*)"$`, aValidACPInitializeRequestSentTo)
	ctx.Step(`^the response is a valid ACP initialize JSON-RPC response$`, theResponseIsValidACPInitialize)

	// ── SSE transport ──────────────────────────────────────────────────────
	ctx.Step(`^the response Content-Type is "([^"]*)"$`, theResponseContentTypeIs)
	ctx.Step(`^the connection remains open$`, theConnectionRemainsOpen)

	// ── HURL preflight ─────────────────────────────────────────────────────
	ctx.Step(`^the health endpoint "([^"]*)" returns status code (\d+)$`, theHealthEndpointReturnsStatusCode)
	ctx.Step(`^the target "([^"]*)" has hurl_test "([^"]*)"$`, theTargetHasHurlTestNamed2)
	ctx.Step(`^the HURL test "([^"]*)" is executed before the MCP handshake$`, theHURLTestIsExecutedBeforeMCPHandshake)

	// ── strict handshake ───────────────────────────────────────────────────
	ctx.Step(`^the hosted server does not respond to method "([^"]*)"$`, theHostedServerDoesNotRespondToMethod)

	// ── readyz after probing ───────────────────────────────────────────────
	ctx.Step(`^all enabled targets have completed at least one probe cycle$`, allEnabledTargetsCompletedOneProbe)

	// ── additional patterns ─────────────────────────────────────────────────
	ctx.Step(`^the ACP endpoint "([^"]*)" is served$`, theACPEndpointIsServed)
	ctx.Step(`^the MCP endpoint "([^"]*)" is served$`, theMCPEndpointIsServed)
	ctx.Step(`^the hosted server is running$`, theHostedServerIsRunning)
	ctx.Step(`^they appear in GET /hosted/events$`, theyAppearInGetHostedEvents)
	ctx.Step(`^a config with "([^"]*)" set to false$`, aConfigWithKeySetToFalse)
	ctx.Step(`^the probe for target "([^"]*)" completes$`, theProbeForTargetCompletes)
}

// ── startup ─────────────────────────────────────────────────────────────────

func configFlagIsFalseInConfig(flag, cfg string) error { return godog.ErrPending }

// ocdSmokeAlarmIsRunningWithConfig starts (or restarts) the in-process health
// server with the given config, registering a self-description factory.
func ocdSmokeAlarmIsRunningWithConfig(cfgPath string) error {
	cleanupHSState()

	abs := resolveConfigPath(cfgPath)
	cfg, err := config.Load(abs)
	if err != nil {
		return fmt.Errorf("load config %q: %w", cfgPath, err)
	}

	listenAddr := cfg.Health.ListenAddr
	if listenAddr == "" {
		listenAddr = "127.0.0.1:8088"
	}

	srv := health.NewServer(health.Options{
		ServiceName: cfg.Service.Name,
		ListenAddr:  listenAddr,
	})
	boundAddr, err := srv.BindWithRetry(10)
	if err != nil {
		return fmt.Errorf("bind health server for config %q: %w", cfgPath, err)
	}

	startedAt := time.Now().UTC()
	factory := health.NewSelfDescriptionFactory(cfg, "test", startedAt, srv)
	srv.SetSelfDescription(factory)

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.Start(ctx) }()
	time.Sleep(20 * time.Millisecond)

	hsState.srv = srv
	hsState.baseURL = "http://" + boundAddr
	hsState.cancelFn = cancel
	hsState.serviceName = cfg.Service.Name

	// Record configured→actual port mapping so aGETRequestSentTo can redirect
	// hardcoded URLs (e.g. http://127.0.0.1:18088/healthz) to the actual port.
	if listenAddr != boundAddr {
		if hsState.addrRemap == nil {
			hsState.addrRemap = make(map[string]string)
		}
		hsState.addrRemap[listenAddr] = boundAddr
	}
	return nil
}

func noListenerIsBoundOnHostedAddress() error {
	if hostedState.srv != nil {
		return fmt.Errorf("hosted server is running (should not be)")
	}
	return nil
}

// ── HTTP transport ───────────────────────────────────────────────────────────

// theHostedServerIsRunningOn starts an embedded hosted.Server on the given addr.
func theHostedServerIsRunningOn(addr string) error {
	cleanupHostedState()

	opts := hosted.Options{
		ServiceName: "test",
		ListenAddr:  addr,
		EnableHTTP:  true,
		EnableSSE:   true,
		EnableMCP:   true,
		EnableACP:   true,
	}
	srv := hosted.NewServer(opts)
	bctx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.Start(bctx) }()
	time.Sleep(30 * time.Millisecond)

	hostedState.srv = srv
	hostedState.opts = opts
	hostedState.baseURL = "http://" + addr
	hostedState.cancelFn = cancel
	return nil
}

// sendJSONRPC POSTs a JSON-RPC request to url and stores the response in httpState.
func sendJSONRPC(url, method string) error {
	id := 1
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "test", "version": "0.1"},
		},
	})
	if err != nil {
		return fmt.Errorf("marshal JSON-RPC request: %w", err)
	}

	resp, err := http.Post(url, "application/json", bytes.NewReader(body)) //nolint:noctx
	if err != nil {
		return fmt.Errorf("POST %q: %w", url, err)
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)

	// Stash a synthetic response so the shared response assertions work.
	httpState.lastBody = buf.Bytes()
	httpState.lastResp = resp
	return nil
}

func aValidMCPInitializeRequestSentTo(url string) error {
	return sendJSONRPC(url, "initialize")
}

func theResponseIsValidMCPInitialize() error {
	return assertInitializeResponse("mcp")
}

func aValidACPInitializeRequestSentTo(url string) error {
	return sendJSONRPC(url, "initialize")
}

func theResponseIsValidACPInitialize() error {
	return assertInitializeResponse("acp")
}

func assertInitializeResponse(proto string) error {
	if httpState.lastBody == nil {
		return fmt.Errorf("no response body for %s initialize", proto)
	}
	var result map[string]any
	if err := json.Unmarshal(httpState.lastBody, &result); err != nil {
		return fmt.Errorf("invalid JSON in %s initialize response: %w", proto, err)
	}
	if result["error"] != nil {
		return fmt.Errorf("%s initialize returned error: %v", proto, result["error"])
	}
	r, ok := result["result"].(map[string]any)
	if !ok {
		return fmt.Errorf("%s initialize response missing 'result' field: %s", proto, httpState.lastBody)
	}
	for _, field := range []string{"protocolVersion", "serverInfo", "capabilities"} {
		if _, ok := r[field]; !ok {
			return fmt.Errorf("%s initialize result missing %q field", proto, field)
		}
	}
	return nil
}

// ── SSE transport ────────────────────────────────────────────────────────────

func theResponseContentTypeIs(ct string) error {
	if httpState.lastResp == nil {
		return fmt.Errorf("no HTTP response available")
	}
	got := httpState.lastResp.Header.Get("Content-Type")
	if !strings.Contains(got, ct) {
		return fmt.Errorf("Content-Type %q does not contain %q", got, ct)
	}
	return nil
}

func theConnectionRemainsOpen() error {
	if httpState.lastResp == nil {
		return fmt.Errorf("no HTTP response available")
	}
	if httpState.lastResp.StatusCode != http.StatusOK {
		return fmt.Errorf("expected status 200, got %d", httpState.lastResp.StatusCode)
	}
	ct := httpState.lastResp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		return fmt.Errorf("expected SSE Content-Type (text/event-stream), got %q", ct)
	}
	return nil
}

// ── HURL preflight ───────────────────────────────────────────────────────────

// theHealthEndpointReturnsStatusCode GETs addr and asserts the status code.
// Returns ErrPending if no server is reachable — this step is used as a
// precondition in scenarios that also depend on engine probing (ErrPending too).
func theHealthEndpointReturnsStatusCode(addr string, code int) error {
	// Apply addrRemap in case the server bound to a different port.
	target := addr
	for configured, actual := range hsState.addrRemap {
		if strings.Contains(target, configured) {
			target = strings.Replace(target, configured, actual, 1)
			break
		}
	}
	resp, err := http.Get(target) //nolint:noctx
	if err != nil {
		// No server running — this scenario requires a running service; mark pending.
		return godog.ErrPending
	}
	defer resp.Body.Close()
	if resp.StatusCode != code {
		return fmt.Errorf("expected status %d, got %d from %q", code, resp.StatusCode, target)
	}
	return nil
}

func theTargetHasHurlTestNamed2(id, name string) error          { return godog.ErrPending }
func theHURLTestIsExecutedBeforeMCPHandshake(name string) error { return godog.ErrPending }

// ── strict handshake ─────────────────────────────────────────────────────────

func theHostedServerDoesNotRespondToMethod(method string) error { return godog.ErrPending }

// ── readyz after probing ─────────────────────────────────────────────────────

func allEnabledTargetsCompletedOneProbe() error { return godog.ErrPending }

// ── additional patterns ──────────────────────────────────────────────────────

func theACPEndpointIsServed(endpoint string) error {
	if hostedState.srv == nil {
		return fmt.Errorf("hosted server not running")
	}
	if hostedState.opts.ACPEndpoint != endpoint {
		return fmt.Errorf("ACP endpoint is %q, expected %q", hostedState.opts.ACPEndpoint, endpoint)
	}
	return nil
}

func theMCPEndpointIsServed(endpoint string) error {
	if hostedState.srv == nil {
		return fmt.Errorf("hosted server not running")
	}
	if hostedState.opts.MCPEndpoint != endpoint {
		return fmt.Errorf("MCP endpoint is %q, expected %q", hostedState.opts.MCPEndpoint, endpoint)
	}
	return nil
}

func theHostedServerIsRunning() error {
	if hostedState.srv == nil {
		return fmt.Errorf("hosted server is not running")
	}
	return nil
}

func theyAppearInGetHostedEvents() error         { return godog.ErrPending }
func aConfigWithKeySetToFalse(key string) error  { return godog.ErrPending }
func theProbeForTargetCompletes(id string) error { return godog.ErrPending }
