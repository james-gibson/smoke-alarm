package integration_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/hosted"
)

func TestHostedServer_MCPACPHTTPJSONRPC(t *testing.T) {
	t.Parallel()

	addr := freeTCPAddr(t)
	srv := hosted.NewServer(hosted.Options{
		ServiceName: "itest-hosted",
		Version:     "test",
		ListenAddr:  addr,

		EnableHTTP: true,
		EnableSSE:  false,

		EnableMCP: true,
		EnableACP: true,
		EnableA2A: false,

		MCPEndpoint: "/mcp",
		ACPEndpoint: "/acp",
		A2AEndpoint: "/a2a",
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- srv.Start(ctx)
	}()

	baseURL := "http://" + addr
	waitForStatus(t, baseURL+"/hosted/status", http.StatusOK, 3*time.Second)

	// MCP initialize
	mcpInit := rpcRequest{
		JSONRPC: "2.0",
		ID:      intPtr(1),
		Method:  "initialize",
		Params: map[string]any{
			"clientInfo": map[string]any{"name": "itest", "version": "0.0.1"},
		},
	}
	var mcpInitResp rpcResponse
	postRPC(t, baseURL+"/mcp", mcpInit, &mcpInitResp)
	if mcpInitResp.Error != nil {
		t.Fatalf("mcp initialize rpc error: %+v", mcpInitResp.Error)
	}

	// MCP tools/list
	mcpTools := rpcRequest{
		JSONRPC: "2.0",
		ID:      intPtr(2),
		Method:  "tools/list",
		Params:  map[string]any{},
	}
	var mcpToolsResp rpcResponse
	postRPC(t, baseURL+"/mcp", mcpTools, &mcpToolsResp)
	if mcpToolsResp.Error != nil {
		t.Fatalf("mcp tools/list rpc error: %+v", mcpToolsResp.Error)
	}

	// ACP session/setup
	acpSetup := rpcRequest{
		JSONRPC: "2.0",
		ID:      intPtr(3),
		Method:  "session/setup",
		Params: map[string]any{
			"clientInfo": map[string]any{"name": "itest", "version": "0.0.1"},
		},
	}
	var acpSetupResp rpcResponse
	postRPC(t, baseURL+"/acp", acpSetup, &acpSetupResp)
	if acpSetupResp.Error != nil {
		t.Fatalf("acp session/setup rpc error: %+v", acpSetupResp.Error)
	}

	// ACP prompt/turn
	acpTurn := rpcRequest{
		JSONRPC: "2.0",
		ID:      intPtr(4),
		Method:  "prompt/turn",
		Params: map[string]any{
			"prompt": "health check",
			"input":  "ping",
		},
	}
	var acpTurnResp rpcResponse
	postRPC(t, baseURL+"/acp", acpTurn, &acpTurnResp)
	if acpTurnResp.Error != nil {
		t.Fatalf("acp prompt/turn rpc error: %+v", acpTurnResp.Error)
	}

	cancel()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("hosted server returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("hosted server did not shut down in time")
	}
}

func TestHostedServer_SSEEndpoints(t *testing.T) {
	t.Parallel()

	addr := freeTCPAddr(t)
	srv := hosted.NewServer(hosted.Options{
		ServiceName: "itest-hosted-sse",
		Version:     "test",
		ListenAddr:  addr,

		EnableHTTP: true,
		EnableSSE:  true,

		EnableMCP: true,
		EnableACP: true,
		EnableA2A: false,

		MCPEndpoint: "/mcp",
		ACPEndpoint: "/acp",
		A2AEndpoint: "/a2a",
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- srv.Start(ctx)
	}()

	baseURL := "http://" + addr
	waitForStatus(t, baseURL+"/hosted/status", http.StatusOK, 3*time.Second)

	// Verify MCP SSE stream emits ready event quickly.
	reqCtx, reqCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer reqCancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, baseURL+"/mcp?transport=sse", http.NoBody)
	if err != nil {
		t.Fatalf("create sse request: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do sse request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from sse endpoint, got %d", resp.StatusCode)
	}
	if ct := strings.ToLower(resp.Header.Get("Content-Type")); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected text/event-stream content-type, got %q", ct)
	}

	reader := bufio.NewReader(resp.Body)
	line1, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read first sse line: %v", err)
	}
	line2, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read second sse line: %v", err)
	}

	if !strings.Contains(strings.ToLower(line1), "event: ready") {
		t.Fatalf("expected ready event line, got %q", line1)
	}
	if !strings.Contains(strings.ToLower(line2), "data:") {
		t.Fatalf("expected data line, got %q", line2)
	}

	// Close active SSE stream before shutting down hosted server to allow graceful exit.
	_ = resp.Body.Close()
	reqCancel()

	cancel()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("hosted server returned unexpected error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("hosted server did not shut down in time")
	}
}

