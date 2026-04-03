package stepdefinitions

// dynamic_config_steps.go — step definitions for features/dynamic-config.feature
//
// Approach: all assertions run in-process against dynamicconfig.Store.
// Discovery is static-only (cfg.Discovery.Enabled=false) to avoid network calls.
// AllowOverwrite is forced true in the test runner to prevent cross-scenario
// interference; the overwrite scenarios depend on shared pending steps anyway.
//
// State held in dcState (reset per scenario via BeforeScenario).
//
// Steps that depend on "{string} is true" / "{string} is false in config {string}"
// (owned by oauth_mock_steps.go / hosted_server_steps.go) remain ErrPending until
// those shared steps are implemented.

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
	"github.com/james-gibson/smoke-alarm/internal/config"
	"github.com/james-gibson/smoke-alarm/internal/discovery"
	"github.com/james-gibson/smoke-alarm/internal/dynamicconfig"
)

// dcState holds per-scenario dynamic-config state.
var dcState struct {
	artifacts         []dynamicconfig.SavedArtifact
	lastDirectory     string
	lastCfg           *config.Config
	cleanBeforeRun    bool
	overrideFormats   []string
	overrideDirectory string // set by theDynamicConfigDirectoryDoesNotExist
}

func resetDCState() {
	dcState.artifacts = nil
	dcState.lastDirectory = ""
	dcState.lastCfg = nil
	dcState.cleanBeforeRun = false
	dcState.overrideFormats = nil
	dcState.overrideDirectory = ""
}

func InitializeDynamicConfigSteps(ctx *godog.ScenarioContext) {
	ctx.BeforeScenario(func(_ *godog.Scenario) {
		resetDCState()
	})

	// ── persist subcommand ─────────────────────────────────────────────────
	ctx.Step(`^I run the "([^"]*)" subcommand with args "([^"]*)"$`, iRunSubcommandWithArgs)
	ctx.Step(`^the dynamic config directory "([^"]*)" is empty$`, theDynamicConfigDirectoryIsEmpty)
	ctx.Step(`^a JSON file exists under "([^"]*)"$`, aJSONFileExistsUnder)
	ctx.Step(`^a Markdown file exists under "([^"]*)"$`, aMarkdownFileExistsUnder)
	ctx.Step(`^a JSON file exists under the config's dynamic_config\.directory$`, aJSONFileExistsUnderConfigDir)

	// ── artifact uniqueness ────────────────────────────────────────────────
	ctx.Step(`^the dynamic config directory is empty$`, theDynamicConfigDirectoryIsEmptyNoPath)
	ctx.Step(`^all JSON artifacts under the output directory have distinct "([^"]*)" fields$`, allJSONArtifactsHaveDistinctField)
	ctx.Step(`^a JSON artifact with id "([^"]*)" already exists in the output directory$`, aJSONArtifactWithIDExists)

	// ── overwrite behaviour ────────────────────────────────────────────────
	ctx.Step(`^an artifact already exists at the expected output path$`, anArtifactAlreadyExistsAtExpectedPath)
	ctx.Step(`^the existing artifact is replaced with updated content$`, theExistingArtifactIsReplaced)

	// ── artifact content ───────────────────────────────────────────────────
	ctx.Step(`^each JSON artifact contains a "([^"]*)" field$`, eachJSONArtifactContainsField)
	ctx.Step(`^no JSON artifact contains a raw keychain secret$`, noJSONArtifactContainsRawSecret)
	ctx.Step(`^token placeholders match the pattern "([^"]*)"$`, tokenPlaceholdersMatchPattern)
	ctx.Step(`^client_secret placeholders match the pattern "([^"]*)"$`, clientSecretPlaceholdersMatchPattern)
	ctx.Step(`^each Markdown artifact contains at least one target ID as a heading$`, eachMarkdownContainsTargetIDHeading)
	ctx.Step(`^the Markdown is valid CommonMark$`, theMarkdownIsValidCommonMark)

	// ── output directory ───────────────────────────────────────────────────
	ctx.Step(`^the directory "([^"]*)" is created$`, theDirectoryIsCreated)

	// ── formats ────────────────────────────────────────────────────────────
	ctx.Step(`^a config with dynamic_config\.formats set to \["([^"]*)"\]$`, aConfigWithDynamicConfigFormats)
	ctx.Step(`^only "([^"]*)" format artifacts are written to the output directory$`, onlyFormatArtifactsAreWritten)

	// ── additional patterns ─────────────────────────────────────────────────
	ctx.Step(`^the dynamic config directory "([^"]*)" does not exist$`, theDynamicConfigDirectoryDoesNotExist)
}

// ── persist subcommand ──────────────────────────────────────────────────────

func iRunSubcommandWithArgs(subcmd, args string) error {
	if subcmd == "dynamic-config" && strings.Contains(args, "persist") {
		return runDynamicConfigPersist(args)
	}
	return godog.ErrPending
}

