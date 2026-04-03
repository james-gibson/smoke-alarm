package stepdefinitions

// ops_steps.go — step definitions for features/ops.feature
// see: common_steps.go for shared steps

import "github.com/cucumber/godog"

func InitializeOpsScenario(ctx *godog.ScenarioContext) {
	ctx.Step(`^a validated config loaded from "([^"]*)"$`, aValidatedConfigLoadedFrom)
	ctx.Step(`^NewRuntime is called$`, newRuntimeIsCalled)
	ctx.Step(`^the Runtime is initialized with the config's engine, health, and alert settings$`, runtimeIsInitialized)
	ctx.Step(`^a config with log_level "([^"]*)"$`, aConfigWithLogLevel)
	ctx.Step(`^the runtime logger uses that log level$`, runtimeLoggerUsesLogLevel)
	ctx.Step(`^a lock_file path configured as "([^"]*)"$`, aLockFilePathConfiguredAs)
	ctx.Step(`^Run is called$`, runIsCalled)
	ctx.Step(`^the lock file is created before the engine starts$`, lockFileCreatedBeforeEngine)
	ctx.Step(`^the lock file contains the current process PID$`, lockFileContainsPID)
	ctx.Step(`^a lock file at "([^"]*)" exists containing an active PID$`, lockFileExistsWithActivePID)
	ctx.Step(`^the error message contains "([^"]*)"$`, theErrorMessageContains)
	ctx.Step(`^the runtime exits without starting the engine$`, runtimeExitsWithoutEngine)
	ctx.Step(`^the runtime is running and holds the lock$`, runtimeIsRunningAndHoldsLock)
	ctx.Step(`^the context is cancelled$`, theContextIsCancelled)
	ctx.Step(`^the lock file is removed after shutdown completes$`, lockFileRemovedAfterShutdown)
	ctx.Step(`^a config with service\.mode "([^"]*)"$`, aConfigWithServiceMode)
	ctx.Step(`^the Bubble Tea dashboard is started$`, bubbleTeaDashboardIsStarted)
	ctx.Step(`^the process blocks until the dashboard quits$`, processBlocksUntilDashboardQuits)
	ctx.Step(`^no Bubble Tea dashboard is started$`, noBubbleTeaDashboardStarted)
	ctx.Step(`^the process blocks until the context is cancelled$`, processBlocksUntilContextCancelled)
	ctx.Step(`^a valid config with health\.enabled true$`, aValidConfigWithHealthEnabled)
	ctx.Step(`^bootstrap is called$`, bootstrapIsCalled)
	ctx.Step(`^the engine is running$`, theEngineIsRunning)
	ctx.Step(`^the health server is bound to the configured listen_addr$`, healthServerBoundToListenAddr)
	ctx.Step(`^the notifier is wired to the engine's event channel$`, notifierWiredToEngine)
	ctx.Step(`^the discovery subsystem is running if discovery\.enabled is true$`, discoveryRunningIfEnabled)
	ctx.Step(`^a config with discovery\.enabled false$`, aConfigWithDiscoveryDisabled)
	ctx.Step(`^the discovery subsystem is not started$`, discoverySubsystemNotStarted)
	ctx.Step(`^the runtime is running$`, theRuntimeIsRunning)
	ctx.Step(`^(\d+) seconds elapse$`, nSecondsElapse)
	ctx.Step(`^the health server snapshot is updated at least twice$`, healthServerSnapshotUpdatedTwice)
	ctx.Step(`^the snapshot reflects the engine's most recent target states$`, snapshotReflectsEngineTargetStates)
	ctx.Step(`^runtime\.graceful_shutdown_timeout is set to "([^"]*)"$`, gracefulShutdownTimeoutSetTo)
	ctx.Step(`^all subsystems stop within that timeout$`, allSubsystemsStopWithinTimeout)
	ctx.Step(`^no goroutine is left running after the timeout$`, noGoroutineLeftRunning)
	ctx.Step(`^a Plan with stop_command "([^"]*)", start_command "([^"]*)", and verify_command "([^"]*)"$`, aPlanWithCommands)
	ctx.Step(`^NewLifecycleController is called with that Plan$`, newLifecycleControllerCalled)
	ctx.Step(`^the controller holds the plan configuration$`, controllerHoldsPlanConfig)
	ctx.Step(`^a LifecycleController with valid stop, start, and verify commands$`, aLifecycleControllerWithValidCommands)
	ctx.Step(`^Execute is called$`, executeIsCalled)
	ctx.Step(`^the journal records a step entry for "([^"]*)"$`, journalRecordsStepEntry)
	ctx.Step(`^then a step entry for "([^"]*)"$`, thenAStepEntryFor)
	ctx.Step(`^the UpdateResult\.Success is true$`, updateResultSuccessIsTrue)
	ctx.Step(`^remote_agent\.safety\.require_lock is true$`, requireLockIsTrue)
	ctx.Step(`^the lock is held before stop_command runs$`, lockHeldBeforeStopCommand)
	ctx.Step(`^the lock is released after commit$`, lockReleasedAfterCommit)
	ctx.Step(`^remote_agent\.safety\.require_lock is false$`, requireLockIsFalse)
	ctx.Step(`^no lock file is written$`, noLockFileWritten)
	ctx.Step(`^the workflow proceeds directly to stop$`, workflowProceedsToStop)
	ctx.Step(`^the restart command succeeds$`, restartCommandSucceeds)
	ctx.Step(`^the instance becomes healthy after (\d+) poll attempts$`, instanceBecomesHealthyAfterPolls)
	ctx.Step(`^Execute reaches the verify step$`, executeReachesVerifyStep)
	ctx.Step(`^/healthz is polled until a 200 response is received$`, healthzPolledUntil200)
	ctx.Step(`^the verify_command runs only after /healthz returns healthy$`, verifyCommandRunsAfterHealthy)
	ctx.Step(`^remote_agent\.update\.max_wait_for_healthy is "([^"]*)"$`, maxWaitForHealthyIs)
	ctx.Step(`^the instance never becomes healthy$`, instanceNeverBecomesHealthy)
	ctx.Step(`^Execute runs the verify step$`, executeRunsVerifyStep)
	ctx.Step(`^the UpdateResult\.Success is false$`, updateResultSuccessIsFalse)
	ctx.Step(`^the failure reason contains "([^"]*)"$`, failureReasonContains)
	ctx.Step(`^the verify_command exits with a non-zero status$`, verifyCommandExitsNonZero)
	ctx.Step(`^remote_agent\.update\.rollback_on_failure is true$`, rollbackOnFailureIsTrue)
	ctx.Step(`^Execute runs$`, executeRuns)
	ctx.Step(`^the journal contains a "([^"]*)" step entry$`, journalContainsStepEntry)
	ctx.Step(`^the UpdateResult\.RolledBack is true$`, updateResultRolledBackIsTrue)
	ctx.Step(`^the verify_command fails$`, verifyCommandFails)
	ctx.Step(`^remote_agent\.update\.rollback_on_failure is false$`, rollbackOnFailureIsFalse)
	ctx.Step(`^no rollback step is recorded in the journal$`, noRollbackStepRecorded)
	ctx.Step(`^the previous version was running before the update began$`, previousVersionWasRunning)
	ctx.Step(`^rollback executes$`, rollbackExecutes)
	ctx.Step(`^the stop_command is run to stop the failed version$`, stopCommandRunsToStopFailed)
	ctx.Step(`^the start_command is run to restore the previous version$`, startCommandRunsToRestore)
	ctx.Step(`^Execute completes$`, executeCompletes)
	ctx.Step(`^each JournalEntry has a non-empty step field$`, eachJournalEntryHasStep)
	ctx.Step(`^each JournalEntry has an outcome of "([^"]*)" or "([^"]*)"$`, eachJournalEntryHasOutcome)
	ctx.Step(`^each JournalEntry has a valid RFC3339 timestamp$`, eachJournalEntryHasTimestamp)
	ctx.Step(`^each JournalEntry has a positive duration$`, eachJournalEntryHasDuration)
	ctx.Step(`^Execute completes$`, executeCompletesAlias)
	ctx.Step(`^the journal file exists on disk$`, journalFileExistsOnDisk)
	ctx.Step(`^the lock is still held at the moment the journal is written$`, lockStillHeldWhenJournalWritten)
	ctx.Step(`^the running instance reports version "([^"]*)" via /status$`, runningInstanceReportsVersion)
	ctx.Step(`^the update deploys version "([^"]*)"$`, updateDeploysVersion)
	ctx.Step(`^Execute completes successfully$`, executeCompletesSuccessfully)
	ctx.Step(`^the UpdateResult\.PreviousVersion is the first version$`, updateResultPreviousVersionIsFirst)
	ctx.Step(`^the UpdateResult\.NewVersion is the second version$`, updateResultNewVersionIsSecond)
	ctx.Step(`^a stop_command "([^"]*)"$`, aStopCommand)
	ctx.Step(`^the stop phase runs$`, theStopPhaseRuns)
	ctx.Step(`^the command is executed by the CommandRunner$`, commandExecutedByCommandRunner)
	ctx.Step(`^the exit code is captured in the journal$`, exitCodeCapturedInJournal)
	ctx.Step(`^a Plan with an empty stop_command$`, aPlanWithEmptyStopCommand)
	ctx.Step(`^Execute runs the stop phase$`, executeRunsStopPhase)
	ctx.Step(`^no command is executed$`, noCommandIsExecuted)
	ctx.Step(`^the journal records stop as "([^"]*)"$`, journalRecordsStopAs)
	ctx.Step(`^a Plan with an empty verify_command$`, aPlanWithEmptyVerifyCommand)
	ctx.Step(`^Execute runs the verify phase$`, executeRunsVerifyPhase)
	ctx.Step(`^the journal records verify as "([^"]*)"$`, journalRecordsVerifyAs)
	ctx.Step(`^health polling still occurs if a control_endpoint is configured$`, healthPollingStillOccurs)

	// ── additional patterns ─────────────────────────────────────────────────
	ctx.Step(`^the managed update completes$`, theManagedUpdateCompletes)
}

