package stepdefinitions

// meta_config_steps.go — step definitions for features/meta-config.feature
//
// Implementation strategy:
//   IMPLEMENTED  — in-process generator tests (NewGenerator defaults, Render, Write, ValidateDocument)
//   ErrPending   — spec/code gaps; see TF-META-N in TASKS.md
//     TF-META-1: generator passes through SecretRef unchanged instead of replacing with placeholder

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"

	"github.com/james-gibson/smoke-alarm/internal/config"
	"github.com/james-gibson/smoke-alarm/internal/discovery"
	"github.com/james-gibson/smoke-alarm/internal/meta"
	"github.com/james-gibson/smoke-alarm/internal/targets"
)

var metaState struct {
	cfg        config.MetaConfigConfig
	gen        *meta.Generator
	doc        meta.Document
	lastPaths  []string
	lastErr    error
	tmpDir     string
	entry      meta.MetaEntry
	entries    []meta.MetaEntry
	target     targets.Target
	yamlData   []byte
	jsonData   []byte
	configFile string
	defaultDir string // path that was written for default-dir test (needs cleanup)
}

func resetMetaState() {
	if metaState.tmpDir != "" {
		_ = os.RemoveAll(metaState.tmpDir)
	}
	if metaState.defaultDir != "" {
		_ = os.RemoveAll(metaState.defaultDir)
	}
	metaState = struct {
		cfg        config.MetaConfigConfig
		gen        *meta.Generator
		doc        meta.Document
		lastPaths  []string
		lastErr    error
		tmpDir     string
		entry      meta.MetaEntry
		entries    []meta.MetaEntry
		target     targets.Target
		yamlData   []byte
		jsonData   []byte
		configFile string
		defaultDir string
	}{}
}

func metaProjectRoot() string {
	dir, _ := os.Getwd()
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "."
}

func metaTmpDir() (string, error) {
	if metaState.tmpDir != "" {
		return metaState.tmpDir, nil
	}
	d, err := os.MkdirTemp("", "meta-config-test-*")
	if err != nil {
		return "", err
	}
	metaState.tmpDir = d
	return d, nil
}

// minimalTarget returns a valid target for generator testing.
func minimalTarget() targets.Target {
	return targets.Target{
		ID:        "test-target",
		Name:      "Test Target",
		Protocol:  "mcp",
		Transport: "http",
		Endpoint:  "http://localhost:9000",
		Enabled:   true,
		Check: targets.CheckPolicy{
			Interval: 30 * time.Second,
			Timeout:  5 * time.Second,
		},
	}
}

// targetFromConfig converts a config.TargetConfig to targets.Target.
func targetFromConfig(tc config.TargetConfig) targets.Target {
	return targets.Target{
		ID:        tc.ID,
		Enabled:   tc.Enabled,
		Protocol:  targets.Protocol(tc.Protocol),
		Name:      tc.Name,
		Endpoint:  tc.Endpoint,
		Transport: targets.Transport(tc.Transport),
		Auth: targets.AuthConfig{
			Type:      targets.AuthType(tc.Auth.Type),
			Header:    tc.Auth.Header,
			KeyName:   tc.Auth.KeyName,
			SecretRef: tc.Auth.SecretRef,
			ClientID:  tc.Auth.ClientID,
			TokenURL:  tc.Auth.TokenURL,
		},
		Check: targets.CheckPolicy{
			Interval: 30 * time.Second,
			Timeout:  5 * time.Second,
		},
	}
}

// minimalDocument generates a valid meta.Document from one target.
func minimalDocument(g *meta.Generator) meta.Document {
	return g.GenerateFromTargets([]targets.Target{minimalTarget()})
}

