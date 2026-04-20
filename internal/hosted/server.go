package hosted

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Options configures the embedded hosted service server.
type Options struct {
	ServiceName string
	Version     string
	ListenAddr  string

	EnableHTTP bool
	EnableSSE  bool

	EnableMCP bool
	EnableACP bool
	EnableA2A bool

	MCPEndpoint string
	ACPEndpoint string
	A2AEndpoint string

	ShutdownTimeout time.Duration
}

// HostedEvent records a single protocol event handled by the embedded server.
type HostedEvent struct {
	Time      time.Time `json:"time"`
	Protocol  string    `json:"protocol"`
	Transport string    `json:"transport"`
	Method    string    `json:"method"`
	Path      string    `json:"path"`
	Outcome   string    `json:"outcome"`
	Detail    string    `json:"detail,omitempty"`
}

type HostedCounters struct {
	Total int64 `json:"total"`
	MCP   int64 `json:"mcp"`
	ACP   int64 `json:"acp"`
	A2A   int64 `json:"a2a"`
	SSE   int64 `json:"sse"`
	HTTP  int64 `json:"http"`
}

type Server struct {
	opts    Options
	httpSrv *http.Server

	started atomic.Bool

	totalReq atomic.Int64
	mcpReq   atomic.Int64
	acpReq   atomic.Int64
	a2aReq   atomic.Int64
	sseReq   atomic.Int64
	httpReq  atomic.Int64

	eventsMu  sync.RWMutex
	events    []HostedEvent
	maxEvents int

	sessionsMu sync.RWMutex
	sessions   map[string]time.Time // sessionID -> created

	// Tuner integration state.
	audienceMu sync.RWMutex
	audience   map[string]tunerAudienceMetric

	callerMu   sync.RWMutex
	callerSubs map[string][]chan []byte // channel -> SSE subscriber channels
}

// NewServer creates a hosted service server with sane defaults.
func NewServer(opts Options) *Server {
	if strings.TrimSpace(opts.ServiceName) == "" {
		opts.ServiceName = "ocd-smoke-alarm"
	}
	if strings.TrimSpace(opts.Version) == "" {
		opts.Version = "0.1.0"
	}
	if strings.TrimSpace(opts.ListenAddr) == "" {
		opts.ListenAddr = "localhost:8091"
	}
	if opts.ShutdownTimeout <= 0 {
		opts.ShutdownTimeout = 8 * time.Second
	}

	// Default to HTTP enabled if neither transport is explicitly enabled.
	if !opts.EnableHTTP && !opts.EnableSSE {
		opts.EnableHTTP = true
	}

	// Default to MCP+ACP if no protocol explicitly enabled.
	if !opts.EnableMCP && !opts.EnableACP && !opts.EnableA2A {
		opts.EnableMCP = true
		opts.EnableACP = true
	}

	if strings.TrimSpace(opts.MCPEndpoint) == "" {
		opts.MCPEndpoint = "/mcp"
	}
	if strings.TrimSpace(opts.ACPEndpoint) == "" {
		opts.ACPEndpoint = "/acp"
	}
	if strings.TrimSpace(opts.A2AEndpoint) == "" {
		opts.A2AEndpoint = "/a2a"
	}

	mux := http.NewServeMux()
	s := &Server{
		opts: opts,
		httpSrv: &http.Server{
			Addr:              opts.ListenAddr,
			Handler:           mux,
			ReadHeaderTimeout: 4 * time.Second,
		},
		events:     make([]HostedEvent, 0, 256),
		maxEvents:  256,
		sessions:   make(map[string]time.Time),
		audience:   make(map[string]tunerAudienceMetric),
		callerSubs: make(map[string][]chan []byte),
	}

	if opts.EnableMCP {
		mux.HandleFunc(opts.MCPEndpoint, s.handleProtocol("mcp"))
	}
	if opts.EnableACP {
		mux.HandleFunc(opts.ACPEndpoint, s.handleProtocol("acp"))
	}
	if opts.EnableA2A {
		mux.HandleFunc(opts.A2AEndpoint, s.handleProtocol("a2a"))
	}

	// Lightweight service introspection endpoint.
	mux.HandleFunc("/hosted/status", s.handleHostedStatus)
	mux.HandleFunc("/hosted/events", s.handleHostedEvents)

	// Tuner integration endpoints.
	mux.HandleFunc("/tuner/audience", s.handleTunerAudience)
	mux.HandleFunc("/tuner/caller/", s.handleTunerCaller)

	return s
}

