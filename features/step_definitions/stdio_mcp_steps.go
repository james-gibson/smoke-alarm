package stepdefinitions

// stdio_mcp_steps.go — step definitions for features/stdio-mcp.feature
//
// Classification: stub — all steps return godog.ErrPending
//
// THESIS-FINDING: stdio_mcp_steps.go is stubbed (godog.ErrPending) —
// stdio transport probe (npx/uvx process lifecycle) has nominal, not executable Cucumber coverage.
//
// Registered steps (must not be re-registered in other domain files):
//   "the {string} command is available on PATH"
//   "the target {string} is enabled in config {string}"
//   "the probe for target {string} runs"
//   "the probe for target {string} completes"
//   "a child process is spawned with command {string}"
//   "the process receives an MCP initialize request over stdin"
//   "the process responds with a valid MCP initialize response over stdout"
//   "no orphan child processes remain for target {string}"
//   "the target {string} is classified as {string}"
//   "the target {string} responds to all required methods"
//   "the target {string} does not respond to method {string}"
//   "the probe result contains {string}"
//   "the stdio command {string} is not available on PATH"
//   "the target {string} process exits with code {int} during handshake"
//   "the target {string} has timeout {string}"
//   "the spawned process does not respond within {int} seconds"
//   "the target {string} has env var {string} set to {string}"
//   "the probe for target {string} spawns the process"
//   "the child process environment contains {string}"
//   "the target {string} has cwd {string}"
//   "the child process working directory is the project root"
//   "each request written to stdin is one JSON object terminated by a newline"
//   "no {string} header is written to stdin"
//   "the target {string} replies with LSP {string} framing"
//   "the target {string} is launched through a wrapper that forks the server"
//   "the probe returns within its configured timeout"
//   "no descendant of the wrapper remains"
//   "the target {string} completes the handshake and then ignores stdin EOF"

import (
	"github.com/cucumber/godog"
)

func InitializeStdioMCPSteps(ctx *godog.ScenarioContext) {
	// ── setup ──────────────────────────────────────────────────────────────
	ctx.Step(`^the "([^"]*)" command is available on PATH$`, theCommandIsAvailableOnPATH)
	// theTargetIsEnabledInConfig — owned by common_steps.go

	// ── process lifecycle ──────────────────────────────────────────────────
	// theProbeForTargetRuns — owned by engine_steps.go
	// theProbeForTargetCompletes — owned by hosted_server_steps.go
	ctx.Step(`^a child process is spawned with command "([^"]*)"$`, aChildProcessIsSpawnedWithCommand)
	ctx.Step(`^the process receives an MCP initialize request over stdin$`, theProcessReceivesMCPInitializeOverStdin)
	ctx.Step(`^the process responds with a valid MCP initialize response over stdout$`, theProcessRespondsWithMCPInitialize)
	ctx.Step(`^no orphan child processes remain for target "([^"]*)"$`, noOrphanChildProcessesRemain)

	// ── classification ─────────────────────────────────────────────────────
	// theTargetIsClassifiedAs — owned by engine_steps.go
	ctx.Step(`^the target "([^"]*)" responds to all required methods$`, theTargetRespondsToAllRequiredMethods)
	ctx.Step(`^the target "([^"]*)" does not respond to method "([^"]*)"$`, theTargetDoesNotRespondToMethod)
	ctx.Step(`^the probe result contains "([^"]*)"$`, theProbeResultContains)

	// ── failure scenarios ──────────────────────────────────────────────────
	ctx.Step(`^the stdio command "([^"]*)" is not available on PATH$`, theStdioCommandIsNotAvailable)
	ctx.Step(`^the target "([^"]*)" process exits with code (\d+) during handshake$`, theTargetProcessExitsWithCode)
	ctx.Step(`^the target "([^"]*)" has timeout "([^"]*)"$`, theTargetHasTimeout)
	ctx.Step(`^the spawned process does not respond within (\d+) seconds$`, theSpawnedProcessDoesNotRespond)

	// ── env and cwd ────────────────────────────────────────────────────────
	ctx.Step(`^the target "([^"]*)" has env var "([^"]*)" set to "([^"]*)"$`, theTargetHasEnvVar)
	ctx.Step(`^the probe for target "([^"]*)" spawns the process$`, theProbeForTargetSpawnsProcess)
	ctx.Step(`^the child process environment contains "([^"]*)"$`, theChildProcessEnvironmentContains)
	ctx.Step(`^the target "([^"]*)" has cwd "([^"]*)"$`, theTargetHasCwd)
	ctx.Step(`^the child process working directory is the project root$`, theChildProcessCwdIsProjectRoot)

	// ── wire format (issue #43) ────────────────────────────────────────────
	ctx.Step(`^each request written to stdin is one JSON object terminated by a newline$`, eachStdinRequestIsNewlineDelimited)
	ctx.Step(`^no "([^"]*)" header is written to stdin$`, noHeaderIsWrittenToStdin)
	ctx.Step(`^the target "([^"]*)" replies with LSP "([^"]*)" framing$`, theTargetRepliesWithLSPFraming)

	// ── teardown (issue #44) ───────────────────────────────────────────────
	ctx.Step(`^the target "([^"]*)" is launched through a wrapper that forks the server$`, theTargetIsLaunchedThroughAWrapper)
	ctx.Step(`^the probe returns within its configured timeout$`, theProbeReturnsWithinItsTimeout)
	ctx.Step(`^no descendant of the wrapper remains$`, noDescendantOfTheWrapperRemains)
	ctx.Step(`^the target "([^"]*)" completes the handshake and then ignores stdin EOF$`, theTargetIgnoresStdinEOF)
}