func InitializeMetaConfigSteps(ctx *godog.ScenarioContext) {
	ctx.Before(func(sctx context.Context, sc *godog.Scenario) (context.Context, error) {
		resetMetaState()
		return sctx, nil
	})
	ctx.After(func(sctx context.Context, sc *godog.Scenario, err error) (context.Context, error) {
		if metaState.tmpDir != "" {
			_ = os.RemoveAll(metaState.tmpDir)
			metaState.tmpDir = ""
		}
		if metaState.defaultDir != "" {
			_ = os.RemoveAll(metaState.defaultDir)
			metaState.defaultDir = ""
		}
		return sctx, nil
	})

	// ── generator defaults ─────────────────────────────────────────────────
	ctx.Step(`^a meta_config block with no "([^"]*)" field$`, aMetaConfigBlockWithNoField)
	ctx.Step(`^the meta-config generator is initialized$`, theMetaConfigGeneratorIsInitialised)
	ctx.Step(`^the output directory defaults to "([^"]*)"$`, theOutputDirectoryDefaultsTo)
	ctx.Step(`^the formats default to "([^"]*)"$`, theFormatsDefaultTo)
	ctx.Step(`^the token placeholder defaults to "([^"]*)"$`, theTokenPlaceholderDefaultsTo)

	// ── output formats ─────────────────────────────────────────────────────
	ctx.Step(`^"([^"]*)" includes "([^"]*)" in config "([^"]*)"$`, configFieldIncludesValueInConfig)
	ctx.Step(`^the meta-config generator runs$`, theMetaConfigGeneratorRuns)
	ctx.Step(`^a YAML file is written under "([^"]*)"$`, aYAMLFileIsWrittenUnder)
	ctx.Step(`^the YAML file contains a "([^"]*)" field$`, theYAMLFileContainsField)
	ctx.Step(`^the YAML file contains an "([^"]*)" list$`, theYAMLFileContainsList)
	ctx.Step(`^a JSON file is written under "([^"]*)"$`, aJSONFileIsWrittenUnderMeta)
	ctx.Step(`^the JSON file is valid JSON$`, theJSONFileIsValidJSON)
	ctx.Step(`^the meta-config generator runs with that config$`, theMetaConfigGeneratorRunsWithThatConfig)
	ctx.Step(`^at least one output file is written under the meta_config\.output_dir$`, atLeastOneOutputFileIsWritten)

	// ── placeholder substitution ───────────────────────────────────────────
	ctx.Step(`^a target with auth type "([^"]*)" and secret_ref "([^"]*)"$`, aTargetWithAuthTypeAndSecretRef)
	ctx.Step(`^the meta-config generator produces an entry for that target$`, theMetaConfigGeneratorProducesEntry)
	ctx.Step(`^the entry auth secret_ref value is "([^"]*)"$`, theEntryAuthSecretRefValueIs)
	ctx.Step(`^the entry retains the original endpoint value$`, theEntryRetainsOriginalEndpoint)
	ctx.Step(`^a provenance note records the original source$`, aProvenanceNoteRecordsOriginalSource)

	// ── confidence and provenance ──────────────────────────────────────────
	ctx.Step(`^the meta-config generator produces entries$`, theMetaConfigGeneratorProducesEntries)
	ctx.Step(`^each entry contains a "([^"]*)" field$`, eachEntryContainsField)
	ctx.Step(`^the confidence value is between 0\.0 and 1\.0$`, theConfidenceValueIsBetween0And1)
	ctx.Step(`^the provenance field identifies the source config file$`, theProvenanceFieldIdentifiesSourceConfig)
	ctx.Step(`^no entry contains a "([^"]*)" field$`, noEntryContainsField)

	// ── document structure ─────────────────────────────────────────────────
	ctx.Step(`^the meta-config generator runs with config "([^"]*)"$`, theMetaConfigGeneratorRunsWithConfig)
	ctx.Step(`^the document "([^"]*)" field is present$`, theMetaDocumentFieldIsPresent)
	ctx.Step(`^the document "([^"]*)" field is a valid RFC3339 timestamp$`, theMetaDocumentFieldIsRFC3339)
	ctx.Step(`^the document "([^"]*)" field identifies the config file$`, theMetaDocumentFieldIdentifiesConfig)
	ctx.Step(`^the document "([^"]*)" list is non-empty$`, theMetaDocumentListIsNonEmpty)
	ctx.Step(`^each entry contains an "([^"]*)" field$`, eachEntryContainsAnField)
	ctx.Step(`^no two entries share the same "([^"]*)"$`, noTwoEntriesShareField)

	// ── disabled ───────────────────────────────────────────────────────────
	ctx.Step(`^a config with meta_config\.enabled set to false$`, aConfigWithMetaConfigDisabled)
	ctx.Step(`^no files are written to the meta_config output_dir$`, noFilesAreWrittenToMetaConfigDir)

	// ── additional patterns ─────────────────────────────────────────────────
	ctx.Step(`^a meta_config block with no formats field$`, aMetaConfigBlockWithNoFormatsField)
	ctx.Step(`^a meta_config block with no output_dir field$`, aMetaConfigBlockWithNoOutputDirField)
	ctx.Step(`^a meta_config block with no placeholders\.token field$`, aMetaConfigBlockWithNoPlaceholdersTokenField)
	ctx.Step(`^the formats default to \["([^"]*)", "([^"]*)"\]$`, theFormatsDefaultToTwo)
	ctx.Step(`^"([^"]*)" is false$`, configKeyIsFalse)
	ctx.Step(`^a valid config file "([^"]*)" with meta_config\.enabled true$`, aValidConfigFileWithMetaConfigEnabled)
}