func runDynamicConfigPersist(args string) error {
	configPath := extractConfigFlag(args)
	if configPath == "" {
		return fmt.Errorf("--config flag not found in args %q", args)
	}
	abs := resolveConfigPath(configPath)
	cfg, err := config.Load(abs)
	if err != nil {
		cmdState.ran = true
		cmdState.exitCode = 1
		cmdState.stderr = err.Error()
		return nil
	}
	dcState.lastCfg = &cfg

	dc := cfg.DynamicConfig
	if dcState.overrideFormats != nil {
		dc.Formats = dcState.overrideFormats
	}
	// Force overwrite in tests to prevent cross-scenario interference.
	// Scenarios that test overwrite=false use shared pending steps, so they
	// won't reach this code path until those steps are implemented.
	dc.AllowOverwrite = true

	// Apply directory override (e.g. from theDynamicConfigDirectoryDoesNotExist).
	if dcState.overrideDirectory != "" {
		dc.Directory = dcState.overrideDirectory
	} else if !filepath.IsAbs(dc.Directory) {
		dc.Directory = filepath.Join(projectRoot, dc.Directory)
	}
	dcState.lastDirectory = dc.Directory

	if dcState.cleanBeforeRun {
		if err := cleanDirectory(dc.Directory); err != nil {
			return fmt.Errorf("clean directory %q: %w", dc.Directory, err)
		}
		dcState.cleanBeforeRun = false
	}

	// Static-only discovery: disable all dynamic sources to avoid network calls.
	staticCfg := cfg
	staticCfg.Discovery.Enabled = false
	records := discovery.New().Discover(context.Background(), staticCfg).Records

	store := dynamicconfig.NewStoreFromConfig(dc)
	artifacts, err := store.SaveDiscoveryRecords(context.Background(), records)
	cmdState.ran = true
	if err != nil {
		cmdState.exitCode = 1
		cmdState.stderr = err.Error()
		return nil
	}
	cmdState.exitCode = 0
	dcState.artifacts = artifacts
	return nil
}

func extractConfigFlag(args string) string {
	for _, part := range strings.Fields(args) {
		if strings.HasPrefix(part, "--config=") {
			return strings.TrimPrefix(part, "--config=")
		}
	}
	return ""
}

func theDynamicConfigDirectoryIsEmpty(path string) error {
	abs := path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(projectRoot, path)
	}
	return cleanDirectory(abs)
}

func theDynamicConfigDirectoryIsEmptyNoPath() error {
	// The config hasn't been loaded yet; set a flag so runDynamicConfigPersist
	// cleans the directory just before writing artifacts.
	dcState.cleanBeforeRun = true
	return nil
}

func theDynamicConfigDirectoryDoesNotExist(dir string) error {
	abs := dir
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(projectRoot, dir)
	}
	if err := os.RemoveAll(abs); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove %q: %w", abs, err)
	}
	// Override the output directory so the next persist run writes here,
	// allowing theDirectoryIsCreated to verify the same path.
	dcState.overrideDirectory = abs
	return nil
}

func cleanDirectory(abs string) error {
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.Remove(filepath.Join(abs, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// ── file existence assertions ───────────────────────────────────────────────

func aJSONFileExistsUnder(dir string) error {
	abs := dir
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(projectRoot, dir)
	}
	return assertFileWithExtExists(abs, ".json")
}

func aMarkdownFileExistsUnder(dir string) error {
	abs := dir
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(projectRoot, dir)
	}
	return assertFileWithExtExists(abs, ".md")
}

func aJSONFileExistsUnderConfigDir() error {
	if dcState.lastCfg == nil {
		return fmt.Errorf("no config loaded; run the persist subcommand first")
	}
	return assertFileWithExtExists(dcState.lastDirectory, ".json")
}

func assertFileWithExtExists(dir, ext string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read directory %q: %w", dir, err)
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ext) {
			return nil
		}
	}
	return fmt.Errorf("no %s file found in %q", ext, dir)
}

// ── artifact uniqueness ─────────────────────────────────────────────────────

func allJSONArtifactsHaveDistinctField(field string) error {
	entries, err := os.ReadDir(dcState.lastDirectory)
	if err != nil {
		return fmt.Errorf("read directory %q: %w", dcState.lastDirectory, err)
	}
	seen := map[string]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dcState.lastDirectory, e.Name())
		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %q: %w", path, err)
		}
		var obj map[string]any
		if err := json.Unmarshal(raw, &obj); err != nil {
			return fmt.Errorf("parse %q: %w", path, err)
		}
		val, ok := obj[field]
		if !ok {
			return fmt.Errorf("artifact %q missing field %q", e.Name(), field)
		}
		key := fmt.Sprintf("%v", val)
		if prev, dup := seen[key]; dup {
			return fmt.Errorf("duplicate %q value %q in %q and %q", field, key, prev, e.Name())
		}
		seen[key] = e.Name()
	}
	return nil
}

