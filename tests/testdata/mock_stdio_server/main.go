package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
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

func writeRPCResponse(w io.Writer, resp rpcResponse) error {
	b, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(b))
	if _, err := io.WriteString(w, header); err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

func readFrame(r *bufio.Reader) ([]byte, error) {
	contentLength := -1

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")

		// End of headers
		if line == "" {
			break
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.ToLower(strings.TrimSpace(parts[0]))
		val := strings.TrimSpace(parts[1])

		if key == "content-length" {
			n, err := strconv.Atoi(val)
			if err != nil || n < 0 {
				return nil, fmt.Errorf("invalid Content-Length: %q", val)
			}
			contentLength = n
		}
	}

	if contentLength < 0 {
		return nil, errors.New("missing Content-Length header")
	}

	payload := make([]byte, contentLength)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}
