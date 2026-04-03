package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/config"
	"github.com/james-gibson/smoke-alarm/internal/health"
)

func TestSelfDescription_ServedAtWellKnownPath(t *testing.T) {
	t.Parallel()

	addr := freeAddr(t)
	cfg := config.Config{
		Version: "1",
		Service: config.ServiceConfig{
			Name:        "sd-test",
			Mode:        "foreground",
			Environment: "test",
		},
		Health: config.HealthConfig{
			Enabled:    true,
			ListenAddr: addr,
		},
		Discovery: config.DiscoveryConfig{
			Enabled: true,
			LLMSTxt: config.LLMSTxtDiscoveryConfig{
				Enabled:    true,
				RemoteURIs: []string{"https://modelcontextprotocol.io/llms.txt"},
			},
		},
		KnownState: config.KnownStateConfig{Enabled: true},
		Hosted: config.HostedConfig{
			Enabled:    true,
			ListenAddr: "127.0.0.1:0",
			Transports: []string{"http", "sse"},
			Protocols:  []string{"mcp", "acp"},
			Endpoints:  config.HostedEndpointConfig{MCP: "/mcp", ACP: "/acp"},
		},
		DynamicConfig: config.DynamicConfigConfig{
			Enabled: true,
			Formats: []string{"json", "markdown"},
		},
		Targets: []config.TargetConfig{
			{
				ID:        "test-mcp",
				Enabled:   true,
				Protocol:  "mcp",
				Transport: "http",
				Type:      "remote",
				Endpoint:  "https://example.com/mcp",
				Auth:      config.TargetAuthConfig{Type: "none"},
				Check: config.TargetCheckConfig{
					Interval:         "10s",
					Timeout:          "3s",
					HandshakeProfile: "base",
				},
			},
			{
				ID:       "test-local",
				Enabled:  true,
				Protocol: "mcp",
				Type:     "local",
				Command:  []string{"echo", "test"},
			},
		},
	}
	cfg.ApplyDefaults()

	startedAt := time.Now().UTC()
	srv := health.NewServer(health.Options{
		ServiceName: cfg.Service.Name,
		Version:     "0.1.0-test",
		ListenAddr:  addr,
	})
	srv.SetSelfDescription(health.NewSelfDescriptionFactory(cfg, "0.1.0-test", startedAt, srv))
	srv.SetReady(true, "")

	// Simulate a target state update.
	srv.UpsertTargetStatus(health.TargetStatus{
		ID:       "test-mcp",
		Protocol: "mcp",
		State:    "healthy",
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Start(ctx) }()

	base := "http://" + addr
	waitForHTTPStatus(t, base+"/healthz", http.StatusOK, 2*time.Second)

	// Fetch self-description.
	resp, err := http.Get(base + "/.well-known/smoke-alarm.json")
	if err != nil {
		t.Fatalf("GET /.well-known/smoke-alarm.json: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("unexpected content-type: %s", ct)
	}

	var sd map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&sd); err != nil {
		t.Fatalf("decode self-description: %v", err)
	}

	// Validate top-level structure.
	if sd["version"] != "1" {
		t.Errorf("expected version=1, got %v", sd["version"])
	}

	svc, ok := sd["service"].(map[string]any)
	if !ok {
		t.Fatal("missing service object")
	}
	if svc["name"] != "sd-test" {
		t.Errorf("expected service.name=sd-test, got %v", svc["name"])
	}
	if svc["mode"] != "foreground" {
		t.Errorf("expected service.mode=foreground, got %v", svc["mode"])
	}

	caps, ok := sd["capabilities"].(map[string]any)
	if !ok {
		t.Fatal("missing capabilities object")
	}
	disc, ok := caps["discovery"].(map[string]any)
	if !ok {
		t.Fatal("missing capabilities.discovery")
	}
	if disc["enabled"] != true {
		t.Errorf("expected discovery.enabled=true, got %v", disc["enabled"])
	}

	hosted, ok := caps["hosted"].(map[string]any)
	if !ok {
		t.Fatal("missing capabilities.hosted")
	}
	if hosted["enabled"] != true {
		t.Errorf("expected hosted.enabled=true, got %v", hosted["enabled"])
	}

	// Validate targets include runtime state.
	tgts, ok := sd["targets"].([]any)
	if !ok {
		t.Fatal("missing targets array")
	}
	if len(tgts) != 2 {
		t.Fatalf("expected 2 targets, got %d", len(tgts))
	}

	// Find the remote target and check runtime state was injected.
	for _, raw := range tgts {
		tgt := raw.(map[string]any)
		if tgt["id"] == "test-mcp" {
			if tgt["state"] != "healthy" {
				t.Errorf("expected test-mcp state=healthy, got %v", tgt["state"])
			}
			if tgt["type"] != "remote" {
				t.Errorf("expected test-mcp type=remote, got %v", tgt["type"])
			}
		}
		if tgt["id"] == "test-local" {
			if tgt["type"] != "local" {
				t.Errorf("expected test-local type=local, got %v", tgt["type"])
			}
			if tgt["transport"] != "stdio" {
				t.Errorf("expected test-local transport=stdio, got %v", tgt["transport"])
			}
		}
	}

	// Validate permissions.
	perms, ok := sd["permissions"].(map[string]any)
	if !ok {
		t.Fatal("missing permissions object")
	}
	net, ok := perms["network"].(map[string]any)
	if !ok {
		t.Fatal("missing permissions.network")
	}
	if net["probe"] != "configured_targets_only" {
		t.Errorf("expected probe=configured_targets_only, got %v", net["probe"])
	}

	// Validate health endpoints are described.
	hp, ok := sd["health"].(map[string]any)
	if !ok {
		t.Fatal("missing health object")
	}
	eps, ok := hp["endpoints"].(map[string]any)
	if !ok {
		t.Fatal("missing health.endpoints")
	}
	if eps["self_description"] != "/.well-known/smoke-alarm.json" {
		t.Errorf("expected self_description path, got %v", eps["self_description"])
	}
}

func TestSelfDescription_Returns404WhenNotConfigured(t *testing.T) {
	t.Parallel()

	addr := freeAddr(t)
	srv := health.NewServer(health.Options{
		ServiceName: "no-sd",
		ListenAddr:  addr,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Start(ctx) }()

	base := "http://" + addr
	waitForHTTPStatus(t, base+"/healthz", http.StatusOK, 2*time.Second)

	resp, err := http.Get(base + "/.well-known/smoke-alarm.json")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 when no self-description configured, got %d", resp.StatusCode)
	}
}
