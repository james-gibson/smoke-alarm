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
	"os/exec"
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

	serverFile := writeMockStdioServer(t)

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

	serverFile := writeMockStdioServer(t)

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
			RequiredMethods:  []string{"initialize", "tools/list", "resources/list"},
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
		Endpoint: "http://127.0.0.1:9999/healthz",
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

func writeMockStdioServer(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	file := filepath.Join(dir, "main.go")

	const src = `package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type req struct {
	JSONRPC string          ` + "`json:\"jsonrpc\"`" + `
	ID      int             ` + "`json:\"id\"`" + `
	Method  string          ` + "`json:\"method\"`" + `
	Params  json.RawMessage ` + "`json:\"params,omitempty\"`" + `
}

type rpcErr struct {
	Code    int    ` + "`json:\"code\"`" + `
	Message string ` + "`json:\"message\"`" + `
}

type resp struct {
	JSONRPC string   ` + "`json:\"jsonrpc\"`" + `
	ID      int      ` + "`json:\"id\"`" + `
	Result  any      ` + "`json:\"result,omitempty\"`" + `
	Error   *rpcErr  ` + "`json:\"error,omitempty\"`" + `
}

func main() {
	r := bufio.NewReader(os.Stdin)
	w := bufio.NewWriter(os.Stdout)
	defer w.Flush()

	for {
		body, err := readFrame(r)
		if err != nil {
			if err == io.EOF {
				return
			}
			return
		}

		var in req
		if err := json.Unmarshal(body, &in); err != nil {
			_ = writeFrame(w, resp{
				JSONRPC: "2.0",
				ID:      0,
				Error: &rpcErr{
					Code:    -32700,
					Message: "parse error",
				},
			})
			continue
		}

		switch in.Method {
		case "initialize", "session/setup":
			_ = writeFrame(w, resp{
				JSONRPC: "2.0",
				ID:      in.ID,
				Result: map[string]any{
					"protocolVersion": "2024-11-05",
					"capabilities":    map[string]any{},
					"serverInfo": map[string]any{
						"name":    "mock-stdio-server",
						"version": "0.0.1",
					},
				},
			})
		default:
			_ = writeFrame(w, resp{
				JSONRPC: "2.0",
				ID:      in.ID,
				Error: &rpcErr{
					Code:    -32601,
					Message: "method not found",
				},
			})
		}
	}
}

func readFrame(r *bufio.Reader) ([]byte, error) {
	contentLen := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		lower := strings.ToLower(line)
		if strings.HasPrefix(lower, "content-length:") {
			raw := strings.TrimSpace(line[len("Content-Length:"):])
			n, err := strconv.Atoi(raw)
			if err != nil {
				return nil, err
			}
			contentLen = n
		}
	}
	if contentLen < 0 {
		return nil, fmt.Errorf("missing content-length")
	}
	buf := make([]byte, contentLen)
	_, err := io.ReadFull(r, buf)
	return buf, err
}

func writeFrame(w *bufio.Writer, out resp) error {
	b, err := json.Marshal(out)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(b)); err != nil {
		return err
	}
	if _, err := w.Write(b); err != nil {
		return err
	}
	return w.Flush()
}
`

	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write mock stdio server: %v", err)
	}

	// quick compile sanity for clearer failures
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", file)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("mock stdio server source invalid: %v (%s)", err, strings.TrimSpace(string(out)))
	}

	return file
}

// compile-time guards for API drift.
var (
	_ safety.CommandRunner = (*mockHURLRunner)(nil)
	_                      = errors.New
	_                      = fmt.Sprintf
)
