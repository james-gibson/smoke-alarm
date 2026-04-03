package stepdefinitions

// federation_steps.go — step definitions for features/federation.feature
// see: common_steps.go for shared steps (binary installed, config, log assertions, HTTP)

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/cucumber/godog"
)

func InitializeFederationScenario(ctx *godog.ScenarioContext) {
	// slot election
	ctx.Step(`^no other instance is running on the federation port range$`, noOtherInstanceRunningOnPortRange)
	ctx.Step(`^an instance starts with base_port (\d+) and max_port (\d+)$`, anInstanceStartsWithPorts)
	ctx.Step(`^the instance binds port (\d+)$`, theInstanceBindsPort)
	ctx.Step(`^the instance identity role is "([^"]*)"$`, theInstanceIdentityRoleIs)
	ctx.Step(`^the identity is persisted to "([^"]*)"$`, theIdentityIsPersistedTo)
	ctx.Step(`^an introducer is already bound on port (\d+)$`, anIntroducerAlreadyBoundOnPort)
	ctx.Step(`^a second instance starts with the same base_port and max_port$`, aSecondInstanceStartsSamePorts)
	ctx.Step(`^the second instance binds port (\d+) \+ 1$`, theSecondInstanceBindsPortPlusOne)
	ctx.Step(`^the second instance identity role is "([^"]*)"$`, theSecondInstanceIdentityRoleIs)
	ctx.Step(`^all ports from base_port to max_port are already bound$`, allPortsAlreadyBound)
	ctx.Step(`^ClaimSlot is called$`, claimSlotIsCalled)
	ctx.Step(`^the error is "([^"]*)"$`, theErrorIs)
	ctx.Step(`^a config with no federation\.base_port field$`, aConfigWithNoFederationBasePort)
	ctx.Step(`^ports (\d+) through (\d+) are the candidates$`, portsAreTheCandidates)

	// instance identity
	ctx.Step(`^two ClaimSlot calls with identical hostname, service_name, state_dir, and port$`, twoClaimSlotCallsIdentical)
	ctx.Step(`^both instance IDs are computed$`, bothInstanceIDsAreComputed)
	ctx.Step(`^both IDs are equal$`, bothIDsAreEqual)
	ctx.Step(`^two ClaimSlot calls with identical hostname and service_name but different ports$`, twoClaimSlotCallsDifferentPorts)
	ctx.Step(`^the two instance IDs are different$`, theTwoInstanceIDsAreDifferent)
	ctx.Step(`^a persisted identity\.json exists with port (\d+) in the candidate range$`, aPersistedIdentityExistsWithPort)
	ctx.Step(`^ClaimSlot is called and port (\d+) is free$`, claimSlotCalledAndPortIsFree)
	ctx.Step(`^the instance claims port (\d+) before trying other candidates$`, theInstanceClaimsPortFirst)

	// slot lock
	ctx.Step(`^two processes attempt ClaimSlot simultaneously on the same port range$`, twoProcessesAttemptClaimSlotSimultaneously)
	ctx.Step(`^exactly one process succeeds as introducer$`, exactlyOneProcessSucceedsAsIntroducer)
	ctx.Step(`^the other process becomes a follower or receives ErrNoFreeSlots$`, theOtherProcessBecomesFollowerOrError)
	ctx.Step(`^a slot\.lock file exists containing the PID of a process that is no longer running$`, aStaleLockFileExists)
	ctx.Step(`^the stale lock is removed$`, theStaleLockIsRemoved)
	ctx.Step(`^ClaimSlot proceeds normally$`, claimSlotProceedsNormally)

	// introducer server
	ctx.Step(`^the introducer holds a SlotClaim with listener on port (\d+)$`, theIntroducerHoldsSlotClaimOnPort)
	ctx.Step(`^the federation server starts$`, theFederationServerStarts)
	ctx.Step(`^POST /introductions is served on that port$`, postIntroductionsIsServed)
	ctx.Step(`^POST /heartbeats is served on that port$`, postHeartbeatsIsServed)
	ctx.Step(`^GET /membership is served on that port$`, getMembershipIsServed)
	ctx.Step(`^the federation server is running as introducer$`, theFederationServerIsRunningAsIntroducer)
	ctx.Step(`^a follower POSTs to /introductions with a valid InstanceRecord$`, aFollowerPostsToIntroductions)
	ctx.Step(`^the peer appears in the registry$`, thePeerAppearsInRegistry)
	ctx.Step(`^a POST is sent to /introductions with an empty "([^"]*)" field$`, aPostSentWithEmptyField)
	ctx.Step(`^a GET request is sent to /introductions$`, aGETRequestSentToIntroductions)
	ctx.Step(`^a follower is already registered with the introducer$`, aFollowerAlreadyRegistered)
	ctx.Step(`^the follower POSTs to /heartbeats with its InstanceRecord$`, theFollowerPostsToHeartbeats)
	ctx.Step(`^the peer's last_seen_at is updated in the registry$`, thePeersLastSeenAtIsUpdated)
	ctx.Step(`^the federation server has (\d+) registered peers$`, theFederationServerHasRegisteredPeers)
	ctx.Step(`^the "([^"]*)" array length is (\d+)$`, theArrayLengthIs)

	// age-out
	ctx.Step(`^"([^"]*)" is set to "([^"]*)"$`, configFieldIsSetTo)
	ctx.Step(`^the follower sends no heartbeat for (\d+) seconds$`, theFollowerSendsNoHeartbeatForSeconds)
	ctx.Step(`^the follower is removed from the registry$`, theFollowerIsRemovedFromRegistry)
	ctx.Step(`^the removal reason is "([^"]*)"$`, theRemovalReasonIs)
	ctx.Step(`^the removal is reflected in GET /membership$`, theRemovalIsReflectedInMembership)
	ctx.Step(`^the follower sends a heartbeat at (\d+) seconds$`, theFollowerSendsHeartbeatAtSeconds)
	ctx.Step(`^the follower remains in the registry after (\d+) seconds$`, theFollowerRemainsInRegistryAfterSeconds)

	// peer cap
	ctx.Step(`^the registry already contains (\d+) peers at the MaxPeers limit$`, registryAtMaxPeers)
	ctx.Step(`^Upsert is called with a new peer record$`, upsertCalledWithNewPeerRecord)
	ctx.Step(`^the registry peer count does not increase$`, registryPeerCountDoesNotIncrease)
	ctx.Step(`^no error is returned$`, noErrorIsReturned)
	ctx.Step(`^Upsert is called with a record whose ID matches the registry's own identity$`, upsertCalledWithOwnID)
	ctx.Step(`^the registry peer count does not change$`, registryPeerCountDoesNotChange)
	ctx.Step(`^no event is fired$`, noEventIsFired)

	// follower client
	ctx.Step(`^a follower client is configured with an introducer URL$`, aFollowerClientConfiguredWithIntroducerURL)
	ctx.Step(`^Start is called$`, startIsCalled)
	ctx.Step(`^a POST to /introductions is sent before the first announce_interval elapses$`, postIntroductionsSentBeforeFirstInterval)
	ctx.Step(`^a follower client has not yet successfully introduced itself$`, aFollowerClientNotYetIntroduced)
	ctx.Step(`^the heartbeat_interval elapses$`, theHeartbeatIntervalElapses)
	ctx.Step(`^no POST to /heartbeats is sent$`, noPostToHeartbeatsSent)
	ctx.Step(`^the introducer's introduction response contains (\d+) peer records$`, theIntroducerResponseContainsPeerRecords)
	ctx.Step(`^the follower processes the introduction response$`, theFollowerProcessesIntroductionResponse)
	ctx.Step(`^both peers are upserted into the follower's local registry$`, bothPeersAreUpserted)
	ctx.Step(`^the follower's registry snapshot is saved to disk$`, theFollowerRegistrySnapshotSaved)
	ctx.Step(`^the introducer URL is unreachable$`, theIntroducerURLIsUnreachable)
	ctx.Step(`^the follower attempts to send an introduction$`, theFollowerAttemptsToSendIntroduction)
	ctx.Step(`^the client retries on the next announce_interval$`, theClientRetriesOnNextInterval)
	ctx.Step(`^the follower is introduced and the introducer becomes unreachable$`, theFollowerIntroducedAndIntroducerUnreachable)

	// registry snapshot
	ctx.Step(`^SaveSnapshot is called$`, saveSnapshotIsCalled)
	ctx.Step(`^the file "([^"]*)" exists$`, theFileExists)
	ctx.Step(`^no temporary file remains after the write$`, noTemporaryFileRemains)
	ctx.Step(`^the snapshot JSON contains "([^"]*)"$`, theSnapshotJSONContains)
	ctx.Step(`^the registry is at version (\d+)$`, theRegistryIsAtVersion)
	ctx.Step(`^a peer is upserted$`, aPeerIsUpserted)
	ctx.Step(`^the registry version is (\d+) \+ 1$`, theRegistryVersionIsIncremented)

	// registry events
	ctx.Step(`^the registry has no peer with id "([^"]*)"$`, theRegistryHasNoPeerWithID)
	ctx.Step(`^Upsert is called with a record with id "([^"]*)"$`, upsertCalledWithID)
	ctx.Step(`^the OnChange callback receives an event with type "([^"]*)"$`, theOnChangeCallbackReceivesEvent)
	ctx.Step(`^the registry already has a peer with id "([^"]*)"$`, theRegistryAlreadyHasPeerWithID)
	ctx.Step(`^Upsert is called again with the same id$`, upsertCalledAgainSameID)
	ctx.Step(`^the registry has a peer with id "([^"]*)"$`, theRegistryHasPeerWithID)
	ctx.Step(`^Remove is called with that id$`, removeCalledWithThatID)

	// poller
	ctx.Step(`^a Poller configured with downstream endpoints \["([^"]*)", "([^"]*)"\]$`, aPollerConfiguredWithDownstream)
	ctx.Step(`^a poll cycle runs$`, aPollCycleRuns)
	ctx.Step(`^GET http://([^/]+)/status is requested$`, getStatusIsRequested)
	ctx.Step(`^a downstream at "([^"]*)" returns a target with id "([^"]*)"$`, aDownstreamReturnsTarget)
	ctx.Step(`^the aggregated target ID is "([^"]*)"$`, theAggregatedTargetIDIs)
	ctx.Step(`^a downstream endpoint "([^"]*)" is unreachable$`, aDownstreamEndpointIsUnreachable)
	ctx.Step(`^a target with id "([^"]*)" is included in the update$`, aTargetWithIDIncluded)
	ctx.Step(`^the target state is "([^"]*)"$`, theTargetStateIs)
	ctx.Step(`^a Poller with (\d+) downstream endpoints each returning (\d+) targets$`, aPollerWithDownstreamEndpoints)
	ctx.Step(`^updateFn is called with (\d+) targets$`, updateFnCalledWithTargets)
	ctx.Step(`^endpoints \["([^"]*)", "([^"]*)", "([^"]*)"\]$`, endpointsAre)
	ctx.Step(`^SortEndpoints is called$`, sortEndpointsIsCalled)
	ctx.Step(`^the result is \["([^"]*)", "([^"]*)", "([^"]*)"\]$`, theSortedResultIs)

	// cycle detection
	ctx.Step(`^instance A is the introducer with downstream \[B\]$`, instanceAIsIntroducerWithDownstreamB)
	ctx.Step(`^instance B is a follower with downstream \[C\]$`, instanceBIsFollowerWithDownstreamC)
	ctx.Step(`^instance C has no downstream$`, instanceCHasNoDownstream)
	ctx.Step(`^the topology is validated$`, theTopologyIsValidated)
	ctx.Step(`^no routing cycle is detected$`, noRoutingCycleDetected)
	ctx.Step(`^every message carries a routing trace$`, everyMessageCarriesRoutingTrace)
	ctx.Step(`^instance A has downstream \[B\] and instance B has downstream \[A\]$`, instanceABCycle)
	ctx.Step(`^a cycle error is returned$`, aCycleErrorIsReturned)

	// full lifecycle
	ctx.Step(`^no instances are running on the federation port range$`, noInstancesRunningOnPortRange)
	ctx.Step(`^instance 1 starts with federation enabled$`, instance1StartsWithFederation)
	ctx.Step(`^instance 1 binds base_port and becomes the introducer$`, instance1BindsBasePort)
	ctx.Step(`^instance 2 starts with the same port range$`, instance2StartsWithSamePortRange)
	ctx.Step(`^instance 2 joins as a follower$`, instance2JoinsAsFollower)
	ctx.Step(`^instance 2 appears in GET /membership on the introducer$`, instance2AppearsInMembership)
	ctx.Step(`^instance 3 starts with the same port range$`, instance3StartsWithSamePortRange)
	ctx.Step(`^instance 3 joins as a follower$`, instance3JoinsAsFollower)
	ctx.Step(`^both instance 2 and instance 3 appear in GET /membership$`, bothInstancesAppearInMembership)
	ctx.Step(`^a mesh of 3 instances \(1 introducer \+ 2 followers\) is running$`, aMeshOf3InstancesIsRunning)
	ctx.Step(`^follower instance 2 is stopped$`, followerInstance2IsStopped)
	ctx.Step(`^within (\d+) seconds instance 2 is absent from GET /membership$`, withinSecondsInstance2Absent)
	ctx.Step(`^instance 3 remains present in GET /membership$`, instance3RemainsPresent)

	// ── additional patterns ─────────────────────────────────────────────────
	ctx.Step(`^a GET request is sent to /membership on the introducer$`, aGETRequestSentToMembershipOnIntroducer)
	ctx.Step(`^a GET request is sent to /membership$`, aGETRequestSentToMembershipFed)
	ctx.Step(`^a GET request is sent to /\.well-known/smoke-alarm\.json$`, aGETRequestSentToWellKnown)
	ctx.Step(`^a GET request is sent to /\.well-known/smoke-alarm\.json on the introducer$`, aGETRequestSentToWellKnownOnIntroducer)
	ctx.Step(`^federation is enabled in the config$`, federationIsEnabledInTheConfig)
	ctx.Step(`^no component with empty name appears in the response$`, noComponentWithEmptyNameInResponse)
	ctx.Step(`^a follower is registered with the introducer$`, aFollowerIsRegisteredWithTheIntroducer)
	ctx.Step(`^the running instance reports version "([^"]*)" via /status$`, theRunningInstanceReportsVersionViaStatus)
	ctx.Step(`^the second instance binds port (\d+) \+ (\d+)$`, theSecondInstanceBindsPortPlusDelta)
	ctx.Step(`^the registry version is (\d+) \+ (\d+)$`, theRegistryVersionIsPlus)
	ctx.Step(`^instance "([^"]*)" \(id "([^"]*)"\) is a registered peer$`, instanceIsARegisteredPeer)
}

