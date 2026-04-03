package stepdefinitions

// warroom_simulator_steps.go — step definitions for features/warroom-simulator.feature
//
// Classification: stub — all steps return godog.ErrPending
//
// THESIS-FINDING: warroom_simulator_steps.go is stubbed (godog.ErrPending) —
// warroom-simulator interactive simulation skill has nominal, not executable Cucumber coverage.
//
// Registered steps (must not be re-registered in other domain files):
//   "a warroom-simulator session is configured with scenario {string} and severity {string}"
//   "the simulation is loaded but not yet started"
//   "the output contains {string}"
//   "the output contains a {string} prompt"
//   "a warroom-simulator session is configured and ready"
//   "the user issues {string}"
//   "the simulation status changes to {string}"
//   "the incident elapsed timer begins"
//   "participants are called with join codes"
//   "a warroom-simulator session is active with {string} code {string}"
//   "the participant {string} is marked as joined"
//   "the response time for {string} is recorded"
//   "a warroom-simulator session is active"
//   "the simulator responds with an invalid code message"
//   "no participant is marked as joined"
//   "a warroom-simulator session with P1 alarm {string} active"
//   "the alarm {string} is marked as resolved"
//   "P2 is unblocked"
//   "a warroom-simulator session with P1 and P2 alarms both active"
//   "the simulator rejects the command"
//   "the response states that P1 must be resolved first"
//   "the output includes elapsed time"
//   "the output includes a participant join table"
//   "the output includes an alarm status table"
//   "a warroom-simulator session with {string} join timeout of {int} seconds"
//   "{int} seconds pass without {string} joining"
//   "the simulator triggers an escalation"
//   "a new join code is issued for the escalated role"
//   "a warroom-simulator session with P1 alarm and {string} of {int} minutes"
//   "the simulator escalates to the next tier"
//   "the escalation event is recorded"
//   "a warroom-simulator session is active with an alarm timer running"
//   "alarm timers stop advancing"
//   "a warroom-simulator session is paused"
//   "alarm timers resume from their paused values"
//   "a warroom-simulator session where all alarms have been resolved"
//   "the simulator displays a performance summary"
//   "the summary includes time-to-first-participant"
//   "the summary includes improvement suggestions"
//   "the simulation ends immediately"
//   "a partial summary is displayed"
//
// Steps delegated to other domain files (do not re-register here):
//   "the ocd-smoke-alarm binary is installed"          → engine_steps.go
//   "a Claude Code session is active in this repository" → open_the_pickle_jar_steps.go
//   "the agent invokes the skill {string}"             → open_the_pickle_jar_steps.go
//   "the skill {string} is read"                       → open_the_pickle_jar_steps.go
//   "the agent executes the documented steps in order" → open_the_pickle_jar_steps.go
//   "the P1 alarm has been active for {int} minutes without resolution" → warroom_guide_steps.go
//   "the resolution time is recorded"                  → warroom_guide_steps.go
//   "the summary includes per-alarm resolution time"   → warroom_guide_steps.go

import (
	"github.com/cucumber/godog"
)

