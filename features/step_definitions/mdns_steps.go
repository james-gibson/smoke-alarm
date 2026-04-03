package stepdefinitions

// mdns_steps.go — step definitions for features/mdns.feature
//
// Classification: stub — all steps return godog.ErrPending
//
// THESIS-FINDING: mdns_steps.go is stubbed (godog.ErrPending) —
// mDNS advertiser has nominal, not executable Cucumber coverage.
//
// Registered steps (must not be re-registered in other domain files):
//   "an Advertiser created with service type {string} and no domain"
//   "an Advertiser with service type {string}, domain {string}, and port {int}"
//   "an Advertiser with service type {string} and port {int}"
//   "an Advertiser with TXT record {string} set to {string}"
//   "an Advertiser that has been started"
//   "an Advertiser that has not been started"
//   "an Advertiser that has been started with a cancellable context"
//   "the advertiser domain is {string}"
//   "ServiceID returns {string}"
//   "ParsePort is called with {string}"
//   "the returned port is {int}"
//   "Start is called with a live context"
//   "the service is registered on port {int}"
//   "the registration uses service type {string}"
//   "the registration includes TXT record {string}"
//   "a zeroconf registration error is injected"
//   "Start returns a non-nil error"
//   "Shutdown is called"
//   "the zeroconf server is shut down"
//   "subsequent Start calls return a fresh registration"
//   "no panic occurs"
//   "the context is cancelled"
//   "the config has tuner.advertise set to false"
//   "ocd-smoke-alarm starts"
//   "the mDNS advertiser is not started"
//
// Steps delegated to other domain files (do not re-register here):
//   "the ocd-smoke-alarm binary is installed"          → common_steps.go
//   "a valid config file {string} exists"              → common_steps.go
//   "the configured listen port is occupied by another process" → health_steps.go
//   "BindWithRetry is called with {int} retries"       → health_steps.go
//   "the mDNS advertiser is started with the actual bound address" → health_steps.go
//   "the advertiser registers the service on the actual bound port" → health_steps.go
//   "the advertiser does not register on the configured port" → health_steps.go

import (
	"github.com/cucumber/godog"
)

func InitializeMDNSSteps(ctx *godog.ScenarioContext) {
	// ── defaults ─────────────────────────────────────────────────────────────
	ctx.Step(`^an Advertiser created with service type "([^"]*)" and no domain$`, anAdvertiserWithNoDomain)
	ctx.Step(`^an Advertiser with service type "([^"]*)", domain "([^"]*)", and port (\d+)$`, anAdvertiserWithServiceTypeDomainAndPort)
	ctx.Step(`^an Advertiser with service type "([^"]*)" and port (\d+)$`, anAdvertiserWithServiceTypeAndPort)
	ctx.Step(`^the advertiser domain is "([^"]*)"$`, theAdvertiserDomainIs)
	ctx.Step(`^ServiceID returns "([^"]*)"$`, serviceIDReturns)

	// ── ParsePort ────────────────────────────────────────────────────────────
	ctx.Step(`^ParsePort is called with "([^"]*)"$`, parsePortIsCalledWith)
	ctx.Step(`^the returned port is (\d+)$`, theReturnedPortIs)

	// ── Start ────────────────────────────────────────────────────────────────
	ctx.Step(`^an Advertiser with TXT record "([^"]*)" set to "([^"]*)"$`, anAdvertiserWithTXTRecord)
	ctx.Step(`^Start is called with a live context$`, startIsCalledWithLiveContext)
	ctx.Step(`^the service is registered on port (\d+)$`, theServiceIsRegisteredOnPort)
	ctx.Step(`^the registration uses service type "([^"]*)"$`, theRegistrationUsesServiceType)
	ctx.Step(`^the registration includes TXT record "([^"]*)"$`, theRegistrationIncludesTXTRecord)
	ctx.Step(`^a zeroconf registration error is injected$`, aZeroconfRegistrationErrorIsInjected)
	ctx.Step(`^Start returns a non-nil error$`, startReturnsANonNilError)

	// ── Shutdown ──────────────────────────────────────────────────────────────
	ctx.Step(`^an Advertiser that has been started$`, anAdvertiserThatHasBeenStarted)
	ctx.Step(`^an Advertiser that has not been started$`, anAdvertiserThatHasNotBeenStarted)
	ctx.Step(`^Shutdown is called$`, shutdownIsCalled)
	ctx.Step(`^the zeroconf server is shut down$`, theZeroconfServerIsShutDown)
	ctx.Step(`^subsequent Start calls return a fresh registration$`, subsequentStartCallsReturnFreshRegistration)
	ctx.Step(`^no panic occurs$`, noPanicOccurs)

	// ── context cancellation ──────────────────────────────────────────────────
	ctx.Step(`^an Advertiser that has been started with a cancellable context$`, anAdvertiserStartedWithCancellableContext)
	ctx.Step(`^the context is cancelled$`, theContextIsCancelled)

	// ── config integration ────────────────────────────────────────────────────
	ctx.Step(`^the config has tuner\.advertise set to false$`, theConfigHasTunerAdvertiseFalse)
	ctx.Step(`^ocd-smoke-alarm starts$`, ocdSmokeAlarmStarts)
	ctx.Step(`^the mDNS advertiser is not started$`, theMDNSAdvertiserIsNotStarted)
}

// ── stub implementations ──────────────────────────────────────────────────────

func anAdvertiserWithNoDomain(serviceType string) error { return godog.ErrPending }
func anAdvertiserWithServiceTypeDomainAndPort(st, domain string, port int) error {
	return godog.ErrPending
}
func anAdvertiserWithServiceTypeAndPort(st string, port int) error { return godog.ErrPending }
func theAdvertiserDomainIs(domain string) error                    { return godog.ErrPending }
func serviceIDReturns(expected string) error                       { return godog.ErrPending }
func parsePortIsCalledWith(addr string) error                      { return godog.ErrPending }
func theReturnedPortIs(port int) error                             { return godog.ErrPending }
func anAdvertiserWithTXTRecord(key, value string) error            { return godog.ErrPending }
func startIsCalledWithLiveContext() error                          { return godog.ErrPending }
func theServiceIsRegisteredOnPort(port int) error                  { return godog.ErrPending }
func theRegistrationUsesServiceType(st string) error               { return godog.ErrPending }
func theRegistrationIncludesTXTRecord(record string) error         { return godog.ErrPending }
func aZeroconfRegistrationErrorIsInjected() error                  { return godog.ErrPending }
func startReturnsANonNilError() error                              { return godog.ErrPending }
func anAdvertiserThatHasBeenStarted() error                        { return godog.ErrPending }
func anAdvertiserThatHasNotBeenStarted() error                     { return godog.ErrPending }
func shutdownIsCalled() error                                      { return godog.ErrPending }
func theZeroconfServerIsShutDown() error                           { return godog.ErrPending }
func subsequentStartCallsReturnFreshRegistration() error           { return godog.ErrPending }

// noPanicOccurs — owned by tui_steps.go
// theContextIsCancelled — owned by ops_steps.go
func anAdvertiserStartedWithCancellableContext() error { return godog.ErrPending }
func theConfigHasTunerAdvertiseFalse() error           { return godog.ErrPending }
func ocdSmokeAlarmStarts() error                       { return godog.ErrPending }
func theMDNSAdvertiserIsNotStarted() error             { return godog.ErrPending }
