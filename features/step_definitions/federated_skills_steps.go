package stepdefinitions

// federated_skills_steps.go — step definitions for features/federated-skills.feature
// see: common_steps.go for shared steps
// see: federation_steps.go for mesh setup steps

import (
	"github.com/cucumber/godog"
)

func InitializeFederatedSkillsScenario(ctx *godog.ScenarioContext) {
	// background
	ctx.Step(`^a federation mesh is running with at least (\d+) instances$`, aFederationMeshRunningWithInstances)
	ctx.Step(`^each instance has at least one skill installed$`, eachInstanceHasAtLeastOneSkill)

	// skill announcement
	ctx.Step(`^instance "([^"]*)" has skills \["([^"]*)", "([^"]*)"\] installed$`, instanceHasSkillsInstalled)
	ctx.Step(`^instance "([^"]*)" has skills \["([^"]*)"\] installed$`, instanceHasSingleSkillInstalled)
	ctx.Step(`^inst-a sends a POST to /introductions$`, instASendsPostToIntroductions)
	ctx.Step(`^the InstanceRecord in the request body contains the skill inventory for inst-a$`, instanceRecordContainsSkillInventory)
	ctx.Step(`^inst-a sends a POST to /heartbeats$`, instASendsPostToHeartbeats)
	ctx.Step(`^instance "([^"]*)" is running with skill "([^"]*)" installed$`, instanceIsRunningWithSkill)
	ctx.Step(`^the skill "([^"]*)" is added to inst-a$`, theSkillIsAddedToInstA)
	ctx.Step(`^inst-a sends its next heartbeat$`, instASendsNextHeartbeat)
	ctx.Step(`^the heartbeat InstanceRecord contains both "([^"]*)" and "([^"]*)"$`, heartbeatContainsBothSkills)

	// federated skill namespacing
	ctx.Step(`^instance "([^"]*)" has identity id "([^"]*)" and skill "([^"]*)"$`, instanceHasIdentityAndSkill)
	ctx.Step(`^the introducer aggregates skill inventories from its peers$`, introducerAggregatesSkillInventories)
	ctx.Step(`^the federated skill id is "([^"]*)"$`, theFederatedSkillIDIs)
	ctx.Step(`^the introducer "([^"]*)" has skill "([^"]*)" installed locally$`, theIntroducerHasSkillLocally)
	ctx.Step(`^the skill id is "([^"]*)" without a namespace prefix$`, theSkillIDIsWithoutNamespacePrefix)
	ctx.Step(`^instance "([^"]*)" \(id "([^"]*)"\) has skill "([^"]*)"$`, instanceWithIDHasSkill)
	ctx.Step(`^the introducer aggregates skill inventories$`, introducerAggregatesInventories)
	ctx.Step(`^the federated skill IDs are:$`, theFederatedSkillIDsAre)

	// membership API skill inventory
	ctx.Step(`^(\d+) follower peers each with distinct skill sets$`, followerPeersWithDistinctSkills)
	ctx.Step(`^each entry in the "([^"]*)" array contains a skill inventory field$`, eachEntryContainsSkillInventory)
	ctx.Step(`^the skill inventory for each peer lists that peer's installed skills$`, skillInventoryListsInstalledSkills)
	ctx.Step(`^a follower peer with no skills installed$`, aFollowerPeerWithNoSkills)
	ctx.Step(`^that peer's skill inventory field is an empty list$`, thatPeersSkillInventoryIsEmpty)

	// federated skill lookup
	ctx.Step(`^the federated skill registry contains "([^"]*)"$`, federatedSkillRegistryContains)
	ctx.Step(`^a lookup is performed for federated skill id "([^"]*)"$`, aLookupPerformedForFederatedSkillID)
	ctx.Step(`^the result identifies instance "([^"]*)" as the host$`, theResultIdentifiesInstanceAsHost)
	ctx.Step(`^the local skill name is "([^"]*)"$`, theLocalSkillNameIs)
	ctx.Step(`^the result is not found$`, theResultIsNotFound)

	// skill routing and routing trace
	ctx.Step(`^the local instance is "([^"]*)" \(id "([^"]*)"\)$`, theLocalInstanceIsWithID)
	ctx.Step(`^the federated skill "([^"]*)" is hosted on inst-b$`, theFederatedSkillIsHostedOnInstB)
	ctx.Step(`^the agent on inst-a invokes "([^"]*)"$`, theAgentOnInstAInvokes)
	ctx.Step(`^the invocation is routed to inst-b$`, theInvocationIsRoutedToInstB)
	ctx.Step(`^the routing trace contains \["([^"]*)", "([^"]*)"\]$`, theRoutingTraceContainsTwoHops)
	ctx.Step(`^a 3-instance mesh: inst-a → inst-b → inst-c \(linear fan-out, no cycle\)$`, a3InstanceLinearMesh)
	ctx.Step(`^inst-a invokes a skill hosted on inst-c via inst-b$`, instAInvokesSkillViaInstB)
	ctx.Step(`^the routing trace on arrival at inst-c is \["([^"]*)", "([^"]*)", "([^"]*)"\]$`, theRoutingTraceOnArrivalAtInstC)
	ctx.Step(`^instance "([^"]*)" receives a skill invocation$`, instanceReceivesSkillInvocation)
	ctx.Step(`^the routing trace already contains "([^"]*)"'s own instance ID$`, routingTraceContainsOwnID)
	ctx.Step(`^the invocation is processed$`, theInvocationIsProcessed)
	ctx.Step(`^the invocation is rejected with a cycle error$`, theInvocationIsRejectedWithCycleError)
	ctx.Step(`^instance A invokes a skill on instance B$`, instanceAInvokesSkillOnB)
	ctx.Step(`^instance B attempts to route the same invocation back to instance A$`, instanceBRoutesBackToA)
	ctx.Step(`^instance A receives the re-routed invocation$`, instanceAReceivesReroutedInvocation)
	ctx.Step(`^the invocation is rejected$`, theInvocationIsRejected)
	ctx.Step(`^the error identifies both instance IDs in the cycle$`, theErrorIdentifiesBothInstanceIDs)

	// open-the-pickle-jar federated scenarios
	ctx.Step(`^the agent is operating on instance "([^"]*)" within a federation mesh$`, agentOperatingOnInstance)
	ctx.Step(`^open-the-pickle-jar is invoked with no scope argument on inst-b$`, openPickleJarInvokedNoScope)
	ctx.Step(`^the output lists skills installed on inst-b$`, outputListsSkillsOnInstB)
	ctx.Step(`^the output identifies inst-b by its instance ID and role$`, outputIdentifiesInstBByIDAndRole)
	ctx.Step(`^the output notes which skills are also present on peer instances$`, outputNotesSharedSkills)
	ctx.Step(`^the local instance is the introducer$`, theLocalInstanceIsTheIntroducer)
	ctx.Step(`^instance "([^"]*)" \(id "([^"]*)"\) is a registered peer with skill "([^"]*)"$`, instanceIsRegisteredPeerWithSkill)
	ctx.Step(`^open-the-pickle-jar is invoked with scope "([^"]*)"$`, openPickleJarInvokedWithScope)
	ctx.Step(`^the invocation is routed to inst-b$`, theInvocationIsRoutedToInstBAlias)
	ctx.Step(`^the resulting Gherkin audit covers inst-b's skill domain$`, resultingAuditCoversInstBDomain)
	ctx.Step(`^the output includes inst-b's instance ID in the audit header$`, outputIncludesInstBIDInHeader)
	ctx.Step(`^a federation mesh where inst-a has features/federation\.feature and inst-b does not$`, federationMeshWithFeatureDivergence)
	ctx.Step(`^open-the-pickle-jar is invoked on inst-a with scope "([^"]*)"$`, openPickleJarOnInstAWithScope)
	ctx.Step(`^the audit result notes that federation\.feature exists locally on inst-a$`, auditNotesFederationFeatureOnInstA)
	ctx.Step(`^notes that inst-b has no local federation\.feature$`, auditNotesInstBMissingFederationFeature)

	// peer departure removes federated skills
	ctx.Step(`^instance "([^"]*)" is registered with skills \["([^"]*)"\]$`, instanceRegisteredWithSkills)
	ctx.Step(`^inst-b stops heartbeating for (\d+) seconds$`, instBStopsHeartbeatingForSeconds)
	ctx.Step(`^"([^"]*)" is no longer present in the federated skill registry$`, skillNoLongerPresentInRegistry)
	ctx.Step(`^a registry event of type "([^"]*)" is fired for inst-b's skill entries$`, registryEventFiredForInstBSkills)
	ctx.Step(`^instance "([^"]*)" is registered with skills \["([^"]*)", "([^"]*)"\]$`, instanceRegisteredWithTwoSkills)
	ctx.Step(`^Remove is called for inst-b's ID$`, removeCalledForInstBID)
	ctx.Step(`^both "([^"]*)" and "([^"]*)" are removed$`, bothSkillsAreRemoved)
	ctx.Step(`^the federated skill count decreases by (\d+)$`, federatedSkillCountDecreasesBy)

	// self-description with skills
	ctx.Step(`^the instance has skills \["([^"]*)", "([^"]*)"\] installed$`, theInstanceHasTwoSkillsInstalled)
	ctx.Step(`^the document contains a "([^"]*)" field listing the local skill names$`, documentContainsSkillsField)
	ctx.Step(`^the introducer has (\d+) peers each with (\d+) skill$`, theIntroducerHasPeersWithSkills)
	ctx.Step(`^the document contains a "([^"]*)" field with (\d+) entries$`, documentContainsFieldWithEntries)
	ctx.Step(`^each entry follows the "\[instance-id\] skill-name" format$`, eachEntryFollowsFederatedSkillFormat)
}