// Start runs the hosted server until context cancellation or fatal listen error.
func (s *Server) Start(ctx context.Context) error {
	if s.started.Load() {
		return nil
	}
	s.started.Store(true)

	errCh := make(chan error, 1)
	go func() {
		err := s.httpSrv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		return s.Shutdown(context.Background())
	case err := <-errCh:
		return err
	}
}

// Shutdown gracefully stops the hosted server.
func (s *Server) Shutdown(ctx context.Context) error {
	if !s.started.Load() {
		return nil
	}
	s.started.Store(false)

	timeoutCtx, cancel := context.WithTimeout(ctx, s.opts.ShutdownTimeout)
	defer cancel()
	return s.httpSrv.Shutdown(timeoutCtx)
}

func (s *Server) handleHostedStatus(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service": s.opts.ServiceName,
		"version": s.opts.Version,
		"listen":  s.opts.ListenAddr,
		"transports": map[string]bool{
			"http": s.opts.EnableHTTP,
			"sse":  s.opts.EnableSSE,
		},
		"protocols": map[string]bool{
			"mcp": s.opts.EnableMCP,
			"acp": s.opts.EnableACP,
			"a2a": s.opts.EnableA2A,
		},
		"endpoints": map[string]string{
			"mcp": s.opts.MCPEndpoint,
			"acp": s.opts.ACPEndpoint,
			"a2a": s.opts.A2AEndpoint,
		},
		"counters":      s.snapshotCounters(),
		"recent_events": s.recentEvents(25),
		"time":          time.Now().UTC(),
	})
}

func (s *Server) handleProtocol(proto string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.totalReq.Add(1)
		s.incrementProtocolCounter(proto)

		transport := "http"
		if wantsSSE(r) {
			transport = "sse"
			s.sseReq.Add(1)
		} else {
			s.httpReq.Add(1)
		}

		s.appendEvent(HostedEvent{
			Time:      time.Now().UTC(),
			Protocol:  proto,
			Transport: transport,
			Method:    r.Method,
			Path:      r.URL.RequestURI(),
			Outcome:   "received",
		})

		// MCP Streamable HTTP: GET with Accept: text/event-stream opens an
		// SSE stream for server-to-client notifications on the same URL.
		if r.Method == http.MethodGet && wantsSSE(r) {
			if !s.opts.EnableSSE {
				s.appendEvent(HostedEvent{
					Time:      time.Now().UTC(),
					Protocol:  proto,
					Transport: "sse",
					Method:    r.Method,
					Path:      r.URL.RequestURI(),
					Outcome:   "rejected",
					Detail:    "sse transport disabled",
				})
				writeJSON(w, http.StatusNotImplemented, map[string]any{
					"error":   "sse transport disabled",
					"service": s.opts.ServiceName,
				})
				return
			}
			s.handleSSE(proto, w, r)
			return
		}

		// POST: JSON-RPC request (MCP Streamable HTTP primary path).
		if r.Method == http.MethodPost {
			if !s.opts.EnableHTTP {
				s.appendEvent(HostedEvent{
					Time:      time.Now().UTC(),
					Protocol:  proto,
					Transport: "http",
					Method:    r.Method,
					Path:      r.URL.RequestURI(),
					Outcome:   "rejected",
					Detail:    "http transport disabled",
				})
				writeJSON(w, http.StatusNotImplemented, map[string]any{
					"error":   "http transport disabled",
					"service": s.opts.ServiceName,
				})
				return
			}
			s.handleJSONRPC(proto, w, r)
			return
		}

		// DELETE: session termination (MCP Streamable HTTP).
		if r.Method == http.MethodDelete {
			sid := r.Header.Get("Mcp-Session-Id")
			if s.validSession(sid) {
				s.sessionsMu.Lock()
				delete(s.sessions, sid)
				s.sessionsMu.Unlock()
				w.WriteHeader(http.StatusOK)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
			return
		}

		s.appendEvent(HostedEvent{
			Time:      time.Now().UTC(),
			Protocol:  proto,
			Transport: transport,
			Method:    r.Method,
			Path:      r.URL.RequestURI(),
			Outcome:   "rejected",
			Detail:    "method not allowed",
		})
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{
			"error":   "use POST for JSON-RPC, GET with Accept: text/event-stream for SSE, or DELETE to end session",
			"service": s.opts.ServiceName,
		})
	}
}

