package stepdefinitions

// config_to_simulation_steps.go — step definitions for features/config-to-simulation.feature
//
// Classification: stub — all steps return godog.ErrPending
//
// THESIS-FINDING: config_to_simulation_steps.go is stubbed (godog.ErrPending) —
// config-to-simulation skill has nominal, not executable Cucumber coverage.
//
// Registered steps (must not be re-registered in other domain files):
//   "the required file {string} does not exist"
//   "the output identifies the missing file {string}"
//   "the output suggests a remediation action"
//   "a valid config file {string} exists"
//   "the agent invokes {string} with config {string}"
//   "each target in the config is listed as a simulation participant"
//   "each participant has a join code"
//   "a config with poll_interval {string}"
//   "the agent invokes {string} with that config"
//   "the simulation escalation speed reflects the {string} poll interval"
//   "a config with federation.enabled true"
//   "the simulation scenario includes a federation topology description"
//   "the agent invokes {string} with config and voice {string}"
//   "the output uses urgent imperative language"
//   "the output contains a participant join-code table"
//   "a config with {string}"
//   "the agent invokes {string} with that config and voice {string}"
//   "the simulation is labelled as {string}"
//   "the output uses explanatory question-based language"
//   "the output contains at least one learning question"
//   "the output presents at least {int} named failure scenarios for the user to choose"
//   "the agent invokes {string} with config and no voice specified"
//   "the output prompts for voice selection"
//   "the output offers {string} and {string} as options"
//   "the output contains a warroom section"
//   "the output contains a mentor section"
//   "the detected scenario type is {string}"
//
// Steps delegated to other domain files (do not re-register here):
//   "the ocd-smoke-alarm binary is installed"              → engine_steps.go
//   "a Claude Code session is active in this repository"   → open_the_pickle_jar_steps.go
//   "the agent invokes the skill {string}"                 → open_the_pickle_jar_steps.go
//   "the skill {string} is read"                           → open_the_pickle_jar_steps.go
//   "the agent executes the documented steps in order"     → open_the_pickle_jar_steps.go
//   "the skill {string} has completed successfully once"   → start_here_steps.go
//   "the agent invokes the skill {string} again with identical inputs" → start_here_steps.go
//   "the output is equivalent to the first run"            → start_here_steps.go
//   "no duplicate state files are created under {string}"  → start_here_steps.go
//   "the output contains {string}"                         → warroom_simulator_steps.go

import (
	"github.com/cucumber/godog"
)

func InitializeConfigToSimulationSteps(ctx *godog.ScenarioContext) {
	// ── missing file handling ───────────────────────────────────────────────
	ctx.Step(`^the required file "([^"]*)" does not exist$`, theRequiredFileDoesNotExist)
	ctx.Step(`^the output identifies the missing file "([^"]*)"$`, theOutputIdentifiesMissingFile)
	ctx.Step(`^the output suggests a remediation action$`, theOutputSuggestsRemediationAction)

	// ── config analysis ─────────────────────────────────────────────────────
	ctx.Step(`^a valid config file "([^"]*)" exists$`, aValidConfigFileExists)
	ctx.Step(`^the agent invokes "([^"]*)" with config "([^"]*)"$`, theAgentInvokesWithConfig)
	ctx.Step(`^each target in the config is listed as a simulation participant$`, eachTargetIsListedAsSimulationParticipant)
	ctx.Step(`^each participant has a join code$`, eachParticipantHasAJoinCode)
	ctx.Step(`^a config with poll_interval "([^"]*)"$`, aConfigWithPollInterval)
	ctx.Step(`^the agent invokes "([^"]*)" with that config$`, theAgentInvokesWithThatConfig)
	ctx.Step(`^the simulation escalation speed reflects the "([^"]*)" poll interval$`, theSimulationEscalationSpeedReflects)
	ctx.Step(`^a config with federation\.enabled true$`, aConfigWithFederationEnabled)
	ctx.Step(`^the simulation scenario includes a federation topology description$`, theSimulationScenarioIncludesFederationTopology)

	// ── warroom voice ───────────────────────────────────────────────────────
	ctx.Step(`^the agent invokes "([^"]*)" with config and voice "([^"]*)"$`, theAgentInvokesWithConfigAndVoice)
	ctx.Step(`^the output uses urgent imperative language$`, theOutputUsesUrgentImperativeLanguage)
	ctx.Step(`^the output contains a participant join-code table$`, theOutputContainsParticipantJoinCodeTable)
	ctx.Step(`^a config with "([^"]*)"$`, aConfigWith)
	ctx.Step(`^the agent invokes "([^"]*)" with that config and voice "([^"]*)"$`, theAgentInvokesWithThatConfigAndVoice)
	ctx.Step(`^the simulation is labelled as "([^"]*)"$`, theSimulationIsLabelledAs)

	// ── mentor voice ────────────────────────────────────────────────────────
	ctx.Step(`^the output uses explanatory question-based language$`, theOutputUsesExplanatoryLanguage)
	ctx.Step(`^the output contains at least one learning question$`, theOutputContainsAtLeastOneLearningQuestion)
	ctx.Step(`^the output presents at least (\d+) named failure scenarios for the user to choose$`, theOutputPresentsAtLeastNFailureScenarios)

	// ── voice selection ─────────────────────────────────────────────────────
	ctx.Step(`^the agent invokes "([^"]*)" with config and no voice specified$`, theAgentInvokesWithConfigAndNoVoice)
	ctx.Step(`^the agent invokes "([^"]*)" with that config and no voice specified$`, theAgentInvokesWithThatConfigAndNoVoiceSpecified)
	ctx.Step(`^the output prompts for voice selection$`, theOutputPromptsForVoiceSelection)
	ctx.Step(`^the output offers "([^"]*)" and "([^"]*)" as options$`, theOutputOffersOptions)
	ctx.Step(`^the output contains a warroom section$`, theOutputContainsWarroomSection)
	ctx.Step(`^the output contains a mentor section$`, theOutputContainsMentorSection)

	// ── auto-detection ──────────────────────────────────────────────────────
	ctx.Step(`^the detected scenario type is "([^"]*)"$`, theDetectedScenarioTypeIs)
}

