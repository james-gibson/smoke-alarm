package engine

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/targets"
)

// StdioProber launches local MCP/ACP servers over stdio transport and validates
// a minimal JSON-RPC handshake.
//
// It supports MCP/ACP command transports and falls back to another prober for
// non-stdio targets (if configured).
type StdioProber struct {
	HandshakeTimeout time.Duration
	AppName          string
	AppVersion       string
	MaxStderrBytes   int
	Fallback         Prober
}

// NewStdioProber constructs a stdio prober with conservative defaults.
func NewStdioProber() *StdioProber {
	return &StdioProber{
		HandshakeTimeout: 5 * time.Second,
		AppName:          "ocd-smoke-alarm",
		AppVersion:       "0.1.0",
		MaxStderrBytes:   8192,
		Fallback:         NewHTTPProber(),
	}
}

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
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
	Data    any    `json:"data,omitempty"`
}

func (e *rpcRespError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("rpc error code=%d message=%q", e.Code, e.Message)
}

// Probe implements Prober.
func (p *StdioProber) Probe(ctx context.Context, target targets.Target, _ map[string]string) (targets.CheckResult, error) {
	if target.Transport != targets.TransportStdio {
		if p.Fallback != nil {
			return p.Fallback.Probe(ctx, target, nil)
		}
		return targets.CheckResult{}, fmt.Errorf("stdio prober cannot handle non-stdio transport %q", target.Transport)
	}

	start := time.Now()
	checkAt := time.Now()

	timeout := p.HandshakeTimeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	if target.Check.Timeout > 0 {
		timeout = target.Check.Timeout
	}

	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if strings.TrimSpace(target.Stdio.Command) == "" {
		return targets.CheckResult{
			TargetID:     target.ID,
			Protocol:     target.Protocol,
			State:        targets.StateUnhealthy,
			Severity:     targets.SeverityWarn,
			FailureClass: targets.FailureConfig,
			Message:      "stdio.command is required for stdio targets",
			Latency:      time.Since(start),
			CheckedAt:    checkAt,
		}, nil
	}

	if strings.EqualFold(strings.TrimSpace(target.Check.HandshakeProfile), "none") {
		return targets.CheckResult{
			TargetID:     target.ID,
			Protocol:     target.Protocol,
			State:        targets.StateHealthy,
			Severity:     targets.SeverityInfo,
			FailureClass: targets.FailureNone,
			Message:      "stdio target configured with handshake_profile=none; protocol handshake skipped",
			Latency:      time.Since(start),
			CheckedAt:    checkAt,
			Details: map[string]any{
				"transport": "stdio",
				"profile":   "none",
			},
		}, nil
	}

	stderrText, exercisedMethods, err := p.performHandshake(probeCtx, target)
	if err != nil {
		fc := classifyStdioError(err)
		sev := targets.SeverityWarn
		if fc == targets.FailureConfig {
			sev = targets.SeverityWarn
		}
		if fc == targets.FailureTimeout {
			sev = targets.SeverityWarn
		}

		msg := err.Error()
		if stderrText != "" {
			msg = msg + " | stderr: " + compact(stderrText, 300)
		}

		return targets.CheckResult{
			TargetID:     target.ID,
			Protocol:     target.Protocol,
			State:        targets.StateUnhealthy,
			Severity:     sev,
			FailureClass: fc,
			Message:      msg,
			Latency:      time.Since(start),
			CheckedAt:    checkAt,
			Details: map[string]any{
				"transport":         "stdio",
				"profile":           target.Check.HandshakeProfile,
				"exercised_methods": exercisedMethods,
			},
		}, nil
	}

	return targets.CheckResult{
		TargetID:     target.ID,
		Protocol:     target.Protocol,
		State:        targets.StateHealthy,
		Severity:     targets.SeverityInfo,
		FailureClass: targets.FailureNone,
		Message:      fmt.Sprintf("stdio handshake passed: %s", strings.Join(exercisedMethods, ", ")),
		Latency:      time.Since(start),
		CheckedAt:    checkAt,
		Details: map[string]any{
			"transport":         "stdio",
			"profile":           target.Check.HandshakeProfile,
			"exercised_methods": exercisedMethods,
		},
	}, nil
}

