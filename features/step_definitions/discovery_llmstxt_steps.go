package stepdefinitions

// discovery_llmstxt_steps.go — step definitions for features/discovery-llmstxt.feature
//
// Classification: stub — all steps return godog.ErrPending
//
// THESIS-FINDING: discovery_llmstxt_steps.go is stubbed (godog.ErrPending) —
// llms.txt auto-discovery has nominal, not executable Cucumber coverage.
//
// Registered steps (must not be re-registered in other domain files):
//   "discovery is enabled with llms_txt URIs:" (DataTable)
//   "discovery runs once"
//   "each URI is fetched exactly once per discovery interval"
//   "fetch requests use HTTPS"
//   "discovery is configured with llms_txt URI {string}"
//   "{string} is set to true"
//   "{string} is set to false"
//   "the URI {string} is not fetched"
//   "a warning is logged containing {string}"
//   "an llms.txt URI that does not respond within {int} seconds"
//   "{string} is set to {string}"
//   "the fetch is abandoned after {int} seconds"
//   "an llms.txt document at {string} lists an MCP endpoint {string}"
//   "a probe target is registered for endpoint {string}"
//   "the target protocol is {string}"
//   "an llms.txt endpoint {string} was registered in a previous discovery run"
//   "discovery runs again"
//   "the target registry contains exactly {int} entry for endpoint {string}"
//   "no new targets are added to the target registry"
//   "an llms.txt document lists an endpoint {string} with OAuth metadata"
//   "an OAuth config is registered for endpoint {string}"
//   "an llms.txt endpoint {string} declares OAuth metadata"
//   "no OAuth config is registered for endpoint {string}"
//   "{int} seconds elapse after the first discovery run"
//   "a second discovery run is initiated"
//   "no busy polling occurs between runs"
//   "the health endpoint {string} returned status code {int}"

import (
	"github.com/cucumber/godog"
)

func InitializeDiscoveryLlmstxtSteps(ctx *godog.ScenarioContext) {
	// ── fetch behavior ────────────────────────────────────────────────────
	ctx.Step(`^discovery is enabled with llms_txt URIs:$`, discoveryIsEnabledWithLlmsTxtURIs)
	ctx.Step(`^discovery runs once$`, discoveryRunsOnce)
	ctx.Step(`^each URI is fetched exactly once per discovery interval$`, eachURIIsFetchedOnce)
	ctx.Step(`^fetch requests use HTTPS$`, fetchRequestsUseHTTPS)
	ctx.Step(`^discovery is configured with llms_txt URI "([^"]*)"$`, discoveryIsConfiguredWithURI)
	ctx.Step(`^"([^"]*)" is set to true$`, configFlagIsSetToTrue)
	ctx.Step(`^"([^"]*)" is set to false$`, configFlagIsSetToFalse)
	ctx.Step(`^the URI "([^"]*)" is not fetched$`, theURIIsNotFetched)
	ctx.Step(`^a warning is logged containing "([^"]*)"$`, aWarningIsLoggedContaining)
	ctx.Step(`^an llms\.txt URI that does not respond within (\d+) seconds$`, anLlmsTxtURIThatDoesNotRespond)
	ctx.Step(`^"([^"]*)" is set to "([^"]*)"$`, configFieldIsSetTo)
	ctx.Step(`^the fetch is abandoned after (\d+) seconds$`, theFetchIsAbandonedAfter)

	// ── target auto-registration ───────────────────────────────────────────
	ctx.Step(`^an llms\.txt document at "([^"]*)" lists an MCP endpoint "([^"]*)"$`, anLlmsTxtDocumentListsMCPEndpoint)
	ctx.Step(`^a probe target is registered for endpoint "([^"]*)"$`, aProbeTargetIsRegisteredForEndpoint)
	ctx.Step(`^the target protocol is "([^"]*)"$`, theTargetProtocolIs)
	ctx.Step(`^an llms\.txt endpoint "([^"]*)" was registered in a previous discovery run$`, anLlmsTxtEndpointWasRegisteredPreviously)
	ctx.Step(`^discovery runs again$`, discoveryRunsAgain)
	ctx.Step(`^the target registry contains exactly (\d+) entry for endpoint "([^"]*)"$`, theTargetRegistryContainsExactlyNEntry)
	ctx.Step(`^no new targets are added to the target registry$`, noNewTargetsAreAdded)

	// ── oauth auto-registration ────────────────────────────────────────────
	ctx.Step(`^an llms\.txt document lists an endpoint "([^"]*)" with OAuth metadata$`, anLlmsTxtDocumentListsEndpointWithOAuth)
	ctx.Step(`^an OAuth config is registered for endpoint "([^"]*)"$`, anOAuthConfigIsRegistered)
	ctx.Step(`^an llms\.txt endpoint "([^"]*)" declares OAuth metadata$`, anLlmsTxtEndpointDeclaresOAuth)
	ctx.Step(`^no OAuth config is registered for endpoint "([^"]*)"$`, noOAuthConfigIsRegistered)

	// ── discovery interval ─────────────────────────────────────────────────
	ctx.Step(`^(\d+) seconds elapse after the first discovery run$`, nSecondsElapseAfterFirstRun)
	ctx.Step(`^a second discovery run is initiated$`, aSecondDiscoveryRunIsInitiated)
	ctx.Step(`^no busy polling occurs between runs$`, noBusyPollingOccursBetweenRuns)

	// ── self-health ────────────────────────────────────────────────────────
	ctx.Step(`^the health endpoint "([^"]*)" returned status code (\d+)$`, theHealthEndpointReturnedStatusCode)

	// ── additional patterns ─────────────────────────────────────────────────
	ctx.Step(`^network access to llms\.txt URIs is available$`, networkAccessToLlmsTxtURIsIsAvailable)
}