// ── stub implementations ───────────────────────────────────────────────────

func theRequiredFileDoesNotExist(path string) error                      { return godog.ErrPending }
func theOutputIdentifiesMissingFile(path string) error                   { return godog.ErrPending }
func theOutputSuggestsRemediationAction() error                          { return godog.ErrPending }
// aValidConfigFileExists — owned by common_steps.go
func theAgentInvokesWithConfig(skill, config string) error               { return godog.ErrPending }
func eachTargetIsListedAsSimulationParticipant() error                   { return godog.ErrPending }
func eachParticipantHasAJoinCode() error                                 { return godog.ErrPending }
func aConfigWithPollInterval(interval string) error                      { return godog.ErrPending }
func theAgentInvokesWithThatConfig(skill string) error                   { return godog.ErrPending }
func theSimulationEscalationSpeedReflects(interval string) error         { return godog.ErrPending }
func aConfigWithFederationEnabled() error                                { return godog.ErrPending }
func theSimulationScenarioIncludesFederationTopology() error             { return godog.ErrPending }
func theAgentInvokesWithConfigAndVoice(skill, voice string) error        { return godog.ErrPending }
func theOutputUsesUrgentImperativeLanguage() error                       { return godog.ErrPending }
func theOutputContainsParticipantJoinCodeTable() error                   { return godog.ErrPending }
func aConfigWith(field string) error                                     { return godog.ErrPending }
func theAgentInvokesWithThatConfigAndVoice(skill, voice string) error    { return godog.ErrPending }
func theSimulationIsLabelledAs(label string) error                       { return godog.ErrPending }
func theOutputUsesExplanatoryLanguage() error                            { return godog.ErrPending }
func theOutputContainsAtLeastOneLearningQuestion() error                 { return godog.ErrPending }
func theOutputPresentsAtLeastNFailureScenarios(n int) error              { return godog.ErrPending }
func theAgentInvokesWithConfigAndNoVoice(skill string) error             { return godog.ErrPending }
func theAgentInvokesWithThatConfigAndNoVoiceSpecified(skill string) error { return godog.ErrPending }
func theOutputPromptsForVoiceSelection() error                           { return godog.ErrPending }
func theOutputOffersOptions(opt1, opt2 string) error                     { return godog.ErrPending }
func theOutputContainsWarroomSection() error                             { return godog.ErrPending }
func theOutputContainsMentorSection() error                              { return godog.ErrPending }
func theDetectedScenarioTypeIs(scenario string) error                    { return godog.ErrPending }