// Pending stub implementations — replace with real logic when wiring Cucumber.

func noOtherInstanceRunningOnPortRange() error          { return godog.ErrPending }
func anInstanceStartsWithPorts(base, maxPort int) error { return godog.ErrPending }
func theInstanceBindsPort(port int) error               { return godog.ErrPending }
func theInstanceIdentityRoleIs(role string) error       { return godog.ErrPending }
func theIdentityIsPersistedTo(path string) error        { return godog.ErrPending }
func anIntroducerAlreadyBoundOnPort(port int) error     { return godog.ErrPending }
func aSecondInstanceStartsSamePorts() error             { return godog.ErrPending }
func theSecondInstanceBindsPortPlusOne(port int) error  { return godog.ErrPending }
func theSecondInstanceIdentityRoleIs(role string) error { return godog.ErrPending }
func allPortsAlreadyBound() error                       { return godog.ErrPending }
func claimSlotIsCalled() error                          { return godog.ErrPending }

// theErrorIs — owned by alerts_steps.go
func aConfigWithNoFederationBasePort() error            { return godog.ErrPending }
func portsAreTheCandidates(from, to int) error          { return godog.ErrPending }
func twoClaimSlotCallsIdentical() error                 { return godog.ErrPending }
func bothInstanceIDsAreComputed() error                 { return godog.ErrPending }
func bothIDsAreEqual() error                            { return godog.ErrPending }
func twoClaimSlotCallsDifferentPorts() error            { return godog.ErrPending }
func theTwoInstanceIDsAreDifferent() error              { return godog.ErrPending }
func aPersistedIdentityExistsWithPort(port int) error   { return godog.ErrPending }
func claimSlotCalledAndPortIsFree(port int) error       { return godog.ErrPending }
func theInstanceClaimsPortFirst(port int) error         { return godog.ErrPending }
func twoProcessesAttemptClaimSlotSimultaneously() error { return godog.ErrPending }
func exactlyOneProcessSucceedsAsIntroducer() error      { return godog.ErrPending }
func theOtherProcessBecomesFollowerOrError() error      { return godog.ErrPending }
func aStaleLockFileExists() error                       { return godog.ErrPending }
func theStaleLockIsRemoved() error                      { return godog.ErrPending }
func claimSlotProceedsNormally() error                  { return godog.ErrPending }
func theIntroducerHoldsSlotClaimOnPort(port int) error  { return godog.ErrPending }
func theFederationServerStarts() error                  { return godog.ErrPending }
func postIntroductionsIsServed() error                  { return godog.ErrPending }
func postHeartbeatsIsServed() error                     { return godog.ErrPending }
func getMembershipIsServed() error                      { return godog.ErrPending }
func theFederationServerIsRunningAsIntroducer() error   { return godog.ErrPending }
func aFollowerPostsToIntroductions() error              { return godog.ErrPending }
func thePeerAppearsInRegistry() error                   { return godog.ErrPending }
func aPostSentWithEmptyField(field string) error        { return godog.ErrPending }
func aGETRequestSentToIntroductions() error             { return godog.ErrPending }
func aFollowerAlreadyRegistered() error                 { return godog.ErrPending }
func theFollowerPostsToHeartbeats() error               { return godog.ErrPending }
func thePeersLastSeenAtIsUpdated() error                { return godog.ErrPending }
func theFederationServerHasRegisteredPeers(n int) error { return godog.ErrPending }
func theArrayLengthIs(key string, n int) error          { return godog.ErrPending }