// ── stub implementations ───────────────────────────────────────────────────

func theCommandIsAvailableOnPATH(cmd string) error { return godog.ErrPending }

// theTargetIsEnabledInConfig — owned by common_steps.go
// theProbeForTargetRuns — owned by engine_steps.go
// theProbeForTargetCompletes — owned by hosted_server_steps.go
func aChildProcessIsSpawnedWithCommand(cmd string) error { return godog.ErrPending }
func theProcessReceivesMCPInitializeOverStdin() error    { return godog.ErrPending }
func theProcessRespondsWithMCPInitialize() error         { return godog.ErrPending }
func noOrphanChildProcessesRemain(id string) error       { return godog.ErrPending }

// theTargetIsClassifiedAs — owned by engine_steps.go
func theTargetRespondsToAllRequiredMethods(id string) error   { return godog.ErrPending }
func theTargetDoesNotRespondToMethod(id, method string) error { return godog.ErrPending }
func theProbeResultContains(substr string) error              { return godog.ErrPending }
func theStdioCommandIsNotAvailable(cmd string) error          { return godog.ErrPending }
func theTargetProcessExitsWithCode(id string, code int) error { return godog.ErrPending }
func theTargetHasTimeout(id, timeout string) error            { return godog.ErrPending }
func theSpawnedProcessDoesNotRespond(seconds int) error       { return godog.ErrPending }
func theTargetHasEnvVar(id, key, val string) error            { return godog.ErrPending }
func theProbeForTargetSpawnsProcess(id string) error          { return godog.ErrPending }
func theChildProcessEnvironmentContains(entry string) error   { return godog.ErrPending }
func theTargetHasCwd(id, cwd string) error                    { return godog.ErrPending }
func theChildProcessCwdIsProjectRoot() error                  { return godog.ErrPending }

// ── wire format (issue #43) ────────────────────────────────────────────────
// Executable coverage for newline framing lives in
// internal/engine/stdio_prober_test.go and
// tests/integration/protocol_safety_test.go, which spawn the spec-framed mock
// (tests/testdata/mock_stdio_server) and its Content-Length-framed twin
// (tests/testdata/mock_lsp_stdio_server).
func eachStdinRequestIsNewlineDelimited() error         { return godog.ErrPending }
func noHeaderIsWrittenToStdin(header string) error      { return godog.ErrPending }
func theTargetRepliesWithLSPFraming(id, h string) error { return godog.ErrPending }

// ── teardown (issue #44) ───────────────────────────────────────────────────
// Executable coverage: tests/integration/stdio_teardown_test.go spawns a shell
// wrapper (`sh -c`) whose grandchild holds the pipes, plus
// tests/testdata/mock_stubborn_stdio_server for a server that ignores stdin EOF.
func theTargetIsLaunchedThroughAWrapper(id string) error { return godog.ErrPending }
func theProbeReturnsWithinItsTimeout() error             { return godog.ErrPending }
func noDescendantOfTheWrapperRemains() error             { return godog.ErrPending }
func theTargetIgnoresStdinEOF(id string) error           { return godog.ErrPending }