func (s *Server) handleJSONRPC(proto string, w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()

	var req rpcRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, rpcResponse{
			JSONRPC: "2.0",
			ID:      nil,
			Error: &rpcError{
				Code:    -32700,
				Message: "parse error: invalid JSON",
			},
		})
		return
	}

	if strings.TrimSpace(req.Method) == "" {
		writeJSON(w, http.StatusBadRequest, rpcResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error: &rpcError{
				Code:    -32600,
				Message: "invalid request: method is required",
			},
		})
		return
	}

	resp := s.dispatchRPC(proto, req)

	outcome := "ok"
	detail := strings.TrimSpace(req.Method)
	if resp.Error != nil {
		outcome = "rpc_error"
		detail = resp.Error.Message
	}

	s.appendEvent(HostedEvent{
		Time:      time.Now().UTC(),
		Protocol:  proto,
		Transport: "http",
		Method:    r.Method,
		Path:      r.URL.RequestURI(),
		Outcome:   outcome,
		Detail:    detail,
	})

	// MCP Streamable HTTP: return Mcp-Session-Id on initialize response.
	if req.Method == "initialize" && resp.Error == nil {
		sid := s.createSession()
		w.Header().Set("Mcp-Session-Id", sid)
	}

	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) createSession() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	sid := hex.EncodeToString(b)
	s.sessionsMu.Lock()
	s.sessions[sid] = time.Now().UTC()
	s.sessionsMu.Unlock()
	return sid
}

func (s *Server) validSession(sid string) bool {
	if sid == "" {
		return false
	}
	s.sessionsMu.RLock()
	_, ok := s.sessions[sid]
	s.sessionsMu.RUnlock()
	return ok
}

func (s *Server) dispatchRPC(proto string, req rpcRequest) rpcResponse {
	out := rpcResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
	}

	method := strings.TrimSpace(req.Method)
	switch method {
	case "initialize":
		out.Result = map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]any{
				"tools":     map[string]any{"listChanged": false},
				"resources": map[string]any{"subscribe": false, "listChanged": false},
			},
			"serverInfo": map[string]any{
				"name":    fmt.Sprintf("%s-hosted-%s", s.opts.ServiceName, proto),
				"version": s.opts.Version,
			},
		}
		return out

	case "session/setup":
		if proto != "acp" {
			out.Error = &rpcError{Code: -32601, Message: "method not found for protocol"}
			return out
		}
		out.Result = map[string]any{
			"sessionId": "embedded-session-1",
			"capabilities": map[string]any{
				"prompt/turn": true,
				"tool/calls":  true,
			},
		}
		return out

	case "tools/list":
		if proto != "mcp" {
			out.Error = &rpcError{Code: -32601, Message: "method not found for protocol"}
			return out
		}
		out.Result = map[string]any{
			"tools": []map[string]any{
				{
					"name":        "smoke.health",
					"description": "Return hosted service health summary",
					"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
				},
				{
					"name":        "smoke.tuner_list_channels",
					"description": "List channels available for Tuner observation",
					"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
				},
				{
					"name":        "smoke.tuner_audience",
					"description": "Get current audience metrics per channel",
					"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
				},
				{
					"name":        "smoke.tuner_caller_messages",
					"description": "Receive caller messages from Tuner viewers",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"channel": map[string]any{"type": "string", "description": "Channel to listen on"},
						},
					},
				},
			},
		}
		return out

	case "resources/list":
		if proto != "mcp" {
			out.Error = &rpcError{Code: -32601, Message: "method not found for protocol"}
			return out
		}
		out.Result = map[string]any{
			"resources": []map[string]any{
				{
					"uri":         "hosted://status",
					"name":        "Hosted Status",
					"description": "Embedded hosted service state and capabilities",
					"mimeType":    "application/json",
				},
			},
		}
		return out

	case "prompt/turn":
		if proto != "acp" && proto != "a2a" {
			out.Error = &rpcError{Code: -32601, Message: "method not found for protocol"}
			return out
		}
		out.Result = map[string]any{
			"output": map[string]any{
				"text": "embedded hosted service prompt/turn acknowledged",
			},
			"done": true,
		}
		return out

	case "ping":
		out.Result = map[string]any{
			"ok":       true,
			"protocol": proto,
			"time":     time.Now().UTC().Format(time.RFC3339Nano),
		}
		return out

	default:
		out.Error = &rpcError{
			Code:    -32601,
			Message: "method not found: " + method,
		}
		return out
	}
}