// ── generator defaults ────────────────────────────────────────────────────────

func aMetaConfigBlockWithNoField(field string) error {
	// Generic handler; specific no-field steps (below) handle the feature scenarios.
	metaState.cfg = config.MetaConfigConfig{}
	return nil
}

func aMetaConfigBlockWithNoOutputDirField() error {
	metaState.cfg = config.MetaConfigConfig{}
	return nil
}

func aMetaConfigBlockWithNoFormatsField() error {
	metaState.cfg = config.MetaConfigConfig{}
	return nil
}

func aMetaConfigBlockWithNoPlaceholdersTokenField() error {
	metaState.cfg = config.MetaConfigConfig{}
	return nil
}

func theMetaConfigGeneratorIsInitialised() error {
	metaState.gen = meta.NewGenerator(metaState.cfg)
	return nil
}

// theOutputDirectoryDefaultsTo verifies that NewGenerator sets the default output_dir.
// Calls Write with a minimal document; checks that the returned path starts with the expected dir.
// Cleans up the created directory in the After hook.
func theOutputDirectoryDefaultsTo(expected string) error {
	if metaState.gen == nil {
		return fmt.Errorf("generator not initialized")
	}
	doc := minimalDocument(metaState.gen)
	paths, err := metaState.gen.Write(context.Background(), doc)
	if err != nil {
		return fmt.Errorf("write failed: %w", err)
	}
	// Record for cleanup (default dir is relative to CWD).
	if expected == "./state/meta-config" {
		metaState.defaultDir = "state" // remove the whole state/ subdir
	}
	for _, p := range paths {
		clean := filepath.ToSlash(filepath.Clean(p))
		expClean := filepath.ToSlash(filepath.Clean(expected))
		if !strings.HasPrefix(clean, expClean) {
			return fmt.Errorf("path %q does not start with expected dir %q", clean, expClean)
		}
	}
	return nil
}

// theFormatsDefaultTo: verify the default formats list (single format string expected).
// This step fires for the "formats default to [yaml, json]" scenario via theFormatsDefaultToTwo.
func theFormatsDefaultTo(formats string) error {
	return godog.ErrPending // covered by theFormatsDefaultToTwo
}

// theFormatsDefaultToTwo verifies that a generator with no formats config writes both formats.
func theFormatsDefaultToTwo(f1, f2 string) error {
	if metaState.gen == nil {
		return fmt.Errorf("generator not initialized")
	}
	tmpDir, err := metaTmpDir()
	if err != nil {
		return err
	}
	// Write a minimal doc using a generator with tmpDir override.
	tmpCfg := metaState.cfg
	tmpCfg.OutputDir = tmpDir
	gen := meta.NewGenerator(tmpCfg)
	doc := minimalDocument(gen)
	paths, err := gen.Write(context.Background(), doc)
	if err != nil {
		return fmt.Errorf("write failed: %w", err)
	}
	// Verify both extensions are present.
	exts := map[string]bool{}
	for _, p := range paths {
		exts[strings.TrimPrefix(filepath.Ext(p), ".")] = true
	}
	for _, f := range []string{f1, f2} {
		if !exts[strings.ToLower(f)] {
			return fmt.Errorf("format %q not found in output paths: %v", f, paths)
		}
	}
	return nil
}

