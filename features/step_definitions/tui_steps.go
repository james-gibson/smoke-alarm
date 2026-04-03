package stepdefinitions

// tui_steps.go — step definitions for features/tui.feature
// see: common_steps.go for shared steps

import "github.com/cucumber/godog"

func InitializeTUIScenario(ctx *godog.ScenarioContext) {
	ctx.Step(`^a Dashboard model is constructed with NewDashboard$`, aDashboardModelConstructed)
	ctx.Step(`^Init is called$`, initIsCalled)
	ctx.Step(`^the returned tea\.Cmd contains a tickCmd$`, returnedCmdContainsTickCmd)
	ctx.Step(`^the returned tea\.Cmd contains a loadCmd$`, returnedCmdContainsLoadCmd)
	ctx.Step(`^a Dashboard model at initial state$`, aDashboardModelAtInitialState)
	ctx.Step(`^the model state is unchanged after Init returns$`, modelStateUnchangedAfterInit)
	ctx.Step(`^a Dashboard model is running$`, aDashboardModelIsRunning)
	ctx.Step(`^Update receives a tickMsg$`, updateReceivesTickMsg)
	ctx.Step(`^the returned tea\.Cmd is a loadCmd that will call SnapshotProvider\.Snapshot\(\)$`, returnedCmdIsLoadCmd)
	ctx.Step(`^a new tickCmd is scheduled for the next refresh_interval$`, newTickCmdScheduled)
	ctx.Step(`^the model has a known set of target states$`, modelHasKnownTargetStates)
	ctx.Step(`^the target states are unchanged until the loadMsg arrives$`, targetStatesUnchangedUntilLoadMsg)
	ctx.Step(`^a Dashboard model with a stale snapshot$`, aDashboardModelWithStaleSnapshot)
	ctx.Step(`^Update receives a loadMsg containing a fresher snapshot$`, updateReceivesFresherLoadMsg)
	ctx.Step(`^the model's snapshot is replaced with the loadMsg contents$`, modelSnapshotReplaced)
	ctx.Step(`^the model's last_updated time is updated$`, modelLastUpdatedTimeUpdated)
	ctx.Step(`^Update receives a loadMsg with (\d+) targets$`, updateReceivesLoadMsgWithTargets)
	ctx.Step(`^the model's target list is empty$`, modelTargetListIsEmpty)
	ctx.Step(`^View renders the status table section with no rows$`, viewRendersStatusTableNoRows)
	ctx.Step(`^Update receives a tea\.KeyMsg for key "([^"]*)"$`, updateReceivesKeyMsg)
	ctx.Step(`^the returned tea\.Cmd is tea\.Quit$`, returnedCmdIsTeaQuit)
	ctx.Step(`^the model has (\d+) targets and selected index (\d+)$`, modelHasTargetsAndSelectedIndex)
	ctx.Step(`^the selected index becomes (\d+)$`, selectedIndexBecomes)
	ctx.Step(`^the selected index remains (\d+)$`, selectedIndexRemains)
	ctx.Step(`^the events pane is hidden$`, eventsPaneIsHidden)
	ctx.Step(`^the events pane becomes visible$`, eventsPaneBecomesVisible)
	ctx.Step(`^the events pane is visible$`, eventsPaneIsVisible)
	ctx.Step(`^the events pane becomes hidden$`, eventsPaneBecomesHidden)
	ctx.Step(`^the current terminal width is (\d+) and height is (\d+)$`, currentTerminalSize)
	ctx.Step(`^Update receives a tea\.WindowSizeMsg with width (\d+) and height (\d+)$`, updateReceivesWindowSizeMsg)
	ctx.Step(`^the model width is (\d+)$`, modelWidthIs)
	ctx.Step(`^the model height is (\d+)$`, modelHeightIs)
	ctx.Step(`^the terminal is resized to width (\d+)$`, terminalResizedToWidth)
	ctx.Step(`^View is called$`, viewIsCalled)
	ctx.Step(`^the rendered output width does not exceed (\d+) characters per line$`, renderedOutputWidthDoesNotExceed)
	ctx.Step(`^two Dashboard models with identical state$`, twoDashboardModelsIdenticalState)
	ctx.Step(`^View is called on each$`, viewIsCalledOnEach)
	ctx.Step(`^both return identical output strings$`, bothReturnIdenticalOutput)
	ctx.Step(`^a Dashboard model with populated state$`, aDashboardModelWithPopulatedState)
	ctx.Step(`^no file reads, network calls, or goroutine launches occur during View$`, noSideEffectsDuringView)
	ctx.Step(`^the model snapshot contains (\d+) targets$`, modelSnapshotContainsTargets)
	ctx.Step(`^the status table section contains (\d+) data rows$`, statusTableContainsDataRows)
	ctx.Step(`^a snapshot with target id "([^"]*)", state "([^"]*)", and latency (\d+)ms$`, aSnapshotWithTargetDetails)
	ctx.Step(`^the status table row for that target contains the id, "([^"]*)", and "([^"]*)"$`, statusTableRowContains)
	ctx.Step(`^the second row has a different visual style than the first and third rows$`, selectedRowHasDifferentStyle)
	ctx.Step(`^the model has target "([^"]*)" selected$`, modelHasTargetSelected)
	ctx.Step(`^the details pane contains the target's endpoint, protocol, and transport$`, detailsPaneContainsDetails)
	ctx.Step(`^the details pane contains the target's last-checked timestamp$`, detailsPaneContainsTimestamp)
	ctx.Step(`^the model has (\d+) targets$`, modelHasTargets)
	ctx.Step(`^the details pane renders as empty or with a placeholder message$`, detailsPaneRendersEmpty)
	ctx.Step(`^the model snapshot contains (\d+) events$`, modelSnapshotContainsEvents)
	ctx.Step(`^the events section lists all (\d+) events with their timestamp and message$`, eventsSectionListsAllEvents)
	ctx.Step(`^the rendered output does not contain an events section$`, renderedOutputNoEventsSection)
	ctx.Step(`^Options\.MaxEvents is (\d+)$`, optionsMaxEventsIs)
	ctx.Step(`^the snapshot contains (\d+) events$`, snapshotContainsEvents)
	ctx.Step(`^only (\d+) events are rendered in the events pane$`, onlyNEventsRendered)
	ctx.Step(`^the snapshot contains (\d+) MCP targets and (\d+) ACP target$`, snapshotContainsMCPAndACP)
	ctx.Step(`^the topology pane shows "([^"]*)" and "([^"]*)"$`, topologyPaneShows)
	ctx.Step(`^a Dashboard constructed with Options\.Demo true$`, dashboardWithDemoTrue)
	ctx.Step(`^the rendered output contains an ASCII state machine diagram$`, renderedOutputContainsStateMachine)
	ctx.Step(`^a Dashboard constructed with Options\.Demo false$`, dashboardWithDemoFalse)
	ctx.Step(`^the rendered output does not contain a state machine diagram$`, renderedOutputNoStateMachine)
	ctx.Step(`^a mock SnapshotProvider that returns a fixed snapshot$`, aMockSnapshotProvider)
	ctx.Step(`^the Dashboard is constructed with that provider$`, dashboardConstructedWithProvider)
	ctx.Step(`^a loadMsg cycle runs$`, aLoadMsgCycleRuns)
	ctx.Step(`^the model snapshot matches the mock provider's output$`, modelSnapshotMatchesMockProvider)
	ctx.Step(`^a SnapshotProvider that returns nil$`, aSnapshotProviderReturningNil)
	ctx.Step(`^Update receives the resulting loadMsg$`, updateReceivesResultingLoadMsg)
	ctx.Step(`^the previous model snapshot is retained$`, previousModelSnapshotRetained)
	ctx.Step(`^no panic occurs$`, noPanicOccurs)
	ctx.Step(`^a Dashboard is started via Run$`, aDashboardStartedViaRun)
	ctx.Step(`^the user presses "([^"]*)"$`, theUserPresses)
	ctx.Step(`^Run returns without error$`, runReturnsWithoutError)
	ctx.Step(`^no TTY is available$`, noTTYIsAvailable)
	ctx.Step(`^Run is called$`, runIsCalledTUI)
	ctx.Step(`^Run returns a non-nil error$`, runReturnsNonNilError)

	// ── additional patterns ─────────────────────────────────────────────────
	ctx.Step(`^the model has (\d+) target and selected index (\d+)$`, theModelHasTargetAndSelectedIndex)
	ctx.Step(`^the TUI command contains "([^"]*)"$`, theTUICommandContains)
	ctx.Step(`^the TUI command contains the config path$`, theTUICommandContainsConfigPath)
	ctx.Step(`^the user issues the "([^"]*)" command$`, theUserIssuesTheCommand)
	ctx.Step(`^service\.mode is "([^"]*)" in the config$`, serviceModeIsInTheConfig)
}

