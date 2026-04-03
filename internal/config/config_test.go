package config

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestApplyEfficiencyProfileMediumDefaults(t *testing.T) {
	svc := ServiceConfig{}
	applyEfficiencyProfile(&svc)

	if svc.MaxWorkers != 4 {
		t.Fatalf("expected medium profile to set max workers to 4, got %d", svc.MaxWorkers)
	}
	if svc.PollInterval != "15s" {
		t.Fatalf("expected medium profile to set poll interval to 15s, got %q", svc.PollInterval)
	}
	if svc.Timeout != "5s" {
		t.Fatalf("expected medium profile to set timeout to 5s, got %q", svc.Timeout)
	}
}

func TestApplyEfficiencyProfileLowKeepsConfiguredValues(t *testing.T) {
	svc := ServiceConfig{
		EfficiencyProfile: "low",
		MaxWorkers:        2,
		PollInterval:      "30s",
		Timeout:           "8s",
	}
	applyEfficiencyProfile(&svc)

	if svc.MaxWorkers != 2 {
		t.Fatalf("expected low profile to keep existing max workers, got %d", svc.MaxWorkers)
	}
	if svc.PollInterval != "30s" {
		t.Fatalf("expected low profile to keep existing poll interval, got %q", svc.PollInterval)
	}
	if svc.Timeout != "8s" {
		t.Fatalf("expected low profile to keep existing timeout, got %q", svc.Timeout)
	}
}

func TestApplyEfficiencyProfileHighSetsAggressiveDefaults(t *testing.T) {
	svc := ServiceConfig{
		EfficiencyProfile: "high",
	}
	applyEfficiencyProfile(&svc)

	if svc.MaxWorkers < 8 {
		t.Fatalf("expected high profile to set max workers >= 8, got %d", svc.MaxWorkers)
	}
	if svc.PollInterval != "5s" {
		t.Fatalf("expected high profile to set poll interval to 5s, got %q", svc.PollInterval)
	}
	if svc.Timeout != "3s" {
		t.Fatalf("expected high profile to set timeout to 3s, got %q", svc.Timeout)
	}
}

func TestParsePositiveDuration(t *testing.T) {
	got, err := parsePositiveDuration("750ms")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 750*time.Millisecond {
		t.Fatalf("expected 750ms, got %s", got)
	}

	if _, err := parsePositiveDuration("0s"); err == nil {
		t.Fatalf("expected error for zero duration")
	}

	if _, err := parsePositiveDuration("-1s"); err == nil {
		t.Fatalf("expected error for negative duration")
	}

	if _, err := parsePositiveDuration("not a duration"); err == nil {
		t.Fatalf("expected error for invalid duration string")
	}
}

func TestValidateURL(t *testing.T) {
	if err := validateURL("http://example.com/path"); err != nil {
		t.Fatalf("expected valid URL, got error: %v", err)
	}

	if err := validateURL("stdio://local"); err != nil {
		t.Fatalf("expected stdio scheme to be accepted, got error: %v", err)
	}

	if err := validateURL("example.com"); err == nil {
		t.Fatalf("expected error for URL without scheme")
	}
}

func TestValidateRemoteLLMSTxtURIRequiresHTTPS(t *testing.T) {
	if err := validateRemoteLLMSTxtURI("https://example.com/llms.txt", true); err != nil {
		t.Fatalf("expected https URL to pass when HTTPS required: %v", err)
	}

	if err := validateRemoteLLMSTxtURI("http://example.com/llms.txt", true); err == nil {
		t.Fatalf("expected http URL to fail when HTTPS required")
	}

	if err := validateRemoteLLMSTxtURI("http://localhost/llms.txt", false); err != nil {
		t.Fatalf("expected localhost http URL to be allowed when HTTPS not required: %v", err)
	}
}

func TestInferTransport(t *testing.T) {
	if got := inferTransport("wss://example.com/ws"); got != TransportWebSocket {
		t.Fatalf("expected websocket transport, got %q", got)
	}
	if got := inferTransport("stdio://local"); got != TransportStdio {
		t.Fatalf("expected stdio transport, got %q", got)
	}
	if got := inferTransport("://bad"); got != TransportHTTP {
		t.Fatalf("expected invalid URL to fall back to http transport, got %q", got)
	}
}

func TestConfigValidateAggregatesProblems(t *testing.T) {
	cfg := Config{
		Version: "1",
		Service: ServiceConfig{
			Name:         "itest",
			Mode:         "invalid-mode",
			PollInterval: "5s",
			Timeout:      "5s",
			LogLevel:     LogInfo,
		},
		Runtime: RuntimeConfig{
			LockFile:                "/tmp/lock",
			StateDir:                "/tmp/state",
			BaselineFile:            "/tmp/baseline",
			GracefulShutdownTimeout: "5s",
		},
		Health: HealthConfig{
			ListenAddr: "127.0.0.1:0",
			Endpoints: HealthRoutes{
				Healthz: "/healthz",
				Readyz:  "/readyz",
				Status:  "/status",
			},
		},
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatalf("expected validation error for invalid service mode")
	}

	var ve ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T (%v)", err, err)
	}

	if !containsProblem(ve.Problems, "service.mode") {
		t.Fatalf("expected error to mention service.mode, problems: %v", ve.Problems)
	}
}