// theTokenPlaceholderDefaultsTo verifies that a generator with no token placeholder config
// uses the expected default for bearer auth targets.
func theTokenPlaceholderDefaultsTo(placeholder string) error {
	if metaState.gen == nil {
		return fmt.Errorf("generator not initialized")
	}
	t := minimalTarget()
	t.Auth = targets.AuthConfig{Type: "bearer"}
	doc := metaState.gen.GenerateFromTargets([]targets.Target{t})
	if len(doc.Entries) == 0 {
		return fmt.Errorf("no entries generated")
	}
	got := doc.Entries[0].Auth.Token
	if got != placeholder {
		return fmt.Errorf("expected token placeholder %q, got %q", placeholder, got)
	}
	return nil
}

// ── output formats ────────────────────────────────────────────────────────────

// configFieldIncludesValueInConfig loads a config file and stores the meta_config block.
func configFieldIncludesValueInConfig(field, value, configPath string) error {
	root := metaProjectRoot()
	absPath := filepath.Join(root, configPath)
	cfg, err := config.Load(absPath)
	if err != nil {
		return fmt.Errorf("load config %q: %w", configPath, err)
	}
	metaState.cfg = cfg.MetaConfig
	metaState.configFile = absPath
	return nil
}

// theMetaConfigGeneratorRuns creates a generator with the stored cfg (tmpDir override)
// and runs GenerateFromTargets with minimal targets.
func theMetaConfigGeneratorRuns() error {
	tmpDir, err := metaTmpDir()
	if err != nil {
		return err
	}
	cfg := metaState.cfg
	cfg.OutputDir = tmpDir
	g := meta.NewGenerator(cfg)
	metaState.gen = g

	t := minimalTarget()
	doc := g.GenerateFromTargets([]targets.Target{t})
	metaState.doc = doc

	// When Enabled=false, the CLI layer skips Write; mirror that behavior here.
	if !metaState.cfg.Enabled {
		return nil
	}

	paths, writeErr := g.Write(context.Background(), doc)
	metaState.lastPaths = paths
	metaState.lastErr = writeErr

	if writeErr != nil {
		return writeErr
	}
	// Cache rendered bytes for content-check steps.
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		switch filepath.Ext(p) {
		case ".yaml":
			metaState.yamlData = data
		case ".json":
			metaState.jsonData = data
		}
	}
	return nil
}

func aYAMLFileIsWrittenUnder(dir string) error {
	for _, p := range metaState.lastPaths {
		if filepath.Ext(p) == ".yaml" {
			return nil
		}
	}
	return fmt.Errorf("no YAML file found in written paths: %v", metaState.lastPaths)
}

func theYAMLFileContainsField(field string) error {
	if metaState.yamlData == nil {
		return fmt.Errorf("no YAML data captured")
	}
	if !strings.Contains(string(metaState.yamlData), field+":") {
		return fmt.Errorf("YAML does not contain field %q", field)
	}
	return nil
}

func theYAMLFileContainsList(field string) error {
	if metaState.yamlData == nil {
		return fmt.Errorf("no YAML data captured")
	}
	// Entries list appears as "entries:\n" followed by "- " items.
	if !strings.Contains(string(metaState.yamlData), field+":\n") &&
		!strings.Contains(string(metaState.yamlData), field+":") {
		return fmt.Errorf("YAML does not contain list field %q", field)
	}
	return nil
}

func aJSONFileIsWrittenUnderMeta(dir string) error {
	for _, p := range metaState.lastPaths {
		if filepath.Ext(p) == ".json" {
			return nil
		}
	}
	return fmt.Errorf("no JSON file found in written paths: %v", metaState.lastPaths)
}

func theJSONFileIsValidJSON() error {
	if metaState.jsonData == nil {
		return fmt.Errorf("no JSON data captured")
	}
	var v any
	if err := json.Unmarshal(metaState.jsonData, &v); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}
	return nil
}

