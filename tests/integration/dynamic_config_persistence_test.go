package integration_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/discovery"
	"github.com/james-gibson/smoke-alarm/internal/dynamicconfig"
	"github.com/james-gibson/smoke-alarm/internal/targets"
)

func TestDynamicConfigStore_SaveDiscoveryRecords_UniqueIDsAndArtifacts(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := dynamicconfig.NewStore(dynamicconfig.StoreOptions{
		Directory:        dir,
		Formats:          []string{"json", "markdown"},
		ServeBaseURL:     "/dynamic-config",
		AllowOverwrite:   true,
		RequireUniqueIDs: true,
	})

	records := []discovery.Record{
		{
			Source:     "llms_txt",
			Confidence: 0.88,
			Evidence: map[string]string{
				"remote_uri": "https://example.com/llms.txt",
			},
			Target: targets.Target{
				ID:        "shared-target-id",
				Name:      "MCP Candidate A",
				Enabled:   true,
				Protocol:  targets.ProtocolMCP,
				Transport: targets.TransportHTTP,
				Endpoint:  "https://api-a.example.com/mcp",
				Check: targets.CheckPolicy{
					Interval:         10 * time.Second,
					Timeout:          3 * time.Second,
					Retries:          1,
					HandshakeProfile: "strict",
					RequiredMethods:  []string{"initialize", "tools/list", "resources/list"},
				},
			},
		},
		{
			Source:     "llms_txt",
			Confidence: 0.84,
			Evidence: map[string]string{
				"remote_uri": "https://example.org/llms.txt",
			},
			Target: targets.Target{
				ID:        "shared-target-id", // intentionally same base ID
				Name:      "ACP Candidate B",
				Enabled:   true,
				Protocol:  targets.ProtocolACP,
				Transport: targets.TransportHTTP,
				Endpoint:  "https://agent-b.example.org/acp",
				Check: targets.CheckPolicy{
					Interval:         10 * time.Second,
					Timeout:          3 * time.Second,
					Retries:          1,
					HandshakeProfile: "strict",
					RequiredMethods:  []string{"initialize", "session/setup", "prompt/turn"},
				},
			},
		},
	}

	artifacts, err := store.SaveDiscoveryRecords(context.Background(), records)
	if err != nil {
		t.Fatalf("SaveDiscoveryRecords failed: %v", err)
	}

	// 2 records x (json + markdown) = 4 artifacts
	if got, want := len(artifacts), 4; got != want {
		t.Fatalf("unexpected artifact count: got=%d want=%d", got, want)
	}

	// ensure each generated ID is unique due RequireUniqueIDs=true
	idSet := map[string]struct{}{}
	for _, a := range artifacts {
		if strings.TrimSpace(a.ID) == "" {
			t.Fatalf("artifact has empty ID: %+v", a)
		}
		idSet[a.ID] = struct{}{}

		if !strings.HasPrefix(a.ServeURL, "/dynamic-config/") {
			t.Fatalf("unexpected serve URL %q", a.ServeURL)
		}

		raw, readErr := os.ReadFile(a.Path)
		if readErr != nil {
			t.Fatalf("artifact file missing %q: %v", a.Path, readErr)
		}
		if strings.TrimSpace(string(raw)) == "" {
			t.Fatalf("artifact file %q is empty", a.Path)
		}
	}
	if got, want := len(idSet), 2; got != want {
		t.Fatalf("expected exactly 2 unique IDs for 2 records, got %d", got)
	}

	// verify JSON payload fields exist and are parseable
	var jsonArtifacts []dynamicconfig.SavedArtifact
	for _, a := range artifacts {
		if a.Format == dynamicconfig.FormatJSON {
			jsonArtifacts = append(jsonArtifacts, a)
		}
	}
	if len(jsonArtifacts) != 2 {
		t.Fatalf("expected 2 json artifacts, got %d", len(jsonArtifacts))
	}

	for _, ja := range jsonArtifacts {
		raw, err := os.ReadFile(ja.Path)
		if err != nil {
			t.Fatalf("read JSON artifact %q: %v", ja.Path, err)
		}
		var payload dynamicconfig.PersistedConfig
		if err := json.Unmarshal(raw, &payload); err != nil {
			t.Fatalf("unmarshal JSON artifact %q: %v", ja.Path, err)
		}
		if payload.ID == "" || payload.Target.ID == "" || payload.Target.Protocol == "" {
			t.Fatalf("invalid persisted payload in %q: %+v", ja.Path, payload)
		}
		if payload.Target.Endpoint == "" {
			t.Fatalf("missing target endpoint in %q", ja.Path)
		}
	}
}

