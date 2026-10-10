package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestParseFrontmatter(t *testing.T) {
	valid := "---\n" +
		"title: Demo\n" +
		"description: A demo feature.\n" +
		"source: smoke-alarm/features/demo.feature\n" +
		"gherkin_tags:\n  - \"@demo\"\n  - \"@core\"\n" +
		"---\n\nbody text\n"

	fm, err := parseFrontmatter([]byte(valid))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if fm.Title != "Demo" || fm.Description != "A demo feature." {
		t.Fatalf("unexpected frontmatter: %+v", fm)
	}
	if fm.Source != "smoke-alarm/features/demo.feature" {
		t.Fatalf("unexpected source: %q", fm.Source)
	}
	if len(fm.GherkinTags) != 2 {
		t.Fatalf("expected 2 tags, got %v", fm.GherkinTags)
	}

	if _, err := parseFrontmatter([]byte("no frontmatter here")); err == nil {
		t.Fatal("expected error for missing frontmatter")
	}
	if _, err := parseFrontmatter([]byte("---\ntitle: broken\n")); err == nil {
		t.Fatal("expected error for unterminated frontmatter")
	}
}

func TestDocSummary(t *testing.T) {
	tests := []struct {
		name string
		fm   frontmatter
		want string
	}{
		{"title and description", frontmatter{Title: "Demo", Description: "A demo."}, "Demo — A demo."},
		{"title only", frontmatter{Title: "Demo"}, "Demo"},
		{"description only", frontmatter{Description: "A demo."}, "A demo."},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := (doc{fm: tc.fm}).summary(); got != tc.want {
				t.Fatalf("summary() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAliasFor(t *testing.T) {
	tests := map[string]string{
		"docs/features/catalog/mcp-proxy.md":     "mcp proxy",
		"docs/features/subsystems/ops-safety.md": "ops safety",
		"docs/features/catalog/alerts.md":        "alerts",
	}
	for path, want := range tests {
		if got := aliasFor(path); got != want {
			t.Errorf("aliasFor(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestFeaturePathFor(t *testing.T) {
	tests := []struct {
		source string
		want   string
		ok     bool
	}{
		{"smoke-alarm/features/alerts.feature", "features/alerts.feature", true},
		{"features/alerts.feature", "features/alerts.feature", true},
		{"smoke-alarm/configs/demo.yaml", "", false},
		{"", "", false},
	}
	for _, tc := range tests {
		got, err := featurePathFor("smoke-alarm", tc.source)
		if tc.ok && err != nil {
			t.Errorf("featurePathFor(%q) unexpected error: %v", tc.source, err)
			continue
		}
		if !tc.ok {
			if err == nil {
				t.Errorf("featurePathFor(%q) expected error", tc.source)
			}
			continue
		}
		if got != tc.want {
			t.Errorf("featurePathFor(%q) = %q, want %q", tc.source, got, tc.want)
		}
	}
}

func TestFeatureHeading(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "features", "demo.feature")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("@demo\nFeature: Demo Thing\n\n  Scenario: x\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := featureHeading(root, "features/demo.feature")
	if err != nil {
		t.Fatalf("featureHeading: %v", err)
	}
	if got != "Demo Thing" {
		t.Fatalf("featureHeading = %q, want %q", got, "Demo Thing")
	}
}

func TestCatalogQueriesSkipSharedTags(t *testing.T) {
	alerts := doc{path: "docs/features/catalog/alerts.md", fm: frontmatter{
		Title:       "Alert Routing",
		GherkinTags: []string{"@alerts", "@core"},
	}}
	other := doc{path: "docs/features/catalog/other.md", fm: frontmatter{
		Title:       "Other Feature",
		GherkinTags: []string{"@other", "@core"},
	}}
	frequency := gherkinTagFrequency([]doc{alerts, other})
	queries := catalogQueries(alerts, "Alerts Heading", frequency)

	joined := strings.Join(queries, ",")
	if !strings.Contains(joined, "alerts") {
		t.Errorf("unique tag @alerts missing from queries: %v", queries)
	}
	if strings.Contains(joined, "core") {
		t.Errorf("shared tag @core should be skipped: %v", queries)
	}
	if !strings.Contains(joined, "Alerts Heading") {
		t.Errorf("feature heading missing from queries: %v", queries)
	}
}

func TestReadSSEResult(t *testing.T) {
	stream := "event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"ok\":true}}\n\n"
	raw, err := readSSEResult(strings.NewReader(stream))
	if err != nil {
		t.Fatalf("readSSEResult: %v", err)
	}
	if string(raw) != `{"ok":true}` {
		t.Fatalf("result = %s, want {\"ok\":true}", raw)
	}

	if _, err := readSSEResult(strings.NewReader("event: ping\n\n")); err == nil {
		t.Fatal("expected error when no result frame is present")
	}
}

func TestMCPClientCallTool(t *testing.T) {
	fake := newFakeSynapse(t, map[string]toolHandler{
		"synapse_search_codebase": func(args map[string]any) (string, error) {
			if args["action"] != "regex" {
				return "", fmt.Errorf("unexpected action %v", args["action"])
			}
			return `{"results":[{"chunk_id":"abc","symbol":"feature_demo"}]}`, nil
		},
	})
	ctx := context.Background()
	client, err := dialMCP(ctx, fake.URL(), 5*time.Second)
	if err != nil {
		t.Fatalf("dialMCP: %v", err)
	}
	if client.sessionID != "test-session" {
		t.Fatalf("session id = %q, want test-session", client.sessionID)
	}
	results, err := client.searchChunks(ctx, "smoke-alarm", "features/demo.feature", "^Feature:")
	if err != nil {
		t.Fatalf("searchChunks: %v", err)
	}
	if len(results) != 1 || results[0].ChunkID != "abc" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestResolveFeatureChunk(t *testing.T) {
	fake := newFakeSynapse(t, map[string]toolHandler{
		"synapse_search_codebase": func(map[string]any) (string, error) {
			return `{"results":[
				{"chunk_id":"noise","symbol":"unrelated_symbol"},
				{"chunk_id":"header","symbol":"feature_demo"}
			]}`, nil
		},
	})
	ctx := context.Background()
	client, err := dialMCP(ctx, fake.URL(), 5*time.Second)
	if err != nil {
		t.Fatalf("dialMCP: %v", err)
	}
	ref, err := resolveFeatureChunk(ctx, client, "smoke-alarm", "features/demo.feature")
	if err != nil {
		t.Fatalf("resolveFeatureChunk: %v", err)
	}
	if ref.ChunkID != "header" {
		t.Fatalf("chunk = %q, want header", ref.ChunkID)
	}
}

// --- fake Synapse MCP server -------------------------------------------------

type toolHandler func(args map[string]any) (string, error)

type fakeSynapse struct {
	server   *httptest.Server
	handlers map[string]toolHandler
	mu       sync.Mutex
	methods  []string
}

func newFakeSynapse(t *testing.T, handlers map[string]toolHandler) *fakeSynapse {
	t.Helper()
	fake := &fakeSynapse{handlers: handlers}
	fake.server = httptest.NewServer(http.HandlerFunc(fake.handle))
	t.Cleanup(fake.server.Close)
	return fake
}

func (f *fakeSynapse) URL() string { return f.server.URL }

func (f *fakeSynapse) handle(w http.ResponseWriter, r *http.Request) {
	if !strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		http.Error(w, "client did not advertise SSE accept header", http.StatusBadRequest)
		return
	}
	var request rpcRequest
	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.methods = append(f.methods, request.Method)
	f.mu.Unlock()

	if request.Method == "notifications/initialized" {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("mcp-session-id", "test-session")

	var (
		result any
		rpcErr *rpcError
	)
	switch request.Method {
	case "initialize":
		result = map[string]any{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    map[string]any{},
			"serverInfo":      map[string]any{"name": "fake", "version": "0"},
		}
	case "tools/call":
		params, _ := request.Params.(map[string]any)
		name, _ := params["name"].(string)
		args, _ := params["arguments"].(map[string]any)
		handler, ok := f.handlers[name]
		if !ok {
			rpcErr = &rpcError{Code: -32601, Message: "unknown tool " + name}
			break
		}
		text, err := handler(args)
		if err != nil {
			rpcErr = &rpcError{Code: -32000, Message: err.Error()}
			break
		}
		result = map[string]any{
			"content": []map[string]any{{"type": "text", "text": text}},
			"isError": false,
		}
	default:
		rpcErr = &rpcError{Code: -32601, Message: "unknown method " + request.Method}
	}

	response := rpcResponse{JSONRPC: "2.0", ID: request.ID}
	if rpcErr != nil {
		response.Error = rpcErr
	} else {
		encoded, err := json.Marshal(result)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		response.Result = encoded
	}
	_ = json.NewEncoder(w).Encode(response)
}