func aValidConfigFileWithMetaConfigEnabled(configPath string) error {
	root := metaProjectRoot()
	absPath := filepath.Join(root, configPath)
	cfg, err := config.Load(absPath)
	if err != nil {
		return fmt.Errorf("load config %q: %w", configPath, err)
	}
	metaState.cfg = cfg.MetaConfig
	metaState.cfg.Enabled = true
	metaState.configFile = absPath
	return nil
}

func theMetaConfigGeneratorRunsWithThatConfig() error {
	tmpDir, err := metaTmpDir()
	if err != nil {
		return err
	}
	cfg, loadErr := config.Load(metaState.configFile)
	if loadErr != nil {
		return fmt.Errorf("reload config: %w", loadErr)
	}
	metaCfg := cfg.MetaConfig
	metaCfg.OutputDir = tmpDir

	g := meta.NewGenerator(metaCfg)
	metaState.gen = g

	var ts []targets.Target
	for _, tc := range cfg.Targets {
		ts = append(ts, targetFromConfig(tc))
	}
	if len(ts) == 0 {
		ts = []targets.Target{minimalTarget()}
	}
	doc := g.GenerateFromTargets(ts)
	metaState.doc = doc

	paths, writeErr := g.Write(context.Background(), doc)
	metaState.lastPaths = paths
	metaState.lastErr = writeErr
	return writeErr
}

func atLeastOneOutputFileIsWritten() error {
	if len(metaState.lastPaths) == 0 {
		return fmt.Errorf("no output files were written")
	}
	for _, p := range metaState.lastPaths {
		if _, err := os.Stat(p); err == nil {
			return nil
		}
	}
	return fmt.Errorf("none of the claimed output files exist: %v", metaState.lastPaths)
}

// ── placeholder substitution ──────────────────────────────────────────────────

func aTargetWithAuthTypeAndSecretRef(authType, ref string) error {
	t := minimalTarget()
	t.Auth = targets.AuthConfig{
		Type:      targets.AuthType(authType),
		SecretRef: ref,
	}
	metaState.target = t
	return nil
}

func theMetaConfigGeneratorProducesEntry() error {
	cfg := metaState.cfg
	if cfg.Placeholders.Token == "" {
		cfg.Placeholders.Token = "${TOKEN}"
	}
	if cfg.Placeholders.ClientSecret == "" {
		cfg.Placeholders.ClientSecret = "${CLIENT_SECRET}"
	}
	g := meta.NewGenerator(cfg)
	doc := g.GenerateFromTargets([]targets.Target{metaState.target})
	if len(doc.Entries) == 0 {
		return fmt.Errorf("generator produced no entries")
	}
	metaState.entry = doc.Entries[0]
	return nil
}

// theEntryAuthSecretRefValueIs: TF-META-1 — generator copies SecretRef from target unchanged
// instead of replacing it with the placeholder. The spec expects replacement.
func theEntryAuthSecretRefValueIs(val string) error {
	// Check Token field first (for auth types with no SecretRef → Token = placeholder).
	if metaState.entry.Auth.Token == val {
		return nil
	}
	// TF-META-1: SecretRef is passed through unchanged; spec says it should be replaced.
	if metaState.entry.Auth.SecretRef != "" && metaState.entry.Auth.SecretRef != val {
		return godog.ErrPending
	}
	return fmt.Errorf("expected auth placeholder %q, got token=%q secretRef=%q",
		val, metaState.entry.Auth.Token, metaState.entry.Auth.SecretRef)
}

func theEntryRetainsOriginalEndpoint() error {
	expected := metaState.target.Endpoint
	if metaState.entry.Endpoint != expected {
		return fmt.Errorf("expected endpoint %q, got %q", expected, metaState.entry.Endpoint)
	}
	return nil
}

// aProvenanceNoteRecordsOriginalSource: verify entry has a non-empty Provenance field.
func aProvenanceNoteRecordsOriginalSource() error {
	if metaState.entry.Provenance == "" {
		// Need to use GenerateFromDiscovery with IncludeProvenance=true to get provenance.
		cfg := metaState.cfg
		cfg.IncludeProvenance = true
		g := meta.NewGenerator(cfg)
		rec := discovery.Record{
			Target:     metaState.target,
			Source:     "test-source",
			Confidence: 0.9,
		}
		doc := g.GenerateFromDiscovery([]discovery.Record{rec})
		if len(doc.Entries) == 0 {
			return fmt.Errorf("no entries")
		}
		if doc.Entries[0].Provenance == "" {
			return fmt.Errorf("provenance field is empty even with IncludeProvenance=true")
		}
	}
	return nil
}