func TestHostedServer_EventsSSEAndCountersVisibility(t *testing.T) {
	t.Parallel()

	addr := freeTCPAddr(t)
	srv := hosted.NewServer(hosted.Options{
		ServiceName: "itest-hosted-events",
		Version:     "test",
		ListenAddr:  addr,

		EnableHTTP: true,
		EnableSSE:  true,

		EnableMCP: true,
		EnableACP: true,
		EnableA2A: false,

		MCPEndpoint: "/mcp",
		ACPEndpoint: "/acp",
		A2AEndpoint: "/a2a",
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- srv.Start(ctx)
	}()

	baseURL := "http://" + addr
	waitForStatus(t, baseURL+"/hosted/status", http.StatusOK, 3*time.Second)

	// Generate demo-style traffic so counters/events have meaningful content.
	var mcpInitResp rpcResponse
	postRPC(t, baseURL+"/mcp", rpcRequest{
		JSONRPC: "2.0",
		ID:      intPtr(11),
		Method:  "initialize",
		Params: map[string]any{
			"clientInfo": map[string]any{"name": "itest-events", "version": "0.0.1"},
		},
	}, &mcpInitResp)
	if mcpInitResp.Error != nil {
		t.Fatalf("mcp initialize rpc error: %+v", mcpInitResp.Error)
	}

	var acpSetupResp rpcResponse
	postRPC(t, baseURL+"/acp", rpcRequest{
		JSONRPC: "2.0",
		ID:      intPtr(12),
		Method:  "session/setup",
		Params: map[string]any{
			"clientInfo": map[string]any{"name": "itest-events", "version": "0.0.1"},
		},
	}, &acpSetupResp)
	if acpSetupResp.Error != nil {
		t.Fatalf("acp session/setup rpc error: %+v", acpSetupResp.Error)
	}

	// Connect to hosted events SSE feed.
	eventsCtx, eventsCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer eventsCancel()

	req, err := http.NewRequestWithContext(eventsCtx, http.MethodGet, baseURL+"/hosted/events?transport=sse", http.NoBody)
	if err != nil {
		t.Fatalf("create hosted events sse request: %v", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("open hosted events sse stream: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 from hosted events sse, got %d", resp.StatusCode)
	}
	if ct := strings.ToLower(resp.Header.Get("Content-Type")); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected text/event-stream content-type, got %q", ct)
	}

	reader := bufio.NewReader(resp.Body)
	lineA, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read first events sse line: %v", err)
	}
	lineB, err := reader.ReadString('\n')
	if err != nil {
		t.Fatalf("read second events sse line: %v", err)
	}
	if !strings.Contains(strings.ToLower(lineA), "event:") {
		t.Fatalf("expected event line, got %q", lineA)
	}
	if !strings.Contains(strings.ToLower(lineB), "data:") {
		t.Fatalf("expected data line, got %q", lineB)
	}

	// Verify counters are visible and incremented.
	statusResp, err := http.Get(baseURL + "/hosted/status")
	if err != nil {
		t.Fatalf("get hosted status: %v", err)
	}
	defer statusResp.Body.Close()
	if statusResp.StatusCode != http.StatusOK {
		t.Fatalf("expected hosted status 200, got %d", statusResp.StatusCode)
	}

	var payload map[string]any
	if err := json.NewDecoder(statusResp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode hosted status payload: %v", err)
	}

	countersAny, ok := payload["counters"]
	if !ok {
		t.Fatalf("expected counters in hosted status payload: %v", payload)
	}
	counters, ok := countersAny.(map[string]any)
	if !ok {
		t.Fatalf("expected counters object in hosted status payload: %T", countersAny)
	}

	total, _ := counters["total"].(float64)
	_ = total // TODO: assert total count
}

