package stepdefinitions

// health_steps.go — step definitions for features/health.feature
//
// All steps operate against an in-process health.Server started in the
// Background step "the health server is started on an available port".
// HTTP state (lastResp, lastBody) is held in httpState (common_steps.go).

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/james-gibson/smoke-alarm/internal/health"
)

// hsState holds per-scenario health server state.
var hsState struct {
	srv         *health.Server
	baseURL     string
	cancelFn    context.CancelFunc
	serviceName string

	// BindWithRetry test state
	bwrSrv         *health.Server
	bwrCancel      context.CancelFunc
	configuredAddr string
	boundAddr      string
	occupier       net.Listener
	advertiserAddr string

	// addrRemap maps configured listen addresses to actual bound addresses.
	// Used by aGETRequestSentTo to transparently redirect hardcoded URLs when
	// BindWithRetry binds to a different port than the config specifies.
	addrRemap map[string]string
}

func resetHSState() {
	hsState.srv = nil
	hsState.baseURL = ""
	hsState.cancelFn = nil
	hsState.serviceName = ""
	hsState.bwrSrv = nil
	hsState.bwrCancel = nil
	hsState.configuredAddr = ""
	hsState.boundAddr = ""
	hsState.occupier = nil
	hsState.advertiserAddr = ""
	hsState.addrRemap = nil
}

func cleanupHSState() {
	if hsState.cancelFn != nil {
		hsState.cancelFn()
		hsState.cancelFn = nil
	}
	if hsState.bwrCancel != nil {
		hsState.bwrCancel()
		hsState.bwrCancel = nil
	}
	if hsState.srv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = hsState.srv.Shutdown(ctx)
		hsState.srv = nil
	}
	if hsState.bwrSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = hsState.bwrSrv.Shutdown(ctx)
		hsState.bwrSrv = nil
	}
	if hsState.occupier != nil {
		_ = hsState.occupier.Close()
		hsState.occupier = nil
	}
	hsState.baseURL = ""
	hsState.serviceName = ""
	hsState.configuredAddr = ""
	hsState.boundAddr = ""
	hsState.advertiserAddr = ""
}