func aJSONArtifactWithIDExists(id string) error {
	// These scenarios also use "{string} is true" (require_unique_ids) which is a
	// shared pending step — they won't reach this point. Return ErrPending to be safe.
	return godog.ErrPending
}

// ── overwrite behaviour ─────────────────────────────────────────────────────

func anArtifactAlreadyExistsAtExpectedPath() error { return godog.ErrPending }
func theExistingArtifactIsReplaced() error         { return godog.ErrPending }

// ── artifact content ────────────────────────────────────────────────────────

func eachJSONArtifactContainsField(field string) error {
	if len(dcState.artifacts) == 0 {
		return fmt.Errorf("no artifacts in dcState; run persist first")
	}
	for _, a := range dcState.artifacts {
		if a.Format != dynamicconfig.FormatJSON {
			continue
		}
		raw, err := os.ReadFile(a.Path)
		if err != nil {
			return fmt.Errorf("read %q: %w", a.Path, err)
		}
		var obj map[string]any
		if err := json.Unmarshal(raw, &obj); err != nil {
			return fmt.Errorf("parse %q: %w", a.Path, err)
		}
		if _, ok := obj[field]; !ok {
			return fmt.Errorf("artifact %q missing field %q", a.Path, field)
		}
	}
	return nil
}

// noJSONArtifactContainsRawSecret, tokenPlaceholdersMatchPattern, and
// clientSecretPlaceholdersMatchPattern require secret masking in PersistedConfig
// (not yet implemented — no placeholder substitution in persistence.go).
func noJSONArtifactContainsRawSecret() error              { return godog.ErrPending }
func tokenPlaceholdersMatchPattern(pattern string) error  { return godog.ErrPending }
func clientSecretPlaceholdersMatchPattern(p string) error { return godog.ErrPending }

func eachMarkdownContainsTargetIDHeading() error {
	if len(dcState.artifacts) == 0 {
		return fmt.Errorf("no artifacts in dcState; run persist first")
	}
	for _, a := range dcState.artifacts {
		if a.Format != dynamicconfig.FormatMarkdown {
			continue
		}
		raw, err := os.ReadFile(a.Path)
		if err != nil {
			return fmt.Errorf("read %q: %w", a.Path, err)
		}
		content := string(raw)
		// RenderMarkdown writes "# Dynamic Config: {artifact.ID}" where artifact.ID
		// is derived from targetID (sanitized) + optional hash suffix. Check that
		// the target ID appears somewhere in a heading line.
		found := false
		for _, line := range strings.Split(content, "\n") {
			if strings.HasPrefix(line, "#") && strings.Contains(line, a.TargetID) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("markdown %q has no heading containing target ID %q", a.Path, a.TargetID)
		}
	}
	return nil
}

func theMarkdownIsValidCommonMark() error {
	if len(dcState.artifacts) == 0 {
		return fmt.Errorf("no artifacts in dcState; run persist first")
	}
	for _, a := range dcState.artifacts {
		if a.Format != dynamicconfig.FormatMarkdown {
			continue
		}
		raw, err := os.ReadFile(a.Path)
		if err != nil {
			return fmt.Errorf("read %q: %w", a.Path, err)
		}
		content := strings.TrimSpace(string(raw))
		if len(content) == 0 {
			return fmt.Errorf("markdown artifact %q is empty", a.Path)
		}
		// Minimal structural check: must have at least one ATX heading.
		hasHeading := false
		for _, line := range strings.Split(content, "\n") {
			if strings.HasPrefix(line, "#") {
				hasHeading = true
				break
			}
		}
		if !hasHeading {
			return fmt.Errorf("markdown artifact %q has no headings", a.Path)
		}
	}
	return nil
}

// ── output directory ────────────────────────────────────────────────────────

func theDirectoryIsCreated(path string) error {
	abs := path
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(projectRoot, path)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("directory %q not found: %w", abs, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%q exists but is not a directory", abs)
	}
	return nil
}

// ── formats ─────────────────────────────────────────────────────────────────

func aConfigWithDynamicConfigFormats(format string) error {
	dcState.overrideFormats = []string{format}
	return nil
}

func onlyFormatArtifactsAreWritten(format string) error {
	// Check the artifacts written in this scenario run (dcState.artifacts),
	// not the whole directory (which may have files from prior scenarios).
	if len(dcState.artifacts) == 0 {
		return fmt.Errorf("no artifacts written; run persist first")
	}
	want := format
	if format == "md" {
		want = dynamicconfig.FormatMarkdown
	}
	for _, a := range dcState.artifacts {
		if a.Format != want {
			return fmt.Errorf("artifact %q has format %q, expected only %q", filepath.Base(a.Path), a.Format, want)
		}
	}
	return nil
}
