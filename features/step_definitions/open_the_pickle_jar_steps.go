package stepdefinitions

// open_the_pickle_jar_steps.go — step definitions for features/open-the-pickle-jar.feature
// see: common_steps.go for shared steps (Claude Code session active, binary installed)
// see: skill_system_steps.go for ValidateSkillFile steps

import "github.com/cucumber/godog"

func InitializeOpenThePickleJarScenario(ctx *godog.ScenarioContext) {
	// SKILL.md validity
	ctx.Step(`^the skill "([^"]*)" is installed at "([^"]*)"$`, theSkillIsInstalledAt)
	ctx.Step(`^ValidateSkillFile is called on "([^"]*)"$`, validateSkillFileCalledOn)
	ctx.Step(`^the result is valid with no errors$`, theResultIsValidWithNoErrors)
	ctx.Step(`^the skill name matches the directory name "([^"]*)"$`, skillNameMatchesDirectoryName)
	ctx.Step(`^the skill "([^"]*)" SKILL\.md is read$`, theSkillSKILLMdIsRead)
	ctx.Step(`^the description field contains the word "([^"]*)"$`, descriptionFieldContainsWord)

	// direct invocation
	ctx.Step(`^the agent invokes the skill "([^"]*)" with no scope argument$`, agentInvokesSkillNoScope)
	ctx.Step(`^the output lists all existing feature files in "([^"]*)"$`, outputListsFeatureFilesIn)
	ctx.Step(`^the output identifies any internal packages without Gherkin coverage$`, outputIdentifiesUncoveredPackages)
	ctx.Step(`^the output proposes the highest-value next feature to write$`, outputProposesNextFeature)
	ctx.Step(`^a skill "([^"]*)" is installed at "([^"]*)"$`, aSkillIsInstalledAt)
	ctx.Step(`^no "([^"]*)" exists$`, noFileExists)
	ctx.Step(`^the output flags "([^"]*)" as lacking a feature document$`, outputFlagsSkillLackingFeature)
	ctx.Step(`^a package "([^"]*)" exists with Go source files$`, aPackageExistsWithGoSourceFiles)
	ctx.Step(`^no feature file references "([^"]*)" or its exported types$`, noFeatureFileReferencesPackage)
	ctx.Step(`^the output includes "([^"]*)" in the uncovered packages list$`, outputIncludesInUncoveredList)
	ctx.Step(`^the agent has invoked "([^"]*)" once and the output is recorded$`, agentHasInvokedOnce)
	ctx.Step(`^the agent invokes "([^"]*)" again with no scope argument$`, agentInvokesAgainNoScope)
	ctx.Step(`^the second output is equivalent to the first$`, secondOutputEquivalentToFirst)
	ctx.Step(`^no new files are written to "([^"]*)" or "([^"]*)"$`, noNewFilesWrittenTo)

	// targeted invocation
	ctx.Step(`^the agent invokes "([^"]*)" with scope argument "([^"]*)"$`, agentInvokesWithScope)
	ctx.Step(`^a file is written at "([^"]*)"$`, aFileIsWrittenAt)
	ctx.Step(`^the file begins with a canon record comment containing today's date$`, fileHasCanonRecordComment)
	ctx.Step(`^the file contains exactly one "([^"]*)" block$`, fileContainsExactlyOneBlock)
	ctx.Step(`^the agent writes a feature file for scope "([^"]*)"$`, agentWritesFeatureForScope)
	ctx.Step(`^the feature file contains "@([^"]*)" as a tag$`, featureFileContainsTag)
	ctx.Step(`^source files exist in "([^"]*)"$`, sourceFilesExistIn)
	ctx.Step(`^the agent invokes "([^"]*)" with scope "([^"]*)"$`, agentInvokesWithScopeString)
	ctx.Step(`^the agent reads the Go source files in that directory$`, agentReadsGoSourceFiles)
	ctx.Step(`^the written scenarios reference exported types from those source files$`, writtenScenariosReferenceExportedTypes)
	ctx.Step(`^"([^"]*)" contains scenarios that touch the target domain$`, fileContainsScenariosForDomain)
	ctx.Step(`^the new feature file header includes a "see:" reference to "([^"]*)"$`, featureHeaderIncludesSeeReference)

	// step authoring rules
	ctx.Step(`^the agent produces a feature file for any scope$`, agentProducesFeatureFile)
	ctx.Step(`^every "([^"]*)" step starts with a verb in imperative form$`, everyStepStartsWithVerb)
	ctx.Step(`^no "([^"]*)" step begins with "([^"]*)" or "([^"]*)" as the first word$`, noStepBeginsWithWord)
	ctx.Step(`^no step text contains a hardcoded string literal where \{string\} could be used$`, noHardcodedStringLiterals)
	ctx.Step(`^no step text contains a hardcoded integer where \{int\} could be used$`, noHardcodedIntegerLiterals)
	ctx.Step(`^no two step texts express the same action with different wording$`, noSynonymousSteps)
	ctx.Step(`^no step text uses "([^"]*)" where another uses "([^"]*)" for the same concept$`, noSynonymVerbs)
	ctx.Step(`^the agent produces a feature file containing a Scenario Outline$`, agentProducesScenarioOutline)
	ctx.Step(`^an "([^"]*)" block immediately follows each Scenario Outline$`, examplesBlockFollowsScenarioOutline)
	ctx.Step(`^the Examples table has a header row with column names$`, examplesTableHasHeaderRow)

	// drift audit
	ctx.Step(`^a feature file "([^"]*)" and source code in "([^"]*)"$`, aFeatureFileAndSourceCode)
	ctx.Step(`^the agent performs a drift audit on scope "([^"]*)"$`, agentPerformsDriftAuditOnScope)
	ctx.Step(`^the audit output contains a drift table with columns: Scenario, Feature, Code, Tests, Verdict$`, auditOutputContainsDriftTable)
	ctx.Step(`^"([^"]*)" references a function "([^"]*)" that does not exist in source$`, featureReferencesNonexistentFunction)
	ctx.Step(`^the Verdict for that scenario is "([^"]*)"$`, verdictForScenarioIs)
	ctx.Step(`^the audit suggests updating the feature or the source to reconcile$`, auditSuggestsReconciliation)
	ctx.Step(`^"([^"]*)" tests a behaviour not covered by any scenario$`, testFileTestsUncoveredBehaviour)
	ctx.Step(`^the audit flags the uncovered behaviour as a gap$`, auditFlagsUncoveredBehaviour)
	ctx.Step(`^a drift audit finds (\d+) mismatches$`, driftAuditFindsNMismatches)
	ctx.Step(`^the output contains (\d+) "([^"]*)" entries suitable for appending to TASKS\.md$`, outputContainsNEntries)

	// step definition implementation audit
	ctx.Step(`^step definition files exist in "([^"]*)"$`, stepDefinitionFilesExistIn)
	ctx.Step(`^the output includes a step-definition implementation summary$`, outputIncludesStepDefSummary)
	ctx.Step(`^each step definition file is classified as "([^"]*)", "([^"]*)", or "([^"]*)"$`, eachStepDefFileIsClassified)
	ctx.Step(`^a step definition file "([^"]*)" exists$`, aStepDefinitionFileExists)
	ctx.Step(`^every step function in that file returns godog\.ErrPending$`, everyStepFunctionReturnsPending)
	ctx.Step(`^the agent performs an audit pass$`, theAgentPerformsAuditPass)
	ctx.Step(`^the output contains a THESIS-FINDING entry for "([^"]*)"$`, outputContainsThesisFindingFor)
	ctx.Step(`^the THESIS-FINDING notes that Cucumber coverage is nominal, not executable$`, thesisFindingNotesCoverageNominal)
	ctx.Step(`^some step functions are implemented and some return godog\.ErrPending$`, someStepFunctionsImplementedSomeStubbed)
	ctx.Step(`^the THESIS-FINDING entry classifies the file as "([^"]*)"$`, thesisFindingClassifiesFileAs)
	ctx.Step(`^no step function returns godog\.ErrPending$`, noStepFunctionReturnsPending)
	ctx.Step(`^no THESIS-FINDING is recorded for "([^"]*)"$`, noThesisFindingRecordedFor)
	ctx.Step(`^the audit finds (\d+) stubbed step definition files$`, auditFindsNStubbedFiles)
	ctx.Step(`^the agent writes the audit output$`, theAgentWritesAuditOutput)
	ctx.Step(`^"([^"]*)" contains a "([^"]*)" section$`, fileContainsSection)
	ctx.Step(`^each stubbed file has a corresponding entry in that section$`, eachStubbedFileHasEntry)
	ctx.Step(`^no "([^"]*)" directory exists$`, noDirectoryExists)
	ctx.Step(`^the output notes that no step definition stubs exist$`, outputNotesNoStepDefStubs)
	ctx.Step(`^the output proposes scaffolding stubs for the highest-priority domain$`, outputProposesScaffolding)
	ctx.Step(`^the audit does not fail or exit with an error$`, auditDoesNotFail)

	// output location rules
	ctx.Step(`^the agent writes output for scope "([^"]*)"$`, agentWritesOutputForScope)
	ctx.Step(`^the output file path is "([^"]*)"$`, outputFilePathIs)
	ctx.Step(`^no files are written outside the "([^"]*)" directory$`, noFilesWrittenOutside)
	ctx.Step(`^the agent invokes "([^"]*)" with scope "([^"]*)" and no stub request$`, agentInvokesWithScopeNoStub)
	ctx.Step(`^no file is written to "([^"]*)"$`, noFileWrittenTo)
	ctx.Step(`^the agent invokes "([^"]*)" with scope "([^"]*)" and stub generation requested$`, agentInvokesWithScopeAndStubRequest)
	ctx.Step(`^a stub file is written at "([^"]*)"$`, aStubFileIsWrittenAt)
	ctx.Step(`^each step in the feature file has a corresponding stub function$`, eachStepHasCorrespondingStub)

	// federated context
	ctx.Step(`^the local instance is operating within a federation mesh$`, localInstanceOperatingInFederationMesh)
	ctx.Step(`^the output identifies the local instance by its instance ID and role$`, outputIdentifiesLocalInstanceByIDAndRole)
	ctx.Step(`^the output notes which feature files exist locally on this instance$`, outputNotesLocalFeatureFiles)
	ctx.Step(`^instance "([^"]*)" \(id "([^"]*)"\) is a registered peer$`, instanceIsRegisteredPeer)
	ctx.Step(`^the agent invokes "([^"]*)" with scope "([^"]*)"$`, agentInvokesWithFederatedScope)
	ctx.Step(`^the invocation is routed to inst-b$`, pickleJarInvocationRoutedToInstB)
	ctx.Step(`^the resulting audit output covers inst-b's local feature files and skill domain$`, auditCoversInstBLocalFiles)

	// ── additional patterns ─────────────────────────────────────────────────
	ctx.Step(`^it finds "([^"]*)"$`, itFindsFile)
	ctx.Step(`^it finds features/([^.]+)\.feature$`, itFindsFeaturesFile)
	ctx.Step(`^the agent invokes "([^"]*)" with no scope argument$`, agentInvokesNoScopeArg)
	ctx.Step(`^the scope analysis includes recommendations$`, theScopeAnalysisIncludesRecommendations)
	ctx.Step(`^the skill "([^"]*)" exists at "([^"]*)"$`, theSkillExistsAtPath)
	ctx.Step(`^no step text contains a hardcoded integer where a placeholder could be used$`, noStepTextContainsHardcodedInteger)
	ctx.Step(`^no step text contains a hardcoded string literal where a placeholder could be used$`, noStepTextContainsHardcodedStringLiteral)
}

