package stepdefinitions

// listener_steps.go — step definitions for the self-negotiated listener
// scenarios in features/health.feature and features/selfdescription.feature.
//
// These exercise health.Server.ClaimListener, the same call serve makes, so the
// Gherkin record covers the port a surface actually bound and what /status says
// about it when the configured port was held (smoke-alarm#53).

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"github.com/james-gibson/smoke-alarm/internal/auth"
	"github.com/james-gibson/smoke-alarm/internal/config"
	"github.com/james-gibson/smoke-alarm/internal/health"
	"github.com/james-gibson/smoke-alarm/internal/hosted"
)

// listenerState holds per-scenario listener state. It is deliberately separate
// from hsState: these scenarios own their health server, since /status is the
// assertion.
var listenerState struct {
	held         net.Listener
	heldAddr     string
	configured   string
	boundAddr    string
	bindErr      error
	hs           *health.Server
	healthURL    string
	cancel       context.CancelFunc
	hostedSrv    *hosted.Server
	oauthSrv     *auth.MockRedirectServer
	cfg          config.Config
	targets      config.Config
	targetBefore string
	targetAfter  string
}

func resetListenerState() {
	if listenerState.cancel != nil {
		listenerState.cancel()
		listenerState.cancel = nil
	}
	if listenerState.held != nil {
		_ = listenerState.held.Close()
	}
	if listenerState.hostedSrv != nil {
		_ = listenerState.hostedSrv.Shutdown(context.Background())
	}
	if listenerState.oauthSrv != nil {
		_ = listenerState.oauthSrv.Shutdown(context.Background())
	}
	listenerState = struct {
		held         net.Listener
		heldAddr     string
		configured   string
		boundAddr    string
		bindErr      error
		hs           *health.Server
		healthURL    string
		cancel       context.CancelFunc
		hostedSrv    *hosted.Server
		oauthSrv     *auth.MockRedirectServer
		cfg          config.Config
		targets      config.Config
		targetBefore string
		targetAfter  string
	}{}
}

// holdPort binds a loopback port and keeps it, standing in for the unrelated
// process that holds a fixed listener's port.
func holdPort() (string, error) {
	var lc net.ListenConfig
	ln, err := lc.Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("hold a port: %w", err)
	}
	listenerState.held = ln
	return ln.Addr().String(), nil
}