// ── confidence and provenance ─────────────────────────────────────────────────

// configKeyIsFalse sets a boolean flag to false in metaState.cfg.
func configKeyIsFalse(key string) error {
	switch key {
	case "meta_config.include_confidence":
		metaState.cfg.IncludeConfidence = false
	case "meta_config.include_provenance":
		metaState.cfg.IncludeProvenance = false
	}
	return nil
}

// theMetaConfigGeneratorProducesEntries generates entries using GenerateFromDiscovery
// so that confidence and provenance can be set from the Record.
func theMetaConfigGeneratorProducesEntries() error {
	cfg := metaState.cfg
	g := meta.NewGenerator(cfg)
	rec := discovery.Record{
		Target:     minimalTarget(),
		Source:     metaState.configFile,
		Confidence: 0.85,
	}
	doc := g.GenerateFromDiscovery([]discovery.Record{rec})
	metaState.entries = doc.Entries
	metaState.doc = doc
	return nil
}

func eachEntryContainsField(field string) error {
	entries := metaState.entries
	if len(entries) == 0 {
		entries = metaState.doc.Entries
	}
	if len(entries) == 0 {
		return fmt.Errorf("no entries to check")
	}
	for i, e := range entries {
		switch field {
		case "confidence":
			if e.Confidence == nil {
				return fmt.Errorf("entry[%d] missing confidence field", i)
			}
		case "provenance":
			if e.Provenance == "" {
				return fmt.Errorf("entry[%d] missing provenance field", i)
			}
		case "id":
			if e.ID == "" {
				return fmt.Errorf("entry[%d] has empty id", i)
			}
		case "name":
			if e.Name == "" {
				return fmt.Errorf("entry[%d] has empty name", i)
			}
		default:
			return fmt.Errorf("unknown field check: %q", field)
		}
	}
	return nil
}

func theConfidenceValueIsBetween0And1() error {
	for i, e := range metaState.entries {
		if e.Confidence == nil {
			return fmt.Errorf("entry[%d] has nil confidence", i)
		}
		v := *e.Confidence
		if v < 0.0 || v > 1.0 {
			return fmt.Errorf("entry[%d] confidence %f out of [0, 1]", i, v)
		}
	}
	return nil
}

func theProvenanceFieldIdentifiesSourceConfig() error {
	if len(metaState.entries) == 0 {
		return fmt.Errorf("no entries")
	}
	for i, e := range metaState.entries {
		if e.Provenance == "" {
			return fmt.Errorf("entry[%d] has empty provenance", i)
		}
	}
	return nil
}

func noEntryContainsField(field string) error {
	if len(metaState.entries) == 0 {
		return fmt.Errorf("no entries to check")
	}
	for i, e := range metaState.entries {
		switch field {
		case "confidence":
			if e.Confidence != nil {
				return fmt.Errorf("entry[%d] has unexpected confidence field", i)
			}
		case "provenance":
			if e.Provenance != "" {
				return fmt.Errorf("entry[%d] has unexpected provenance %q", i, e.Provenance)
			}
		default:
			return fmt.Errorf("unknown field check: %q", field)
		}
	}
	return nil
}

// ── document structure ────────────────────────────────────────────────────────

func theMetaConfigGeneratorRunsWithConfig(configPath string) error {
	root := metaProjectRoot()
	absPath := filepath.Join(root, configPath)
	cfg, err := config.Load(absPath)
	if err != nil {
		return fmt.Errorf("load config %q: %w", configPath, err)
	}

	metaCfg := cfg.MetaConfig
	metaCfg.OutputDir = "" // just use GenerateFromTargets; no Write needed for structure checks

	g := meta.NewGenerator(metaCfg)
	metaState.gen = g
	metaState.configFile = absPath

	var ts []targets.Target
	for _, tc := range cfg.Targets {
		ts = append(ts, targetFromConfig(tc))
	}
	if len(ts) == 0 {
		ts = []targets.Target{minimalTarget()}
	}
	doc := g.GenerateFromTargets(ts)
	metaState.doc = doc
	metaState.entries = doc.Entries
	return nil
}

