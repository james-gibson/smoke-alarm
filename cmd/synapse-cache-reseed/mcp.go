package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// mcpProtocolVersion is the Model Context Protocol revision this client
// negotiates with Synapse over the Streamable HTTP transport.
const mcpProtocolVersion = "2024-11-05"

// mcpClient is a minimal MCP client for the Streamable HTTP transport used by
// Synapse MCP. Synapse answers with plain JSON, but SSE responses are decoded
// too so the client keeps working if the server switches to streaming.
type mcpClient struct {
	endpoint  string
	http      *http.Client
	sessionID string
	nextID    int
}

func newMCPClient(endpoint string, timeout time.Duration) *mcpClient {
	return &mcpClient{
		endpoint: endpoint,
		http: &http.Client{
			Timeout: timeout,
			// Open a fresh connection per request. Synapse may close an idle
			// keep-alive connection after a write, and Go does not retry a
			// non-idempotent POST on a reused connection, so a reseed would
			// otherwise fail with a spurious EOF.
			Transport: &http.Transport{DisableKeepAlives: true},
		},
	}
}

// dialMCP opens an MCP session and completes the initialize handshake.
func dialMCP(ctx context.Context, endpoint string, timeout time.Duration) (*mcpClient, error) {
	client := newMCPClient(endpoint, timeout)
	if err := client.initialize(ctx); err != nil {
		return nil, err
	}
	return client, nil
}

func (c *mcpClient) initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": mcpProtocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    appName,
			"version": version,
		},
	}
	if _, err := c.call(ctx, "initialize", params, false); err != nil {
		return fmt.Errorf("initialize: %w", err)
	}
	if _, err := c.call(ctx, "notifications/initialized", nil, true); err != nil {
		return fmt.Errorf("initialized notification: %w", err)
	}
	return nil
}

// call sends one JSON-RPC message, retrying transient connection failures.
func (c *mcpClient) call(ctx context.Context, method string, params any, notify bool) (json.RawMessage, error) {
	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		result, err := c.callOnce(ctx, method, params, notify)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !isTransient(err) {
			return nil, err
		}
		if attempt < maxAttempts {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 250 * time.Millisecond):
			}
		}
	}
	return nil, lastErr
}

// callOnce sends one JSON-RPC message without retrying. When notify is true it
// sends a notification (no id) and discards the response body.
func (c *mcpClient) callOnce(ctx context.Context, method string, params any, notify bool) (json.RawMessage, error) {
	request := rpcRequest{JSONRPC: "2.0", Method: method, Params: params}
	if !notify {
		c.nextID++
		request.ID = c.nextID
	}
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("marshal %s request: %w", method, err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build %s request: %w", method, err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json, text/event-stream")
	if c.sessionID != "" {
		httpReq.Header.Set("mcp-session-id", c.sessionID)
	}
	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send %s request: %w", method, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if session := resp.Header.Get("mcp-session-id"); session != "" {
		c.sessionID = session
	}
	if notify {
		_, _ = io.Copy(io.Discard, resp.Body)
		if resp.StatusCode >= http.StatusBadRequest {
			return nil, fmt.Errorf("%s: unexpected status %s", method, resp.Status)
		}
		return nil, nil
	}
	if strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		return readSSEResult(resp.Body)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s response: %w", method, err)
	}
	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("%s: unexpected status %s: %s", method, resp.Status, strings.TrimSpace(string(data)))
	}
	var out rpcResponse
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", method, err)
	}
	if out.Error != nil {
		return nil, fmt.Errorf("%s: rpc error %d: %s", method, out.Error.Code, out.Error.Message)
	}
	return out.Result, nil
}

func (c *mcpClient) callTool(ctx context.Context, name string, args map[string]any) (any, error) {
	params := map[string]any{"name": name, "arguments": args}
	raw, err := c.call(ctx, "tools/call", params, false)
	if err != nil {
		return nil, err
	}
	return decodeToolResult(name, raw)
}

// callToolInto decodes a tool result into out, which must be a pointer.
func (c *mcpClient) callToolInto(ctx context.Context, name string, args map[string]any, out any) error {
	value, err := c.callTool(ctx, name, args)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("re-encode %s result: %w", name, err)
	}
	if err := json.Unmarshal(encoded, out); err != nil {
		return fmt.Errorf("decode %s result: %w", name, err)
	}
	return nil
}

// callToolNoResult invokes a tool when only its success or failure matters.
func (c *mcpClient) callToolNoResult(ctx context.Context, name string, args map[string]any) error {
	_, err := c.callTool(ctx, name, args)
	return err
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id,omitempty"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int             `json:"id"`
	Result  json.RawMessage `json:"result"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolEnvelope struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	StructuredContent json.RawMessage `json:"structuredContent"`
	IsError           bool            `json:"isError"`
}

// decodeToolResult unwraps a tools/call result. Synapse returns its payload as
// JSON encoded in the first text content block; structuredContent is honored
// when present.
func decodeToolResult(name string, raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var env toolEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("decode %s result: %w", name, err)
	}
	if env.IsError {
		return nil, fmt.Errorf("%s returned an error: %s", name, firstContentText(env))
	}
	if len(env.StructuredContent) > 0 && string(env.StructuredContent) != "null" {
		var structured any
		if err := json.Unmarshal(env.StructuredContent, &structured); err != nil {
			return nil, fmt.Errorf("decode %s structuredContent: %w", name, err)
		}
		return structured, nil
	}
	text := firstContentText(env)
	if text == "" {
		return nil, nil
	}
	return decodeTextPayload(text), nil
}

// decodeTextPayload parses a tool's text content as JSON, falling back to the
// raw string when it is not JSON.
func decodeTextPayload(text string) any {
	var value any
	if err := json.Unmarshal([]byte(text), &value); err != nil {
		return text
	}
	return value
}

func firstContentText(env toolEnvelope) string {
	for _, item := range env.Content {
		if item.Type == "text" && item.Text != "" {
			return item.Text
		}
	}
	return ""
}

// readSSEResult extracts the JSON-RPC result from a text/event-stream body.
func readSSEResult(body io.Reader) (json.RawMessage, error) {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" {
			continue
		}
		var out rpcResponse
		if err := json.Unmarshal([]byte(payload), &out); err != nil {
			continue
		}
		if out.Error != nil {
			return nil, fmt.Errorf("rpc error %d: %s", out.Error.Code, out.Error.Message)
		}
		if len(out.Result) > 0 {
			return out.Result, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read event stream: %w", err)
	}
	return nil, fmt.Errorf("no JSON-RPC result in event stream")
}

// isTransient reports whether err is a connection-level failure worth retrying.
func isTransient(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}