func theSkillIsInstalledAt(name, path string) error                      { return godog.ErrPending }
func validateSkillFileCalledOn(path string) error                        { return godog.ErrPending }
func theResultIsValidWithNoErrors() error                                { return godog.ErrPending }
func skillNameMatchesDirectoryName(name string) error                    { return godog.ErrPending }
func theSkillSKILLMdIsRead(name string) error                            { return godog.ErrPending }
func descriptionFieldContainsWord(word string) error                     { return godog.ErrPending }
func agentInvokesSkillNoScope(skill string) error                        { return godog.ErrPending }
func outputListsFeatureFilesIn(dir string) error                         { return godog.ErrPending }
func outputIdentifiesUncoveredPackages() error                           { return godog.ErrPending }
func outputProposesNextFeature() error                                   { return godog.ErrPending }
func aSkillIsInstalledAt(name, path string) error                        { return godog.ErrPending }
func noFileExists(path string) error                                     { return godog.ErrPending }
func outputFlagsSkillLackingFeature(name string) error                   { return godog.ErrPending }
func aPackageExistsWithGoSourceFiles(pkg string) error                   { return godog.ErrPending }
func noFeatureFileReferencesPackage(pkg string) error                    { return godog.ErrPending }
func outputIncludesInUncoveredList(pkg string) error                     { return godog.ErrPending }
func agentHasInvokedOnce(skill string) error                             { return godog.ErrPending }
func agentInvokesAgainNoScope(skill string) error                        { return godog.ErrPending }
func secondOutputEquivalentToFirst() error                               { return godog.ErrPending }
func noNewFilesWrittenTo(dir1, dir2 string) error                        { return godog.ErrPending }
func agentInvokesWithScope(skill, scope string) error                    { return godog.ErrPending }
func aFileIsWrittenAt(path string) error                                 { return godog.ErrPending }
func fileHasCanonRecordComment() error                                   { return godog.ErrPending }
func fileContainsExactlyOneBlock(block string) error                     { return godog.ErrPending }
func agentWritesFeatureForScope(scope string) error                      { return godog.ErrPending }
func featureFileContainsTag(tag string) error                            { return godog.ErrPending }
func sourceFilesExistIn(dir string) error                                { return godog.ErrPending }
func agentInvokesWithScopeString(skill, scope string) error              { return godog.ErrPending }
func agentReadsGoSourceFiles() error                                     { return godog.ErrPending }
func writtenScenariosReferenceExportedTypes() error                      { return godog.ErrPending }
func fileContainsScenariosForDomain(file string) error                   { return godog.ErrPending }
func featureHeaderIncludesSeeReference(file string) error                { return godog.ErrPending }
func agentProducesFeatureFile() error                                    { return godog.ErrPending }
func everyStepStartsWithVerb(keyword string) error                       { return godog.ErrPending }
func noStepBeginsWithWord(keyword, w1, w2 string) error                  { return godog.ErrPending }
func noHardcodedStringLiterals() error                                   { return godog.ErrPending }
func noHardcodedIntegerLiterals() error                                  { return godog.ErrPending }
func noSynonymousSteps() error                                           { return godog.ErrPending }
func noSynonymVerbs(v1, v2 string) error                                 { return godog.ErrPending }
func agentProducesScenarioOutline() error                                { return godog.ErrPending }
func examplesBlockFollowsScenarioOutline(keyword string) error           { return godog.ErrPending }
func examplesTableHasHeaderRow() error                                   { return godog.ErrPending }
func aFeatureFileAndSourceCode(feature, src string) error                { return godog.ErrPending }
func agentPerformsDriftAuditOnScope(scope string) error                  { return godog.ErrPending }
func auditOutputContainsDriftTable() error                               { return godog.ErrPending }
func featureReferencesNonexistentFunction(feature, fn string) error      { return godog.ErrPending }
func verdictForScenarioIs(verdict string) error                          { return godog.ErrPending }
func auditSuggestsReconciliation() error                                 { return godog.ErrPending }
func testFileTestsUncoveredBehaviour(file string) error                  { return godog.ErrPending }
func auditFlagsUncoveredBehaviour() error                                { return godog.ErrPending }
func driftAuditFindsNMismatches(n int) error                             { return godog.ErrPending }
func outputContainsNEntries(n int, label string) error                   { return godog.ErrPending }
func stepDefinitionFilesExistIn(dir string) error                        { return godog.ErrPending }
func outputIncludesStepDefSummary() error                                { return godog.ErrPending }
func eachStepDefFileIsClassified(c1, c2, c3 string) error                { return godog.ErrPending }
func aStepDefinitionFileExists(path string) error                        { return godog.ErrPending }
func everyStepFunctionReturnsPending() error                             { return godog.ErrPending }
func theAgentPerformsAuditPass() error                                   { return godog.ErrPending }
func outputContainsThesisFindingFor(file string) error                   { return godog.ErrPending }
func thesisFindingNotesCoverageNominal() error                           { return godog.ErrPending }
func someStepFunctionsImplementedSomeStubbed() error                     { return godog.ErrPending }
func thesisFindingClassifiesFileAs(class string) error                   { return godog.ErrPending }
func noStepFunctionReturnsPending() error                                { return godog.ErrPending }
func noThesisFindingRecordedFor(file string) error                       { return godog.ErrPending }
func auditFindsNStubbedFiles(n int) error                                { return godog.ErrPending }
func theAgentWritesAuditOutput() error                                   { return godog.ErrPending }
func fileContainsSection(file, section string) error                     { return godog.ErrPending }
func eachStubbedFileHasEntry() error                                     { return godog.ErrPending }
func noDirectoryExists(dir string) error                                 { return godog.ErrPending }
func outputNotesNoStepDefStubs() error                                   { return godog.ErrPending }
func outputProposesScaffolding() error                                   { return godog.ErrPending }
func auditDoesNotFail() error                                            { return godog.ErrPending }
func agentWritesOutputForScope(scope string) error                       { return godog.ErrPending }
func outputFilePathIs(path string) error                                 { return godog.ErrPending }
func noFilesWrittenOutside(dir string) error                             { return godog.ErrPending }
func agentInvokesWithScopeNoStub(skill, scope string) error              { return godog.ErrPending }
func noFileWrittenTo(dir string) error                                   { return godog.ErrPending }
func agentInvokesWithScopeAndStubRequest(skill, scope string) error      { return godog.ErrPending }
func aStubFileIsWrittenAt(path string) error                             { return godog.ErrPending }
func eachStepHasCorrespondingStub() error                                { return godog.ErrPending }
func localInstanceOperatingInFederationMesh() error                      { return godog.ErrPending }
func outputIdentifiesLocalInstanceByIDAndRole() error                    { return godog.ErrPending }
func outputNotesLocalFeatureFiles() error                                { return godog.ErrPending }
func instanceIsRegisteredPeer(inst, id string) error                     { return godog.ErrPending }
func agentInvokesWithFederatedScope(skill, scope string) error           { return godog.ErrPending }
func pickleJarInvocationRoutedToInstB() error                            { return godog.ErrPending }
func auditCoversInstBLocalFiles() error                                  { return godog.ErrPending }

func itFindsFile(path string) error                                        { return godog.ErrPending }
func itFindsFeaturesFile(name string) error                                { return godog.ErrPending }
func agentInvokesNoScopeArg(skill string) error                            { return godog.ErrPending }
func theScopeAnalysisIncludesRecommendations() error                       { return godog.ErrPending }
func theSkillExistsAtPath(skill, path string) error                        { return godog.ErrPending }
func noStepTextContainsHardcodedInteger() error                            { return godog.ErrPending }
func noStepTextContainsHardcodedStringLiteral() error                      { return godog.ErrPending }