func TestDynamicConfigStore_AllowOverwriteFalse_RejectsSecondWrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := dynamicconfig.NewStore(dynamicconfig.StoreOptions{
		Directory:        dir,
		Formats:          []string{"json"},
		ServeBaseURL:     "/dynamic-config",
		AllowOverwrite:   false,
		RequireUniqueIDs: false, // keep stable ID so second write collides
	})

	record := discovery.Record{
		Source:     "static_config",
		Confidence: 1.0,
		Target: targets.Target{
			ID:        "stable-id",
			Name:      "Stable Target",
			Enabled:   true,
			Protocol:  targets.ProtocolMCP,
			Transport: targets.TransportHTTP,
			Endpoint:  "https://stable.example.com/mcp",
			Check: targets.CheckPolicy{
				Interval: 5 * time.Second,
				Timeout:  2 * time.Second,
				Retries:  0,
			},
		},
	}

	if _, err := store.SaveDiscoveryRecord(context.Background(), record); err != nil {
		t.Fatalf("first save failed unexpectedly: %v", err)
	}

	_, err := store.SaveDiscoveryRecord(context.Background(), record)
	if err == nil {
		t.Fatalf("expected second save to fail due overwrite=false")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "overwrite") &&
		!strings.Contains(strings.ToLower(err.Error()), "exists") {
		t.Fatalf("expected overwrite/exists error, got: %v", err)
	}

	// ensure exactly one file exists
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		t.Fatalf("glob failed: %v", err)
	}
	if got, want := len(files), 1; got != want {
		t.Fatalf("expected one persisted file, got %d", got)
	}
}

func TestRenderMarkdown_ContainsCoreSections(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 25, 12, 0, 0, 0, time.UTC)
	payload := dynamicconfig.PersistedConfig{
		ID:         "dynamic-acp-abc123",
		Version:    "1",
		CreatedAt:  now,
		UpdatedAt:  now,
		Source:     "llms_txt",
		Confidence: 0.91,
		Evidence: map[string]string{
			"remote_uri": "https://example.com/llms.txt",
			"link_url":   "https://example.com/acp",
		},
		Target: dynamicconfig.PersistedTarget{
			ID:        "acp-target",
			Name:      "ACP Target",
			Enabled:   true,
			Protocol:  "acp",
			Transport: "http",
			Endpoint:  "https://example.com/acp",
			Auth: dynamicconfig.PersistedTargetAuth{
				Type:       "oauth",
				ClientID:   "ocd-smoke-alarm",
				TokenURL:   "https://auth.example.com/oauth/token",
				CallbackID: "cb-123",
			},
			Check: dynamicconfig.PersistedTargetCheck{
				Interval:         "10s",
				Timeout:          "3s",
				Retries:          1,
				HandshakeProfile: "strict",
				RequiredMethods:  []string{"initialize", "session/setup", "prompt/turn"},
			},
		},
	}

	md := dynamicconfig.RenderMarkdown(payload)
	needles := []string{
		"# Dynamic Config: dynamic-acp-abc123",
		"**Source:** `llms_txt`",
		"## Target",
		"**Protocol:** `acp`",
		"**Transport:** `http`",
		"**Endpoint:** `https://example.com/acp`",
		"**Auth Type:** `oauth`",
		"**Handshake Profile:** `strict`",
		"**Required Methods:** `initialize`, `session/setup`, `prompt/turn`",
		"## Evidence",
		"**remote_uri:** `https://example.com/llms.txt`",
	}

	for _, n := range needles {
		if !strings.Contains(md, n) {
			t.Fatalf("markdown output missing %q\n---\n%s\n---", n, md)
		}
	}
}
