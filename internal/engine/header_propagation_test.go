package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/james-gibson/smoke-alarm/internal/auth"
	"github.com/james-gibson/smoke-alarm/internal/targets"
)

// bearerTarget builds a non-stdio target pointed at the given endpoint with a
// bearer auth block whose secret is resolved from the named env var.
func bearerTarget(endpoint, envVar string) targets.Target {
	return targets.Target{
		ID:        "t1",
		Enabled:   true,
		Protocol:  targets.ProtocolHTTP,
		Name:      "t1",
		Endpoint:  endpoint,
		Transport: targets.TransportHTTP,
		Expected:  targets.ExpectedBehavior{HealthyStatusCodes: []int{http.StatusOK}},
		Auth: targets.AuthConfig{
			Type:      targets.AuthBearer,
			SecretRef: "env://" + envVar,
		},
	}
}

// buildBearerHeaders resolves the target's bearer auth material the way both
// Engine.executeCheck and runOneShotChecks do, so the test exercises the exact
// material that was being dropped before #85.
func buildBearerHeaders(t *testing.T, target targets.Target) map[string]string {
	t.Helper()
	mat, err := auth.NewManager().BuildHeaders(context.Background(), target.Auth)
	if err != nil {
		t.Fatalf("BuildHeaders: %v", err)
	}
	return mat.Headers
}

// TestStdioProberForwardsAuthHeadersToHTTPFallback is the #85 regression: a
// non-stdio target's auth headers must survive the StdioProber→HTTPProber
// fallback. Without the fix, the fallback receives nil and the guarded surface
// is misreported as unhealthy (401).
func TestStdioProberForwardsAuthHeadersToHTTPFallback(t *testing.T) {
	const secret = "test-secret"
	t.Setenv("SMOKE_ALARM_TEST_KEY", secret)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+secret {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	target := bearerTarget(srv.URL, "SMOKE_ALARM_TEST_KEY")
	headers := buildBearerHeaders(t, target)

	got, err := NewStdioProber().Probe(context.Background(), target, headers)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if got.State != targets.StateHealthy {
		t.Fatalf("state = %s, want healthy (message: %s)", got.State, got.Message)
	}
}

// TestStdioProberForwardsAuthHeadersToMCPHandshake reproduces the exact symptom
// in the issue body: an MCP surface behind a bearer must not answer "initialize
// failed with status 401" when the prober drops the Authorization header.
func TestStdioProberForwardsAuthHeadersToMCPHandshake(t *testing.T) {
	const secret = "mcp-secret"
	t.Setenv("SMOKE_ALARM_TEST_MCP_KEY", secret)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+secret {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		var req struct {
			ID     *int   `json:"id"`
			Method string `json:"method"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if req.Method == "notifications/initialized" {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  initResult("tools"),
		})
	}))
	t.Cleanup(srv.Close)

	target := bearerTarget(srv.URL, "SMOKE_ALARM_TEST_MCP_KEY")
	target.Protocol = targets.ProtocolMCP
	target.Check.HandshakeProfile = "base"

	got, err := NewStdioProber().Probe(context.Background(), target, buildBearerHeaders(t, target))
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if got.State != targets.StateHealthy {
		t.Fatalf("state = %s, want healthy (message: %s)", got.State, got.Message)
	}
}

// TestHTTPProberMergesStaticHeaders covers the related #85 symptom: a static
// targets[].headers map is carried on Target.Headers and must be attached to the
// outgoing request even when no auth block is present.
func TestHTTPProberMergesStaticHeaders(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom") != "value" {
			http.Error(w, "missing static header", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	target := targets.Target{
		ID:        "t1",
		Enabled:   true,
		Protocol:  targets.ProtocolHTTP,
		Name:      "t1",
		Endpoint:  srv.URL,
		Transport: targets.TransportHTTP,
		Expected:  targets.ExpectedBehavior{HealthyStatusCodes: []int{http.StatusOK}},
		Headers:   map[string]string{"X-Custom": "value"},
	}

	got, err := NewHTTPProber().Probe(context.Background(), target, nil)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if got.State != targets.StateHealthy {
		t.Fatalf("state = %s, want healthy (message: %s)", got.State, got.Message)
	}
}

// TestMergeHeadersAuthWinsOnCollision pins the precedence: an auth-derived
// header overrides a static header of the same name, since the auth material is
// resolved from the target's secret at probe time.
func TestMergeHeadersAuthWinsOnCollision(t *testing.T) {
	merged := mergeHeaders(
		map[string]string{"Authorization": "static", "X-Keep": "static"},
		map[string]string{"Authorization": "Bearer dynamic"},
	)
	if merged["Authorization"] != "Bearer dynamic" {
		t.Fatalf("Authorization = %q, want auth-derived value", merged["Authorization"])
	}
	if merged["X-Keep"] != "static" {
		t.Fatalf("X-Keep = %q, want static value preserved", merged["X-Keep"])
	}
}
