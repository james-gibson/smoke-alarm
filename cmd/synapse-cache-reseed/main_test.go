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
	frequency := tagFrequency(docTagLists([]doc{alerts, other}))
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

func TestParseFeatureFile(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "features", "adhd", "headless-mode.feature")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	content := "@adhd @domain-headless\n" +
		"Feature: ADHD Headless Mode\n" +
		"  starts the MCP server and polls\n" +
		"  smoke-alarm endpoints.\n\n" +
		"  Background:\n" +
		"    Given adhd is started\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	fd, err := parseFeatureFile(path, "features/adhd/headless-mode.feature")
	if err != nil {
		t.Fatalf("parseFeatureFile: %v", err)
	}
	if fd.name != "ADHD Headless Mode" {
		t.Errorf("name = %q", fd.name)
	}
	if fd.description != "starts the MCP server and polls smoke-alarm endpoints." {
		t.Errorf("description = %q", fd.description)
	}
	if len(fd.tags) != 2 || fd.tags[0] != "@adhd" || fd.tags[1] != "@domain-headless" {
		t.Errorf("tags = %v", fd.tags)
	}
	wantSummary := "ADHD Headless Mode — starts the MCP server and polls smoke-alarm endpoints."
	if fd.summary() != wantSummary {
		t.Errorf("summary() = %q, want %q", fd.summary(), wantSummary)
	}
	if got := aliasFor(fd.path); got != "headless mode" {
		t.Errorf("aliasFor = %q, want %q", got, "headless mode")
	}
}

func TestParseFeatureFileWithoutDescription(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "features", "bare.feature")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("@core\nFeature: Bare\n\n  Scenario: x\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	fd, err := parseFeatureFile(path, "features/bare.feature")
	if err != nil {
		t.Fatalf("parseFeatureFile: %v", err)
	}
	if fd.description != "" {
		t.Errorf("description = %q, want empty", fd.description)
	}
	if fd.summary() != "Bare" {
		t.Errorf("summary() = %q, want %q", fd.summary(), "Bare")
	}
}

func TestScanFeatures(t *testing.T) {
	root := t.TempDir()
	for _, rel := range []string{"features/adhd/b.feature", "features/adhd/a.feature", "features/adhd/nested/c.feature"} {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte("Feature: "+filepath.Base(rel)+"\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	features, err := scanFeatures(root, "features/adhd")
	if err != nil {
		t.Fatalf("scanFeatures: %v", err)
	}
	if len(features) != 3 {
		t.Fatalf("scanned %d features, want 3", len(features))
	}
	want := []string{"features/adhd/a.feature", "features/adhd/b.feature", "features/adhd/nested/c.feature"}
	for i, f := range features {
		if f.path != want[i] {
			t.Errorf("features[%d].path = %q, want %q", i, f.path, want[i])
		}
	}

	if _, err := scanFeatures(root, "features/missing"); err == nil {
		t.Fatal("expected error scanning a missing directory")
	}
}

func TestFeatureQueriesSkipSharedTags(t *testing.T) {
	a := featureDoc{path: "features/adhd/headless-mode.feature", name: "Headless Mode", tags: []string{"@adhd", "@domain-headless"}}
	b := featureDoc{path: "features/adhd/demo.feature", name: "Demo", tags: []string{"@adhd", "@demo"}}
	frequency := tagFrequency([][]string{a.tags, b.tags})
	queries := featureQueries(a, frequency)

	joined := strings.Join(queries, ",")
	if !strings.Contains(joined, "domain-headless") {
		t.Errorf("unique tag @domain-headless missing from queries: %v", queries)
	}
	if strings.Contains(joined, "adhd") {
		t.Errorf("shared tag @adhd should be skipped: %v", queries)
	}
	if !strings.Contains(joined, "Headless Mode") {
		t.Errorf("feature name missing from queries: %v", queries)
	}
	if !strings.Contains(joined, "headless mode") {
		t.Errorf("alias missing from queries: %v", queries)
	}
}

func TestLoadDocsSubsystemsOptional(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "docs", "features", "catalog")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := "---\ntitle: Demo\nsource: smoke-alarm/features/demo.feature\n---\n"
	if err := os.WriteFile(filepath.Join(dir, "demo.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	set, err := loadDocs(root, "docs/features")
	if err != nil {
		t.Fatalf("loadDocs: %v", err)
	}
	if len(set.catalogs) != 1 {
		t.Errorf("catalogs = %d, want 1", len(set.catalogs))
	}
	if len(set.subsystems) != 0 {
		t.Errorf("subsystems = %d, want 0", len(set.subsystems))
	}
	if _, err := loadDocs(root, "docs/missing"); err == nil {
		t.Error("expected error when the catalog directory is missing")
	}
}

func TestBuildFeaturePlan(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"features/adhd/headless-mode.feature": "@adhd @domain-headless\nFeature: Headless Mode\n  runs headless.\n\n  Scenario: x\n",
		"features/adhd/demo.feature":          "@adhd @domain-demo\nFeature: Demo Mode\n",
	}
	for rel, body := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	fake := newFakeSynapse(t, map[string]toolHandler{
		"synapse_search_codebase": func(args map[string]any) (string, error) {
			path, _ := args["path"].(string)
			return fmt.Sprintf(`{"results":[{"chunk_id":"chunk:%s","symbol":"feature_x"}]}`, path), nil
		},
	})
	ctx := context.Background()
	client, err := dialMCP(ctx, fake.URL(), 5*time.Second)
	if err != nil {
		t.Fatalf("dialMCP: %v", err)
	}

	features, err := scanFeatures(root, "features/adhd")
	if err != nil {
		t.Fatalf("scanFeatures: %v", err)
	}
	opts := options{repoID: "adhd", repoRoot: root, featuresDir: "features/adhd"}
	plan := buildFeaturePlan(ctx, client, opts, features)

	if len(plan.failures) != 0 {
		t.Fatalf("unexpected failures: %v", plan.failures)
	}
	if len(plan.summaries) != 2 {
		t.Fatalf("summaries = %d, want 2", len(plan.summaries))
	}
	if len(plan.inputs) != 1 || plan.inputs[0].label != "feature files" || plan.inputs[0].count != 2 {
		t.Errorf("inputs = %+v", plan.inputs)
	}

	learned := make(map[string]string)
	for _, l := range plan.learns {
		learned[l.query] = l.chunkID
	}
	if _, ok := learned["ADHD Headless Mode"]; !ok {
		if _, ok := learned["Headless Mode"]; !ok {
			t.Errorf("feature name not learned: %v", learned)
		}
	}
	if _, ok := learned["headless mode"]; !ok {
		t.Errorf("alias not learned: %v", learned)
	}
	if _, ok := learned["adhd"]; ok {
		t.Errorf("shared tag @adhd should not be learned: %v", learned)
	}
	if _, ok := learned["domain-headless"]; !ok {
		t.Errorf("unique tag @domain-headless should be learned: %v", learned)
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