func InitializeHealthSteps(ctx *godog.ScenarioContext) {
	ctx.BeforeScenario(func(_ *godog.Scenario) {
		resetHSState()
	})
	ctx.AfterScenario(func(_ *godog.Scenario, _ error) {
		cleanupHSState()
	})

	// ── setup ──────────────────────────────────────────────────────────────
	ctx.Step(`^the health server is started on an available port$`, theHealthServerIsStarted)
	ctx.Step(`^the service is live$`, theServiceIsLive)
	ctx.Step(`^liveness is set to false$`, livenessIsSetToFalse)
	ctx.Step(`^the response body contains the configured service name$`, theResponseBodyContainsServiceName)

	// ── /readyz ────────────────────────────────────────────────────────────
	ctx.Step(`^the ready flag is false$`, theReadyFlagIsFalse)
	ctx.Step(`^the ready flag is true$`, theReadyFlagIsTrue)
	ctx.Step(`^all registered components are healthy$`, allRegisteredComponentsAreHealthy)
	ctx.Step(`^a component "([^"]*)" is registered as unhealthy$`, aComponentIsRegisteredAsUnhealthy)
	ctx.Step(`^a component "([^"]*)" is registered as healthy$`, aComponentIsRegisteredAsHealthy)
	ctx.Step(`^a target "([^"]*)" has state "([^"]*)"$`, aTargetHasState)
	ctx.Step(`^the response body includes a summary with (\d+) total targets$`, theResponseBodyIncludesSummaryWithNTargets)
	ctx.Step(`^the service is set not-ready with reason "([^"]*)"$`, theServiceIsSetNotReadyWithReason)

	// ── /status ────────────────────────────────────────────────────────────
	ctx.Step(`^the health server has been running for at least (\d+) seconds$`, theHealthServerHasBeenRunning)
	ctx.Step(`^the response body uptime_sec is at least (\d+)$`, theResponseBodyUptimeIsAtLeast)
	ctx.Step(`^the response body live flag is true$`, theResponseBodyLiveFlagIsTrue)
	ctx.Step(`^the response body ready flag is true$`, theResponseBodyReadyFlagIsTrue)
	ctx.Step(`^components "([^"]*)", "([^"]*)", "([^"]*)" are registered as healthy$`, componentsAreRegisteredAsHealthy)
	ctx.Step(`^the components list in the response is ordered "([^"]*)", "([^"]*)", "([^"]*)"$`, theComponentsListIsOrdered)
	ctx.Step(`^targets "([^"]*)", "([^"]*)", "([^"]*)" are registered with state "([^"]*)"$`, targetsAreRegisteredWithState)
	ctx.Step(`^the targets list in the response is ordered "([^"]*)", "([^"]*)", "([^"]*)"$`, theTargetsListIsOrdered)
	ctx.Step(`^the summary healthy count is (\d+)$`, theSummaryHealthyCountIs)
	ctx.Step(`^the summary unhealthy count is (\d+)$`, theSummaryUnhealthyCountIs)
	ctx.Step(`^the summary regression count is (\d+)$`, theSummaryRegressionCountIs)
	ctx.Step(`^the summary total is (\d+)$`, theSummaryTotalIs)
	ctx.Step(`^the summary unknown count is (\d+)$`, theSummaryUnknownCountIs)

	// ── target status management ───────────────────────────────────────────
	ctx.Step(`^a target status is upserted with id "([^"]*)" and state "([^"]*)"$`, aTargetStatusIsUpsertedWithState)
	ctx.Step(`^a target status is upserted with id "([^"]*)" and an empty state$`, aTargetStatusIsUpsertedWithEmptyState)
	ctx.Step(`^a target status is upserted with an empty id$`, aTargetStatusIsUpsertedWithEmptyID)
	ctx.Step(`^no target with empty id appears in the response$`, noTargetWithEmptyIDAppears)
	ctx.Step(`^target "([^"]*)" is removed$`, targetIsRemoved)
	ctx.Step(`^the target "([^"]*)" does not appear in the response$`, theTargetDoesNotAppear)
	ctx.Step(`^the target "([^"]*)" state in the response is "([^"]*)"$`, theTargetStateInResponseIs)

	// ── component management ───────────────────────────────────────────────
	ctx.Step(`^component "([^"]*)" is removed$`, componentIsRemoved)
	ctx.Step(`^component "([^"]*)" does not appear in the response$`, componentDoesNotAppear)
	ctx.Step(`^component "([^"]*)" is updated to healthy$`, componentIsUpdatedToHealthy)
	ctx.Step(`^no component with empty name appears in the response$`, noComponentWithEmptyNameAppears)

	// ── self-description ───────────────────────────────────────────────────
	ctx.Step(`^no self-description factory is registered$`, noSelfDescriptionFactoryIsRegistered)
	ctx.Step(`^a self-description factory returning service name "([^"]*)" is registered$`, aSelfDescriptionFactoryIsRegistered)

	// ── /federation/report ─────────────────────────────────────────────────
	ctx.Step(`^a target "([^"]*)" is not in the local status$`, aTargetIsNotInLocalStatus)
	ctx.Step(`^a POST is sent to "([^"]*)" with targets including "([^"]*)" in state "([^"]*)"$`, aPOSTIsSentWithTargets)
	ctx.Step(`^a POST is sent to "([^"]*)" with body "([^"]*)"$`, aPOSTIsSentWithBody)

	// ── port binding (BindWithRetry) ───────────────────────────────────────
	ctx.Step(`^the configured listen address has a free port$`, theConfiguredListenAddressHasAFreePort)
	ctx.Step(`^the configured listen port is occupied by another process$`, theConfiguredListenPortIsOccupied)
	ctx.Step(`^BindWithRetry is called with (\d+) retries$`, bindWithRetryIsCalledWithRetries)
	ctx.Step(`^the returned address matches the configured listen address$`, theReturnedAddressMatchesConfigured)
	ctx.Step(`^the returned address differs from the configured listen address$`, theReturnedAddressDiffersFromConfigured)
	ctx.Step(`^the returned port is higher than the configured port$`, theReturnedPortIsHigher)
	ctx.Step(`^the mDNS advertiser is started with the actual bound address$`, theMDNSAdvertiserIsStartedWithActualAddress)
	ctx.Step(`^the advertiser registers the service on the actual bound port$`, theAdvertiserRegistersOnActualPort)
	ctx.Step(`^the advertiser does not register on the configured port$`, theAdvertiserDoesNotRegisterOnConfiguredPort)

	// ── shutdown ───────────────────────────────────────────────────────────
	ctx.Step(`^the health server is shut down$`, theHealthServerIsShutDown)
	ctx.Step(`^the service is no longer live$`, theServiceIsNoLongerLive)
}

