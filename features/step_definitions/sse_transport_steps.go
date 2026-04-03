package stepdefinitions

// sse_transport_steps.go — step definitions for features/sse-transport.feature
//
// Classification: stub — all steps return godog.ErrPending
//
// THESIS-FINDING: sse_transport_steps.go is stubbed (godog.ErrPending) —
// SSE transport probe has nominal, not executable Cucumber coverage.
//
// Registered steps (must not be re-registered in other domain files):
//   "the SSE endpoint {string} is reachable"
//   "the endpoint streams valid MCP events"
//   "the endpoint streams valid ACP events"
//   "the probe establishes an SSE connection"
//   "the SSE endpoint {string} is unreachable"
//   "the SSE endpoint {string} closes the stream prematurely"
//   "the target {string} has auth type {string}"
//   "the secret ref {string} resolves to a valid token"
//   "the SSE connection request contains an {string} header"
//   "the token value is not logged in plaintext"
//   "the target {string} has handshake_profile {string}"
//   "no required_methods check is performed"
//   "classification is based solely on HTTP status code"
//   "the endpoint responds to all methods in {string}"
//   "the target {string} has a hurl_test named {string}"
//   "the preflight endpoint {string} returns status code {int}"
//   "the HURL preflight {string} is executed before the SSE connection"
//   "the SSE connection is only attempted after preflight passes"
//   "the SSE connection is not attempted"
//   "the config {string} has {string} set to {int}"
//   "all four targets are enabled"
//   "a probe cycle runs"
//   "all four probes are dispatched concurrently"
//   "no more than {int} worker goroutines are active at once"
//   "{string} is true in config {string}"
//   "a managed update is triggered"
//   "the {string} is executed first"
//   "the {string} is executed after stop succeeds"
//   "the {string} is executed after start"
//   "on verify failure the previous version is restored"
//   "the verify_command exits with a non-zero code"
//   "the previous binary version is restored"
//   "a REGRESSION event is emitted"

import (
	"github.com/cucumber/godog"
)

func InitializeSSETransportSteps(ctx *godog.ScenarioContext) {
	// ── SSE connection ─────────────────────────────────────────────────────
	ctx.Step(`^the SSE endpoint "([^"]*)" is reachable$`, theSSEEndpointIsReachable)
	ctx.Step(`^the endpoint streams valid MCP events$`, theEndpointStreamsValidMCPEvents)
	ctx.Step(`^the endpoint streams valid ACP events$`, theEndpointStreamsValidACPEvents)
	ctx.Step(`^the probe establishes an SSE connection$`, theProbeEstablishesSSEConnection)
	ctx.Step(`^the SSE endpoint "([^"]*)" is unreachable$`, theSSEEndpointIsUnreachable)
	ctx.Step(`^the SSE endpoint "([^"]*)" closes the stream prematurely$`, theSSEEndpointClosesStreamPrematurely)

	// ── bearer auth ────────────────────────────────────────────────────────
	ctx.Step(`^the target "([^"]*)" has auth type "([^"]*)"$`, theTargetHasAuthType)
	ctx.Step(`^the secret ref "([^"]*)" resolves to a valid token$`, theSecretRefResolvesToValidToken)
	ctx.Step(`^the SSE connection request contains an "([^"]*)" header$`, theSSEConnectionRequestContainsHeader)
	ctx.Step(`^the token value is not logged in plaintext$`, theTokenValueIsNotLoggedInPlaintext)

	// ── handshake profile ──────────────────────────────────────────────────
	ctx.Step(`^the target "([^"]*)" has handshake_profile "([^"]*)"$`, theTargetHasHandshakeProfile)
	ctx.Step(`^no required_methods check is performed$`, noRequiredMethodsCheckIsPerformed)
	ctx.Step(`^classification is based solely on HTTP status code$`, classificationIsBasedSolelyOnHTTPStatus)
	ctx.Step(`^the endpoint responds to all methods in "([^"]*)"$`, theEndpointRespondsToAllMethodsIn)

	// ── HURL preflight ─────────────────────────────────────────────────────
	ctx.Step(`^the target "([^"]*)" has a hurl_test named "([^"]*)"$`, theTargetHasHurlTestNamed)
	ctx.Step(`^the preflight endpoint "([^"]*)" returns status code (\d+)$`, thePreflightEndpointReturnsStatusCode)
	ctx.Step(`^the HURL preflight "([^"]*)" is executed before the SSE connection$`, theHURLPreflightIsExecutedBeforeSSE)
	ctx.Step(`^the SSE connection is only attempted after preflight passes$`, theSSEConnectionOnlyAfterPreflightPasses)
	ctx.Step(`^the SSE connection is not attempted$`, theSSEConnectionIsNotAttempted)

	// ── mixed target set ───────────────────────────────────────────────────
	ctx.Step(`^the config "([^"]*)" has "([^"]*)" set to (\d+)$`, theConfigHasSettingSetToInt)
	ctx.Step(`^all four targets are enabled$`, allFourTargetsAreEnabled)
	ctx.Step(`^a probe cycle runs$`, aProbeCycleRuns)
	ctx.Step(`^all four probes are dispatched concurrently$`, allFourProbesAreDispatchedConcurrently)
	ctx.Step(`^no more than (\d+) worker goroutines are active at once$`, noMoreThanNWorkersActive)

	// ── remote_agent lifecycle ─────────────────────────────────────────────
	ctx.Step(`^"([^"]*)" is true in config "([^"]*)"$`, configFlagIsTrueInConfig)
	ctx.Step(`^a managed update is triggered$`, aManagedUpdateIsTriggered)
	ctx.Step(`^the "([^"]*)" is executed first$`, theCommandIsExecutedFirst)
	ctx.Step(`^the "([^"]*)" is executed after stop succeeds$`, theCommandIsExecutedAfterStop)
	ctx.Step(`^the "([^"]*)" is executed after start$`, theCommandIsExecutedAfterStart)
	ctx.Step(`^on verify failure the previous version is restored$`, onVerifyFailurePreviousVersionRestored)
	ctx.Step(`^the verify_command exits with a non-zero code$`, theVerifyCommandExitsNonZero)
	ctx.Step(`^the previous binary version is restored$`, thePreviousBinaryVersionIsRestored)
	ctx.Step(`^a REGRESSION event is emitted$`, aREGRESSIONEventIsEmitted)
}

