package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/engine"
	"github.com/james-gibson/smoke-alarm/internal/safety"
	"github.com/james-gibson/smoke-alarm/internal/targets"
)

func TestStdioProber_InitializeHandshakePass(t *testing.T) {
	t.Parallel()

	serverFile := mockStdioServer(t)

	prober := engine.NewStdioProber()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	target := targets.Target{
		ID:        "mcp-stdio-local",
		Enabled:   true,
		Protocol:  targets.ProtocolMCP,
		Name:      "Local Mock MCP Stdio",
		Transport: targets.TransportStdio,
		Endpoint:  "stdio://local",
		Stdio: targets.StdioCommand{
			Command: "go",
			Args:    []string{"run", serverFile},
		},
		Check: targets.CheckPolicy{
			Interval: 5 * time.Second,
			Timeout:  5 * time.Second,
			Retries:  0,
		},
	}

	res, err := prober.Probe(ctx, target, nil)
	if err != nil {
		t.Fatalf("stdio probe returned error: %v", err)
	}
	if res.State != targets.StateHealthy {
		t.Fatalf("expected healthy state, got %s (%s)", res.State, res.Message)
	}
	if res.FailureClass != targets.FailureNone {
		t.Fatalf("expected no failure class, got %s", res.FailureClass)
	}
	if !strings.Contains(strings.ToLower(res.Message), "handshake passed") {
		t.Fatalf("expected handshake success message, got %q", res.Message)
	}
}

func TestStdioProber_StrictHandshakeFailsWhenRequiredMethodsMissing(t *testing.T) {
	t.Parallel()

	serverFile := mockStdioServer(t)

	prober := engine.NewStdioProber()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	target := targets.Target{
		ID:        "mcp-stdio-strict-fail",
		Enabled:   true,
		Protocol:  targets.ProtocolMCP,
		Name:      "Local Mock MCP Stdio Strict Fail",
		Transport: targets.TransportStdio,
		Endpoint:  "stdio://local",
		Stdio: targets.StdioCommand{
			Command: "go",
			Args:    []string{"run", serverFile},
		},
		Check: targets.CheckPolicy{
			Interval:         5 * time.Second,
			Timeout:          5 * time.Second,
			Retries:          0,
			HandshakeProfile: "strict",
			// The spec-framed mock implements initialize/tools/list/resources/list
			// but not prompts/list, so the strict profile fails on a genuinely
			// missing method rather than on broken framing.
			RequiredMethods: []string{"initialize", "tools/list", "prompts/list"},
		},
	}

	res, err := prober.Probe(ctx, target, nil)
	if err != nil {
		t.Fatalf("stdio strict probe returned error: %v", err)
	}
	if res.State == targets.StateHealthy {
		t.Fatalf("expected strict handshake failure due to missing methods, got healthy (%s)", res.Message)
	}
	if res.FailureClass != targets.FailureProtocol {
		t.Fatalf("expected protocol failure class, got %s", res.FailureClass)
	}
	if !strings.Contains(strings.ToLower(res.Message), "rejected") &&
		!strings.Contains(strings.ToLower(res.Message), "method") {
		t.Fatalf("expected method rejection message, got %q", res.Message)
	}
}

// TestStdioProber_StrictHandshakePassesAgainstNewlineFramedServer pins the MCP
// stdio framing requirement from the outside: the mock only accepts
// newline-delimited JSON, so a strict handshake can only pass if the prober
// writes the spec framing and reads it back.
func TestStdioProber_StrictHandshakePassesAgainstNewlineFramedServer(t *testing.T) {
	t.Parallel()

	prober := engine.NewStdioProber()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	target := stdioMockTarget("mcp-stdio-newline-strict", mockStdioServer(t))
	target.Check.HandshakeProfile = "strict"
	target.Check.Timeout = 15 * time.Second
	target.Check.RequiredMethods = []string{"initialize", "tools/list", "resources/list"}

	res, err := prober.Probe(ctx, target, nil)
	if err != nil {
		t.Fatalf("stdio strict probe returned error: %v", err)
	}
	if res.State != targets.StateHealthy {
		t.Fatalf("expected healthy state against a newline-framed server, got %s (%s)", res.State, res.Message)
	}
	methods, _ := res.Details["exercised_methods"].([]string)
	if len(methods) != 3 {
		t.Fatalf("expected all three strict methods exercised, got %v (%s)", res.Details["exercised_methods"], res.Message)
	}
}