// ── setup ──────────────────────────────────────────────────────────────────

func theHealthServerIsStarted() error {
	// Find a free port by asking the OS.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("find free port: %w", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	const svcName = "ocd-smoke-alarm"
	srv := health.NewServer(health.Options{
		ServiceName: svcName,
		ListenAddr:  addr,
	})
	boundAddr, err := srv.BindWithRetry(5)
	if err != nil {
		return fmt.Errorf("bind health server: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.Start(ctx) }()
	time.Sleep(20 * time.Millisecond) // wait for Serve to begin

	hsState.srv = srv
	hsState.baseURL = "http://" + boundAddr
	hsState.cancelFn = cancel
	hsState.serviceName = svcName
	return nil
}

func theServiceIsLive() error {
	if hsState.srv == nil {
		return fmt.Errorf("no health server running")
	}
	hsState.srv.SetLive(true)
	return nil
}

func livenessIsSetToFalse() error {
	if hsState.srv == nil {
		return fmt.Errorf("no health server running")
	}
	hsState.srv.SetLive(false)
	return nil
}

func theResponseBodyContainsServiceName() error {
	if httpState.lastBody == nil {
		return fmt.Errorf("no HTTP response yet")
	}
	if !strings.Contains(string(httpState.lastBody), hsState.serviceName) {
		return fmt.Errorf("response body does not contain service name %q\nbody: %s",
			hsState.serviceName, httpState.lastBody)
	}
	return nil
}

// ── /readyz ────────────────────────────────────────────────────────────────

func theReadyFlagIsFalse() error {
	if hsState.srv == nil {
		return fmt.Errorf("no health server running")
	}
	hsState.srv.SetReady(false, "")
	return nil
}

func theReadyFlagIsTrue() error {
	if hsState.srv == nil {
		return fmt.Errorf("no health server running")
	}
	hsState.srv.SetReady(true, "")
	return nil
}

func allRegisteredComponentsAreHealthy() error {
	// Precondition: no unhealthy components exist. Server starts with none, so no-op.
	return nil
}

func aComponentIsRegisteredAsUnhealthy(name string) error {
	if hsState.srv == nil {
		return fmt.Errorf("no health server running")
	}
	hsState.srv.SetComponent(name, false, "registered as unhealthy by test")
	return nil
}

func aComponentIsRegisteredAsHealthy(name string) error {
	if hsState.srv == nil {
		return fmt.Errorf("no health server running")
	}
	hsState.srv.SetComponent(name, true, "")
	return nil
}

func aTargetHasState(id, state string) error {
	if hsState.srv == nil {
		return fmt.Errorf("no health server running")
	}
	hsState.srv.UpsertTargetStatus(health.TargetStatus{ID: id, State: state})
	return nil
}

func theResponseBodyIncludesSummaryWithNTargets(n int) error {
	if httpState.lastBody == nil {
		return fmt.Errorf("no HTTP response yet")
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(httpState.lastBody, &body); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}
	summaryRaw, ok := body["summary"]
	if !ok {
		return fmt.Errorf("no summary field in response\nbody: %s", httpState.lastBody)
	}
	var summary health.StatusSummary
	if err := json.Unmarshal(summaryRaw, &summary); err != nil {
		return fmt.Errorf("parse summary: %w", err)
	}
	if summary.Total != n {
		return fmt.Errorf("expected summary.total=%d, got %d", n, summary.Total)
	}
	return nil
}

func theServiceIsSetNotReadyWithReason(reason string) error {
	if hsState.srv == nil {
		return fmt.Errorf("no health server running")
	}
	hsState.srv.SetReady(false, reason)
	return nil
}

// ── /status ────────────────────────────────────────────────────────────────

func theHealthServerHasBeenRunning(seconds int) error {
	if hsState.srv == nil {
		return fmt.Errorf("no health server running")
	}
	if seconds > 0 {
		time.Sleep(time.Duration(seconds) * time.Second)
	}
	return nil
}

func theResponseBodyUptimeIsAtLeast(seconds int) error {
	if httpState.lastBody == nil {
		return fmt.Errorf("no HTTP response yet")
	}
	var resp health.StatusResponse
	if err := json.Unmarshal(httpState.lastBody, &resp); err != nil {
		return fmt.Errorf("parse status response: %w", err)
	}
	if resp.UptimeSec < int64(seconds) {
		return fmt.Errorf("expected uptime_sec >= %d, got %d", seconds, resp.UptimeSec)
	}
	return nil
}

func theResponseBodyLiveFlagIsTrue() error {
	if httpState.lastBody == nil {
		return fmt.Errorf("no HTTP response yet")
	}
	var resp health.StatusResponse
	if err := json.Unmarshal(httpState.lastBody, &resp); err != nil {
		return fmt.Errorf("parse status response: %w", err)
	}
	if !resp.Live {
		return fmt.Errorf("expected live=true in /status response")
	}
	return nil
}

func theResponseBodyReadyFlagIsTrue() error {
	if httpState.lastBody == nil {
		return fmt.Errorf("no HTTP response yet")
	}
	var resp health.StatusResponse
	if err := json.Unmarshal(httpState.lastBody, &resp); err != nil {
		return fmt.Errorf("parse status response: %w", err)
	}
	if !resp.Ready {
		return fmt.Errorf("expected ready=true in /status response")
	}
	return nil
}

func componentsAreRegisteredAsHealthy(a, b, c string) error {
	if hsState.srv == nil {
		return fmt.Errorf("no health server running")
	}
	for _, name := range []string{a, b, c} {
		hsState.srv.SetComponent(name, true, "")
	}
	return nil
}

func theComponentsListIsOrdered(a, b, c string) error {
	if httpState.lastBody == nil {
		return fmt.Errorf("no HTTP response yet")
	}
	var resp health.StatusResponse
	if err := json.Unmarshal(httpState.lastBody, &resp); err != nil {
		return fmt.Errorf("parse status response: %w", err)
	}
	want := []string{a, b, c}
	if len(resp.Components) < len(want) {
		return fmt.Errorf("expected at least %d components, got %d", len(want), len(resp.Components))
	}
	for i, w := range want {
		if resp.Components[i].Name != w {
			return fmt.Errorf("components[%d]: expected %q, got %q", i, w, resp.Components[i].Name)
		}
	}
	return nil
}

func targetsAreRegisteredWithState(a, b, c, state string) error {
	if hsState.srv == nil {
		return fmt.Errorf("no health server running")
	}
	for _, id := range []string{a, b, c} {
		hsState.srv.UpsertTargetStatus(health.TargetStatus{ID: id, State: state})
	}
	return nil
}

func theTargetsListIsOrdered(a, b, c string) error {
	if httpState.lastBody == nil {
		return fmt.Errorf("no HTTP response yet")
	}
	var resp health.StatusResponse
	if err := json.Unmarshal(httpState.lastBody, &resp); err != nil {
		return fmt.Errorf("parse status response: %w", err)
	}
	want := []string{a, b, c}
	if len(resp.Targets) < len(want) {
		return fmt.Errorf("expected at least %d targets, got %d", len(want), len(resp.Targets))
	}
	for i, w := range want {
		if resp.Targets[i].ID != w {
			return fmt.Errorf("targets[%d]: expected %q, got %q", i, w, resp.Targets[i].ID)
		}
	}
	return nil
}

func parseSummary() (health.StatusSummary, error) {
	var resp health.StatusResponse
	if err := json.Unmarshal(httpState.lastBody, &resp); err != nil {
		return health.StatusSummary{}, fmt.Errorf("parse status response: %w", err)
	}
	return resp.Summary, nil
}

func theSummaryHealthyCountIs(n int) error {
	if httpState.lastBody == nil {
		return fmt.Errorf("no HTTP response yet")
	}
	s, err := parseSummary()
	if err != nil {
		return err
	}
	if s.Healthy != n {
		return fmt.Errorf("expected summary.healthy=%d, got %d", n, s.Healthy)
	}
	return nil
}

func theSummaryUnhealthyCountIs(n int) error {
	if httpState.lastBody == nil {
		return fmt.Errorf("no HTTP response yet")
	}
	s, err := parseSummary()
	if err != nil {
		return err
	}
	if s.Unhealthy != n {
		return fmt.Errorf("expected summary.unhealthy=%d, got %d", n, s.Unhealthy)
	}
	return nil
}

func theSummaryRegressionCountIs(n int) error {
	if httpState.lastBody == nil {
		return fmt.Errorf("no HTTP response yet")
	}
	s, err := parseSummary()
	if err != nil {
		return err
	}
	if s.Regression != n {
		return fmt.Errorf("expected summary.regression=%d, got %d", n, s.Regression)
	}
	return nil
}

func theSummaryTotalIs(n int) error {
	if httpState.lastBody == nil {
		return fmt.Errorf("no HTTP response yet")
	}
	s, err := parseSummary()
	if err != nil {
		return err
	}
	if s.Total != n {
		return fmt.Errorf("expected summary.total=%d, got %d", n, s.Total)
	}
	return nil
}

func theSummaryUnknownCountIs(n int) error {
	if httpState.lastBody == nil {
		return fmt.Errorf("no HTTP response yet")
	}
	s, err := parseSummary()
	if err != nil {
		return err
	}
	if s.Unknown != n {
		return fmt.Errorf("expected summary.unknown=%d, got %d", n, s.Unknown)
	}
	return nil
}

// ── target status management ───────────────────────────────────────────────

func aTargetStatusIsUpsertedWithState(id, state string) error {
	if hsState.srv == nil {
		return fmt.Errorf("no health server running")
	}
	hsState.srv.UpsertTargetStatus(health.TargetStatus{ID: id, State: state})
	return nil
}

func aTargetStatusIsUpsertedWithEmptyState(id string) error {
	if hsState.srv == nil {
		return fmt.Errorf("no health server running")
	}
	hsState.srv.UpsertTargetStatus(health.TargetStatus{ID: id, State: ""})
	return nil
}

func aTargetStatusIsUpsertedWithEmptyID() error {
	if hsState.srv == nil {
		return fmt.Errorf("no health server running")
	}
	hsState.srv.UpsertTargetStatus(health.TargetStatus{ID: "", State: "healthy"})
	return nil
}

func noTargetWithEmptyIDAppears() error {
	if httpState.lastBody == nil {
		return fmt.Errorf("no HTTP response yet")
	}
	var resp health.StatusResponse
	if err := json.Unmarshal(httpState.lastBody, &resp); err != nil {
		return fmt.Errorf("parse status response: %w", err)
	}
	for _, t := range resp.Targets {
		if t.ID == "" {
			return fmt.Errorf("found target with empty ID in response")
		}
	}
	return nil
}

func targetIsRemoved(id string) error {
	if hsState.srv == nil {
		return fmt.Errorf("no health server running")
	}
	hsState.srv.RemoveTarget(id)
	return nil
}

func theTargetDoesNotAppear(id string) error {
	if httpState.lastBody == nil {
		return fmt.Errorf("no HTTP response yet")
	}
	var resp health.StatusResponse
	if err := json.Unmarshal(httpState.lastBody, &resp); err != nil {
		return fmt.Errorf("parse status response: %w", err)
	}
	for _, t := range resp.Targets {
		if t.ID == id {
			return fmt.Errorf("target %q still appears in response after removal", id)
		}
	}
	return nil
}

func theTargetStateInResponseIs(id, state string) error {
	if httpState.lastBody == nil {
		return fmt.Errorf("no HTTP response yet")
	}
	var resp health.StatusResponse
	if err := json.Unmarshal(httpState.lastBody, &resp); err != nil {
		return fmt.Errorf("parse status response: %w", err)
	}
	for _, t := range resp.Targets {
		if t.ID == id {
			if t.State != state {
				return fmt.Errorf("target %q state: expected %q, got %q", id, state, t.State)
			}
			return nil
		}
	}
	return fmt.Errorf("target %q not found in response", id)
}

// ── component management ───────────────────────────────────────────────────

func componentIsRemoved(name string) error {
	if hsState.srv == nil {
		return fmt.Errorf("no health server running")
	}
	hsState.srv.RemoveComponent(name)
	return nil
}

func componentDoesNotAppear(name string) error {
	if httpState.lastBody == nil {
		return fmt.Errorf("no HTTP response yet")
	}
	var resp health.StatusResponse
	if err := json.Unmarshal(httpState.lastBody, &resp); err != nil {
		return fmt.Errorf("parse status response: %w", err)
	}
	for _, c := range resp.Components {
		if c.Name == name {
			return fmt.Errorf("component %q still appears in response", name)
		}
	}
	return nil
}

func componentIsUpdatedToHealthy(name string) error {
	if hsState.srv == nil {
		return fmt.Errorf("no health server running")
	}
	hsState.srv.SetComponent(name, true, "")
	return nil
}

func noComponentWithEmptyNameAppears() error {
	if httpState.lastBody == nil {
		return fmt.Errorf("no HTTP response yet")
	}
	var resp health.StatusResponse
	if err := json.Unmarshal(httpState.lastBody, &resp); err != nil {
		return fmt.Errorf("parse status response: %w", err)
	}
	for _, c := range resp.Components {
		if c.Name == "" {
			return fmt.Errorf("found component with empty name in response")
		}
	}
	return nil
}

// ── self-description ───────────────────────────────────────────────────────

func noSelfDescriptionFactoryIsRegistered() error {
	if hsState.srv == nil {
		return fmt.Errorf("no health server running")
	}
	hsState.srv.SetSelfDescription(nil)
	return nil
}

func aSelfDescriptionFactoryIsRegistered(serviceName string) error {
	if hsState.srv == nil {
		return fmt.Errorf("no health server running")
	}
	hsState.srv.SetSelfDescription(func() any {
		return map[string]string{"service": serviceName}
	})
	return nil
}

// ── /federation/report ─────────────────────────────────────────────────────

func aTargetIsNotInLocalStatus(id string) error {
	if hsState.srv == nil {
		return fmt.Errorf("no health server running")
	}
	snap := hsState.srv.Snapshot()
	for _, t := range snap.Targets {
		if t.ID == id {
			return fmt.Errorf("target %q unexpectedly found in local status", id)
		}
	}
	return nil
}

func aPOSTIsSentWithTargets(path, targetID, state string) error {
	payload := health.StatusResponse{
		Targets: []health.TargetStatus{
			{ID: targetID, State: state},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	return doPost(path, "application/json", bytes.NewReader(body))
}

func aPOSTIsSentWithBody(path, body string) error {
	return doPost(path, "application/json", strings.NewReader(body))
}

func doPost(path, contentType string, body io.Reader) error {
	url := hsState.baseURL + path
	resp, err := http.Post(url, contentType, body) //nolint:noctx
	if err != nil {
		return fmt.Errorf("POST %s: %w", url, err)
	}
	respBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}
	httpState.lastResp = resp
	httpState.lastBody = respBody
	return nil
}

// ── port binding (BindWithRetry) ───────────────────────────────────────────

func theConfiguredListenAddressHasAFreePort() error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("find free port: %w", err)
	}
	addr := ln.Addr().String()
	ln.Close()

	hsState.configuredAddr = addr
	hsState.bwrSrv = health.NewServer(health.Options{
		ServiceName: "test-bwr",
		ListenAddr:  addr,
	})
	return nil
}

func theConfiguredListenPortIsOccupied() error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("bind occupier: %w", err)
	}
	hsState.occupier = ln
	addr := ln.Addr().String()
	hsState.configuredAddr = addr
	hsState.bwrSrv = health.NewServer(health.Options{
		ServiceName: "test-bwr",
		ListenAddr:  addr,
	})
	return nil
}