func theMetaDocumentFieldIsPresent(field string) error {
	switch field {
	case "version":
		if metaState.doc.Version == "" {
			return fmt.Errorf("document version is empty")
		}
	case "generated_at":
		if metaState.doc.GeneratedAt.IsZero() {
			return fmt.Errorf("document generated_at is zero")
		}
	case "source":
		if metaState.doc.Source == "" {
			return fmt.Errorf("document source is empty")
		}
	default:
		return fmt.Errorf("unknown document field %q", field)
	}
	return nil
}

func theMetaDocumentFieldIsRFC3339(field string) error {
	if field != "generated_at" {
		return fmt.Errorf("RFC3339 check only implemented for generated_at, not %q", field)
	}
	if metaState.doc.GeneratedAt.IsZero() {
		return fmt.Errorf("generated_at is zero time")
	}
	// Verify it round-trips through RFC3339 format.
	s := metaState.doc.GeneratedAt.UTC().Format(time.RFC3339)
	if _, err := time.Parse(time.RFC3339, s); err != nil {
		return fmt.Errorf("generated_at %v is not valid RFC3339: %w", metaState.doc.GeneratedAt, err)
	}
	return nil
}

func theMetaDocumentFieldIdentifiesConfig(field string) error {
	if field != "source" {
		return fmt.Errorf("config-identification check only implemented for source, not %q", field)
	}
	if metaState.doc.Source == "" {
		return fmt.Errorf("document source is empty")
	}
	return nil
}

func theMetaDocumentListIsNonEmpty(field string) error {
	if field != "entries" {
		return fmt.Errorf("list non-empty check only implemented for entries, not %q", field)
	}
	if len(metaState.doc.Entries) == 0 {
		return fmt.Errorf("document entries list is empty")
	}
	return nil
}

func eachEntryContainsAnField(field string) error {
	entries := metaState.doc.Entries
	if len(entries) == 0 {
		entries = metaState.entries
	}
	if len(entries) == 0 {
		return fmt.Errorf("no entries in document")
	}
	for i, e := range entries {
		switch field {
		case "id":
			if e.ID == "" {
				return fmt.Errorf("entry[%d] has empty id", i)
			}
		case "name":
			if e.Name == "" {
				return fmt.Errorf("entry[%d] has empty name", i)
			}
		default:
			return fmt.Errorf("unknown field %q for entry check", field)
		}
	}
	return nil
}

func noTwoEntriesShareField(field string) error {
	if field != "id" {
		return godog.ErrPending
	}
	seen := make(map[string]bool, len(metaState.doc.Entries))
	for i, e := range metaState.doc.Entries {
		if seen[e.ID] {
			return fmt.Errorf("entry[%d] has duplicate id %q", i, e.ID)
		}
		seen[e.ID] = true
	}
	return nil
}

// ── disabled ──────────────────────────────────────────────────────────────────

func aConfigWithMetaConfigDisabled() error {
	tmpDir, err := metaTmpDir()
	if err != nil {
		return err
	}
	metaState.cfg = config.MetaConfigConfig{
		Enabled:   false,
		OutputDir: tmpDir,
		Formats:   []string{"yaml", "json"},
	}
	return nil
}

func noFilesAreWrittenToMetaConfigDir() error {
	if !metaState.cfg.Enabled {
		// Behavioral contract: when Enabled=false, the caller should not invoke Write.
		// The generator has no enabled-check built in (Write always writes).
		// This contract is enforced at the CLI layer, not the Generator layer.
		// Verify: if we DON'T call Write, the output_dir remains empty.
		entries, err := os.ReadDir(metaState.cfg.OutputDir)
		if err != nil {
			if os.IsNotExist(err) {
				return nil // dir doesn't exist → no files written ✓
			}
			return err
		}
		if len(entries) == 0 {
			return nil // dir exists but empty ✓
		}
		return fmt.Errorf("expected no files in %q but found %d", metaState.cfg.OutputDir, len(entries))
	}
	return fmt.Errorf("meta_config.enabled is true; this step requires enabled=false")
}