// TestStdioProber_ClassifiesContentLengthFramedServerAsProtocolFailure pins the
// other direction: a server that answers with LSP Content-Length headers is not
// an MCP stdio server, and the prober must say so instead of hanging until the
// timeout or misreading the header as a JSON frame.
func TestStdioProber_ClassifiesContentLengthFramedServerAsProtocolFailure(t *testing.T) {
	t.Parallel()

	prober := engine.NewStdioProber()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	target := stdioMockTarget("mcp-stdio-contentlength", mockLSPStdioServer(t))
	target.Check.Timeout = 15 * time.Second

	res, err := prober.Probe(ctx, target, nil)
	if err != nil {
		t.Fatalf("stdio probe returned error: %v", err)
	}
	if res.State == targets.StateHealthy {
		t.Fatalf("expected header-framed server to be unhealthy, got healthy (%s)", res.Message)
	}
	if res.FailureClass != targets.FailureProtocol {
		t.Fatalf("expected protocol failure class, got %s (%s)", res.FailureClass, res.Message)
	}
	msg := strings.ToLower(res.Message)
	if !strings.Contains(msg, "content-length") || !strings.Contains(msg, "newline") {
		t.Fatalf("expected a framing mismatch message naming Content-Length and newline framing, got %q", res.Message)
	}
}

func stdioMockTarget(id, serverPath string) targets.Target {
	return targets.Target{
		ID:        id,
		Enabled:   true,
		Protocol:  targets.ProtocolMCP,
		Name:      id,
		Transport: targets.TransportStdio,
		Endpoint:  "stdio://local",
		Stdio: targets.StdioCommand{
			Command: "go",
			Args:    []string{"run", serverPath},
		},
		Check: targets.CheckPolicy{
			Interval: 5 * time.Second,
			Timeout:  15 * time.Second,
		},
	}
}

func TestHTTPProber_StrictMCPHandshakePassAndRequiredMethodFailure(t *testing.T) {
	t.Parallel()

	type rpcReq struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      int             `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params,omitempty"`
	}
	type rpcErr struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	type rpcResp struct {
		JSONRPC string  `json:"jsonrpc"`
		ID      int     `json:"id"`
		Result  any     `json:"result,omitempty"`
		Error   *rpcErr `json:"error,omitempty"`
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = r.Body.Close()

		var req rpcReq
		if err := json.Unmarshal(body, &req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		resp := rpcResp{
			JSONRPC: "2.0",
			ID:      req.ID,
		}

		switch req.Method {
		case "initialize", "tools/list", "resources/list":
			resp.Result = map[string]any{"ok": true}
		default:
			resp.Error = &rpcErr{
				Code:    -32601,
				Message: "method not found",
			}
		}

		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	prober := engine.NewHTTPProber()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	passTarget := targets.Target{
		ID:        "mcp-http-strict-pass",
		Enabled:   true,
		Protocol:  targets.ProtocolMCP,
		Name:      "MCP HTTP Strict Pass",
		Transport: targets.TransportHTTP,
		Endpoint:  server.URL,
		Check: targets.CheckPolicy{
			Interval:         5 * time.Second,
			Timeout:          2 * time.Second,
			Retries:          0,
			HandshakeProfile: "strict",
			RequiredMethods:  []string{"initialize", "tools/list", "resources/list"},
		},
	}
	passRes, err := prober.Probe(ctx, passTarget, nil)
	if err != nil {
		t.Fatalf("http strict pass probe returned error: %v", err)
	}
	if passRes.State != targets.StateHealthy {
		t.Fatalf("expected strict pass target to be healthy, got %s (%s)", passRes.State, passRes.Message)
	}

	failTarget := passTarget
	failTarget.ID = "mcp-http-strict-fail"
	failTarget.Check.RequiredMethods = []string{"initialize", "tools/list", "resources/list", "methods/that-do-not-exist"}

	failRes, err := prober.Probe(ctx, failTarget, nil)
	if err != nil {
		t.Fatalf("http strict fail probe returned error: %v", err)
	}
	if failRes.State == targets.StateHealthy {
		t.Fatalf("expected strict fail target to be unhealthy, got healthy")
	}
	if failRes.FailureClass != targets.FailureProtocol {
		t.Fatalf("expected protocol failure class, got %s", failRes.FailureClass)
	}
	if !strings.Contains(strings.ToLower(failRes.Message), "rejected") &&
		!strings.Contains(strings.ToLower(failRes.Message), "failed") {
		t.Fatalf("expected rejection/failure message, got %q", failRes.Message)
	}
}

func TestSafetyScanner_RegisterAndRunEndpointPass(t *testing.T) {
	t.Parallel()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer api.Close()

	scanner := safety.NewScanner()

	target := targets.Target{
		ID:       "safety-endpoint-pass",
		Endpoint: api.URL,
		Check: targets.CheckPolicy{
			HURLTests: []targets.HURLTest{
				{
					Name:     "basic-http-smoke",
					Endpoint: api.URL,
					Method:   http.MethodGet,
				},
			},
		},
	}

	if err := scanner.RegisterTarget(target); err != nil {
		t.Fatalf("RegisterTarget failed: %v", err)
	}
	if !scanner.HasRegistered(target.ID) {
		t.Fatalf("expected target to be registered")
	}

	report := scanner.RunTarget(context.Background(), target)
	if !report.AnyTestsSeen {
		t.Fatalf("expected AnyTestsSeen=true")
	}
	if report.HasBlocking {
		t.Fatalf("expected no blocking failures")
	}
	if report.Passed != 1 || report.Failed != 0 || report.Skipped != 0 {
		t.Fatalf("unexpected report counts: passed=%d failed=%d skipped=%d", report.Passed, report.Failed, report.Skipped)
	}
	if len(report.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(report.Results))
	}
	if report.Results[0].Outcome != safety.OutcomePass {
		t.Fatalf("expected pass outcome, got %s", report.Results[0].Outcome)
	}
}