// configFieldIsSetTo — owned by discovery_llmstxt_steps.go
func theFollowerSendsNoHeartbeatForSeconds(n int) error    { return godog.ErrPending }
func theFollowerIsRemovedFromRegistry() error              { return godog.ErrPending }
func theRemovalReasonIs(reason string) error               { return godog.ErrPending }
func theRemovalIsReflectedInMembership() error             { return godog.ErrPending }
func theFollowerSendsHeartbeatAtSeconds(n int) error       { return godog.ErrPending }
func theFollowerRemainsInRegistryAfterSeconds(n int) error { return godog.ErrPending }
func registryAtMaxPeers(n int) error                       { return godog.ErrPending }
func upsertCalledWithNewPeerRecord() error                 { return godog.ErrPending }
func registryPeerCountDoesNotIncrease() error              { return godog.ErrPending }
func noErrorIsReturned() error {
	if ksState.lastErr != nil {
		return fmt.Errorf("expected no error, got: %w", ksState.lastErr)
	}
	return nil
}
func upsertCalledWithOwnID() error                         { return godog.ErrPending }
func registryPeerCountDoesNotChange() error                { return godog.ErrPending }
func noEventIsFired() error                                { return godog.ErrPending }
func aFollowerClientConfiguredWithIntroducerURL() error    { return godog.ErrPending }
func startIsCalled() error                                 { return godog.ErrPending }
func postIntroductionsSentBeforeFirstInterval() error      { return godog.ErrPending }
func aFollowerClientNotYetIntroduced() error               { return godog.ErrPending }
func theHeartbeatIntervalElapses() error                   { return godog.ErrPending }
func noPostToHeartbeatsSent() error                        { return godog.ErrPending }
func theIntroducerResponseContainsPeerRecords(n int) error { return godog.ErrPending }
func theFollowerProcessesIntroductionResponse() error      { return godog.ErrPending }
func bothPeersAreUpserted() error                          { return godog.ErrPending }
func theFollowerRegistrySnapshotSaved() error              { return godog.ErrPending }
func theIntroducerURLIsUnreachable() error                 { return godog.ErrPending }
func theFollowerAttemptsToSendIntroduction() error         { return godog.ErrPending }
func theClientRetriesOnNextInterval() error                { return godog.ErrPending }
func theFollowerIntroducedAndIntroducerUnreachable() error { return godog.ErrPending }
func saveSnapshotIsCalled() error                          { return godog.ErrPending }
func theFileExists(path string) error {
	abs := resolveTestFilePath(path)
	if _, err := os.Stat(abs); err != nil {
		return fmt.Errorf("file %q does not exist: %w", abs, err)
	}
	return nil
}

