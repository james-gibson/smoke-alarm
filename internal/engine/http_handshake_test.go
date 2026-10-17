package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/james-gibson/smoke-alarm/internal/targets"
)

// handshakeStub serves one JSON-RPC answer per method. A method absent from
// `answers` is refused with a JSON-RPC error, the way a surface that does not
// implement it would.
func handshakeStub(t *testing.T, answers map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		if res, ok := answers[req.Method]; ok {
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": res})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0", "id": req.ID,
			"error": map[string]any{"code": -32601, "message": "method not found: " + req.Method},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// initResult is an `initialize` answer advertising the named capability families.
func initResult(families ...string) map[string]any {
	caps := map[string]any{}
	for _, name := range families {
		caps[name] = map[string]any{}
	}
	return map[string]any{"protocolVersion": "2024-11-05", "capabilities": caps, "serverInfo": map[string]any{"name": "stub"}}
}

func handshakeTarget(endpoint string, minCaps []string) targets.Target {
	return targets.Target{
		ID: "t1", Enabled: true, Protocol: targets.ProtocolMCP, Name: "t1",
		Endpoint: endpoint, Transport: "http",
		Expected: targets.ExpectedBehavior{MinCapabilities: minCaps},
		Check: targets.CheckPolicy{
			HandshakeProfile: "strict",
			RequiredMethods:  []string{"initialize", "tools/list"},
		},
	}
}

// TestHTTPHandshakeReachesDegraded is the point of #34: a target check must be
// able to say "answering, but not as declared" — the middle rung — instead of
// collapsing it into either healthy or down.
func TestHTTPHandshakeReachesDegraded(t *testing.T) {
	p := NewHTTPProber()
	ctx := context.Background()

	cases := []struct {
		name      string
		answers   map[string]any
		minCaps   []string
		wantState targets.HealthState
	}{
		{
			name: "everything answered and nothing declared",
			answers: map[string]any{
				"initialize": initResult("tools", "resources"),
				"tools/list": map[string]any{"tools": []any{}},
			},
			wantState: targets.StateHealthy,
		},
		{
			name: "everything answered and everything declared is advertised",
			answers: map[string]any{
				"initialize": initResult("tools", "resources"),
				"tools/list": map[string]any{"tools": []any{}},
			},
			minCaps:   []string{"tools/list", "resources/list"},
			wantState: targets.StateHealthy,
		},
		{
			name: "answers, but does not advertise what the target declared",
			answers: map[string]any{
				"initialize": initResult("tools"),
				"tools/list": map[string]any{"tools": []any{}},
			},
			minCaps:   []string{"tools/list", "resources/list"},
			wantState: targets.StateDegraded,
		},
		{
			name: "answers some of the required methods",
			answers: map[string]any{
				"initialize": initResult("tools"),
			},
			wantState: targets.StateDegraded,
		},
		{
			name:      "answers none of the required methods",
			answers:   map[string]any{},
			wantState: targets.StateUnhealthy,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := handshakeStub(t, tc.answers)
			got, err := p.Probe(ctx, handshakeTarget(srv.URL, tc.minCaps), nil)
			if err != nil {
				t.Fatalf("Probe: %v", err)
			}
			if got.State != tc.wantState {
				t.Errorf("state = %s, want %s\n  message: %s", got.State, tc.wantState, got.Message)
			}
		})
	}
}

// The result must carry what was *advertised*, not only that the handshake
// passed: the gap between what a target declares and what a surface promises is
// the thing worth reporting.
func TestHTTPHandshakeReportsAdvertisedCapabilities(t *testing.T) {
	p := NewHTTPProber()
	srv := handshakeStub(t, map[string]any{
		"initialize": initResult("tools"),
		"tools/list": map[string]any{"tools": []any{}},
	})

	got, err := p.Probe(context.Background(), handshakeTarget(srv.URL, []string{"resources/list"}), nil)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if len(got.Capabilities) != 1 || got.Capabilities[0] != "tools" {
		t.Errorf("capabilities = %v, want [tools]", got.Capabilities)
	}
	gap, _ := got.Details["capability_gap"].([]string)
	if len(gap) != 1 || gap[0] != "resources/list" {
		t.Errorf("capability_gap = %v, want [resources/list]", gap)
	}
	if len(got.Details["exercised_methods"].([]string)) != 2 {
		t.Errorf("exercised = %v, want both methods", got.Details["exercised_methods"])
	}
}

// A surface that does not reach the endpoint at all is an outage, not a partial
// handshake — the transport error is returned unchanged.
func TestHTTPHandshakeKeepsTransportErrorsAsErrors(t *testing.T) {
	p := NewHTTPProber()
	target := handshakeTarget("http://127.0.0.1:1/mcp", nil)

	if _, err := p.Probe(context.Background(), target, nil); err == nil {
		t.Fatal("expected a transport error for an unreachable endpoint")
	}
}