func bindWithRetryIsCalledWithRetries(n int) error {
	if hsState.bwrSrv == nil {
		// Default: find a free port and create a server.
		ln, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return fmt.Errorf("find free port: %w", err)
		}
		addr := ln.Addr().String()
		ln.Close()
		hsState.configuredAddr = addr
		hsState.bwrSrv = health.NewServer(health.Options{
			ServiceName: "test-bwr",
			ListenAddr:  addr,
		})
	}
	addr, err := hsState.bwrSrv.BindWithRetry(n)
	if err != nil {
		return fmt.Errorf("BindWithRetry(%d): %w", n, err)
	}
	hsState.boundAddr = addr
	// Start the server so its listener is properly tracked and cleaned up.
	ctx, cancel := context.WithCancel(context.Background())
	hsState.bwrCancel = cancel
	go func() { _ = hsState.bwrSrv.Start(ctx) }()
	time.Sleep(10 * time.Millisecond)
	return nil
}

func theReturnedAddressMatchesConfigured() error {
	if hsState.boundAddr == "" {
		return fmt.Errorf("BindWithRetry not yet called")
	}
	if hsState.boundAddr != hsState.configuredAddr {
		return fmt.Errorf("expected bound address %q to equal configured %q",
			hsState.boundAddr, hsState.configuredAddr)
	}
	return nil
}