// resolveTestFilePath resolves a path for file-existence assertions.
// If ksState.storeBaseDir is set (known-state scenario), relative paths are
// resolved against it; otherwise they are resolved against projectRoot.
func resolveTestFilePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	if ksState.storeBaseDir != "" {
		return filepath.Join(ksState.storeBaseDir, path)
	}
	return filepath.Join(projectRoot, path)
}
func noTemporaryFileRemains() error                               { return godog.ErrPending }
func theSnapshotJSONContains(key string) error                    { return godog.ErrPending }
func theRegistryIsAtVersion(n int) error                          { return godog.ErrPending }
func aPeerIsUpserted() error                                      { return godog.ErrPending }
func theRegistryVersionIsIncremented(base int) error              { return godog.ErrPending }
func theRegistryHasNoPeerWithID(id string) error                  { return godog.ErrPending }
func upsertCalledWithID(id string) error                          { return godog.ErrPending }
func theOnChangeCallbackReceivesEvent(eventType string) error     { return godog.ErrPending }
func theRegistryAlreadyHasPeerWithID(id string) error             { return godog.ErrPending }
func upsertCalledAgainSameID() error                              { return godog.ErrPending }
func theRegistryHasPeerWithID(id string) error                    { return godog.ErrPending }
func removeCalledWithThatID() error                               { return godog.ErrPending }
func aPollerConfiguredWithDownstream(ep1, ep2 string) error       { return godog.ErrPending }
func aPollCycleRuns() error                                       { return godog.ErrPending }
func getStatusIsRequested(host string) error                      { return godog.ErrPending }
func aDownstreamReturnsTarget(endpoint, id string) error          { return godog.ErrPending }
func theAggregatedTargetIDIs(id string) error                     { return godog.ErrPending }
func aDownstreamEndpointIsUnreachable(ep string) error            { return godog.ErrPending }
func aTargetWithIDIncluded(id string) error                       { return godog.ErrPending }
func theTargetStateIs(state string) error                         { return godog.ErrPending }
func aPollerWithDownstreamEndpoints(endpoints, targets int) error { return godog.ErrPending }
func updateFnCalledWithTargets(n int) error                       { return godog.ErrPending }
func endpointsAre(ep1, ep2, ep3 string) error                     { return godog.ErrPending }
func sortEndpointsIsCalled() error                                { return godog.ErrPending }
func theSortedResultIs(ep1, ep2, ep3 string) error                { return godog.ErrPending }
func instanceAIsIntroducerWithDownstreamB() error                 { return godog.ErrPending }
func instanceBIsFollowerWithDownstreamC() error                   { return godog.ErrPending }
func instanceCHasNoDownstream() error                             { return godog.ErrPending }
func theTopologyIsValidated() error                               { return godog.ErrPending }
func noRoutingCycleDetected() error                               { return godog.ErrPending }
func everyMessageCarriesRoutingTrace() error                      { return godog.ErrPending }
func instanceABCycle() error                                      { return godog.ErrPending }
func aCycleErrorIsReturned() error                                { return godog.ErrPending }
func noInstancesRunningOnPortRange() error                        { return godog.ErrPending }
func instance1StartsWithFederation() error                        { return godog.ErrPending }
func instance1BindsBasePort() error                               { return godog.ErrPending }
func instance2StartsWithSamePortRange() error                     { return godog.ErrPending }
func instance2JoinsAsFollower() error                             { return godog.ErrPending }
func instance2AppearsInMembership() error                         { return godog.ErrPending }
func instance3StartsWithSamePortRange() error                     { return godog.ErrPending }
func instance3JoinsAsFollower() error                             { return godog.ErrPending }
func bothInstancesAppearInMembership() error                      { return godog.ErrPending }
func aMeshOf3InstancesIsRunning() error                           { return godog.ErrPending }
func followerInstance2IsStopped() error                           { return godog.ErrPending }
func withinSecondsInstance2Absent(n int) error                    { return godog.ErrPending }
func instance3RemainsPresent() error                              { return godog.ErrPending }

func aGETRequestSentToMembershipOnIntroducer() error            { return godog.ErrPending }
func aGETRequestSentToMembershipFed() error                     { return godog.ErrPending }
func aGETRequestSentToWellKnown() error                         { return godog.ErrPending }
func aGETRequestSentToWellKnownOnIntroducer() error             { return godog.ErrPending }
func federationIsEnabledInTheConfig() error                     { return godog.ErrPending }
func noComponentWithEmptyNameInResponse() error                 { return godog.ErrPending }
func aFollowerIsRegisteredWithTheIntroducer() error             { return godog.ErrPending }
func theRunningInstanceReportsVersionViaStatus(v string) error  { return godog.ErrPending }
func theSecondInstanceBindsPortPlusDelta(base, delta int) error { return godog.ErrPending }
func theRegistryVersionIsPlus(base, delta int) error            { return godog.ErrPending }
func instanceIsARegisteredPeer(name, id string) error           { return godog.ErrPending }