// startHealthForListeners starts the health server whose /status the assertions
// read, and keeps the config the self-description is built from.
func startHealthForListeners() error {
	hs := health.NewServer(health.Options{ServiceName: "listener-test"})
	bound, err := hs.BindWithRetry(2)
	if err != nil {
		return fmt.Errorf("bind health server: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	listenerState.cancel = cancel
	go func() { _ = hs.Start(ctx) }()

	listenerState.hs = hs
	listenerState.healthURL = "http://" + bound
	listenerState.cfg = config.Config{
		Service: config.ServiceConfig{Name: "listener-test", Mode: config.ModeHeadless},
		Health: config.HealthConfig{
			Enabled:    true,
			ListenAddr: bound,
			Endpoints: config.HealthRoutes{
				Healthz: "/healthz",
				Readyz:  "/readyz",
				Status:  "/status",
			},
		},
		Hosted: config.HostedConfig{
			Enabled:    true,
			ListenAddr: listenerState.heldAddr,
			Transports: []string{"http"},
			Protocols:  []string{"mcp"},
			Endpoints:  config.HostedEndpointConfig{MCP: "/mcp", ACP: "/acp"},
		},
		Auth: config.GlobalAuthConfig{
			OAuth: config.GlobalOAuthConfig{
				MockRedirect: config.OAuthMockRedirectConfig{
					Enabled:    true,
					ListenAddr: listenerState.heldAddr,
					Path:       "/oauth/callback",
					Mode:       "allow",
				},
			},
		},
	}
	listenerState.targets = listenerState.cfg
	return nil
}

func InitializeListenerSteps(ctx *godog.ScenarioContext) {
	ctx.BeforeScenario(func(_ *godog.Scenario) { resetListenerState() })
	ctx.AfterScenario(func(_ *godog.Scenario, _ error) { resetListenerState() })

	// ── the port is held by someone else ───────────────────────────────────
	ctx.Step(`^another process holds the port the hosted server would use$`, anotherProcessHoldsHostedPort)
	ctx.Step(`^another process holds the port the OAuth mock redirect would use$`, anotherProcessHoldsOAuthPort)

	// ── claiming ───────────────────────────────────────────────────────────
	ctx.Step(`^the hosted listener is claimed with retry$`, hostedListenerIsClaimedWithRetry)
	ctx.Step(`^the OAuth mock redirect listener is claimed with retry$`, oauthListenerIsClaimedWithRetry)
	ctx.Step(`^the OAuth mock redirect listener is claimed with no retries$`, oauthListenerIsClaimedWithoutRetries)

	// ── the port actually bound ────────────────────────────────────────────
	ctx.Step(`^the hosted listener bound a port other than the held one$`, hostedListenerBoundAnotherPort)
	ctx.Step(`^the OAuth mock redirect bound a port other than the held one$`, oauthListenerBoundAnotherPort)

	// ── what /status says ─────────────────────────────────────────────────
	ctx.Step(`^/status reports the "([^"]*)" listener on the address it bound$`, statusReportsListenerOnBoundAddress)
	ctx.Step(`^/status reports the "([^"]*)" component unhealthy$`, statusReportsComponentUnhealthy)
	ctx.Step(`^the component detail names the held port$`, componentDetailNamesHeldPort)

	// ── self-references follow the move ───────────────────────────────────
	ctx.Step(`^a target named "([^"]*)" points at the held address$`, aTargetPointsAtHeldAddress)
	ctx.Step(`^the target endpoint names the bound hosted port$`, theTargetEndpointNamesBoundPort)

	// ── self-description reports the bound addresses ──────────────────────
	ctx.Step(`^the self-description hosted listen address is the bound hosted address$`, selfDescriptionReportsBoundHosted)
	ctx.Step(`^the self-description oauth mock listen address is the bound OAuth address$`, selfDescriptionReportsBoundOAuth)
}

// ── arrange ─────────────────────────────────────────────────────────────────

func anotherProcessHoldsHostedPort() error {
	addr, err := holdPort()
	if err != nil {
		return err
	}
	listenerState.heldAddr = addr
	listenerState.configured = addr
	if err := startHealthForListeners(); err != nil {
		return err
	}
	listenerState.hostedSrv = hosted.NewServer(hosted.Options{
		ServiceName: "listener-test",
		ListenAddr:  addr,
		EnableHTTP:  true,
		EnableMCP:   true,
	})
	return nil
}

func anotherProcessHoldsOAuthPort() error {
	addr, err := holdPort()
	if err != nil {
		return err
	}
	listenerState.heldAddr = addr
	listenerState.configured = addr
	if err := startHealthForListeners(); err != nil {
		return err
	}
	listenerState.oauthSrv = auth.NewMockRedirectServer(auth.MockRedirectOptions{
		ListenAddr: addr,
		Path:       "/oauth/callback",
		Mode:       auth.MockRedirectAllow,
	})
	return nil
}

func aTargetPointsAtHeldAddress(name string) error {
	endpoint := fmt.Sprintf("http://0.0.0.0:%s/mcp", portOfAddr(listenerState.heldAddr))
	listenerState.targets.Targets = []config.TargetConfig{{
		ID:       name,
		Enabled:  true,
		Protocol: config.ProtocolMCP,
		Endpoint: endpoint,
	}}
	listenerState.targetBefore = endpoint
	return nil
}

// ── act ─────────────────────────────────────────────────────────────────────

func hostedListenerIsClaimedWithRetry() error {
	addr, err := listenerState.hs.ClaimListener(health.ListenerHosted, listenerState.configured, listenerState.hostedSrv, nil, 5)
	if err != nil {
		return fmt.Errorf("claim hosted listener: %w", err)
	}
	listenerState.boundAddr = addr
	// The process's own targets follow the port that actually bound.
	listenerState.targets = config.RewriteSelfReferences(listenerState.targets, map[string]string{
		listenerState.configured: addr,
	})
	if len(listenerState.targets.Targets) > 0 {
		listenerState.targetAfter = listenerState.targets.Targets[0].Endpoint
	}
	return nil
}

func oauthListenerIsClaimedWithRetry() error {
	addr, err := listenerState.hs.ClaimListener(health.ListenerOAuthRedirect, listenerState.configured, listenerState.oauthSrv, nil, 5)
	if err != nil {
		return fmt.Errorf("claim OAuth mock listener: %w", err)
	}
	listenerState.boundAddr = addr
	return nil
}

func oauthListenerIsClaimedWithoutRetries() error {
	addr, err := listenerState.hs.ClaimListener(health.ListenerOAuthRedirect, listenerState.configured, listenerState.oauthSrv, nil, 0)
	listenerState.boundAddr = addr
	listenerState.bindErr = err
	if err == nil {
		return fmt.Errorf("expected the claim to fail while %s is held, bound %q", listenerState.configured, addr)
	}
	return nil
}

// ── assert ──────────────────────────────────────────────────────────────────

func hostedListenerBoundAnotherPort() error {
	if listenerState.boundAddr == "" {
		return fmt.Errorf("hosted listener was not claimed")
	}
	if listenerState.boundAddr == listenerState.heldAddr {
		return fmt.Errorf("hosted listener bound the held address %s", listenerState.heldAddr)
	}
	return nil
}

func oauthListenerBoundAnotherPort() error {
	if listenerState.boundAddr == "" {
		return fmt.Errorf("OAuth mock redirect was not claimed")
	}
	if listenerState.boundAddr == listenerState.heldAddr {
		return fmt.Errorf("OAuth mock redirect bound the held address %s", listenerState.heldAddr)
	}
	return nil
}

// fetchStatus reads /status from the health server under test.
func fetchStatus() (health.StatusResponse, error) {
	var status health.StatusResponse
	resp, err := http.Get(listenerState.healthURL + "/status") //nolint:noctx
	if err != nil {
		return status, fmt.Errorf("GET /status: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return status, fmt.Errorf("decode /status: %w", err)
	}
	return status, nil
}

func statusReportsListenerOnBoundAddress(name string) error {
	status, err := fetchStatus()
	if err != nil {
		return err
	}
	got := status.Listeners[name]
	if got == "" {
		return fmt.Errorf("/status does not report listener %q (listeners: %v)", name, status.Listeners)
	}
	if got != listenerState.boundAddr {
		return fmt.Errorf("/status reports %q on %q, want the bound address %q", name, got, listenerState.boundAddr)
	}
	return nil
}

func statusReportsComponentUnhealthy(name string) error {
	status, err := fetchStatus()
	if err != nil {
		return err
	}
	for _, c := range status.Components {
		if c.Name != name {
			continue
		}
		if c.Healthy {
			return fmt.Errorf("component %q is healthy after a failed bind", name)
		}
		return nil
	}
	return fmt.Errorf("/status has no component %q (components: %v)", name, status.Components)
}

func componentDetailNamesHeldPort() error {
	status, err := fetchStatus()
	if err != nil {
		return err
	}
	port := portOfAddr(listenerState.heldAddr)
	for _, c := range status.Components {
		if c.Healthy {
			continue
		}
		if strings.Contains(c.Detail, listenerState.heldAddr) || strings.Contains(c.Detail, ":"+port) {
			return nil
		}
	}
	return fmt.Errorf("no unhealthy component detail names the held port %s in %v", port, status.Components)
}

func theTargetEndpointNamesBoundPort() error {
	if listenerState.targetAfter == "" {
		return fmt.Errorf("no rewritten target to inspect")
	}
	u, err := url.Parse(listenerState.targetAfter)
	if err != nil {
		return fmt.Errorf("parse rewritten endpoint %q: %w", listenerState.targetAfter, err)
	}
	_, boundPort, _ := net.SplitHostPort(listenerState.boundAddr)
	if u.Port() != boundPort {
		return fmt.Errorf("target endpoint %q does not follow the bound port %s", listenerState.targetAfter, boundPort)
	}
	return nil
}

func selfDescriptionReportsBoundHosted() error {
	doc, err := listenerSelfDescription()
	if err != nil {
		return err
	}
	if got := doc.Capabilities.Hosted.ListenAddr; got != listenerState.boundAddr {
		return fmt.Errorf("self-description hosted listen_addr = %q, want the bound address %q", got, listenerState.boundAddr)
	}
	return nil
}

func selfDescriptionReportsBoundOAuth() error {
	doc, err := listenerSelfDescription()
	if err != nil {
		return err
	}
	if got := doc.Capabilities.OAuth.MockRedirect.ListenAddr; got != listenerState.boundAddr {
		return fmt.Errorf("self-description oauth mock listen_addr = %q, want the bound address %q", got, listenerState.boundAddr)
	}
	return nil
}

func listenerSelfDescription() (health.SelfDescription, error) {
	factory := health.NewSelfDescriptionFactory(listenerState.cfg, "test", time.Now().UTC(), listenerState.hs)
	raw, ok := factory().(health.SelfDescription)
	if !ok {
		return health.SelfDescription{}, fmt.Errorf("self-description factory returned an unexpected type")
	}
	return raw, nil
}

func portOfAddr(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return ""
	}
	return port
}
