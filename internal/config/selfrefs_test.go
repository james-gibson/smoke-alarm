package config

import "testing"

func TestRewriteSelfReferencesFollowsMovedListeners(t *testing.T) {
	moved := map[string]string{
		"localhost:18091": "localhost:18092",
		"localhost:28877": "localhost:28878",
	}

	cfg := Config{
		Targets: []TargetConfig{
			{
				ID:       "hosted-mcp-http",
				Endpoint: "http://0.0.0.0:18091/mcp",
			},
			{
				ID:       "hosted-acp-sse",
				Endpoint: "http://0.0.0.0:18091/acp?transport=sse",
			},
			{
				ID:       "oauth-callback",
				Endpoint: "http://localhost:28877/oauth/mock/status",
				Auth:     TargetAuthConfig{RedirectURL: "http://localhost:28877/oauth/callback"},
			},
			{
				ID:       "hurl-own-health",
				Endpoint: "http://localhost:28088/healthz",
				Check: TargetCheckConfig{
					HURLTests: []HURLTestConfig{{Name: "own", Endpoint: "http://0.0.0.0:18091/hosted/status"}},
				},
			},
			{
				ID:       "remote",
				Endpoint: "https://mcp.example.com:18091/mcp",
			},
			{
				ID:       "other-port",
				Endpoint: "http://localhost:18099/mcp",
			},
		},
	}

	got := RewriteSelfReferences(cfg, moved)

	wantEndpoints := map[string]string{
		"hosted-mcp-http": "http://0.0.0.0:18092/mcp",
		"hosted-acp-sse":  "http://0.0.0.0:18092/acp?transport=sse",
		"oauth-callback":  "http://localhost:28878/oauth/mock/status",
		"hurl-own-health": "http://localhost:28088/healthz",
		// A remote host that happens to use the port is not ours to rewrite.
		"remote":     "https://mcp.example.com:18091/mcp",
		"other-port": "http://localhost:18099/mcp",
	}
	for _, target := range got.Targets {
		want, ok := wantEndpoints[target.ID]
		if !ok {
			continue
		}
		if target.Endpoint != want {
			t.Errorf("target %s endpoint = %q, want %q", target.ID, target.Endpoint, want)
		}
	}

	if got.Targets[2].Auth.RedirectURL != "http://localhost:28878/oauth/callback" {
		t.Errorf("redirect_url = %q, want the bound callback", got.Targets[2].Auth.RedirectURL)
	}
	if hurl := got.Targets[3].Check.HURLTests[0].Endpoint; hurl != "http://0.0.0.0:18092/hosted/status" {
		t.Errorf("hurl endpoint = %q, want the bound hosted address", hurl)
	}
}

func TestRewriteSelfReferencesWithoutMovesIsAPassThrough(t *testing.T) {
	cfg := Config{Targets: []TargetConfig{{ID: "t", Endpoint: "http://localhost:18091/mcp"}}}
	got := RewriteSelfReferences(cfg, nil)
	if got.Targets[0].Endpoint != cfg.Targets[0].Endpoint {
		t.Fatalf("endpoint = %q, want it untouched", got.Targets[0].Endpoint)
	}
}

func TestSameListenerAddr(t *testing.T) {
	cases := []struct {
		name       string
		target     string
		configured string
		want       bool
	}{
		{"exact", "localhost:18091", "localhost:18091", true},
		{"wildcard host same port", "0.0.0.0:18091", "localhost:18091", true},
		{"loopback host same port", "127.0.0.1:18091", "localhost:18091", true},
		{"empty configured host same port", "0.0.0.0:18091", ":18091", true},
		{"different port", "localhost:18092", "localhost:18091", false},
		{"remote host same port", "mcp.example.com:18091", "localhost:18091", false},
		{"ephemeral configured", "0.0.0.0:18091", ":0", false},
		{"no port", "localhost", "localhost:18091", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SameListenerAddr(tc.target, tc.configured); got != tc.want {
				t.Fatalf("SameListenerAddr(%q, %q) = %v, want %v", tc.target, tc.configured, got, tc.want)
			}
		})
	}
}