func TestHostedServer_MCPStreamableHTTPSession(t *testing.T) {
	t.Parallel()

	addr := freeAddr(t)
	srv := hosted.NewServer(hosted.Options{
		ServiceName: "itest-streamable",
		Version:     "test",
		ListenAddr:  addr,
		EnableHTTP:  true,
		EnableSSE:   true,
		EnableMCP:   true,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Start(ctx) }()

	baseURL := "http://" + addr
	waitForStatus(t, baseURL+"/hosted/status", http.StatusOK, 3*time.Second)

	// Step 1: POST initialize — should get Mcp-Session-Id header back.
	initReq := rpcRequest{
		JSONRPC: "2.0",
		ID:      intPtr(1),
		Method:  "initialize",
		Params: map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo":      map[string]any{"name": "generic-client", "version": "1.0"},
		},
	}
	initBody, _ := json.Marshal(initReq)
	resp, err := http.Post(baseURL+"/mcp", "application/json", strings.NewReader(string(initBody)))
	if err != nil {
		t.Fatalf("POST initialize: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, raw)
	}

	sessionID := resp.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		t.Fatal("expected Mcp-Session-Id header on initialize response, got empty")
	}
	t.Logf("session ID: %s", sessionID)

	var initResp rpcResponse
	if decodeErr := json.NewDecoder(resp.Body).Decode(&initResp); decodeErr != nil {
		t.Fatalf("decode initialize response: %v", decodeErr)
	}
	if initResp.Error != nil {
		t.Fatalf("initialize returned error: %+v", initResp.Error)
	}

	// Step 2: POST tools/list with session header.
	var toolsResp rpcResponse
	postRPCWithSession(t, baseURL+"/mcp", sessionID, rpcRequest{
		JSONRPC: "2.0",
		ID:      intPtr(2),
		Method:  "tools/list",
		Params:  map[string]any{},
	}, &toolsResp)
	if toolsResp.Error != nil {
		t.Fatalf("tools/list returned error: %+v", toolsResp.Error)
	}

	// Step 3: DELETE session.
	delReq, _ := http.NewRequest(http.MethodDelete, baseURL+"/mcp", http.NoBody)
	delReq.Header.Set("Mcp-Session-Id", sessionID)
	delResp, err := (&http.Client{Timeout: 3 * time.Second}).Do(delReq)
	if err != nil {
		t.Fatalf("DELETE session: %v", err)
	}
	delResp.Body.Close()
	if delResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on session delete, got %d", delResp.StatusCode)
	}

	// Step 4: DELETE again should 404.
	delReq2, _ := http.NewRequest(http.MethodDelete, baseURL+"/mcp", http.NoBody)
	delReq2.Header.Set("Mcp-Session-Id", sessionID)
	delResp2, err := (&http.Client{Timeout: 3 * time.Second}).Do(delReq2)
	if err != nil {
		t.Fatalf("DELETE session again: %v", err)
	}
	delResp2.Body.Close()
	if delResp2.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 on deleted session, got %d", delResp2.StatusCode)
	}
}

func postRPCWithSession(t *testing.T, url, sessionID string, in rpcRequest, out *rpcResponse) {
	t.Helper()
	b, _ := json.Marshal(in)
	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	resp, err := (&http.Client{Timeout: 3 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("rpc request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, raw)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      *int   `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string        `json:"jsonrpc"`
	ID      *int          `json:"id,omitempty"`
	Result  any           `json:"result,omitempty"`
	Error   *rpcRespError `json:"error,omitempty"`
}

type rpcRespError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func postRPC(t *testing.T, url string, in rpcRequest, out *rpcResponse) {
	t.Helper()

	b, err := json.Marshal(in)
	if err != nil {
		t.Fatalf("marshal rpc request: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, url, strings.NewReader(string(b)))
	if err != nil {
		t.Fatalf("new rpc request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("do rpc request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected status 200, got %d body=%s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("decode rpc response: %v", err)
	}
}

func waitForStatus(t *testing.T, url string, want int, timeout time.Duration) {
	t.Helper()

	client := &http.Client{Timeout: 300 * time.Millisecond}
	deadline := time.Now().Add(timeout)

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
		t.Fatalf("wait for status %d failed: %v", want, lastErr)
	}
	t.Fatalf("wait for status %d failed: last code=%d", want, lastCode)
}

func freeTCPAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen free addr: %v", err)
	}
	defer ln.Close()
	return ln.Addr().String()
}

func intPtr(v int) *int { return &v }