func (p *StdioProber) performHandshake(ctx context.Context, target targets.Target) (string, []string, error) {
	cmd := exec.CommandContext(ctx, target.Stdio.Command, target.Stdio.Args...)
	if target.Stdio.Cwd != "" {
		cmd.Dir = target.Stdio.Cwd
	}
	cmd.Env = mergeEnv(target.Stdio.Env)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", nil, fmt.Errorf("open stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", nil, fmt.Errorf("open stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", nil, fmt.Errorf("open stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", nil, fmt.Errorf("start stdio server: %w", err)
	}
	defer func() {
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	}()

	maxErr := p.MaxStderrBytes
	if maxErr <= 0 {
		maxErr = 8192
	}
	stderrBuf := newLimitedBuffer(maxErr)
	var stderrWG sync.WaitGroup
	stderrWG.Add(1)
	go func() {
		defer stderrWG.Done()
		_, _ = io.Copy(stderrBuf, stderr)
	}()

	reader := bufio.NewReader(stdout)
	methods := p.handshakeMethods(target)
	if len(methods) == 0 {
		_ = stdin.Close()
		stderrWG.Wait()
		return stderrBuf.String(), nil, errors.New("no handshake methods configured")
	}

	exercised := make([]string, 0, len(methods))
	for i, method := range methods {
		req := rpcRequest{
			JSONRPC: "2.0",
			ID:      1 + i,
			Method:  method,
			Params:  p.requestParamsForMethod(target.Protocol, method),
		}
		if err := writeRPCFrame(stdin, req); err != nil {
			_ = stdin.Close()
			stderrWG.Wait()
			return stderrBuf.String(), exercised, fmt.Errorf("write rpc request %q: %w", method, err)
		}

		resp, err := readRPCResponseByID(ctx, reader, req.ID)
		if err != nil {
			_ = stdin.Close()
			stderrWG.Wait()
			return stderrBuf.String(), exercised, fmt.Errorf("read rpc response for %q: %w", method, err)
		}
		if resp.Error != nil {
			_ = stdin.Close()
			stderrWG.Wait()
			return stderrBuf.String(), exercised, fmt.Errorf("handshake method %q rejected: %s", method, resp.Error.Error())
		}
		exercised = append(exercised, method)
	}

	_ = stdin.Close()
	stderrWG.Wait()
	return stderrBuf.String(), exercised, nil
}

func (p *StdioProber) defaultParams(proto targets.Protocol) map[string]any {
	params := map[string]any{
		"clientInfo": map[string]any{
			"name":    p.AppName,
			"version": p.AppVersion,
		},
		"capabilities": map[string]any{},
	}
	// MCP-specific shape is widely accepted; ACP implementations usually ignore unknown fields.
	if proto == targets.ProtocolMCP {
		params["protocolVersion"] = "2024-11-05"
	}
	return params
}

func (p *StdioProber) requestParamsForMethod(proto targets.Protocol, method string) any {
	switch strings.ToLower(strings.TrimSpace(method)) {
	case "initialize":
		return p.defaultParams(proto)
	case "session/setup":
		return map[string]any{
			"clientInfo": map[string]any{
				"name":    p.AppName,
				"version": p.AppVersion,
			},
			"capabilities": map[string]any{},
		}
	case "tools/list", "resources/list", "ping":
		return map[string]any{}
	case "prompt/turn":
		return map[string]any{
			"prompt": "smoke-alarm handshake validation",
			"input":  "health-check",
		}
	default:
		return map[string]any{}
	}
}

func (p *StdioProber) handshakeMethods(target targets.Target) []string {
	if len(target.Check.RequiredMethods) > 0 {
		return normalizeMethods(target.Check.RequiredMethods)
	}

	profile := strings.ToLower(strings.TrimSpace(target.Check.HandshakeProfile))
	if profile == "" {
		profile = "base"
	}

	switch profile {
	case "strict":
		switch target.Protocol {
		case targets.ProtocolMCP:
			return []string{"initialize", "tools/list", "resources/list"}
		case targets.ProtocolACP:
			return []string{"initialize", "session/setup", "prompt/turn"}
		default:
			return []string{"initialize", "ping"}
		}
	default:
		return []string{"initialize"}
	}
}