func aDashboardModelConstructed() error                         { return godog.ErrPending }
func initIsCalled() error                                       { return godog.ErrPending }
func returnedCmdContainsTickCmd() error                         { return godog.ErrPending }
func returnedCmdContainsLoadCmd() error                         { return godog.ErrPending }
func aDashboardModelAtInitialState() error                      { return godog.ErrPending }
func modelStateUnchangedAfterInit() error                       { return godog.ErrPending }
func aDashboardModelIsRunning() error                           { return godog.ErrPending }
func updateReceivesTickMsg() error                              { return godog.ErrPending }
func returnedCmdIsLoadCmd() error                               { return godog.ErrPending }
func newTickCmdScheduled() error                                { return godog.ErrPending }
func modelHasKnownTargetStates() error                          { return godog.ErrPending }
func targetStatesUnchangedUntilLoadMsg() error                  { return godog.ErrPending }
func aDashboardModelWithStaleSnapshot() error                   { return godog.ErrPending }
func updateReceivesFresherLoadMsg() error                       { return godog.ErrPending }
func modelSnapshotReplaced() error                              { return godog.ErrPending }
func modelLastUpdatedTimeUpdated() error                        { return godog.ErrPending }
func updateReceivesLoadMsgWithTargets(n int) error              { return godog.ErrPending }
func modelTargetListIsEmpty() error                             { return godog.ErrPending }
func viewRendersStatusTableNoRows() error                       { return godog.ErrPending }
func updateReceivesKeyMsg(key string) error                     { return godog.ErrPending }
func returnedCmdIsTeaQuit() error                               { return godog.ErrPending }
func modelHasTargetsAndSelectedIndex(targets, index int) error  { return godog.ErrPending }
func selectedIndexBecomes(n int) error                          { return godog.ErrPending }
func selectedIndexRemains(n int) error                          { return godog.ErrPending }
func eventsPaneIsHidden() error                                 { return godog.ErrPending }
func eventsPaneBecomesVisible() error                           { return godog.ErrPending }
func eventsPaneIsVisible() error                                { return godog.ErrPending }
func eventsPaneBecomesHidden() error                            { return godog.ErrPending }
func currentTerminalSize(w, h int) error                        { return godog.ErrPending }
func updateReceivesWindowSizeMsg(w, h int) error                { return godog.ErrPending }
func modelWidthIs(n int) error                                  { return godog.ErrPending }
func modelHeightIs(n int) error                                 { return godog.ErrPending }
func terminalResizedToWidth(n int) error                        { return godog.ErrPending }
func viewIsCalled() error                                       { return godog.ErrPending }
func renderedOutputWidthDoesNotExceed(n int) error              { return godog.ErrPending }
func twoDashboardModelsIdenticalState() error                   { return godog.ErrPending }
func viewIsCalledOnEach() error                                 { return godog.ErrPending }
func bothReturnIdenticalOutput() error                          { return godog.ErrPending }
func aDashboardModelWithPopulatedState() error                  { return godog.ErrPending }
func noSideEffectsDuringView() error                            { return godog.ErrPending }
func modelSnapshotContainsTargets(n int) error                  { return godog.ErrPending }
func statusTableContainsDataRows(n int) error                   { return godog.ErrPending }
func aSnapshotWithTargetDetails(id, state string, ms int) error { return godog.ErrPending }
func statusTableRowContains(state, latency string) error        { return godog.ErrPending }
func selectedRowHasDifferentStyle() error                       { return godog.ErrPending }
func modelHasTargetSelected(id string) error                    { return godog.ErrPending }
func detailsPaneContainsDetails() error                         { return godog.ErrPending }
func detailsPaneContainsTimestamp() error                       { return godog.ErrPending }
func modelHasTargets(n int) error                               { return godog.ErrPending }
func detailsPaneRendersEmpty() error                            { return godog.ErrPending }
func modelSnapshotContainsEvents(n int) error                   { return godog.ErrPending }
func eventsSectionListsAllEvents(n int) error                   { return godog.ErrPending }
func renderedOutputNoEventsSection() error                      { return godog.ErrPending }
func optionsMaxEventsIs(n int) error                            { return godog.ErrPending }
func snapshotContainsEvents(n int) error                        { return godog.ErrPending }
func onlyNEventsRendered(n int) error                           { return godog.ErrPending }
func snapshotContainsMCPAndACP(mcp, acp int) error              { return godog.ErrPending }
func topologyPaneShows(s1, s2 string) error                     { return godog.ErrPending }
func dashboardWithDemoTrue() error                              { return godog.ErrPending }
func renderedOutputContainsStateMachine() error                 { return godog.ErrPending }
func dashboardWithDemoFalse() error                             { return godog.ErrPending }
func renderedOutputNoStateMachine() error                       { return godog.ErrPending }
func aMockSnapshotProvider() error                              { return godog.ErrPending }
func dashboardConstructedWithProvider() error                   { return godog.ErrPending }
func aLoadMsgCycleRuns() error                                  { return godog.ErrPending }
func modelSnapshotMatchesMockProvider() error                   { return godog.ErrPending }
func aSnapshotProviderReturningNil() error                      { return godog.ErrPending }
func updateReceivesResultingLoadMsg() error                     { return godog.ErrPending }
func previousModelSnapshotRetained() error                      { return godog.ErrPending }
func noPanicOccurs() error                                      { return godog.ErrPending }
func aDashboardStartedViaRun() error                            { return godog.ErrPending }
func theUserPresses(key string) error                           { return godog.ErrPending }
func runReturnsWithoutError() error                             { return godog.ErrPending }
func noTTYIsAvailable() error                                   { return godog.ErrPending }
func runIsCalledTUI() error                                     { return godog.ErrPending }
func runReturnsNonNilError() error                              { return godog.ErrPending }

func theModelHasTargetAndSelectedIndex(count, idx int) error { return godog.ErrPending }
func theTUICommandContains(s string) error                   { return godog.ErrPending }
func theTUICommandContainsConfigPath() error                 { return godog.ErrPending }
func theUserIssuesTheCommand(cmd string) error               { return godog.ErrPending }
func serviceModeIsInTheConfig(mode string) error             { return godog.ErrPending }