func aValidatedConfigLoadedFrom(path string) error                { return godog.ErrPending }
func newRuntimeIsCalled() error                                   { return godog.ErrPending }
func runtimeIsInitialized() error                                 { return godog.ErrPending }
func aConfigWithLogLevel(level string) error                      { return godog.ErrPending }
func runtimeLoggerUsesLogLevel() error                            { return godog.ErrPending }
func aLockFilePathConfiguredAs(path string) error                 { return godog.ErrPending }
func runIsCalled() error                                          { return godog.ErrPending }
func lockFileCreatedBeforeEngine() error                          { return godog.ErrPending }
func lockFileContainsPID() error                                  { return godog.ErrPending }
func lockFileExistsWithActivePID(path string) error               { return godog.ErrPending }
func theErrorMessageContains(msg string) error                    { return godog.ErrPending }
func runtimeExitsWithoutEngine() error                            { return godog.ErrPending }
func runtimeIsRunningAndHoldsLock() error                         { return godog.ErrPending }
func theContextIsCancelled() error                                { return godog.ErrPending }
func lockFileRemovedAfterShutdown() error                         { return godog.ErrPending }
func aConfigWithServiceMode(mode string) error                    { return godog.ErrPending }
func bubbleTeaDashboardIsStarted() error                          { return godog.ErrPending }
func processBlocksUntilDashboardQuits() error                     { return godog.ErrPending }
func noBubbleTeaDashboardStarted() error                          { return godog.ErrPending }
func processBlocksUntilContextCancelled() error                   { return godog.ErrPending }
func aValidConfigWithHealthEnabled() error                        { return godog.ErrPending }
func bootstrapIsCalled() error                                    { return godog.ErrPending }
func theEngineIsRunning() error                                   { return godog.ErrPending }
func healthServerBoundToListenAddr() error                        { return godog.ErrPending }
func notifierWiredToEngine() error                                { return godog.ErrPending }
func discoveryRunningIfEnabled() error                            { return godog.ErrPending }
func aConfigWithDiscoveryDisabled() error                         { return godog.ErrPending }
func discoverySubsystemNotStarted() error                         { return godog.ErrPending }
func theRuntimeIsRunning() error                                  { return godog.ErrPending }
func nSecondsElapse(n int) error                                  { return godog.ErrPending }
func healthServerSnapshotUpdatedTwice() error                     { return godog.ErrPending }
func snapshotReflectsEngineTargetStates() error                   { return godog.ErrPending }
func gracefulShutdownTimeoutSetTo(d string) error                 { return godog.ErrPending }
func allSubsystemsStopWithinTimeout() error                       { return godog.ErrPending }
func noGoroutineLeftRunning() error                               { return godog.ErrPending }
func aPlanWithCommands(stop, start, verify string) error          { return godog.ErrPending }
func newLifecycleControllerCalled() error                         { return godog.ErrPending }
func controllerHoldsPlanConfig() error                            { return godog.ErrPending }
func aLifecycleControllerWithValidCommands() error                { return godog.ErrPending }
func executeIsCalled() error                                      { return godog.ErrPending }
func journalRecordsStepEntry(step string) error                   { return godog.ErrPending }
func thenAStepEntryFor(step string) error                         { return godog.ErrPending }
func updateResultSuccessIsTrue() error                            { return godog.ErrPending }
func requireLockIsTrue() error                                    { return godog.ErrPending }
func lockHeldBeforeStopCommand() error                            { return godog.ErrPending }
func lockReleasedAfterCommit() error                              { return godog.ErrPending }
func requireLockIsFalse() error                                   { return godog.ErrPending }
func noLockFileWritten() error                                    { return godog.ErrPending }
func workflowProceedsToStop() error                               { return godog.ErrPending }
func restartCommandSucceeds() error                               { return godog.ErrPending }
func instanceBecomesHealthyAfterPolls(n int) error                { return godog.ErrPending }
func executeReachesVerifyStep() error                             { return godog.ErrPending }
func healthzPolledUntil200() error                                { return godog.ErrPending }
func verifyCommandRunsAfterHealthy() error                        { return godog.ErrPending }
func maxWaitForHealthyIs(d string) error                          { return godog.ErrPending }
func instanceNeverBecomesHealthy() error                          { return godog.ErrPending }
func executeRunsVerifyStep() error                                { return godog.ErrPending }
func updateResultSuccessIsFalse() error                           { return godog.ErrPending }
func failureReasonContains(msg string) error                      { return godog.ErrPending }
func verifyCommandExitsNonZero() error                            { return godog.ErrPending }
func rollbackOnFailureIsTrue() error                              { return godog.ErrPending }
func executeRuns() error                                          { return godog.ErrPending }
func journalContainsStepEntry(step string) error                  { return godog.ErrPending }
func updateResultRolledBackIsTrue() error                         { return godog.ErrPending }
func verifyCommandFails() error                                   { return godog.ErrPending }
func rollbackOnFailureIsFalse() error                             { return godog.ErrPending }
func noRollbackStepRecorded() error                               { return godog.ErrPending }
func previousVersionWasRunning() error                            { return godog.ErrPending }
func rollbackExecutes() error                                     { return godog.ErrPending }
func stopCommandRunsToStopFailed() error                          { return godog.ErrPending }
func startCommandRunsToRestore() error                            { return godog.ErrPending }
func executeCompletes() error                                     { return godog.ErrPending }
func eachJournalEntryHasStep() error                              { return godog.ErrPending }
func eachJournalEntryHasOutcome(o1, o2 string) error              { return godog.ErrPending }
func eachJournalEntryHasTimestamp() error                         { return godog.ErrPending }
func eachJournalEntryHasDuration() error                          { return godog.ErrPending }
func executeCompletesAlias() error                                { return godog.ErrPending }
func journalFileExistsOnDisk() error                              { return godog.ErrPending }
func lockStillHeldWhenJournalWritten() error                      { return godog.ErrPending }
func runningInstanceReportsVersion(v string) error                { return godog.ErrPending }
func updateDeploysVersion(v string) error                         { return godog.ErrPending }
func executeCompletesSuccessfully() error                         { return godog.ErrPending }
func updateResultPreviousVersionIsFirst() error                   { return godog.ErrPending }
func updateResultNewVersionIsSecond() error                       { return godog.ErrPending }
func aStopCommand(cmd string) error                               { return godog.ErrPending }
func theStopPhaseRuns() error                                     { return godog.ErrPending }
func commandExecutedByCommandRunner() error                       { return godog.ErrPending }
func exitCodeCapturedInJournal() error                            { return godog.ErrPending }
func aPlanWithEmptyStopCommand() error                            { return godog.ErrPending }
func executeRunsStopPhase() error                                 { return godog.ErrPending }
func noCommandIsExecuted() error                                  { return godog.ErrPending }
func journalRecordsStopAs(outcome string) error                   { return godog.ErrPending }
func aPlanWithEmptyVerifyCommand() error                          { return godog.ErrPending }
func executeRunsVerifyPhase() error                               { return godog.ErrPending }
func journalRecordsVerifyAs(outcome string) error                 { return godog.ErrPending }
func healthPollingStillOccurs() error                             { return godog.ErrPending }

func theManagedUpdateCompletes() error                            { return godog.ErrPending }
