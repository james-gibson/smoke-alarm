package stepdefinitions

// demo_capabilities_steps.go — step definitions for features/demo-capabilities.feature
//
// Classification: stub — all steps return godog.ErrPending
//
// THESIS-FINDING: demo_capabilities_steps.go is stubbed (godog.ErrPending) —
// demo-capabilities skill has nominal, not executable Cucumber coverage.
//
// Registered steps (must not be re-registered in other domain files):
//   "each subdirectory of {string} is scanned for a SKILL.md file"
//   "the output lists each discovered skill by name"
//   "the output contains a skill inventory table"
//   "each row in the inventory table includes a validation status"
//   "a SKILL.md at {string} with name and description fields"
//   "the skill {string} appears in the inventory as valid"
//   "a SKILL.md exists with a name field that does not match its directory"
//   "that skill appears in the inventory as invalid"
//   "the output identifies the validation failure reason"
//   "the output includes the presence status of {string}"

import (
	"github.com/cucumber/godog"
)

func InitializeDemoCapabilitiesSteps(ctx *godog.ScenarioContext) {
	// ── discovery ──────────────────────────────────────────────────────────
	ctx.Step(`^each subdirectory of "([^"]*)" is scanned for a SKILL\.md file$`, eachSubdirIsScanned)
	ctx.Step(`^the output lists each discovered skill by name$`, theOutputListsEachDiscoveredSkill)

	// ── validation ─────────────────────────────────────────────────────────
	ctx.Step(`^the output contains a skill inventory table$`, theOutputContainsSkillInventoryTable)
	ctx.Step(`^each row in the inventory table includes a validation status$`, eachRowIncludesValidationStatus)
	ctx.Step(`^a SKILL\.md at "([^"]*)" with name and description fields$`, aSKILLMdAtWithRequiredFields)
	ctx.Step(`^the skill "([^"]*)" appears in the inventory as valid$`, theSkillAppearsAsValid)
	ctx.Step(`^a SKILL\.md exists with a name field that does not match its directory$`, aSKILLMdWithMismatchedName)
	ctx.Step(`^that skill appears in the inventory as invalid$`, thatSkillAppearsAsInvalid)
	ctx.Step(`^the output identifies the validation failure reason$`, theOutputIdentifiesValidationFailureReason)

	// ── config files ───────────────────────────────────────────────────────
	ctx.Step(`^the output includes the presence status of "([^"]*)"$`, theOutputIncludesPresenceStatusOf)
}

// ── stub implementations ───────────────────────────────────────────────────

func eachSubdirIsScanned(dir string) error                                { return godog.ErrPending }
func theOutputListsEachDiscoveredSkill() error                            { return godog.ErrPending }
func theOutputContainsSkillInventoryTable() error                         { return godog.ErrPending }
func eachRowIncludesValidationStatus() error                              { return godog.ErrPending }
func aSKILLMdAtWithRequiredFields(path string) error                      { return godog.ErrPending }
func theSkillAppearsAsValid(skill string) error                           { return godog.ErrPending }
func aSKILLMdWithMismatchedName() error                                   { return godog.ErrPending }
func thatSkillAppearsAsInvalid() error                                    { return godog.ErrPending }
func theOutputIdentifiesValidationFailureReason() error                   { return godog.ErrPending }
func theOutputIncludesPresenceStatusOf(path string) error                 { return godog.ErrPending }
