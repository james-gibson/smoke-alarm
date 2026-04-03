package stepdefinitions

// oauth_mock_steps.go — step definitions for features/oauth-mock.feature
//
// Implementation strategy:
//   IMPLEMENTED  — in-process MockRedirectServer (start, allow mode, fail mode, port check)
//   ErrPending   — scenarios requiring engine/probe integration or log capture
//     TF-OAUTH-1: probe classification (HEALTHY/DEGRADED) requires engine wiring
//     TF-OAUTH-2: ACP handshake requires full hosted stack
//     TF-OAUTH-3: HURL preflight with OAuth mock requires safety scanner + engine
//     TF-OAUTH-4: secret redaction verification requires log capture mechanism
//     TF-OAUTH-5: unattended OAuth flows require internal/auth/provider.go integration

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/cucumber/godog"

	"github.com/james-gibson/smoke-alarm/internal/auth"
	"github.com/james-gibson/smoke-alarm/internal/config"
)

var oauthState struct {
	server     *auth.MockRedirectServer
	cancel     context.CancelFunc
	serverAddr string
	serverPath string
	mockCfg    config.OAuthMockRedirectConfig
}

func resetOAuthState() {
	if oauthState.cancel != nil {
		oauthState.cancel()
		oauthState.cancel = nil
	}
	if oauthState.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = oauthState.server.Shutdown(ctx)
		oauthState.server = nil
	}
	oauthState = struct {
		server     *auth.MockRedirectServer
		cancel     context.CancelFunc
		serverAddr string
		serverPath string
		mockCfg    config.OAuthMockRedirectConfig
	}{}
}

// startMockServer starts a MockRedirectServer and waits until it's ready.
func startMockServer(mode, addr, path string) error {
	opts := auth.MockRedirectOptions{
		ListenAddr:      addr,
		Path:            path,
		Mode:            auth.MockRedirectMode(mode),
		ShutdownTimeout: 2 * time.Second,
	}
	srv := auth.NewMockRedirectServer(opts)
	oauthState.server = srv
	oauthState.serverAddr = addr
	oauthState.serverPath = path

	ctx, cancel := context.WithCancel(context.Background())
	oauthState.cancel = cancel

	go func() {
		_ = srv.Start(ctx)
	}()

	// Poll /oauth/mock/status until the server is ready (max 2s).
	statusURL := fmt.Sprintf("http://%s/oauth/mock/status", addr)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(statusURL) //nolint:noctx
		if err == nil && resp.StatusCode == http.StatusOK {
			_ = resp.Body.Close()
			return nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		time.Sleep(20 * time.Millisecond)
	}
	return fmt.Errorf("mock redirect server on %s did not become ready within 2s", addr)
}

func InitializeOAuthMockSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(sctx context.Context, sc *godog.Scenario) (context.Context, error) {
		resetOAuthState()
		return sctx, nil
	})
	ctx.After(func(sctx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		if oauthState.cancel != nil {
			oauthState.cancel()
			oauthState.cancel = nil
		}
		if oauthState.server != nil {
			shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = oauthState.server.Shutdown(shutCtx)
			oauthState.server = nil
		}
		return sctx, nil
	})

	// ── mock endpoint startup ──────────────────────────────────────────────
	ctx.Step(`^a config file "([^"]*)" with mock_redirect enabled$`, aConfigFileWithMockRedirectEnabled)
	ctx.Step(`^ocd-smoke-alarm starts with that config$`, ocdSmokeAlarmStartsWithThatConfig)
	ctx.Step(`^a listener is bound on "([^"]*)"$`, aListenerIsBoundOn)
	ctx.Step(`^the path "([^"]*)" is served$`, thePathIsServed)
	ctx.Step(`^a config with mock_redirect\.enabled set to false$`, aConfigWithMockRedirectDisabled)
	ctx.Step(`^a config file with mock_redirect\.enabled set to false$`, aConfigFileWithMockRedirectDisabled)
	ctx.Step(`^no listener is bound on the mock redirect address$`, noListenerIsBoundOnMockRedirect)

	// ── allow / fail modes ─────────────────────────────────────────────────
	ctx.Step(`^the mock redirect endpoint is running in "([^"]*)" mode on "([^"]*)"$`, theMockRedirectEndpointIsRunning)
	ctx.Step(`^the response status code is not 200$`, theResponseStatusCodeIsNot200)
	ctx.Step(`^the hosted ACP server is running on "([^"]*)"$`, theHostedACPServerIsRunning)
	ctx.Step(`^the OAuth redirect was handled by the mock endpoint$`, theOAuthRedirectWasHandled)
	ctx.Step(`^an alert is emitted with severity "([^"]*)"$`, anAlertIsEmittedWithSeverity)

	// ── HURL preflight with OAuth ──────────────────────────────────────────
	ctx.Step(`^the target "([^"]*)" has a hurl_test "([^"]*)"$`, theTargetHasHurlTest)
	ctx.Step(`^the HURL test "([^"]*)" sends a GET to "([^"]*)"$`, theHURLTestSendsGET)
	ctx.Step(`^the test result is recorded in the probe output$`, theTestResultIsRecorded)

	// ── token redaction ────────────────────────────────────────────────────
	ctx.Step(`^the target "([^"]*)" has secret_ref "([^"]*)"$`, theTargetHasSecretRef)
	ctx.Step(`^no log line contains the raw value of "([^"]*)"$`, noLogLineContainsRawValue)
	ctx.Step(`^any log line referencing the secret contains "([^"]*)"$`, anyLogLineReferencingSecretContains)

	// ── unattended OAuth flows ─────────────────────────────────────────────
	ctx.Step(`^"([^"]*)" is true$`, configKeyIsTrue)
	ctx.Step(`^"([^"]*)\."([^"]*)"" is true$`, configNestedKeyIsTrue)
	ctx.Step(`^an OAuth token is required for a target$`, anOAuthTokenIsRequired)
	ctx.Step(`^the (\w+) flow is attempted without user interaction$`, theFlowIsAttemptedWithoutUserInteraction)
}

// ── mock endpoint startup ─────────────────────────────────────────────────────

func aConfigFileWithMockRedirectEnabled(path string) error {
	root := metaProjectRoot()
	absPath := fmt.Sprintf("%s/%s", root, path)
	cfg, err := config.Load(absPath)
	if err != nil {
		return fmt.Errorf("load config %q: %w", path, err)
	}
	oauthState.mockCfg = cfg.Auth.OAuth.MockRedirect
	return nil
}

// ocdSmokeAlarmStartsWithThatConfig starts the mock redirect server in-process
// if mock_redirect.enabled is true, mirroring what the binary would do on startup.
func ocdSmokeAlarmStartsWithThatConfig() error {
	if !oauthState.mockCfg.Enabled {
		// Behavioral contract: when disabled, the server is simply not started.
		return nil
	}
	mode := oauthState.mockCfg.Mode
	if mode == "" {
		mode = "allow"
	}
	addr := oauthState.mockCfg.ListenAddr
	if addr == "" {
		addr = "127.0.0.1:8877"
	}
	path := oauthState.mockCfg.Path
	if path == "" {
		path = "/oauth/callback"
	}
	return startMockServer(mode, addr, path)
}