func normalizeMethods(in []string) []string {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, m := range in {
		m = strings.TrimSpace(m)
		if m == "" {
			continue
		}
		if _, ok := seen[m]; ok {
			continue
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}
	return out
}

func writeRPCFrame(w io.Writer, req rpcRequest) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, writeErr := io.WriteString(w, header); writeErr != nil {
		return writeErr
	}
	_, err = w.Write(body)
	return err
}

func readRPCResponseByID(ctx context.Context, r *bufio.Reader, id int) (rpcResponse, error) {
	// Skip unrelated notifications/messages and only return matching response ID.
	for {
		select {
		case <-ctx.Done():
			return rpcResponse{}, ctx.Err()
		default:
		}

		payload, err := readRPCFrame(r)
		if err != nil {
			return rpcResponse{}, err
		}

		var resp rpcResponse
		if err := json.Unmarshal(payload, &resp); err != nil {
			// Ignore malformed frames and continue looking for valid response.
			continue
		}
		if resp.ID == nil {
			// Notification/event.
			continue
		}
		if *resp.ID != id {
			continue
		}
		return resp, nil
	}
}

func readRPCFrame(r *bufio.Reader) ([]byte, error) {
	// Parse headers.
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
			if err != nil || n < 0 {
				return nil, fmt.Errorf("invalid content-length %q", raw)
			}
			contentLen = n
		}
	}

	if contentLen < 0 {
		return nil, errors.New("missing content-length header")
	}

	payload := make([]byte, contentLen)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func classifyStdioError(err error) targets.FailureClass {
	if err == nil {
		return targets.FailureNone
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return targets.FailureTimeout
	}

	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "method not found"),
		strings.Contains(msg, "rpc"),
		strings.Contains(msg, "json"),
		strings.Contains(msg, "content-length"),
		strings.Contains(msg, "handshake"):
		return targets.FailureProtocol
	case strings.Contains(msg, "timeout"),
		strings.Contains(msg, "deadline exceeded"):
		return targets.FailureTimeout
	case strings.Contains(msg, "executable file not found"),
		strings.Contains(msg, "no such file"),
		strings.Contains(msg, "permission denied"),
		strings.Contains(msg, "stdio.command"):
		return targets.FailureConfig
	case strings.Contains(msg, "not found"):
		// Keep generic "not found" as protocol-oriented when emitted by JSON-RPC method errors,
		// otherwise fall back to config.
		if strings.Contains(msg, "method") {
			return targets.FailureProtocol
		}
		return targets.FailureConfig
	default:
		return targets.FailureUnknown
	}
}

func mergeEnv(extra map[string]string) []string {
	base := os.Environ()
	if len(extra) == 0 {
		return base
	}

	seen := make(map[string]int, len(base))
	out := make([]string, len(base))
	copy(out, base)

	for i, kv := range out {
		if eq := strings.IndexByte(kv, '='); eq > 0 {
			seen[kv[:eq]] = i
		}
	}
	for k, v := range extra {
		pair := k + "=" + v
		if idx, ok := seen[k]; ok {
			out[idx] = pair
			continue
		}
		out = append(out, pair)
	}
	return out
}

type limitedBuffer struct {
	max int
	buf bytes.Buffer
}

func newLimitedBuffer(maxBytes int) *limitedBuffer {
	return &limitedBuffer{max: maxBytes}
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	if l.max <= 0 {
		return len(p), nil
	}
	remaining := l.max - l.buf.Len()
	if remaining <= 0 {
		return len(p), nil
	}
	if len(p) > remaining {
		_, _ = l.buf.Write(p[:remaining])
		return len(p), nil
	}
	_, _ = l.buf.Write(p)
	return len(p), nil
}

func (l *limitedBuffer) String() string {
	return l.buf.String()
}

func compact(v string, n int) string {
	v = strings.TrimSpace(v)
	if n <= 0 || len(v) <= n {
		return v
	}
	if n == 1 {
		return v[:1]
	}
	return v[:n-1] + "…"
}