func (s *Server) handleSSE(proto string, w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		s.appendEvent(HostedEvent{
			Time:      time.Now().UTC(),
			Protocol:  proto,
			Transport: "sse",
			Method:    r.Method,
			Path:      r.URL.RequestURI(),
			Outcome:   "error",
			Detail:    "streaming unsupported by response writer",
		})
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": "streaming unsupported by response writer",
		})
		return
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")

	w.WriteHeader(http.StatusOK)

	s.appendEvent(HostedEvent{
		Time:      time.Now().UTC(),
		Protocol:  proto,
		Transport: "sse",
		Method:    r.Method,
		Path:      r.URL.RequestURI(),
		Outcome:   "connected",
	})

	defer s.appendEvent(HostedEvent{
		Time:      time.Now().UTC(),
		Protocol:  proto,
		Transport: "sse",
		Method:    r.Method,
		Path:      r.URL.RequestURI(),
		Outcome:   "disconnected",
	})

	sendSSE(w, "ready", map[string]any{
		"service":  s.opts.ServiceName,
		"version":  s.opts.Version,
		"protocol": proto,
		"counters": s.snapshotCounters(),
		"time":     time.Now().UTC().Format(time.RFC3339Nano),
	})
	flusher.Flush()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case ts := <-ticker.C:
			sendSSE(w, "heartbeat", map[string]any{
				"ok":       true,
				"protocol": proto,
				"counters": s.snapshotCounters(),
				"time":     ts.UTC().Format(time.RFC3339Nano),
			})
			flusher.Flush()
		}
	}
}

func sendSSE(w http.ResponseWriter, event string, payload any) {
	b, _ := json.Marshal(payload)
	_, _ = fmt.Fprintf(w, "event: %s\n", event)
	_, _ = fmt.Fprintf(w, "data: %s\n\n", string(b))
}

func wantsSSE(r *http.Request) bool {
	accept := strings.ToLower(strings.TrimSpace(r.Header.Get("Accept")))
	if strings.Contains(accept, "text/event-stream") {
		return true
	}
	qTransport := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("transport")))
	return qTransport == "sse"
}

func (s *Server) handleHostedEvents(w http.ResponseWriter, r *http.Request) {
	if !wantsSSE(r) {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":   "set Accept: text/event-stream or ?transport=sse",
			"service": s.opts.ServiceName,
		})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"error": "streaming unsupported by response writer",
		})
		return
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)

	for _, ev := range s.recentEvents(40) {
		sendSSE(w, "event", ev)
	}
	sendSSE(w, "snapshot", map[string]any{
		"counters": s.snapshotCounters(),
		"time":     time.Now().UTC().Format(time.RFC3339Nano),
	})
	flusher.Flush()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			sendSSE(w, "snapshot", map[string]any{
				"counters": s.snapshotCounters(),
				"time":     time.Now().UTC().Format(time.RFC3339Nano),
			})
			flusher.Flush()
		}
	}
}

func (s *Server) snapshotCounters() HostedCounters {
	return HostedCounters{
		Total: s.totalReq.Load(),
		MCP:   s.mcpReq.Load(),
		ACP:   s.acpReq.Load(),
		A2A:   s.a2aReq.Load(),
		SSE:   s.sseReq.Load(),
		HTTP:  s.httpReq.Load(),
	}
}

func (s *Server) incrementProtocolCounter(proto string) {
	switch strings.ToLower(strings.TrimSpace(proto)) {
	case "mcp":
		s.mcpReq.Add(1)
	case "acp":
		s.acpReq.Add(1)
	case "a2a":
		s.a2aReq.Add(1)
	}
}

func (s *Server) appendEvent(ev HostedEvent) {
	s.eventsMu.Lock()
	defer s.eventsMu.Unlock()

	if s.maxEvents <= 0 {
		s.maxEvents = 256
	}
	if len(s.events) < s.maxEvents {
		s.events = append(s.events, ev)
		return
	}
	copy(s.events, s.events[1:])
	s.events[len(s.events)-1] = ev
}