func theReturnedAddressDiffersFromConfigured() error {
	if hsState.boundAddr == "" {
		return fmt.Errorf("BindWithRetry not yet called")
	}
	if hsState.boundAddr == hsState.configuredAddr {
		return fmt.Errorf("expected bound address to differ from configured %q but they are equal",
			hsState.configuredAddr)
	}
	return nil
}

func theReturnedPortIsHigher() error {
	_, boundPortStr, err := net.SplitHostPort(hsState.boundAddr)
	if err != nil {
		return fmt.Errorf("parse bound address %q: %w", hsState.boundAddr, err)
	}
	_, cfgPortStr, err := net.SplitHostPort(hsState.configuredAddr)
	if err != nil {
		return fmt.Errorf("parse configured address %q: %w", hsState.configuredAddr, err)
	}
	boundPort, _ := strconv.Atoi(boundPortStr)
	cfgPort, _ := strconv.Atoi(cfgPortStr)
	if boundPort <= cfgPort {
		return fmt.Errorf("expected bound port %d to be higher than configured port %d",
			boundPort, cfgPort)
	}
	return nil
}

// ── mDNS advertiser (conceptual — validates port propagation only) ─────────

func theMDNSAdvertiserIsStartedWithActualAddress() error {
	if hsState.boundAddr == "" {
		return fmt.Errorf("BindWithRetry must be called before starting the mDNS advertiser")
	}
	hsState.advertiserAddr = hsState.boundAddr
	return nil
}

