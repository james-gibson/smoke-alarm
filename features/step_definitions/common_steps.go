package stepdefinitions

// common_steps.go — shared step definitions reused across all feature files.
// Steps registered here must not be re-registered in domain-specific files.
//
// Shared steps (registered once, used everywhere):
//   "the ocd-smoke-alarm binary is installed"
//   "a valid config file {string} exists"
//   "the exit code is {int}"
//   "the exit code is non-zero"
//   "stdout contains {string}"
//   "stderr contains {string}"
//   "a {string} log entry is written containing {string}"
//   "a GET request is sent to {string}"
//   "the response status code is {int}"
//   "the response body contains {string}"
//   "a Claude Code session is active in this repository"
//   "the agent invokes the skill {string}"
//   "the skill {string} is read"
//   "the agent executes the documented steps in order"

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/cucumber/godog"
)

// httpState holds the result of the most-recently executed HTTP request.
// Reset at the start of each scenario.
var httpState struct {
	lastResp *http.Response
	lastBody []byte
}

// projectRoot is the absolute path to the repository root, computed once.
var projectRoot = func() string {
	_, file, _, _ := runtime.Caller(0)
	// file is …/features/step_definitions/common_steps.go
	return filepath.Join(filepath.Dir(file), "..", "..")
}()

// cmdState holds the result of the most-recently executed subcommand.
// Reset at the start of each scenario via the Before hook in InitializeCommonSteps.
var cmdState struct {
	exitCode int
	stdout   string
	stderr   string
	ran      bool
}

func InitializeCommonSteps(ctx *godog.ScenarioContext) {
	ctx.BeforeScenario(func(_ *godog.Scenario) {
		cmdState.exitCode = 0
		cmdState.stdout = ""
		cmdState.stderr = ""
		cmdState.ran = false
		httpState.lastResp = nil
		httpState.lastBody = nil
	})

	ctx.Step(`^the ocd-smoke-alarm binary is installed$`, theOcdSmokeAlarmBinaryIsInstalled)
	ctx.Step(`^a valid config file "([^"]*)" exists$`, aValidConfigFileExists)
	ctx.Step(`^the exit code is (\d+)$`, theExitCodeIs)
	ctx.Step(`^the exit code is non-zero$`, theExitCodeIsNonZero)
	ctx.Step(`^stdout contains "([^"]*)"$`, stdoutContains)
	ctx.Step(`^stderr contains "([^"]*)"$`, stderrContains)
	ctx.Step(`^a "([^"]*)" log entry is written containing "([^"]*)"$`, aLogEntryWrittenContaining)
	ctx.Step(`^a GET request is sent to "([^"]*)"$`, aGETRequestSentTo)
	ctx.Step(`^the response status code is (\d+)$`, theResponseStatusCodeIs)
	ctx.Step(`^the response body contains "([^"]*)"$`, theResponseBodyContains)
	ctx.Step(`^a Claude Code session is active in this repository$`, aClaudeCodeSessionIsActive)

	// ── skill-agent contract (used by every skill feature) ──────────────────
	ctx.Step(`^the agent invokes the skill "([^"]*)"$`, theAgentInvokesTheSkill)
	ctx.Step(`^the skill "([^"]*)" is read$`, theSkillIsRead)
	ctx.Step(`^the agent executes the documented steps in order$`, theAgentExecutesDocumentedStepsInOrder)

	// ── shared target/probe steps ───────────────────────────────────────────
	ctx.Step(`^the target "([^"]*)" is enabled in config "([^"]*)"$`, theTargetIsEnabledInConfig)
	ctx.Step(`^the "([^"]*)" target is enabled in config "([^"]*)"$`, theTargetIsEnabledInConfig)
}

func theOcdSmokeAlarmBinaryIsInstalled() error {
	// Verify the binary can be built; skip actual build here — steps that run
	// subcommands use runSubcommand() which builds on demand.
	return nil
}

func aValidConfigFileExists(path string) error {
	abs := resolveConfigPath(path)
	if _, err := os.Stat(abs); err != nil {
		return fmt.Errorf("config file %q not found: %w", abs, err)
	}
	return nil
}

func theExitCodeIs(code int) error {
	if !cmdState.ran {
		return godog.ErrPending
	}
	if cmdState.exitCode != code {
		return fmt.Errorf("expected exit code %d, got %d\nstdout: %s\nstderr: %s",
			code, cmdState.exitCode, cmdState.stdout, cmdState.stderr)
	}
	return nil
}