func (s *Server) recentEvents(n int) []HostedEvent {
	s.eventsMu.RLock()
	defer s.eventsMu.RUnlock()

	if n <= 0 || n > len(s.events) {
		n = len(s.events)
	}
	start := len(s.events) - n
	if start < 0 {
		start = 0
	}
	out := make([]HostedEvent, n)
	copy(out, s.events[start:])
	return out
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int            `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      *int      `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// --- Tuner integration types and handlers ---

type tunerAudienceMetric struct {
	Channel   string         `json:"channel"`
	Count     int            `json:"count"`
	Signal    float64        `json:"signal"`
	Metadata  map[string]any `json:"metadata,omitempty"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// handleTunerAudience accepts POST of audience metrics from Tuner.
func (s *Server) handleTunerAudience(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// GET returns current audience state.
	if r.Method == http.MethodGet {
		s.audienceMu.RLock()
		out := make([]tunerAudienceMetric, 0, len(s.audience))
		for _, m := range s.audience {
			out = append(out, m)
		}
		s.audienceMu.RUnlock()
		writeJSON(w, http.StatusOK, map[string]any{"audience": out})
		return
	}

	// POST updates audience for a channel.
	var metric tunerAudienceMetric
	if err := json.NewDecoder(r.Body).Decode(&metric); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if metric.Channel == "" {
		http.Error(w, "channel required", http.StatusBadRequest)
		return
	}
	metric.UpdatedAt = time.Now()

	s.audienceMu.Lock()
	s.audience[metric.Channel] = metric
	s.audienceMu.Unlock()

	s.appendEvent(HostedEvent{
		Time:     time.Now().UTC(),
		Protocol: "tuner",
		Method:   "audience",
		Path:     r.URL.Path,
		Outcome:  "ok",
		Detail:   fmt.Sprintf("channel=%s count=%d signal=%.2f", metric.Channel, metric.Count, metric.Signal),
	})

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleTunerCaller handles caller line: GET for SSE subscribe, POST to send message.
func (s *Server) handleTunerCaller(w http.ResponseWriter, r *http.Request) {
	// Parse channel from /tuner/caller/{channel}/sse or /tuner/caller/{channel}
	path := strings.TrimPrefix(r.URL.Path, "/tuner/caller/")
	parts := strings.SplitN(path, "/", 2)
	channel := parts[0]
	if channel == "" {
		http.Error(w, "channel required in path", http.StatusBadRequest)
		return
	}

	suffix := ""
	if len(parts) > 1 {
		suffix = parts[1]
	}

	switch {
	case r.Method == http.MethodGet && suffix == "sse":
		s.handleCallerSSE(w, r, channel)
	case r.Method == http.MethodPost:
		s.handleCallerPost(w, r, channel)
	default:
		http.Error(w, "use GET /tuner/caller/{channel}/sse or POST /tuner/caller/{channel}", http.StatusBadRequest)
	}
}

func (s *Server) handleCallerSSE(w http.ResponseWriter, r *http.Request, channel string) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan []byte, 16)

	s.callerMu.Lock()
	s.callerSubs[channel] = append(s.callerSubs[channel], ch)
	s.callerMu.Unlock()

	defer func() {
		s.callerMu.Lock()
		subs := s.callerSubs[channel]
		for i, sub := range subs {
			if sub == ch {
				s.callerSubs[channel] = append(subs[:i], subs[i+1:]...)
				break
			}
		}
		s.callerMu.Unlock()
		close(ch)
	}()

	// Initial connected event.
	_, _ = fmt.Fprintf(w, "event: connected\ndata: {\"channel\":%q}\n\n", channel) //nolint:gosec
	flusher.Flush()

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			_, _ = fmt.Fprintf(w, "event: caller\ndata: %s\n\n", msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) handleCallerPost(w http.ResponseWriter, r *http.Request, channel string) {
	var msg struct {
		Message  string `json:"message"`
		From     string `json:"from"`
		Priority string `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	payload, _ := json.Marshal(map[string]any{
		"channel":  channel,
		"message":  msg.Message,
		"from":     msg.From,
		"priority": msg.Priority,
		"time":     time.Now().UTC().Format(time.RFC3339Nano),
	})

	// Fan out to SSE subscribers.
	s.callerMu.RLock()
	subs := s.callerSubs[channel]
	for _, ch := range subs {
		select {
		case ch <- payload:
		default: // drop if subscriber is slow
		}
	}
	s.callerMu.RUnlock()

	s.appendEvent(HostedEvent{
		Time:     time.Now().UTC(),
		Protocol: "tuner",
		Method:   "caller",
		Path:     r.URL.Path,
		Outcome:  "ok",
		Detail:   fmt.Sprintf("channel=%s from=%s", channel, msg.From),
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "received",
		"channel":     channel,
		"subscribers": len(subs),
	})
}
