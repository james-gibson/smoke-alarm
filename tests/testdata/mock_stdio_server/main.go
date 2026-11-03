// mock_stdio_server speaks the MCP stdio transport as the specification
// defines it: one JSON-RPC message per line, no framing headers.
//
// MCP § transports/stdio (2025-06-18): "Messages are delimited by newlines, and
// MUST NOT contain embedded newlines."
//
// It deliberately refuses LSP-style Content-Length frames rather than tolerating
// them, so a prober that regresses to header framing fails the suite instead of
// agreeing with its own mock (issue #43).
package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

func main() {
	reader := bufio.NewReader(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)

	for {
		payload, err := readFrame(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				_ = writer.Flush()
				return
			}
			_ = writeRPCError(writer, nil, -32700, "parse error: "+err.Error())
			_ = writer.Flush()
			continue
		}

		var req rpcRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			_ = writeRPCError(writer, nil, -32700, "invalid JSON")
			_ = writer.Flush()
			continue
		}

		resp := handle(req)
		if err := writeRPCResponse(writer, resp); err != nil {
			// best effort logging to stderr for tests/debug
			_, _ = fmt.Fprintf(os.Stderr, "write response failed: %v\n", err)
			return
		}
		if err := writer.Flush(); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "flush failed: %v\n", err)
			return
		}

		// Support graceful termination flow if client sends "exit".
		if req.Method == "exit" {
			return
		}
	}
}

func handle(req rpcRequest) rpcResponse {
	// Default JSON-RPC version
	if req.JSONRPC == "" {
		req.JSONRPC = "2.0"
	}

	switch req.Method {
	case "initialize":
		return rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]any{
					"tools":     map[string]any{"listChanged": false},
					"resources": map[string]any{"subscribe": false, "listChanged": false},
				},
				"serverInfo": map[string]any{
					"name":    "mock-stdio-server",
					"version": "0.1.0",
				},
			},
		}

	case "session/setup":
		return rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"sessionId": "mock-session-1",
				"capabilities": map[string]any{
					"prompt/turn": true,
					"tool/calls":  true,
				},
			},
		}

	case "tools/list":
		return rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"tools": []any{},
			},
		}

	case "resources/list":
		return rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"resources": []any{},
			},
		}

	case "ping":
		return rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]any{
				"ok": true,
				"ts": time.Now().UTC().Format(time.RFC3339Nano),
			},
		}

	case "shutdown":
		return rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  map[string]any{"ok": true},
		}

	default:
		return rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &rpcError{
				Code:    -32601,
				Message: "method not found: " + req.Method,
			},
		}
	}
}

func writeRPCError(w io.Writer, id json.RawMessage, code int, msg string) error {
	resp := rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: &rpcError{
			Code:    code,
			Message: msg,
		},
	}
	return writeRPCResponse(w, resp)
}

// writeRPCResponse writes one newline-delimited JSON object. No Content-Length
// header: MCP and ACP stdio transports are line-delimited.
func writeRPCResponse(w io.Writer, resp rpcResponse) error {
	b, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = w.Write(b)
	return err
}

// readFrame reads one newline-delimited JSON-RPC message and rejects header
// framing, which is what a Content-Length-speaking client would send.
func readFrame(r *bufio.Reader) ([]byte, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimRight(line, "\r\n")
	if strings.HasPrefix(strings.ToLower(line), "content-length:") {
		return nil, errors.New("received LSP Content-Length framing; MCP stdio is newline-delimited")
	}
	if !strings.HasPrefix(strings.TrimSpace(line), "{") {
		return nil, fmt.Errorf("expected a newline-delimited JSON object, got %q", line)
	}
	return []byte(line), nil
}