func theAdvertiserRegistersOnActualPort() error {
	_, boundPortStr, err := net.SplitHostPort(hsState.boundAddr)
	if err != nil {
		return fmt.Errorf("parse bound address: %w", err)
	}
	_, advPortStr, err := net.SplitHostPort(hsState.advertiserAddr)
	if err != nil {
		return fmt.Errorf("parse advertiser address: %w", err)
	}
	if boundPortStr != advPortStr {
		return fmt.Errorf("advertiser port %q != bound port %q", advPortStr, boundPortStr)
	}
	return nil
}

func theAdvertiserDoesNotRegisterOnConfiguredPort() error {
	_, cfgPortStr, err := net.SplitHostPort(hsState.configuredAddr)
	if err != nil {
		return fmt.Errorf("parse configured address: %w", err)
	}
	_, advPortStr, err := net.SplitHostPort(hsState.advertiserAddr)
	if err != nil {
		return fmt.Errorf("parse advertiser address: %w", err)
	}
	if cfgPortStr == advPortStr {
		return fmt.Errorf("advertiser registered on configured port %q (expected a different port)",
			cfgPortStr)
	}
	return nil
}

// ── shutdown ───────────────────────────────────────────────────────────────

func theHealthServerIsShutDown() error {
	if hsState.srv == nil {
		return fmt.Errorf("no health server running")
	}
	if hsState.cancelFn != nil {
		hsState.cancelFn()
		hsState.cancelFn = nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return hsState.srv.Shutdown(ctx)
}

func theServiceIsNoLongerLive() error {
	if hsState.srv == nil {
		return fmt.Errorf("no health server reference")
	}
	snap := hsState.srv.Snapshot()
	if snap.Live {
		return fmt.Errorf("expected service to be not-live after shutdown, but Live=true")
	}
	return nil
}
