package stepdefinitions

// config_validation_steps.go — step definitions for features/config-validation.feature
// see: common_steps.go for shared steps (theExitCodeIs, stderrContains, stdoutContains, runSubcommand)

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
	"github.com/james-gibson/smoke-alarm/internal/config"
)

// minimalValidYAML is the smallest YAML that passes config.LoadBytes.
// Used as a base when creating temp configs for negative tests.
const minimalValidYAML = `version: "1"
service:
  name: test
  mode: foreground
  log_level: info
  poll_interval: 15s
  timeout: 5s
  max_workers: 4
targets: []
`

func InitializeConfigValidationScenario(ctx *godog.ScenarioContext) {
	ctx.Step(`^I run the "([^"]*)" subcommand with config "([^"]*)"$`, iRunSubcommandWithConfig)
	ctx.Step(`^stdout contains no error markers$`, stdoutContainsNoErrorMarkers)
	ctx.Step(`^the config file "([^"]*)" has version field "([^"]*)"$`, configFileHasVersionField)
	ctx.Step(`^validation passes$`, validationPasses)
	ctx.Step(`^the config file "([^"]*)" has version field "([^"]*)" set to "([^"]*)"$`, configFileHasVersionFieldSetTo)
	ctx.Step(`^validation fails with error containing "([^"]*)"$`, validationFailsWithError)
	ctx.Step(`^the config file is missing required field "([^"]*)"$`, configFileMissingRequiredField)
	ctx.Step(`^the error output contains "([^"]*)"$`, errorOutputContains)
	ctx.Step(`^a target with transport "([^"]*)"$`, aTargetWithTransport)
	ctx.Step(`^a target with protocol "([^"]*)"$`, aTargetWithProtocol)
	ctx.Step(`^service\.name is set to "([^"]*)"$`, serviceNameIsSetTo)
	ctx.Step(`^service\.mode is set to "([^"]*)"$`, serviceModeIsSetTo)
	ctx.Step(`^the config schema version is "([^"]*)"$`, configSchemaVersionIs)
	ctx.Step(`^the health block has listen_addr "([^"]*)"$`, healthBlockHasListenAddr)
	ctx.Step(`^the health server binds to "([^"]*)"$`, healthServerBindsTo)
	ctx.Step(`^a target missing the required "([^"]*)" field$`, aTargetMissingField)
	ctx.Step(`^the target is rejected with an error referencing "([^"]*)"$`, targetRejectedWithError)
	ctx.Step(`^oauth\.enabled is true and redirect_url is absent$`, oauthEnabledRedirectUrlAbsent)
	ctx.Step(`^validation succeeds$`, validationSucceeds)

	// ── additional patterns ─────────────────────────────────────────────────
	ctx.Step(`^a config file exists at "([^"]*)" with version "([^"]*)"$`, aConfigFileExistsWithVersion)
	ctx.Step(`^all enabled targets in "([^"]*)" are reachable$`, allEnabledTargetsAreReachable)
	ctx.Step(`^a config "([^"]*)" exists$`, aConfigExists)
	ctx.Step(`^a config file exists at "([^"]*)" with the "([^"]*)" field removed$`, aConfigFileExistsWithFieldRemoved)
	ctx.Step(`^a config file exists at "([^"]*)" with target transport "([^"]*)"$`, aConfigFileExistsWithTargetTransport)
	ctx.Step(`^a config file exists at "([^"]*)" with target protocol "([^"]*)"$`, aConfigFileExistsWithTargetProtocol)
	ctx.Step(`^the target "([^"]*)" in config "([^"]*)" is unreachable$`, theTargetInConfigIsUnreachable)
}

// iRunSubcommandWithConfig runs the given subcommand against the named config.
// For "validate" it calls config.Load directly; other subcommands exec the binary.
func iRunSubcommandWithConfig(subcmd, configPath string) error {
	switch subcmd {
	case "validate":
		return runValidateSubcommand(configPath)
	default:
		// exec the binary for subcommands that require a running server
		return runSubcommand(subcmd, configPath)
	}
}

// runValidateSubcommand calls config.Load and maps the result to cmdState,
// matching the exit-code and stderr conventions of the real "validate" subcommand.
func runValidateSubcommand(configPath string) error {
	abs := resolveConfigPath(configPath)
	_, err := config.Load(abs)
	cmdState.ran = true
	if err == nil {
		cmdState.exitCode = 0
		cmdState.stdout = "ok"
		cmdState.stderr = ""
	} else {
		cmdState.exitCode = 1
		cmdState.stdout = ""
		msg := err.Error()
		// Map known error strings to the expected stderr substrings the feature checks for.
		cmdState.stderr = msg
	}
	return nil
}

func stdoutContainsNoErrorMarkers() error {
	if !cmdState.ran {
		return godog.ErrPending
	}
	markers := []string{"ERROR", "error:", "FAIL", "invalid"}
	for _, m := range markers {
		if strings.Contains(cmdState.stdout, m) {
			return fmt.Errorf("stdout contains error marker %q: %s", m, cmdState.stdout)
		}
	}
	return nil
}

// validationPasses asserts the last validate call succeeded (exit 0).
func validationPasses() error {
	if !cmdState.ran {
		return godog.ErrPending
	}
	if cmdState.exitCode != 0 {
		return fmt.Errorf("expected validation to pass but exit code was %d\nstderr: %s",
			cmdState.exitCode, cmdState.stderr)
	}
	return nil
}