func TestSafetyScanner_BlockingFailureOnEndpointError(t *testing.T) {
	t.Parallel()

	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"ok":false}`))
	}))
	defer api.Close()

	scanner := safety.NewScanner()

	target := targets.Target{
		ID:       "safety-endpoint-fail",
		Endpoint: api.URL,
		Check: targets.CheckPolicy{
			HURLTests: []targets.HURLTest{
				{
					Name:     "should-fail-on-500",
					Endpoint: api.URL,
					Method:   http.MethodGet,
				},
			},
		},
	}

	if err := scanner.RegisterTarget(target); err != nil {
		t.Fatalf("RegisterTarget failed: %v", err)
	}

	report := scanner.RunTarget(context.Background(), target)
	if !report.HasBlocking {
		t.Fatalf("expected blocking failure")
	}
	if report.Failed != 1 {
		t.Fatalf("expected 1 failed test, got %d", report.Failed)
	}
	if len(report.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(report.Results))
	}
	got := report.Results[0]
	if got.Outcome != safety.OutcomeFail {
		t.Fatalf("expected fail outcome, got %s", got.Outcome)
	}
	if got.FailureClass != targets.FailureProtocol {
		t.Fatalf("expected protocol failure class, got %s", got.FailureClass)
	}
}

func TestSafetyScanner_FileRegistrationWithRunner(t *testing.T) {
	t.Parallel()

	mockFile := filepath.Join(t.TempDir(), "smoke.hurl")
	if err := os.WriteFile(mockFile, []byte("GET {{ENDPOINT}}\nHTTP 200\n"), 0o644); err != nil {
		t.Fatalf("write mock hurl file: %v", err)
	}

	runner := &mockHURLRunner{}
	scanner := safety.NewScanner(
		safety.WithCommandRunner(runner),
		safety.WithHURLBinary("hurl"),
	)

	target := targets.Target{
		ID:       "safety-file-pass",
		Endpoint: "http://localhost:9999/healthz",
		Check: targets.CheckPolicy{
			HURLTests: []targets.HURLTest{
				{
					Name: "file-smoke-test",
					File: mockFile,
				},
			},
		},
	}

	if err := scanner.RegisterTarget(target); err != nil {
		t.Fatalf("RegisterTarget failed: %v", err)
	}

	report := scanner.RunTarget(context.Background(), target)
	if report.HasBlocking {
		t.Fatalf("expected no blocking failures")
	}
	if report.Passed != 1 {
		t.Fatalf("expected 1 pass, got %d", report.Passed)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected runner call count=1, got %d", len(runner.calls))
	}

	call := runner.calls[0]
	if call.name != "hurl" {
		t.Fatalf("expected hurl binary invocation, got %q", call.name)
	}
	if !contains(call.args, "--test") || !contains(call.args, mockFile) {
		t.Fatalf("expected --test and file args, got %v", call.args)
	}
}

type runnerCall struct {
	name string
	args []string
}

type mockHURLRunner struct {
	calls []runnerCall
}

func (m *mockHURLRunner) Run(_ context.Context, name string, args ...string) (string, string, error) {
	m.calls = append(m.calls, runnerCall{name: name, args: append([]string(nil), args...)})
	// Simulate successful hurl invocation.
	return "ok", "", nil
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}

// mockStdioServer returns the path to the shared stdio mock, which speaks the
// spec framing: one newline-delimited JSON-RPC message per line and no headers.
// tests/testdata/mock_stdio_server refuses Content-Length frames outright, so a
// prober that regressed to LSP framing fails here instead of proving itself
// correct against its own wire format (issue #43).
func mockStdioServer(t *testing.T) string {
	t.Helper()
	return mockServerPath(t, "mock_stdio_server")
}

// mockLSPStdioServer returns the path to the negative twin, which replies with
// LSP Content-Length framing. Nothing may complete a handshake against it.
func mockLSPStdioServer(t *testing.T) string {
	t.Helper()
	return mockServerPath(t, "mock_lsp_stdio_server")
}

func mockServerPath(t *testing.T, name string) string {
	t.Helper()
	dir := filepath.Join("..", "testdata", name)
	if _, err := os.Stat(filepath.Join(dir, "main.go")); err != nil {
		t.Fatalf("mock stdio server %q not found: %v", name, err)
	}
	return dir
}

// compile-time guards for API drift.
var (
	_ safety.CommandRunner = (*mockHURLRunner)(nil)
	_                      = errors.New
	_                      = fmt.Sprintf
)