func theExitCodeIsNonZero() error {
	if !cmdState.ran {
		return godog.ErrPending
	}
	if cmdState.exitCode == 0 {
		return fmt.Errorf("expected non-zero exit code\nstdout: %s\nstderr: %s",
			cmdState.stdout, cmdState.stderr)
	}
	return nil
}

func stdoutContains(s string) error {
	if !cmdState.ran {
		return godog.ErrPending
	}
	if !strings.Contains(cmdState.stdout, s) {
		return fmt.Errorf("stdout does not contain %q\nstdout: %s", s, cmdState.stdout)
	}
	return nil
}

func stderrContains(s string) error {
	if !cmdState.ran {
		return godog.ErrPending
	}
	if !strings.Contains(cmdState.stderr, s) {
		return fmt.Errorf("stderr does not contain %q\nstderr: %s", s, cmdState.stderr)
	}
	return nil
}

func aLogEntryWrittenContaining(level, msg string) error {
	return godog.ErrPending
}

func aGETRequestSentTo(rawURL string) error {
	url := rawURL
	if strings.HasPrefix(rawURL, "/") {
		if hsState.baseURL == "" {
			return fmt.Errorf("no health server running; cannot resolve relative URL %q", rawURL)
		}
		url = hsState.baseURL + rawURL
	} else {
		// Transparently remap hardcoded addresses when BindWithRetry bound to a
		// different port (e.g. config says :18088 but actual is :18089).
		for configured, actual := range hsState.addrRemap {
			if strings.Contains(url, configured) {
				url = strings.Replace(url, configured, actual, 1)
				break
			}
		}
	}
	// Use a 3-second timeout so SSE streams (which never close) don't block forever.
	// LimitReader caps body reads at 64KB — enough for JSON responses and the first
	// few SSE events, but prevents unbounded blocking on long-lived streams.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("build GET request for %q: %w", url, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil && resp == nil {
		return fmt.Errorf("GET %s: %w", url, err)
	}
	if resp != nil {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		_ = resp.Body.Close()
		httpState.lastResp = resp
		httpState.lastBody = body
	}
	return nil
}

func theResponseStatusCodeIs(code int) error {
	if httpState.lastResp == nil {
		return fmt.Errorf("no HTTP response yet")
	}
	if httpState.lastResp.StatusCode != code {
		return fmt.Errorf("expected status %d, got %d\nbody: %s",
			code, httpState.lastResp.StatusCode, httpState.lastBody)
	}
	return nil
}

func theResponseBodyContains(key string) error {
	if httpState.lastResp == nil {
		return fmt.Errorf("no HTTP response yet")
	}
	if !strings.Contains(string(httpState.lastBody), key) {
		return fmt.Errorf("response body does not contain %q\nbody: %s", key, httpState.lastBody)
	}
	return nil
}

func aClaudeCodeSessionIsActive() error {
	return nil // satisfied by the fact that this test is running inside Claude Code
}

func theAgentInvokesTheSkill(skill string) error          { return godog.ErrPending }
func theSkillIsRead(path string) error                    { return godog.ErrPending }
func theAgentExecutesDocumentedStepsInOrder() error       { return godog.ErrPending }
func theTargetIsEnabledInConfig(id, config string) error  { return godog.ErrPending }

// resolveConfigPath resolves a config path relative to the project root.
func resolveConfigPath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(projectRoot, path)
}

// runSubcommand builds the binary if needed and executes the given subcommand,
// storing the result in cmdState.
func runSubcommand(subcmd, configPath string) error {
	binPath := filepath.Join(projectRoot, "bin", "ocd-smoke-alarm")
	if _, err := os.Stat(binPath); err != nil {
		// Build the binary on demand.
		build := exec.Command("go", "build", "-o", binPath, "./cmd/ocd-smoke-alarm")
		build.Dir = projectRoot
		if out, err := build.CombinedOutput(); err != nil {
			return fmt.Errorf("build failed: %w\n%s", err, out)
		}
	}

	abs := resolveConfigPath(configPath)
	cmd := exec.Command(binPath, subcmd, "--config="+abs)
	cmd.Dir = projectRoot
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	runErr := cmd.Run()

	cmdState.ran = true
	cmdState.stdout = outBuf.String()
	cmdState.stderr = errBuf.String()
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			cmdState.exitCode = exitErr.ExitCode()
		} else {
			cmdState.exitCode = 1
		}
	} else {
		cmdState.exitCode = 0
	}
	return nil
}
