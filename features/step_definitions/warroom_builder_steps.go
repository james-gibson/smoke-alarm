package stepdefinitions

// warroom_builder_steps.go — step definitions for features/warroom-builder.feature
//
// Classification: stub — all steps return godog.ErrPending
//
// THESIS-FINDING: warroom_builder_steps.go is stubbed (godog.ErrPending) —
// warroom-builder skill has nominal, not executable Cucumber coverage.
//
// Registered steps (must not be re-registered in other domain files):
//   "no incident parameters have been provided"
//   "the output includes a questionnaire requesting incident type"
//   "the output lists {string} as an incident type option"
//   "the output includes severity options {string}, {string}, and {string}"
//   "incident parameters with type {string} and severity {string}"
//   "participants {string}, {string}, and {string} are required"
//   "the agent invokes the skill {string} with those parameters"
//   "a YAML config is generated with version {string}"
//   "the YAML config contains a {string} block with one entry per participant"
//   "the YAML config contains an {string} block with {string}"
//   "the generated YAML config has a poll_interval of {string}"
//   "the generated YAML config has {string} under known_state"
//   "incident parameters with severity {string}"
//   "an incident script is generated with a participant join-code table"
//   "the script contains an alarm sequence section"
//   "the skill {string} is invoked twice with identical parameters"
//   "the join codes in the second run differ from the first run"
//   "the output identifies the generated config path as starting with {string}"

import (
	"github.com/cucumber/godog"
)

func InitializeWarroomBuilderSteps(ctx *godog.ScenarioContext) {
	// ── parameter collection ───────────────────────────────────────────────
	ctx.Step(`^no incident parameters have been provided$`, noIncidentParametersProvided)
	ctx.Step(`^the output includes a questionnaire requesting incident type$`, theOutputIncludesIncidentTypeQuestionnaire)
	ctx.Step(`^the output lists "([^"]*)" as an incident type option$`, theOutputListsIncidentTypeOption)
	ctx.Step(`^the output includes severity options "([^"]*)", "([^"]*)", and "([^"]*)"$`, theOutputIncludesSeverityOptions)

	// ── YAML generation ────────────────────────────────────────────────────
	ctx.Step(`^incident parameters with type "([^"]*)" and severity "([^"]*)"$`, incidentParametersWithTypeAndSeverity)
	ctx.Step(`^participants "([^"]*)", "([^"]*)", and "([^"]*)" are required$`, participantsAreRequired)
	ctx.Step(`^the agent invokes the skill "([^"]*)" with those parameters$`, theAgentInvokesSkillWithParameters)
	ctx.Step(`^a YAML config is generated with version "([^"]*)"$`, aYAMLConfigIsGeneratedWithVersion)
	ctx.Step(`^the YAML config contains a "([^"]*)" block with one entry per participant$`, theYAMLConfigContainsBlock)
	ctx.Step(`^the YAML config contains an "([^"]*)" block with "([^"]*)"$`, theYAMLConfigContainsBlockWithField)
	ctx.Step(`^the generated YAML config has a poll_interval of "([^"]*)"$`, theGeneratedYAMLHasPollInterval)
	ctx.Step(`^the generated YAML config has "([^"]*)" under known_state$`, theGeneratedYAMLHasKnownStateField)
	ctx.Step(`^incident parameters with severity "([^"]*)"$`, incidentParametersWithSeverity)

	// ── script generation ──────────────────────────────────────────────────
	ctx.Step(`^an incident script is generated with a participant join-code table$`, anIncidentScriptIsGenerated)
	ctx.Step(`^the script contains an alarm sequence section$`, theScriptContainsAlarmSequence)

	// ── uniqueness ─────────────────────────────────────────────────────────
	ctx.Step(`^the skill "([^"]*)" is invoked twice with identical parameters$`, theSkillIsInvokedTwice)
	ctx.Step(`^the join codes in the second run differ from the first run$`, joinCodesInSecondRunDiffer)

	// ── output location ────────────────────────────────────────────────────
	ctx.Step(`^the output identifies the generated config path as starting with "([^"]*)"$`, theOutputIdentifiesGeneratedConfigPath)
}

// ── stub implementations ───────────────────────────────────────────────────

func noIncidentParametersProvided() error                             { return godog.ErrPending }
func theOutputIncludesIncidentTypeQuestionnaire() error               { return godog.ErrPending }
func theOutputListsIncidentTypeOption(option string) error            { return godog.ErrPending }
func theOutputIncludesSeverityOptions(sev1, sev2, sev3 string) error  { return godog.ErrPending }
func incidentParametersWithTypeAndSeverity(incType, sev string) error { return godog.ErrPending }
func participantsAreRequired(r1, r2, r3 string) error                 { return godog.ErrPending }
func theAgentInvokesSkillWithParameters(skill string) error           { return godog.ErrPending }
func aYAMLConfigIsGeneratedWithVersion(version string) error          { return godog.ErrPending }
func theYAMLConfigContainsBlock(block string) error                   { return godog.ErrPending }
func theYAMLConfigContainsBlockWithField(block, field string) error   { return godog.ErrPending }
func theGeneratedYAMLHasPollInterval(interval string) error           { return godog.ErrPending }
func theGeneratedYAMLHasKnownStateField(field string) error           { return godog.ErrPending }
func incidentParametersWithSeverity(sev string) error                 { return godog.ErrPending }
func anIncidentScriptIsGenerated() error                              { return godog.ErrPending }
func theScriptContainsAlarmSequence() error                           { return godog.ErrPending }
func theSkillIsInvokedTwice(skill string) error                       { return godog.ErrPending }
func joinCodesInSecondRunDiffer() error                               { return godog.ErrPending }
func theOutputIdentifiesGeneratedConfigPath(prefix string) error      { return godog.ErrPending }
