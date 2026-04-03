package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/hosted"
)

func TestTunerIntegration_AudienceEndpoint(t *testing.T) {
	t.Parallel()

	addr := freeTCPAddr(t)
	srv := hosted.NewServer(hosted.Options{
		ServiceName: "itest-tuner",
		Version:     "test",
		ListenAddr:  addr,
		EnableHTTP:  true,
		EnableMCP:   true,
		MCPEndpoint: "/mcp",
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- srv.Start(ctx) }()

	baseURL := "http://" + addr
	waitForStatus(t, baseURL+"/hosted/status", http.StatusOK, 3*time.Second)

	// POST audience metric.
	body, _ := json.Marshal(map[string]any{
		"channel": "ntp",
		"count":   5,
		"signal":  0.75,
	})
	resp, err := http.Post(baseURL+"/tuner/audience", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post audience: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var postResp map[string]string
	json.NewDecoder(resp.Body).Decode(&postResp)
	if postResp["status"] != "ok" {
		t.Fatalf("expected status ok, got %q", postResp["status"])
	}

	// GET audience metrics.
	getResp, err := http.Get(baseURL + "/tuner/audience")
	if err != nil {
		t.Fatalf("get audience: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", getResp.StatusCode)
	}

	var getBody map[string]any
	json.NewDecoder(getResp.Body).Decode(&getBody)
	audience, ok := getBody["audience"].([]any)
	if !ok || len(audience) != 1 {
		t.Fatalf("expected 1 audience entry, got %v", getBody["audience"])
	}

	cancel()
	select {
	case srvErr := <-done:
		if srvErr != nil && !errors.Is(srvErr, context.Canceled) {
			t.Fatalf("server error: %v", srvErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not shut down")
	}
}

func TestTunerIntegration_CallerPostAndResponse(t *testing.T) {
	t.Parallel()

	addr := freeTCPAddr(t)
	srv := hosted.NewServer(hosted.Options{
		ServiceName: "itest-tuner-caller",
		Version:     "test",
		ListenAddr:  addr,
		EnableHTTP:  true,
		EnableMCP:   true,
		MCPEndpoint: "/mcp",
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- srv.Start(ctx) }()

	baseURL := "http://" + addr
	waitForStatus(t, baseURL+"/hosted/status", http.StatusOK, 3*time.Second)

	// POST caller message.
	body, _ := json.Marshal(map[string]any{
		"message":  "hello from viewer",
		"from":     "viewer1",
		"priority": "normal",
	})
	resp, err := http.Post(baseURL+"/tuner/caller/ntp", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("post caller: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var callerResp map[string]any
	json.NewDecoder(resp.Body).Decode(&callerResp)
	if callerResp["status"] != "received" {
		t.Fatalf("expected status received, got %v", callerResp["status"])
	}
	if callerResp["channel"] != "ntp" {
		t.Fatalf("expected channel ntp, got %v", callerResp["channel"])
	}
	// No subscribers, should be 0.
	subs, _ := callerResp["subscribers"].(float64)
	if subs != 0 {
		t.Fatalf("expected 0 subscribers, got %v", subs)
	}

	cancel()
	select {
	case srvErr := <-done:
		if srvErr != nil && !errors.Is(srvErr, context.Canceled) {
			t.Fatalf("server error: %v", srvErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not shut down")
	}
}

func TestTunerIntegration_MCPToolsListIncludesTunerTools(t *testing.T) {
	t.Parallel()

	addr := freeTCPAddr(t)
	srv := hosted.NewServer(hosted.Options{
		ServiceName: "itest-tuner-mcp",
		Version:     "test",
		ListenAddr:  addr,
		EnableHTTP:  true,
		EnableMCP:   true,
		MCPEndpoint: "/mcp",
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- srv.Start(ctx) }()

	baseURL := "http://" + addr
	waitForStatus(t, baseURL+"/hosted/status", http.StatusOK, 3*time.Second)

	// Initialize MCP session.
	var initResp rpcResponse
	postRPC(t, baseURL+"/mcp", rpcRequest{
		JSONRPC: "2.0",
		ID:      intPtr(1),
		Method:  "initialize",
		Params: map[string]any{
			"clientInfo": map[string]any{"name": "itest", "version": "0.1"},
		},
	}, &initResp)
	if initResp.Error != nil {
		t.Fatalf("mcp init error: %+v", initResp.Error)
	}

	// List tools.
	var toolsResp rpcResponse
	postRPC(t, baseURL+"/mcp", rpcRequest{
		JSONRPC: "2.0",
		ID:      intPtr(2),
		Method:  "tools/list",
		Params:  map[string]any{},
	}, &toolsResp)
	if toolsResp.Error != nil {
		t.Fatalf("tools/list error: %+v", toolsResp.Error)
	}

	// Parse tools result.
	result, ok := toolsResp.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", toolsResp.Result)
	}
	tools, ok := result["tools"].([]any)
	if !ok {
		t.Fatalf("expected tools array, got %T", result["tools"])
	}

	toolNames := map[string]bool{}
	for _, tool := range tools {
		tm, ok := tool.(map[string]any)
		if !ok {
			continue
		}
		if name, ok := tm["name"].(string); ok {
			toolNames[name] = true
		}
	}

	expected := []string{
		"smoke.health",
		"smoke.tuner_list_channels",
		"smoke.tuner_audience",
		"smoke.tuner_caller_messages",
	}
	for _, name := range expected {
		if !toolNames[name] {
			t.Errorf("missing MCP tool: %q", name)
		}
	}

	cancel()
	select {
	case srvErr := <-done:
		if srvErr != nil && !errors.Is(srvErr, context.Canceled) {
			t.Fatalf("server error: %v", srvErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not shut down")
	}
}

func TestTunerIntegration_AudienceMethodNotAllowed(t *testing.T) {
	t.Parallel()

	addr := freeTCPAddr(t)
	srv := hosted.NewServer(hosted.Options{
		ServiceName: "itest-tuner-method",
		Version:     "test",
		ListenAddr:  addr,
		EnableHTTP:  true,
		EnableMCP:   true,
		MCPEndpoint: "/mcp",
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- srv.Start(ctx) }()

	baseURL := "http://" + addr
	waitForStatus(t, baseURL+"/hosted/status", http.StatusOK, 3*time.Second)

	// DELETE should be 405.
	req, _ := http.NewRequest("DELETE", baseURL+"/tuner/audience", http.NoBody)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", resp.StatusCode)
	}

	cancel()
	select {
	case srvErr := <-done:
		if srvErr != nil && !errors.Is(srvErr, context.Canceled) {
			t.Fatalf("server error: %v", srvErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not shut down")
	}
}

func TestTunerIntegration_CallerBadRequest(t *testing.T) {
	t.Parallel()

	addr := freeTCPAddr(t)
	srv := hosted.NewServer(hosted.Options{
		ServiceName: "itest-tuner-bad",
		Version:     "test",
		ListenAddr:  addr,
		EnableHTTP:  true,
		EnableMCP:   true,
		MCPEndpoint: "/mcp",
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- srv.Start(ctx) }()

	baseURL := "http://" + addr
	waitForStatus(t, baseURL+"/hosted/status", http.StatusOK, 3*time.Second)

	// POST bad JSON to caller.
	resp, err := http.Post(baseURL+"/tuner/caller/ntp", "application/json", bytes.NewReader([]byte("not json")))
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}

	cancel()
	select {
	case srvErr := <-done:
		if srvErr != nil && !errors.Is(srvErr, context.Canceled) {
			t.Fatalf("server error: %v", srvErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not shut down")
	}
}

func TestTunerIntegration_EventsAppear(t *testing.T) {
	t.Parallel()

	addr := freeTCPAddr(t)
	srv := hosted.NewServer(hosted.Options{
		ServiceName: "itest-tuner-events",
		Version:     "test",
		ListenAddr:  addr,
		EnableHTTP:  true,
		EnableSSE:   true,
		EnableMCP:   true,
		MCPEndpoint: "/mcp",
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- srv.Start(ctx) }()

	baseURL := "http://" + addr
	waitForStatus(t, baseURL+"/hosted/status", http.StatusOK, 3*time.Second)

	// Post audience and caller to generate events.
	body1, _ := json.Marshal(map[string]any{"channel": "test", "count": 1, "signal": 1.0})
	if resp1, err := http.Post(baseURL+"/tuner/audience", "application/json", bytes.NewReader(body1)); err == nil {
		_ = resp1.Body.Close()
	}

	body2, _ := json.Marshal(map[string]any{"message": "hi", "from": "tester"})
	if resp2, err := http.Post(baseURL+"/tuner/caller/test", "application/json", bytes.NewReader(body2)); err == nil {
		_ = resp2.Body.Close()
	}

	// Verify events via hosted/status which includes recent_events.
	statusResp, err := http.Get(baseURL + "/hosted/status")
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	defer statusResp.Body.Close()

	var statusBody map[string]any
	json.NewDecoder(statusResp.Body).Decode(&statusBody)

	// Check recent_events for tuner protocol events.
	events, ok := statusBody["recent_events"].([]any)
	if !ok {
		t.Fatalf("expected recent_events array, got %T", statusBody["recent_events"])
	}
	tunerEvents := 0
	for _, ev := range events {
		em, ok := ev.(map[string]any)
		if !ok {
			continue
		}
		if em["protocol"] == "tuner" {
			tunerEvents++
		}
	}
	if tunerEvents < 2 {
		t.Fatalf("expected at least 2 tuner events, got %d (total events: %d)", tunerEvents, len(events))
	}

	cancel()
	select {
	case srvErr := <-done:
		if srvErr != nil && !errors.Is(srvErr, context.Canceled) {
			t.Fatalf("server error: %v", srvErr)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("server did not shut down")
	}
}