func TestInSet(t *testing.T) {
	if !inSet("alpha", "beta", "alpha", "gamma") {
		t.Fatalf("expected value to be found in set")
	}
	if inSet("delta", "alpha", "beta") {
		t.Fatalf("did not expect value to be found in set")
	}
}

func TestReconcileOpenCodeLocalTarget(t *testing.T) {
	raw := `
version: "1"
targets:
  - id: "local-mcp"
    enabled: true
    protocol: "mcp"
    type: "local"
    command: ["npx", "-y", "@modelcontextprotocol/server-everything"]
    environment:
      NODE_ENV: "production"
`
	cfg, err := LoadBytes([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tgt := cfg.Targets[0]
	if tgt.Transport != TransportStdio {
		t.Errorf("expected transport=stdio, got %q", tgt.Transport)
	}
	if tgt.Type != TargetTypeLocal {
		t.Errorf("expected type=local, got %q", tgt.Type)
	}
	if tgt.Stdio.Command != "npx" {
		t.Errorf("expected stdio.command=npx, got %q", tgt.Stdio.Command)
	}
	if len(tgt.Stdio.Args) != 2 || tgt.Stdio.Args[0] != "-y" {
		t.Errorf("expected stdio.args=[-y @modelcontextprotocol/server-everything], got %v", tgt.Stdio.Args)
	}
	if tgt.Stdio.Env["NODE_ENV"] != "production" {
		t.Errorf("expected stdio.env[NODE_ENV]=production, got %v", tgt.Stdio.Env)
	}
}

func TestReconcileOpenCodeRemoteTarget(t *testing.T) {
	raw := `
version: "1"
targets:
  - id: "remote-mcp"
    enabled: true
    protocol: "mcp"
    type: "remote"
    url: "https://mcp.example.com/v1"
    headers:
      X-Custom: "value"
    oauth:
      client_id: "my-client"
      client_secret_ref: "env://MCP_SECRET"
      token_url: "https://auth.example.com/token"
      scope: "read write"
`
	cfg, err := LoadBytes([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tgt := cfg.Targets[0]
	if tgt.Type != TargetTypeRemote {
		t.Errorf("expected type=remote, got %q", tgt.Type)
	}
	if tgt.Endpoint != "https://mcp.example.com/v1" {
		t.Errorf("expected endpoint from url, got %q", tgt.Endpoint)
	}
	if tgt.Auth.Type != AuthOAuth {
		t.Errorf("expected auth.type=oauth, got %q", tgt.Auth.Type)
	}
	if tgt.Auth.ClientID != "my-client" {
		t.Errorf("expected auth.client_id=my-client, got %q", tgt.Auth.ClientID)
	}
	if tgt.Auth.SecretRef != "env://MCP_SECRET" {
		t.Errorf("expected auth.secret_ref from oauth.client_secret_ref, got %q", tgt.Auth.SecretRef)
	}
	if tgt.Auth.TokenURL != "https://auth.example.com/token" {
		t.Errorf("expected auth.token_url from oauth, got %q", tgt.Auth.TokenURL)
	}
	if len(tgt.Auth.Scopes) != 2 || tgt.Auth.Scopes[0] != "read" {
		t.Errorf("expected auth.scopes=[read write], got %v", tgt.Auth.Scopes)
	}
	if tgt.Headers["X-Custom"] != "value" {
		t.Errorf("expected headers[X-Custom]=value, got %v", tgt.Headers)
	}
}

func TestReconcileExistingFieldsTakePrecedence(t *testing.T) {
	raw := `
version: "1"
targets:
  - id: "both-set"
    enabled: true
    protocol: "mcp"
    endpoint: "https://canonical.example.com"
    url: "https://should-be-ignored.example.com"
    transport: "http"
    stdio:
      command: "real-cmd"
    command: ["ignored-cmd"]
`
	cfg, err := LoadBytes([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tgt := cfg.Targets[0]
	if tgt.Endpoint != "https://canonical.example.com" {
		t.Errorf("expected canonical endpoint to win, got %q", tgt.Endpoint)
	}
	if tgt.Stdio.Command != "real-cmd" {
		t.Errorf("expected canonical stdio.command to win, got %q", tgt.Stdio.Command)
	}
	if tgt.Type != TargetTypeRemote {
		t.Errorf("expected type=remote (inferred from transport=http), got %q", tgt.Type)
	}
}

func TestTypeInferredFromTransport(t *testing.T) {
	raw := `
version: "1"
targets:
  - id: "stdio-no-type"
    enabled: true
    protocol: "mcp"
    transport: "stdio"
    stdio:
      command: "test-cmd"
  - id: "http-no-type"
    enabled: true
    protocol: "mcp"
    endpoint: "https://example.com"
    transport: "http"
`
	cfg, err := LoadBytes([]byte(raw))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Targets[0].Type != TargetTypeLocal {
		t.Errorf("expected stdio target type=local, got %q", cfg.Targets[0].Type)
	}
	if cfg.Targets[1].Type != TargetTypeRemote {
		t.Errorf("expected http target type=remote, got %q", cfg.Targets[1].Type)
	}
}

func containsProblem(problems []string, want string) bool {
	for _, p := range problems {
		if strings.Contains(p, want) {
			return true
		}
	}
	return false
}
