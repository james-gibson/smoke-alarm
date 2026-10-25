// mock_lsp_stdio_server is the negative twin of mock_stdio_server: it answers on
// stdout with LSP-style `Content-Length: N\r\n\r\n` headers instead of
// newline-delimited JSON.
//
// Neither MCP nor ACP defines header framing on stdio, so a probe must NOT be
// able to complete a handshake against this server. It exists so the suite can
// prove the prober reports the framing mismatch instead of hanging until its
// timeout or — worse — silently agreeing with itself (issue #43).
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
}

func main() {
	reader := bufio.NewReader(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)
	defer func() { _ = writer.Flush() }()

	for {
		payload, err := readFrame(reader)
		if err != nil {
			// Includes io.EOF: the client closed stdin or never produced a
			// frame this server could parse.
			return
		}

		var req rpcRequest
		if err := json.Unmarshal(payload, &req); err != nil {
			continue
		}

		var result any
		switch req.Method {
		case "initialize":
			result = map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{},
				"serverInfo": map[string]any{
					"name":    "mock-lsp-stdio-server",
					"version": "0.1.0",
				},
			}
		default:
			result = map[string]any{"ok": true}
		}

		if err := writeFrame(writer, rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result}); err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "write response failed: %v\n", err)
			return
		}
		if err := writer.Flush(); err != nil {
			return
		}
	}
}

// writeFrame uses LSP framing — the thing this mock exists to be wrong about.
func writeFrame(w *bufio.Writer, resp rpcResponse) error {
	b, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(b)); err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}

// readFrame tolerates either framing on the way in so the probe's own request
// shape is not what fails this scenario; only the reply shape is wrong.
func readFrame(r *bufio.Reader) ([]byte, error) {
	contentLength := -1
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if strings.HasPrefix(strings.TrimSpace(line), "{") {
			return []byte(line), nil
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(parts[0]), "content-length") {
			n, err := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil || n < 0 {
				return nil, fmt.Errorf("invalid Content-Length: %q", parts[1])
			}
			contentLength = n
		}
	}

	if contentLength < 0 {
		return nil, fmt.Errorf("missing Content-Length header")
	}
	payload := make([]byte, contentLength)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}