func aFederationMeshRunningWithInstances(n int) error        { return godog.ErrPending }
func eachInstanceHasAtLeastOneSkill() error                  { return godog.ErrPending }
func instanceHasSkillsInstalled(inst, s1, s2 string) error   { return godog.ErrPending }
func instanceHasSingleSkillInstalled(inst, skill string) error { return godog.ErrPending }
func instASendsPostToIntroductions() error                   { return godog.ErrPending }
func instanceRecordContainsSkillInventory() error            { return godog.ErrPending }
func instASendsPostToHeartbeats() error                      { return godog.ErrPending }
func instanceIsRunningWithSkill(inst, skill string) error    { return godog.ErrPending }
func theSkillIsAddedToInstA(skill string) error              { return godog.ErrPending }
func instASendsNextHeartbeat() error                         { return godog.ErrPending }
func heartbeatContainsBothSkills(s1, s2 string) error        { return godog.ErrPending }
func instanceHasIdentityAndSkill(inst, id, skill string) error { return godog.ErrPending }
func introducerAggregatesSkillInventories() error            { return godog.ErrPending }
func theFederatedSkillIDIs(id string) error                  { return godog.ErrPending }
func theIntroducerHasSkillLocally(inst, skill string) error  { return godog.ErrPending }
func theSkillIDIsWithoutNamespacePrefix(id string) error     { return godog.ErrPending }
func instanceWithIDHasSkill(inst, id, skill string) error    { return godog.ErrPending }
func introducerAggregatesInventories() error                 { return godog.ErrPending }
func theFederatedSkillIDsAre(table *godog.Table) error       { return godog.ErrPending }
func followerPeersWithDistinctSkills(n int) error            { return godog.ErrPending }
func eachEntryContainsSkillInventory(array string) error     { return godog.ErrPending }
func skillInventoryListsInstalledSkills() error              { return godog.ErrPending }
func aFollowerPeerWithNoSkills() error                       { return godog.ErrPending }
func thatPeersSkillInventoryIsEmpty() error                  { return godog.ErrPending }
func federatedSkillRegistryContains(id string) error         { return godog.ErrPending }
func aLookupPerformedForFederatedSkillID(id string) error    { return godog.ErrPending }
func theResultIdentifiesInstanceAsHost(inst string) error    { return godog.ErrPending }
func theLocalSkillNameIs(name string) error                  { return godog.ErrPending }
func theResultIsNotFound() error                             { return godog.ErrPending }
func theLocalInstanceIsWithID(inst, id string) error         { return godog.ErrPending }
func theFederatedSkillIsHostedOnInstB(skill string) error    { return godog.ErrPending }
func theAgentOnInstAInvokes(skill string) error              { return godog.ErrPending }
func theInvocationIsRoutedToInstB() error                    { return godog.ErrPending }
func theRoutingTraceContainsTwoHops(h1, h2 string) error     { return godog.ErrPending }
func a3InstanceLinearMesh() error                            { return godog.ErrPending }
func instAInvokesSkillViaInstB() error                       { return godog.ErrPending }
func theRoutingTraceOnArrivalAtInstC(h1, h2, h3 string) error { return godog.ErrPending }
func instanceReceivesSkillInvocation(inst string) error      { return godog.ErrPending }
func routingTraceContainsOwnID(inst string) error            { return godog.ErrPending }
func theInvocationIsProcessed() error                        { return godog.ErrPending }
func theInvocationIsRejectedWithCycleError() error           { return godog.ErrPending }
func instanceAInvokesSkillOnB() error                        { return godog.ErrPending }
func instanceBRoutesBackToA() error                          { return godog.ErrPending }
func instanceAReceivesReroutedInvocation() error             { return godog.ErrPending }
func theInvocationIsRejected() error                         { return godog.ErrPending }
func theErrorIdentifiesBothInstanceIDs() error               { return godog.ErrPending }
func agentOperatingOnInstance(inst string) error             { return godog.ErrPending }
func openPickleJarInvokedNoScope() error                     { return godog.ErrPending }
func outputListsSkillsOnInstB() error                        { return godog.ErrPending }
func outputIdentifiesInstBByIDAndRole() error                { return godog.ErrPending }
func outputNotesSharedSkills() error                         { return godog.ErrPending }
func theLocalInstanceIsTheIntroducer() error                 { return godog.ErrPending }
func instanceIsRegisteredPeerWithSkill(inst, id, skill string) error { return godog.ErrPending }
func openPickleJarInvokedWithScope(scope string) error       { return godog.ErrPending }
func theInvocationIsRoutedToInstBAlias() error               { return godog.ErrPending }
func resultingAuditCoversInstBDomain() error                 { return godog.ErrPending }
func outputIncludesInstBIDInHeader() error                   { return godog.ErrPending }
func federationMeshWithFeatureDivergence() error             { return godog.ErrPending }
func openPickleJarOnInstAWithScope(scope string) error       { return godog.ErrPending }
func auditNotesFederationFeatureOnInstA() error              { return godog.ErrPending }
func auditNotesInstBMissingFederationFeature() error         { return godog.ErrPending }
func instanceRegisteredWithSkills(inst, skill string) error  { return godog.ErrPending }
func instBStopsHeartbeatingForSeconds(n int) error           { return godog.ErrPending }
func skillNoLongerPresentInRegistry(id string) error         { return godog.ErrPending }
func registryEventFiredForInstBSkills(eventType string) error { return godog.ErrPending }
func instanceRegisteredWithTwoSkills(inst, s1, s2 string) error { return godog.ErrPending }
func removeCalledForInstBID() error                          { return godog.ErrPending }
func bothSkillsAreRemoved(s1, s2 string) error               { return godog.ErrPending }
func federatedSkillCountDecreasesBy(n int) error             { return godog.ErrPending }
func theInstanceHasTwoSkillsInstalled(s1, s2 string) error   { return godog.ErrPending }
func documentContainsSkillsField(field string) error         { return godog.ErrPending }
func theIntroducerHasPeersWithSkills(peers, skills int) error { return godog.ErrPending }
func documentContainsFieldWithEntries(field string, n int) error { return godog.ErrPending }
func eachEntryFollowsFederatedSkillFormat() error            { return godog.ErrPending }