// ── stub implementations ───────────────────────────────────────────────────

func theSSEEndpointIsReachable(endpoint string) error                       { return godog.ErrPending }
func theEndpointStreamsValidMCPEvents() error                               { return godog.ErrPending }
func theEndpointStreamsValidACPEvents() error                               { return godog.ErrPending }
func theProbeEstablishesSSEConnection() error                               { return godog.ErrPending }
func theSSEEndpointIsUnreachable(endpoint string) error                     { return godog.ErrPending }
func theSSEEndpointClosesStreamPrematurely(endpoint string) error           { return godog.ErrPending }
func theTargetHasAuthType(id, authType string) error                        { return godog.ErrPending }
func theSecretRefResolvesToValidToken(ref string) error                     { return godog.ErrPending }
func theSSEConnectionRequestContainsHeader(header string) error             { return godog.ErrPending }
func theTokenValueIsNotLoggedInPlaintext() error                            { return godog.ErrPending }
func theTargetHasHandshakeProfile(id, profile string) error                 { return godog.ErrPending }
func noRequiredMethodsCheckIsPerformed() error                              { return godog.ErrPending }
func classificationIsBasedSolelyOnHTTPStatus() error                        { return godog.ErrPending }
func theEndpointRespondsToAllMethodsIn(field string) error                  { return godog.ErrPending }
func theTargetHasHurlTestNamed(id, name string) error                       { return godog.ErrPending }
func thePreflightEndpointReturnsStatusCode(endpoint string, code int) error { return godog.ErrPending }
func theHURLPreflightIsExecutedBeforeSSE(name string) error                 { return godog.ErrPending }
func theSSEConnectionOnlyAfterPreflightPasses() error                       { return godog.ErrPending }
func theSSEConnectionIsNotAttempted() error                                 { return godog.ErrPending }
func theConfigHasSettingSetToInt(config, key string, val int) error         { return godog.ErrPending }
func allFourTargetsAreEnabled() error                                       { return godog.ErrPending }
func aProbeCycleRuns() error                                                { return godog.ErrPending }
func allFourProbesAreDispatchedConcurrently() error                         { return godog.ErrPending }
func noMoreThanNWorkersActive(n int) error                                  { return godog.ErrPending }
func configFlagIsTrueInConfig(flag, config string) error                    { return godog.ErrPending }
func aManagedUpdateIsTriggered() error                                      { return godog.ErrPending }
func theCommandIsExecutedFirst(cmd string) error                            { return godog.ErrPending }
func theCommandIsExecutedAfterStop(cmd string) error                        { return godog.ErrPending }
func theCommandIsExecutedAfterStart(cmd string) error                       { return godog.ErrPending }
func onVerifyFailurePreviousVersionRestored() error                         { return godog.ErrPending }
func theVerifyCommandExitsNonZero() error                                   { return godog.ErrPending }
func thePreviousBinaryVersionIsRestored() error                             { return godog.ErrPending }
func aREGRESSIONEventIsEmitted() error                                      { return godog.ErrPending }
