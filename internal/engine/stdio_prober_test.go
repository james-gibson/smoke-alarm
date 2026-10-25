package engine

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/james-gibson/smoke-alarm/internal/targets"
)

// MCP § transports/stdio (2025-06-18) and ACP § transports/stdio both define the
// stdio framing as one JSON-RPC message per line, delimited by newlines and
// containing no embedded newlines. These tests pin that wire format; see
// https://github.com/james-gibson/smoke-alarm/issues/43.

func TestWriteRPCFrameIsOneNewlineDelimitedObject(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	req := rpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "initialize",
		Params:  map[string]any{"protocolVersion": "2024-11-05"},
	}
	if err := writeRPCFrame(&buf, req); err != nil {
		t.Fatalf("writeRPCFrame returned error: %v", err)
	}

	out := buf.Bytes()
	if bytes.Contains(bytes.ToLower(out), []byte("content-length")) {
		t.Fatalf("frame must not carry LSP headers, got %q", out)
	}
	if n := bytes.Count(out, []byte("\n")); n != 1 {
		t.Fatalf("expected exactly one newline (the frame delimiter), got %d in %q", n, out)
	}
	if !bytes.HasSuffix(out, []byte("\n")) {
		t.Fatalf("frame must end with the newline delimiter, got %q", out)
	}

	var decoded rpcRequest
	if err := json.Unmarshal(bytes.TrimSuffix(out, []byte("\n")), &decoded); err != nil {
		t.Fatalf("frame body is not valid JSON: %v (%q)", err, out)
	}
	if decoded.Method != req.Method || decoded.ID != req.ID {
		t.Fatalf("round-tripped request mismatch: %+v", decoded)
	}
}

func TestWriteRPCFrameEscapesEmbeddedNewlines(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	req := rpcRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "prompt/turn",
		Params:  map[string]any{"prompt": "line one\nline two\r\nline three"},
	}
	if err := writeRPCFrame(&buf, req); err != nil {
		t.Fatalf("writeRPCFrame returned error: %v", err)
	}
	if n := bytes.Count(buf.Bytes(), []byte("\n")); n != 1 {
		t.Fatalf("embedded newlines must be escaped, found %d newlines in %q", n, buf.String())
	}
}

func TestReadRPCFrameReadsOneLine(t *testing.T) {
	t.Parallel()

	r := bufio.NewReader(strings.NewReader(`{"jsonrpc":"2.0","id":1,"result":{}}` + "\n"))

	frame, err := readRPCFrame(r)
	if err != nil {
		t.Fatalf("readRPCFrame returned error: %v", err)
	}
	if string(frame) != `{"jsonrpc":"2.0","id":1,"result":{}}` {
		t.Fatalf("unexpected frame %q", frame)
	}
}

func TestReadRPCFrameSkipsBlankLinesAndToleratesCRLF(t *testing.T) {
	t.Parallel()

	r := bufio.NewReader(strings.NewReader("\n\r\n" + `{"jsonrpc":"2.0","id":1,"result":{}}` + "\r\n"))

	frame, err := readRPCFrame(r)
	if err != nil {
		t.Fatalf("readRPCFrame returned error: %v", err)
	}
	if strings.ContainsAny(string(frame), "\r\n") {
		t.Fatalf("frame must not retain line endings, got %q", frame)
	}
}

func TestReadRPCFrameRejectsContentLengthFraming(t *testing.T) {
	t.Parallel()

	r := bufio.NewReader(strings.NewReader("Content-Length: 17\r\n\r\n{\"jsonrpc\":\"2.0\"}"))

	_, err := readRPCFrame(r)
	if err == nil {
		t.Fatal("expected header framing to be rejected")
	}
	msg := strings.ToLower(err.Error())
	if !strings.Contains(msg, "content-length") || !strings.Contains(msg, "newline") {
		t.Fatalf("expected an explicit framing mismatch error, got %q", err.Error())
	}
	if got := classifyStdioError(err); got != targets.FailureProtocol {
		t.Fatalf("expected protocol failure class, got %s", got)
	}
}

func TestReadRPCFrameRejectsOversizedFrame(t *testing.T) {
	t.Parallel()

	r := bufio.NewReader(strings.NewReader(strings.Repeat("x", maxRPCFrameBytes+1) + "\n"))

	_, err := readRPCFrame(r)
	if err == nil {
		t.Fatal("expected an oversized frame to be rejected")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected an exceeds-limit error, got %q", err.Error())
	}
}

func TestReadRPCFrameRejectsTruncatedFrame(t *testing.T) {
	t.Parallel()

	r := bufio.NewReader(strings.NewReader(`{"jsonrpc":"2.0","id":1}`))

	_, err := readRPCFrame(r)
	if err == nil {
		t.Fatal("expected a frame without a trailing newline to be rejected")
	}
	if !strings.Contains(err.Error(), "truncated") {
		t.Fatalf("expected a truncated frame error, got %q", err.Error())
	}
}

func TestReadRPCResponseByIDSkipsNotificationsAndNoise(t *testing.T) {
	t.Parallel()

	stream := strings.Join([]string{
		`{"jsonrpc":"2.0","method":"notifications/message","params":{}}`,
		`server starting on stderr-but-actually-stdout`,
		`{"jsonrpc":"2.0","id":2,"result":{}}`,
		`{"jsonrpc":"2.0","id":1,"result":{"ok":true}}`,
	}, "\n") + "\n"

	reqID := 1
	resp, err := readRPCResponseByID(context.Background(), bufio.NewReader(strings.NewReader(stream)), reqID)
	if err != nil {
		t.Fatalf("readRPCResponseByID returned error: %v", err)
	}
	if resp.ID == nil || *resp.ID != reqID {
		t.Fatalf("expected response for id %d, got %+v", reqID, resp)
	}
	if resp.Result == nil {
		t.Fatalf("expected result payload, got %+v", resp)
	}
}

func TestReadRPCResponseByIDReportsUnparsableStdoutOnEOF(t *testing.T) {
	t.Parallel()

	stream := "listening on stdio\n"

	_, err := readRPCResponseByID(context.Background(), bufio.NewReader(strings.NewReader(stream)), 1)
	if err == nil {
		t.Fatal("expected EOF to be reported")
	}
	if !strings.Contains(err.Error(), "unparsable stdout") {
		t.Fatalf("expected the offending stdout line in the error, got %q", err.Error())
	}
}