func InitializeWarroomSimulatorSteps(ctx *godog.ScenarioContext) {
	// ── simulation startup ──────────────────────────────────────────────────
	ctx.Step(`^a warroom-simulator session is configured with scenario "([^"]*)" and severity "([^"]*)"$`, aWarroomSimulatorSessionIsConfiguredWithScenarioAndSeverity)
	ctx.Step(`^the simulation is loaded but not yet started$`, theSimulationIsLoadedButNotStarted)
	ctx.Step(`^the output contains "([^"]*)"$`, theOutputContains)
	ctx.Step(`^the output contains a "([^"]*)" prompt$`, theOutputContainsPrompt)
	ctx.Step(`^a warroom-simulator session is configured and ready$`, aWarroomSimulatorSessionIsConfiguredAndReady)
	ctx.Step(`^the user issues "([^"]*)"$`, theUserIssuesCommand)
	ctx.Step(`^the simulation status changes to "([^"]*)"$`, theSimulationStatusChangesTo)
	ctx.Step(`^the incident elapsed timer begins$`, theIncidentElapsedTimerBegins)
	ctx.Step(`^participants are called with join codes$`, participantsAreCalledWithJoinCodes)

	// ── /join command ───────────────────────────────────────────────────────
	ctx.Step(`^a warroom-simulator session is active with "([^"]*)" code "([^"]*)"$`, aWarroomSimulatorSessionIsActiveWithCode)
	ctx.Step(`^the participant "([^"]*)" is marked as joined$`, theParticipantIsMarkedAsJoined)
	ctx.Step(`^the response time for "([^"]*)" is recorded$`, theResponseTimeIsRecordedFor)
	ctx.Step(`^a warroom-simulator session is active$`, aWarroomSimulatorSessionIsActive)
	ctx.Step(`^the simulator responds with an invalid code message$`, theSimulatorRespondsWithInvalidCodeMessage)
	ctx.Step(`^no participant is marked as joined$`, noParticipantIsMarkedAsJoined)

	// ── /resolve command ────────────────────────────────────────────────────
	ctx.Step(`^a warroom-simulator session with P1 alarm "([^"]*)" active$`, aWarroomSimulatorSessionWithP1AlarmActive)
	ctx.Step(`^the alarm "([^"]*)" is marked as resolved$`, theAlarmIsMarkedAsResolved)
	ctx.Step(`^P2 is unblocked$`, p2IsUnblocked)
	ctx.Step(`^a warroom-simulator session with P1 and P2 alarms both active$`, aWarroomSimulatorSessionWithP1AndP2BothActive)
	ctx.Step(`^the simulator rejects the command$`, theSimulatorRejectsTheCommand)
	ctx.Step(`^the response states that P1 must be resolved first$`, theResponseStatesThatP1MustBeResolvedFirst)

	// ── /status command ─────────────────────────────────────────────────────
	ctx.Step(`^the output includes elapsed time$`, theOutputIncludesElapsedTime)
	ctx.Step(`^the output includes a participant join table$`, theOutputIncludesParticipantJoinTable)
	ctx.Step(`^the output includes an alarm status table$`, theOutputIncludesAlarmStatusTable)

	// ── escalation ──────────────────────────────────────────────────────────
	ctx.Step(`^a warroom-simulator session with "([^"]*)" join timeout of (\d+) seconds$`, aWarroomSimulatorSessionWithJoinTimeout)
	ctx.Step(`^(\d+) seconds pass without "([^"]*)" joining$`, nSecondsPassWithoutJoining)
	ctx.Step(`^the simulator triggers an escalation$`, theSimulatorTriggersAnEscalation)
	ctx.Step(`^a new join code is issued for the escalated role$`, aNewJoinCodeIsIssuedForEscalatedRole)
	ctx.Step(`^a warroom-simulator session with P1 alarm and "([^"]*)" of (\d+) minutes$`, aWarroomSimulatorSessionWithP1AlarmAndDismissTimeout)
	ctx.Step(`^the simulator escalates to the next tier$`, theSimulatorEscalatesToNextTier)
	ctx.Step(`^the escalation event is recorded$`, theEscalationEventIsRecorded)

	// ── /pause and /resume ──────────────────────────────────────────────────
	ctx.Step(`^a warroom-simulator session is active with an alarm timer running$`, aWarroomSimulatorSessionIsActiveWithAlarmTimerRunning)
	ctx.Step(`^alarm timers stop advancing$`, alarmTimersStopAdvancing)
	ctx.Step(`^a warroom-simulator session is paused$`, aWarroomSimulatorSessionIsPaused)
	ctx.Step(`^alarm timers resume from their paused values$`, alarmTimersResumeFromPausedValues)

	// ── post-simulation feedback ────────────────────────────────────────────
	ctx.Step(`^a warroom-simulator session where all alarms have been resolved$`, aWarroomSimulatorSessionWhereAllAlarmsResolved)
	ctx.Step(`^the simulator displays a performance summary$`, theSimulatorDisplaysPerformanceSummary)
	ctx.Step(`^the summary includes time-to-first-participant$`, theSummaryIncludesTimeToFirstParticipant)
	ctx.Step(`^the summary includes improvement suggestions$`, theSummaryIncludesImprovementSuggestions)

	// ── /abort ──────────────────────────────────────────────────────────────
	ctx.Step(`^the simulation ends immediately$`, theSimulationEndsImmediately)
	ctx.Step(`^a partial summary is displayed$`, aPartialSummaryIsDisplayed)
}

