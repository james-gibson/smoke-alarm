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
