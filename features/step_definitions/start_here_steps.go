package stepdefinitions

// start_here_steps.go — step definitions for features/start-here.feature
//
// Classification: stub — all steps return godog.ErrPending
//
// THESIS-FINDING: start_here_steps.go is stubbed (godog.ErrPending) —
// start-here welcome skill has nominal, not executable Cucumber coverage.
//
// Registered steps (must not be re-registered in other domain files):
//   "the skill {string} has completed successfully once"
//   "the agent invokes the skill {string} again with identical inputs"
//   "the output is equivalent to the first run"
//   "no duplicate state files are created under {string}"
//   "the output shows {string} is present"
//   "the output contains a skill inventory with at least {int} skill"
//   "the output lists {string} as an available skill"
//   "the agent invokes the skill {string}"
//   "the skill {string} is read" — alias registered by open_the_pickle_jar_steps.go; reuse

import (
	"github.com/cucumber/godog"
)

func InitializeStartHereSteps(ctx *godog.ScenarioContext) {
	// ── skill contract (shared pattern for all skill features) ─────────────
	ctx.Step(`^the skill "([^"]*)" has completed successfully once$`, theSkillHasCompletedSuccessfullyOnce)
	ctx.Step(`^the agent invokes the skill "([^"]*)" again with identical inputs$`, theAgentInvokesSkillAgain)
	ctx.Step(`^the output is equivalent to the first run$`, theOutputIsEquivalentToFirstRun)
	ctx.Step(`^no duplicate state files are created under "([^"]*)"$`, noDuplicateStateFilesCreated)

	// ── start-here specific ────────────────────────────────────────────────
	ctx.Step(`^the output shows "([^"]*)" is present$`, theOutputShowsFileIsPresent)
	ctx.Step(`^the output contains a skill inventory with at least (\d+) skill$`, theOutputContainsSkillInventory)
	ctx.Step(`^the output lists "([^"]*)" as an available skill$`, theOutputListsAsAvailableSkill)
}

// ── stub implementations ───────────────────────────────────────────────────

func theSkillHasCompletedSuccessfullyOnce(skill string) error             { return godog.ErrPending }
func theAgentInvokesSkillAgain(skill string) error                        { return godog.ErrPending }
func theOutputIsEquivalentToFirstRun() error                              { return godog.ErrPending }
func noDuplicateStateFilesCreated(dir string) error                       { return godog.ErrPending }
func theOutputShowsFileIsPresent(path string) error                       { return godog.ErrPending }
func theOutputContainsSkillInventory(n int) error                         { return godog.ErrPending }
func theOutputListsAsAvailableSkill(skill string) error                   { return godog.ErrPending }