// ── stub implementations ───────────────────────────────────────────────────

func aWarroomSimulatorSessionIsConfiguredWithScenarioAndSeverity(scenario, sev string) error {
	return godog.ErrPending
}
func theSimulationIsLoadedButNotStarted() error { return godog.ErrPending }

// theOutputContains — owned by skill_system_steps.go
func theOutputContainsPrompt(prompt string) error                            { return godog.ErrPending }
func aWarroomSimulatorSessionIsConfiguredAndReady() error                    { return godog.ErrPending }
func theUserIssuesCommand(cmd string) error                                  { return godog.ErrPending }
func theSimulationStatusChangesTo(status string) error                       { return godog.ErrPending }
func theIncidentElapsedTimerBegins() error                                   { return godog.ErrPending }
func participantsAreCalledWithJoinCodes() error                              { return godog.ErrPending }
func aWarroomSimulatorSessionIsActiveWithCode(role, code string) error       { return godog.ErrPending }
func theParticipantIsMarkedAsJoined(role string) error                       { return godog.ErrPending }
func theResponseTimeIsRecordedFor(role string) error                         { return godog.ErrPending }
func aWarroomSimulatorSessionIsActive() error                                { return godog.ErrPending }
func theSimulatorRespondsWithInvalidCodeMessage() error                      { return godog.ErrPending }
func noParticipantIsMarkedAsJoined() error                                   { return godog.ErrPending }
func aWarroomSimulatorSessionWithP1AlarmActive(alarmID string) error         { return godog.ErrPending }
func theAlarmIsMarkedAsResolved(alarmID string) error                        { return godog.ErrPending }
func p2IsUnblocked() error                                                   { return godog.ErrPending }
func aWarroomSimulatorSessionWithP1AndP2BothActive() error                   { return godog.ErrPending }
func theSimulatorRejectsTheCommand() error                                   { return godog.ErrPending }
func theResponseStatesThatP1MustBeResolvedFirst() error                      { return godog.ErrPending }
func theOutputIncludesElapsedTime() error                                    { return godog.ErrPending }
func theOutputIncludesParticipantJoinTable() error                           { return godog.ErrPending }
func theOutputIncludesAlarmStatusTable() error                               { return godog.ErrPending }
func aWarroomSimulatorSessionWithJoinTimeout(role string, seconds int) error { return godog.ErrPending }
func nSecondsPassWithoutJoining(seconds int, role string) error              { return godog.ErrPending }
func theSimulatorTriggersAnEscalation() error                                { return godog.ErrPending }
func aNewJoinCodeIsIssuedForEscalatedRole() error                            { return godog.ErrPending }
func aWarroomSimulatorSessionWithP1AlarmAndDismissTimeout(field string, minutes int) error {
	return godog.ErrPending
}
func theSimulatorEscalatesToNextTier() error                       { return godog.ErrPending }
func theEscalationEventIsRecorded() error                          { return godog.ErrPending }
func aWarroomSimulatorSessionIsActiveWithAlarmTimerRunning() error { return godog.ErrPending }
func alarmTimersStopAdvancing() error                              { return godog.ErrPending }
func aWarroomSimulatorSessionIsPaused() error                      { return godog.ErrPending }
func alarmTimersResumeFromPausedValues() error                     { return godog.ErrPending }
func aWarroomSimulatorSessionWhereAllAlarmsResolved() error        { return godog.ErrPending }
func theSimulatorDisplaysPerformanceSummary() error                { return godog.ErrPending }
func theSummaryIncludesTimeToFirstParticipant() error              { return godog.ErrPending }
func theSummaryIncludesImprovementSuggestions() error              { return godog.ErrPending }
func theSimulationEndsImmediately() error                          { return godog.ErrPending }
func aPartialSummaryIsDisplayed() error                            { return godog.ErrPending }