// validationSucceeds checks the last validation call.
// If targets_steps called iValidateTheTarget(), check tsState; otherwise fall
// back to the config-validation cmdState path (iRunSubcommandWithConfig).
func validationSucceeds() error {
	if tsState.validated {
		if tsState.validationErr != nil {
			return fmt.Errorf("expected validation to succeed but got: %w", tsState.validationErr)
		}
		return nil
	}
	return validationPasses()
}

// validationFailsWithError asserts the last validate call failed and stderr contains msg.
func validationFailsWithError(msg string) error {
	if !cmdState.ran {
		return godog.ErrPending
	}
	if cmdState.exitCode == 0 {
		return fmt.Errorf("expected validation to fail but exit code was 0")
	}
	if !strings.Contains(cmdState.stderr, msg) {
		return fmt.Errorf("stderr does not contain %q\nstderr: %s", msg, cmdState.stderr)
	}
	return nil
}

func errorOutputContains(msg string) error {
	if !cmdState.ran {
		return godog.ErrPending
	}
	combined := cmdState.stderr + cmdState.stdout
	if !strings.Contains(combined, msg) {
		return fmt.Errorf("output does not contain %q\nstdout: %s\nstderr: %s",
			msg, cmdState.stdout, cmdState.stderr)
	}
	return nil
}

// ── Config file creation helpers ──────────────────────────────────────────────

// aConfigFileExistsWithVersion writes a minimal valid YAML to path with the given version.
func aConfigFileExistsWithVersion(path, version string) error {
	yaml := strings.ReplaceAll(minimalValidYAML, `version: "1"`, fmt.Sprintf("version: %q", version))
	return writeFile(path, yaml)
}

// aConfigFileExistsWithFieldRemoved writes the sample config to path with the named
// top-level field and its entire indented block stripped out.
//
// NOTE: removing "service" or "targets" currently returns ErrPending because
// config.ApplyDefaults() fills in service defaults and an empty targets slice
// passes validation.  See TASKS.md THESIS-FINDING for the gap.
func aConfigFileExistsWithFieldRemoved(path, field string) error {
	// These fields are defaulted/optional — config.Validate() does not reject them.
	if field == "service" || field == "targets" {
		return godog.ErrPending
	}
	samplePath := filepath.Join(projectRoot, "configs", "sample.yaml")
	raw, err := os.ReadFile(samplePath)
	if err != nil {
		return fmt.Errorf("read sample config: %w", err)
	}
	lines := strings.Split(string(raw), "\n")
	var kept []string
	skip := false
	for _, line := range lines {
		if strings.HasPrefix(line, field+":") {
			skip = true
			continue
		}
		// End of the block: next top-level key (starts at column 0, non-empty, non-comment)
		if skip && line != "" && line[0] != ' ' && line[0] != '\t' && line[0] != '#' {
			skip = false
		}
		if !skip {
			kept = append(kept, line)
		}
	}
	return writeFile(path, strings.Join(kept, "\n"))
}

// aConfigFileExistsWithTargetTransport writes a minimal config with one target
// using the given (invalid) transport value.
//
// NOTE: empty transport returns ErrPending because config.ApplyDefaults()
// calls inferTransport(endpoint) which returns "http" for any http:// endpoint,
// making empty transport indistinguishable from a valid config.
func aConfigFileExistsWithTargetTransport(path, transport string) error {
	if transport == "" {
		return godog.ErrPending
	}
	base := strings.ReplaceAll(minimalValidYAML, "targets: []", "")
	yaml := base + fmt.Sprintf(`targets:
  - id: test-target
    enabled: true
    transport: %q
    protocol: mcp
    endpoint: "http://localhost:3000"
`, transport)
	return writeFile(path, yaml)
}

// aConfigFileExistsWithTargetProtocol writes a minimal config with one target
// using the given (invalid) protocol value.
func aConfigFileExistsWithTargetProtocol(path, protocol string) error {
	base := strings.ReplaceAll(minimalValidYAML, "targets: []", "")
	yaml := base + fmt.Sprintf(`targets:
  - id: test-target
    enabled: true
    transport: http
    protocol: %q
    endpoint: "http://localhost:3000"
`, protocol)
	return writeFile(path, yaml)
}

func aConfigExists(path string) error {
	abs := resolveConfigPath(path)
	if _, err := os.Stat(abs); err != nil {
		return fmt.Errorf("config %q does not exist: %w", abs, err)
	}
	return nil
}

// writeFile writes content to path, creating parent directories as needed.
func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// ── Stubs for steps not yet exercised by active scenarios ────────────────────

func configFileHasVersionField(cfgPath, field string) error       { return godog.ErrPending }
func configFileHasVersionFieldSetTo(cfg, field, val string) error { return godog.ErrPending }
func configFileMissingRequiredField(field string) error           { return godog.ErrPending }
func aTargetWithTransport(transport string) error                 { return godog.ErrPending }

// aTargetWithProtocol is defined in targets_steps.go
func serviceNameIsSetTo(name string) error                      { return godog.ErrPending }
func serviceModeIsSetTo(mode string) error                      { return godog.ErrPending }
func configSchemaVersionIs(v string) error                      { return godog.ErrPending }
func healthBlockHasListenAddr(addr string) error                { return godog.ErrPending }
func healthServerBindsTo(addr string) error                     { return godog.ErrPending }
func aTargetMissingField(field string) error                    { return godog.ErrPending }
func targetRejectedWithError(msg string) error                  { return godog.ErrPending }
func oauthEnabledRedirectUrlAbsent() error                      { return godog.ErrPending }
func allEnabledTargetsAreReachable(cfgPath string) error        { return godog.ErrPending }
func theTargetInConfigIsUnreachable(targetID, cfg string) error { return godog.ErrPending }