// ── stub implementations ───────────────────────────────────────────────────

func discoveryIsEnabledWithLlmsTxtURIs(table *godog.Table) error          { return godog.ErrPending }
func discoveryRunsOnce() error                                            { return godog.ErrPending }
func eachURIIsFetchedOnce() error                                         { return godog.ErrPending }
func fetchRequestsUseHTTPS() error                                        { return godog.ErrPending }
func discoveryIsConfiguredWithURI(uri string) error                       { return godog.ErrPending }
func configFlagIsSetToTrue(flag string) error                             { return godog.ErrPending }
func configFlagIsSetToFalse(flag string) error                            { return godog.ErrPending }
func theURIIsNotFetched(uri string) error                                 { return godog.ErrPending }
func aWarningIsLoggedContaining(msg string) error                         { return godog.ErrPending }
func anLlmsTxtURIThatDoesNotRespond(seconds int) error                    { return godog.ErrPending }
func configFieldIsSetTo(key, val string) error                            { return godog.ErrPending }
func theFetchIsAbandonedAfter(seconds int) error                          { return godog.ErrPending }
func anLlmsTxtDocumentListsMCPEndpoint(uri, endpoint string) error        { return godog.ErrPending }
func aProbeTargetIsRegisteredForEndpoint(endpoint string) error           { return godog.ErrPending }
func theTargetProtocolIs(protocol string) error                           { return godog.ErrPending }
func anLlmsTxtEndpointWasRegisteredPreviously(endpoint string) error      { return godog.ErrPending }
func discoveryRunsAgain() error                                           { return godog.ErrPending }
func theTargetRegistryContainsExactlyNEntry(n int, endpoint string) error { return godog.ErrPending }
func noNewTargetsAreAdded() error                                         { return godog.ErrPending }
func anLlmsTxtDocumentListsEndpointWithOAuth(endpoint string) error       { return godog.ErrPending }
func anOAuthConfigIsRegistered(endpoint string) error                     { return godog.ErrPending }
func anLlmsTxtEndpointDeclaresOAuth(endpoint string) error                { return godog.ErrPending }
func noOAuthConfigIsRegistered(endpoint string) error                     { return godog.ErrPending }
func nSecondsElapseAfterFirstRun(n int) error                             { return godog.ErrPending }
func aSecondDiscoveryRunIsInitiated() error                               { return godog.ErrPending }
func noBusyPollingOccursBetweenRuns() error                               { return godog.ErrPending }
func theHealthEndpointReturnedStatusCode(addr string, code int) error     { return godog.ErrPending }

func networkAccessToLlmsTxtURIsIsAvailable() error { return godog.ErrPending }
