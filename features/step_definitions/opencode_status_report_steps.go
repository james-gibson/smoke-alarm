package stepdefinitions

// opencode_status_report_steps.go — step definitions for features/opencode-status-report.feature
//
// Classification: stub — all steps return godog.ErrPending
//
// THESIS-FINDING: opencode_status_report_steps.go is stubbed (godog.ErrPending) —
// opencode-status-report skill has nominal, not executable Cucumber coverage.
//
// Registered steps (must not be re-registered in other domain files):
//   "{string} exists in the project root"
//   "sample config files exist under {string}"
//   "the report includes the status of {string}"
//   "the report lists each skill found under {string}"
//   "each skill entry includes its validation status"
//   "the report includes at least one sample config entry"
//   "the report shows {string} as valid"
//   "the report contains {string}"
//   "{string} does not exist in the project root"
//   "the report shows {string} as missing"
//   "the validation summary failed count is at least {int}"

import (
	"github.com/cucumber/godog"
)

func InitializeOpencodeStatusReportSteps(ctx *godog.ScenarioContext) {
	// ── file existence ─────────────────────────────────────────────────────
	ctx.Step(`^"([^"]*)" exists in the project root$`, fileExistsInProjectRoot)
	ctx.Step(`^"([^"]*)" does not exist in the project root$`, fileDoesNotExistInProjectRoot)
	ctx.Step(`^sample config files exist under "([^"]*)"$`, sampleConfigFilesExistUnder)

	// ── report content ─────────────────────────────────────────────────────
	ctx.Step(`^the report includes the status of "([^"]*)"$`, theReportIncludesStatusOf)
	ctx.Step(`^the report lists each skill found under "([^"]*)"$`, theReportListsEachSkillFoundUnder)
	ctx.Step(`^each skill entry includes its validation status$`, eachSkillEntryIncludesValidationStatus)
	ctx.Step(`^the report includes at least one sample config entry$`, theReportIncludesAtLeastOneSampleConfig)
	ctx.Step(`^the report shows "([^"]*)" as valid$`, theReportShowsAsValid)
	ctx.Step(`^the report contains "([^"]*)"$`, theReportContains)
	ctx.Step(`^the report shows "([^"]*)" as missing$`, theReportShowsAsMissing)
	ctx.Step(`^the validation summary failed count is at least (\d+)$`, theValidationSummaryFailedCountIsAtLeast)
}

// ── stub implementations ───────────────────────────────────────────────────

func fileExistsInProjectRoot(path string) error            { return godog.ErrPending }
func fileDoesNotExistInProjectRoot(path string) error      { return godog.ErrPending }
func sampleConfigFilesExistUnder(dir string) error         { return godog.ErrPending }
func theReportIncludesStatusOf(path string) error          { return godog.ErrPending }
func theReportListsEachSkillFoundUnder(dir string) error   { return godog.ErrPending }
func eachSkillEntryIncludesValidationStatus() error        { return godog.ErrPending }
func theReportIncludesAtLeastOneSampleConfig() error       { return godog.ErrPending }
func theReportShowsAsValid(path string) error              { return godog.ErrPending }
func theReportContains(text string) error                  { return godog.ErrPending }
func theReportShowsAsMissing(path string) error            { return godog.ErrPending }
func theValidationSummaryFailedCountIsAtLeast(n int) error { return godog.ErrPending }