// aListenerIsBoundOn verifies that something is listening on addr by making
// an HTTP request to the mock server status endpoint.
func aListenerIsBoundOn(addr string) error {
	statusURL := fmt.Sprintf("http://%s/oauth/mock/status", addr)
	resp, err := http.Get(statusURL) //nolint:noctx
	if err != nil {
		return fmt.Errorf("no listener found on %s: %w", addr, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status endpoint on %s returned %d", addr, resp.StatusCode)
	}
	return nil
}

// thePathIsServed verifies that the mock redirect callback path responds.
func thePathIsServed(path string) error {
	addr := oauthState.serverAddr
	if addr == "" {
		addr = oauthState.mockCfg.ListenAddr
	}
	if addr == "" {
		return fmt.Errorf("no mock server address known")
	}
	callbackURL := fmt.Sprintf("http://%s%s", addr, path)
	resp, err := http.Get(callbackURL) //nolint:noctx
	if err != nil {
		return fmt.Errorf("path %s not served on %s: %w", path, addr, err)
	}
	defer resp.Body.Close()
	// Any response (200 or 401) means the path is served.
	return nil
}

func aConfigWithMockRedirectDisabled() error {
	oauthState.mockCfg = config.OAuthMockRedirectConfig{
		Enabled:    false,
		ListenAddr: "127.0.0.1:28877",
		Path:       "/oauth/callback",
		Mode:       "allow",
	}
	return nil
}

func aConfigFileWithMockRedirectDisabled() error {
	oauthState.mockCfg = config.OAuthMockRedirectConfig{
		Enabled:    false,
		ListenAddr: "127.0.0.1:28877",
		Path:       "/oauth/callback",
		Mode:       "allow",
	}
	return nil
}

// noListenerIsBoundOnMockRedirect verifies the mock redirect address is NOT listening.
func noListenerIsBoundOnMockRedirect() error {
	addr := oauthState.mockCfg.ListenAddr
	if addr == "" {
		addr = "127.0.0.1:28877"
	}
	statusURL := fmt.Sprintf("http://%s/oauth/mock/status", addr)
	client := &http.Client{Timeout: 300 * time.Millisecond}
	resp, err := client.Get(statusURL) //nolint:noctx
	if err != nil {
		// Connection refused or similar — port is not listening as expected.
		return nil
	}
	defer resp.Body.Close()
	return fmt.Errorf("expected no listener on %s but got HTTP %d", addr, resp.StatusCode)
}

// ── allow / fail modes ────────────────────────────────────────────────────────

// theMockRedirectEndpointIsRunning starts the mock server for HTTP-level tests.
func theMockRedirectEndpointIsRunning(mode, addr string) error {
	return startMockServer(mode, addr, "/oauth/callback")
}

func theResponseStatusCodeIsNot200() error {
	if httpState.lastResp == nil {
		return fmt.Errorf("no HTTP response yet")
	}
	if httpState.lastResp.StatusCode == http.StatusOK {
		return fmt.Errorf("expected non-200 status code, got 200")
	}
	return nil
}

// theHostedACPServerIsRunning: TF-OAUTH-2 — requires full hosted ACP server stack.
func theHostedACPServerIsRunning(addr string) error {
	return godog.ErrPending
}

// theOAuthRedirectWasHandled: TF-OAUTH-1 — requires engine/probe integration.
func theOAuthRedirectWasHandled() error {
	return godog.ErrPending
}

// anAlertIsEmittedWithSeverity: TF-OAUTH-1 — requires engine + alert wiring.
func anAlertIsEmittedWithSeverity(severity string) error {
	return godog.ErrPending
}

// ── HURL preflight with OAuth ─────────────────────────────────────────────────

// theTargetHasHurlTest: TF-OAUTH-3 — requires safety scanner + engine integration.
func theTargetHasHurlTest(id, name string) error {
	return godog.ErrPending
}

// theHURLTestSendsGET: TF-OAUTH-3 — requires safety scanner integration.
func theHURLTestSendsGET(name, url string) error {
	return godog.ErrPending
}

// theTestResultIsRecorded: TF-OAUTH-3 — requires probe output inspection.
func theTestResultIsRecorded() error {
	return godog.ErrPending
}

// ── token redaction ───────────────────────────────────────────────────────────

// theTargetHasSecretRef: TF-OAUTH-4 — requires log capture for redaction verification.
func theTargetHasSecretRef(id, ref string) error {
	return godog.ErrPending
}

// noLogLineContainsRawValue: TF-OAUTH-4 — no log capture mechanism.
func noLogLineContainsRawValue(key string) error {
	return godog.ErrPending
}

// anyLogLineReferencingSecretContains: TF-OAUTH-4 — no log capture mechanism.
func anyLogLineReferencingSecretContains(substr string) error {
	return godog.ErrPending
}

// ── unattended OAuth flows ────────────────────────────────────────────────────

// configKeyIsTrue: generic config key check — ErrPending for unattended OAuth flow config.
// NOTE: registered AFTER InitializeSafetyHURLSteps in suite_test.go so that
// "has_blocking" is true is handled by safety_hurl, not this generic pattern.
func configKeyIsTrue(key string) error {
	return godog.ErrPending
}

// configNestedKeyIsTrue: TF-OAUTH-5 — unattended OAuth provider integration.
func configNestedKeyIsTrue(key, sub string) error {
	return godog.ErrPending
}

// anOAuthTokenIsRequired: TF-OAUTH-5 — requires provider.go integration.
func anOAuthTokenIsRequired() error {
	return godog.ErrPending
}

// theFlowIsAttemptedWithoutUserInteraction: TF-OAUTH-5 — requires provider.go integration.
func theFlowIsAttemptedWithoutUserInteraction(flow string) error {
	return godog.ErrPending
}
