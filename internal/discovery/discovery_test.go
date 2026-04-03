package discovery

import (
	"reflect"
	"testing"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/targets"
)

func TestSplitEnvTargets(t *testing.T) {
	raw := "https://a.example/mcp, http://b.example/acp;ws://c.example/ws\nwss://d.example/stream"
	got := splitEnvTargets(raw)

	want := []string{
		"https://a.example/mcp",
		"http://b.example/acp",
		"ws://c.example/ws",
		"wss://d.example/stream",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitEnvTargets() mismatch\n got: %#v\nwant: %#v", got, want)
	}
}

func TestEnvCandidateToTargetSuccess(t *testing.T) {
	target, err := envCandidateToTarget("MCP_TARGETS", "https://example.com/mcp", 0)
	if err != nil {
		t.Fatalf("envCandidateToTarget returned error: %v", err)
	}

	if target.Protocol != targets.ProtocolMCP {
		t.Fatalf("expected protocol MCP, got %q", target.Protocol)
	}
	if target.Transport != targets.TransportHTTP {
		t.Fatalf("expected transport HTTP, got %q", target.Transport)
	}
	if target.Endpoint != "https://example.com/mcp" {
		t.Fatalf("unexpected endpoint: %s", target.Endpoint)
	}
	if dur := target.Check.Interval; dur != 20*time.Second {
		t.Fatalf("expected interval 20s, got %s", dur)
	}
	if dur := target.Check.Timeout; dur != 5*time.Second {
		t.Fatalf("expected timeout 5s, got %s", dur)
	}
	if target.Tags["env_var"] != "MCP_TARGETS" {
		t.Fatalf("expected env_var tag to be set, got %+v", target.Tags)
	}
}

func TestEnvCandidateToTargetUnsupportedScheme(t *testing.T) {
	_, err := envCandidateToTarget("BAD_TARGETS", "ftp://example.com/service", 1)
	if err == nil {
		t.Fatalf("expected error for unsupported scheme")
	}
}

func TestInferProtoFromEndpoint(t *testing.T) {
	if proto, _ := inferProtoFromEndpoint("http://example.com/service/acp"); proto != targets.ProtocolACP {
		t.Fatalf("expected /acp endpoint to infer ACP protocol, got %q", proto)
	}
	if proto, _ := inferProtoFromEndpoint("https://example.com/api/mcp/status"); proto != targets.ProtocolMCP {
		t.Fatalf("expected /mcp endpoint to infer MCP protocol, got %q", proto)
	}
	if proto, _ := inferProtoFromEndpoint("https://example.com/other"); proto != targets.ProtocolHTTP {
		t.Fatalf("expected default HTTP protocol, got %q", proto)
	}
}

func TestInferProtoFromEnvOrURL(t *testing.T) {
	if proto := inferProtoFromEnvOrURL("ACP_TARGETS", "https://host/service"); proto != targets.ProtocolACP {
		t.Fatalf("expected env hint to choose ACP, got %q", proto)
	}
	if proto := inferProtoFromEnvOrURL("GENERIC_TARGETS", "https://host/api/mcp"); proto != targets.ProtocolMCP {
		t.Fatalf("expected URL hint to choose MCP, got %q", proto)
	}
	if proto := inferProtoFromEnvOrURL("GENERIC_TARGETS", "https://host/api"); proto != targets.ProtocolHTTP {
		t.Fatalf("expected fallback HTTP protocol, got %q", proto)
	}
}

func TestCandidateLooksLikeMCPOrACP(t *testing.T) {
	if !candidateLooksLikeMCPOrACP("Model Context Protocol", "https://example.com", "", "") {
		t.Fatalf("expected MCP keywords to be detected")
	}
	if !candidateLooksLikeMCPOrACP("Agent Client Protocol", "https://example.com", "", "") {
		t.Fatalf("expected ACP keywords to be detected")
	}
	if candidateLooksLikeMCPOrACP("Other Service", "https://example.com", "notes", "section") {
		t.Fatalf("did not expect unrelated candidate to match")
	}
}

func TestTransportFromURL(t *testing.T) {
	if got := transportFromURL("ws://example.com/socket"); got != targets.TransportWebSocket {
		t.Fatalf("expected websocket transport, got %q", got)
	}
	if got := transportFromURL("https://example.com/events/stream"); got != targets.TransportSSE {
		t.Fatalf("expected SSE transport, got %q", got)
	}
	if got := transportFromURL("invalid url"); got != targets.TransportHTTP {
		t.Fatalf("expected fallback HTTP transport, got %q", got)
	}
}

func TestDefaultStatusCodesForTransport(t *testing.T) {
	tests := map[targets.Transport][]int{
		targets.TransportWebSocket: {101},
		targets.TransportSSE:       {200},
		targets.TransportHTTP:      {200},
	}
	for transport, want := range tests {
		got := defaultStatusCodesForTransport(transport)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("defaultStatusCodesForTransport(%q) = %v, want %v", transport, got, want)
		}
	}
}

func TestDedupeKey(t *testing.T) {
	key := dedupeKey(targets.Target{
		Protocol:  targets.ProtocolMCP,
		Transport: targets.TransportHTTP,
		Endpoint:  "https://Example.com/MCP",
	})
	want := "mcp|http|https://example.com/mcp"
	if key != want {
		t.Fatalf("dedupeKey returned %q, want %q", key, want)
	}
}

func TestHostKey(t *testing.T) {
	if got := hostKey("https://Example.COM/path"); got != "example-com" {
		t.Fatalf("expected sanitized host key, got %q", got)
	}
	if got := hostKey("://bad"); got != "llms" {
		t.Fatalf("expected fallback host key for invalid URL, got %q", got)
	}
}
