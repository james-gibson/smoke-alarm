package stepdefinitions

// skill_system_steps.go — step definitions for features/skill-system.feature
//
// Implemented in-process against internal/skills (ValidateSkillFile, FindSkills,
// ValidateProjectConfig, GenerateStartHereReport). Uses os.MkdirTemp for
// scenarios that need synthetic SKILL.md fixtures; project-contract scenarios
// exercise the actual .opencode/skills/ directory on disk.
//
// Note: "the error message contains {string}" is registered here (InitializeSkillSystemSteps
// runs before InitializeOpsScenario in suite_test.go). ops_steps.go also registers
// this pattern but its registration is a no-op duplicate — godog uses the first match.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
	"github.com/james-gibson/smoke-alarm/internal/skills"
)

// ── per-scenario state ────────────────────────────────────────────────────────

var skillState struct {
	tmpDir       string
	rootDir      string // tmpDir by default; realProjectRoot() for project-contract scenarios
	skillPath    string
	lastResult   *skills.ValidationResult
	lastErr      error
	findResults  []skills.ValidationResult
	findErr      error
	configResult map[string]any
	reportOutput string
}

func resetSkillState() {
	if skillState.tmpDir != "" {
		_ = os.RemoveAll(skillState.tmpDir)
	}
	skillState.tmpDir = ""
	skillState.rootDir = ""
	skillState.skillPath = ""
	skillState.lastResult = nil
	skillState.lastErr = nil
	skillState.findResults = nil
	skillState.findErr = nil
	skillState.configResult = nil
	skillState.reportOutput = ""
}

func ensureTmpDir() error {
	if skillState.tmpDir == "" {
		d, err := os.MkdirTemp("", "skill-test-*")
		if err != nil {
			return fmt.Errorf("create tmp dir: %w", err)
		}
		skillState.tmpDir = d
		skillState.rootDir = d
	}
	return nil
}

// realSkillProjectRoot walks up from the working directory to find go.mod,
// returning the project root for project-contract scenarios.
func realSkillProjectRoot() string {
	dir, _ := filepath.Abs(".")
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

// createSkillFile writes content to absPath, creating parent dirs as needed.
func createSkillFile(absPath, content string) error {
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return fmt.Errorf("mkdir %q: %w", filepath.Dir(absPath), err)
	}
	return os.WriteFile(absPath, []byte(content), 0o644)
}

// skillYAML builds a minimal YAML SKILL.md content string.
func skillYAML(name, desc string, extras map[string]string) string {
	var sb strings.Builder
	if name != "" {
		sb.WriteString("name: " + name + "\n")
	}
	if desc != "" {
		sb.WriteString("description: " + desc + "\n")
	}
	for k, v := range extras {
		sb.WriteString(k + ": " + v + "\n")
	}
	return sb.String()
}

// InitializeSkillSystemSteps registers all step definitions for skill-system.feature.
func InitializeSkillSystemSteps(ctx *godog.ScenarioContext) {
	ctx.BeforeScenario(func(_ *godog.Scenario) { resetSkillState() })
	ctx.AfterScenario(func(_ *godog.Scenario, _ error) { resetSkillState() })

	// ── ValidateSkillFile ──────────────────────────────────────────────────
	ctx.Step(`^a SKILL\.md file exists at "([^"]*)" with:$`, aSKILLMdFileExistsAtWithTable)
	ctx.Step(`^ValidateSkillFile is called with that path$`, validateSkillFileIsCalledWithThatPath)
	ctx.Step(`^the result is valid$`, theSkillResultIsValid)
	ctx.Step(`^the validation log contains "([^"]*)"$`, theValidationLogContains)
	ctx.Step(`^the result contains at least one error entry$`, theResultContainsAtLeastOneErrorEntry)
	ctx.Step(`^a SKILL\.md file exists with description but no name field$`, aSKILLMdWithDescriptionNoName)
	ctx.Step(`^a SKILL\.md file exists with name but no description field$`, aSKILLMdWithNameNoDescription)
	ctx.Step(`^the result is invalid$`, theSkillResultIsInvalid)
	ctx.Step(`^the error message contains "([^"]*)"$`, skillErrorMessageContains)
	ctx.Step(`^a SKILL\.md file exists at "([^"]*)" with name "([^"]*)"$`, aSKILLMdFileExistsAtWithName)
	ctx.Step(`^a SKILL\.md file exists with malformed YAML frontmatter$`, aSKILLMdWithMalformedYAML)
	ctx.Step(`^ValidateSkillFile is called with "([^"]*)"$`, validateSkillFileIsCalledWith)
	ctx.Step(`^an error is returned$`, anSkillErrorIsReturned)
	ctx.Step(`^a SKILL\.md file with name, description, license "([^"]*)", and compatibility "([^"]*)"$`, aSKILLMdWithLicenseAndCompatibility)
	ctx.Step(`^the metadata license is "([^"]*)"$`, theMetadataLicenseIs)
	ctx.Step(`^the metadata compatibility is "([^"]*)"$`, theMetadataCompatibilityIs)

	// ── FindSkills ─────────────────────────────────────────────────────────
	ctx.Step(`^valid SKILL\.md files exist at:$`, validSKILLMdFilesExistAt)
	ctx.Step(`^FindSkills is called on the project root$`, findSkillsIsCalledOnProjectRoot)
	ctx.Step(`^(\d+) results are returned$`, nResultsAreReturned)
	ctx.Step(`^all (\d+) results are valid$`, allNResultsAreValid)
	ctx.Step(`^a project root with no \.opencode/skills, \.claude/skills, or \.claude/agents directories$`, aProjectRootWithNoSkillDirs)
	ctx.Step(`^a valid skill at "([^"]*)"$`, aValidSkillAt)
	ctx.Step(`^an invalid skill at "([^"]*)" with name "([^"]*)"$`, anInvalidSkillAtWithName)
	ctx.Step(`^(\d+) result is valid$`, nResultIsValid)
	ctx.Step(`^(\d+) result is invalid$`, nResultIsInvalid)
	ctx.Step(`^a file "([^"]*)" exists directly under "([^"]*)"$`, aFileExistsDirectlyUnder)

	// ── project skills contract ────────────────────────────────────────────
	ctx.Step(`^the ocd-smoke-alarm project root$`, theOcdSmokeAlarmProjectRoot)
	ctx.Step(`^at least (\d+) skills are found$`, atLeastNSkillsAreFound)
	ctx.Step(`^all found skills are valid$`, allFoundSkillsAreValid)
	ctx.Step(`^the project root contains "([^"]*)"$`, theProjectRootContains)
	ctx.Step(`^the metadata name is "([^"]*)"$`, theMetadataNameIs)
	ctx.Step(`^the metadata description contains "([^"]*)"$`, theMetadataDescriptionContains)

	// ── ValidateProjectConfig ──────────────────────────────────────────────
	ctx.Step(`^a project root containing "([^"]*)", "([^"]*)", "([^"]*)", "([^"]*)"$`, aProjectRootContainingFiles)
	ctx.Step(`^ValidateProjectConfig is called on the project root$`, validateProjectConfigIsCalledOnRoot)
	ctx.Step(`^the result shows "([^"]*)" exists$`, theResultShowsExists)
	ctx.Step(`^a project root with (\d+) valid skill$`, aProjectRootWithNValidSkills)
	ctx.Step(`^the skills count in the result is (\d+)$`, theSkillsCountInResultIs)

	// ── GenerateStartHereReport ────────────────────────────────────────────
	ctx.Step(`^a project root with at least one valid skill$`, aProjectRootWithAtLeastOneValidSkill)
	ctx.Step(`^GenerateStartHereReport is called on the project root$`, generateStartHereReportIsCalledOnRoot)
	ctx.Step(`^the output contains "([^"]*)"$`, theOutputContains)
	ctx.Step(`^a project root containing skill "([^"]*)" with description "([^"]*)"$`, aProjectRootContainingSkill)
}

// ── ValidateSkillFile ─────────────────────────────────────────────────────────

func aSKILLMdFileExistsAtWithTable(relPath string, table *godog.Table) error {
	if err := ensureTmpDir(); err != nil {
		return err
	}
	fields := make(map[string]string)
	for _, row := range table.Rows[1:] { // skip header row
		if len(row.Cells) >= 2 {
			fields[row.Cells[0].Value] = row.Cells[1].Value
		}
	}
	name := fields["name"]
	desc := fields["description"]
	delete(fields, "name")
	delete(fields, "description")

	absPath := filepath.Join(skillState.tmpDir, relPath)
	if err := createSkillFile(absPath, skillYAML(name, desc, fields)); err != nil {
		return err
	}
	skillState.skillPath = absPath
	return nil
}

func validateSkillFileIsCalledWithThatPath() error {
	skillState.lastResult, skillState.lastErr = skills.ValidateSkillFile(skillState.skillPath)
	return nil
}

func theSkillResultIsValid() error {
	if skillState.lastResult == nil {
		return fmt.Errorf("no validation result")
	}
	if !skillState.lastResult.Valid {
		return fmt.Errorf("expected valid=true; errors: %v", skillState.lastResult.Errors)
	}
	return nil
}

func theValidationLogContains(msg string) error {
	if skillState.lastResult == nil {
		return fmt.Errorf("no validation result")
	}
	for _, v := range skillState.lastResult.Validations {
		if strings.Contains(v, msg) {
			return nil
		}
	}
	return fmt.Errorf("validation log %v does not contain %q", skillState.lastResult.Validations, msg)
}

func theResultContainsAtLeastOneErrorEntry() error {
	if skillState.lastResult == nil || len(skillState.lastResult.Errors) == 0 {
		return fmt.Errorf("no error entries in result")
	}
	return nil
}

func aSKILLMdWithDescriptionNoName() error {
	if err := ensureTmpDir(); err != nil {
		return err
	}
	absPath := filepath.Join(skillState.tmpDir, "no-name-skill", "SKILL.md")
	if err := createSkillFile(absPath, skillYAML("", "A test skill with no name", nil)); err != nil {
		return err
	}
	skillState.skillPath = absPath
	return nil
}

func aSKILLMdWithNameNoDescription() error {
	if err := ensureTmpDir(); err != nil {
		return err
	}
	// dir name must match name field to avoid name-mismatch firing before missing-description
	absPath := filepath.Join(skillState.tmpDir, "my-skill", "SKILL.md")
	if err := createSkillFile(absPath, skillYAML("my-skill", "", nil)); err != nil {
		return err
	}
	skillState.skillPath = absPath
	return nil
}

func theSkillResultIsInvalid() error {
	if skillState.lastResult == nil {
		return fmt.Errorf("no validation result")
	}
	if skillState.lastResult.Valid {
		return fmt.Errorf("expected valid=false, got valid=true")
	}
	return nil
}

func skillErrorMessageContains(substr string) error {
	if skillState.lastResult == nil {
		return fmt.Errorf("no validation result")
	}
	for _, e := range skillState.lastResult.Errors {
		if strings.Contains(e, substr) {
			return nil
		}
	}
	// Also check the error itself.
	if skillState.lastErr != nil && strings.Contains(skillState.lastErr.Error(), substr) {
		return nil
	}
	return fmt.Errorf("no error message containing %q; errors=%v err=%w", substr, skillState.lastResult.Errors, skillState.lastErr)
}

func aSKILLMdFileExistsAtWithName(relPath, name string) error {
	if err := ensureTmpDir(); err != nil {
		return err
	}
	absPath := filepath.Join(skillState.tmpDir, relPath)
	if err := createSkillFile(absPath, skillYAML(name, "A test skill", nil)); err != nil {
		return err
	}
	skillState.skillPath = absPath
	return nil
}

func aSKILLMdWithMalformedYAML() error {
	if err := ensureTmpDir(); err != nil {
		return err
	}
	absPath := filepath.Join(skillState.tmpDir, "bad-skill", "SKILL.md")
	if err := createSkillFile(absPath, "{invalid: yaml: [unclosed\n"); err != nil {
		return err
	}
	skillState.skillPath = absPath
	return nil
}

func validateSkillFileIsCalledWith(path string) error {
	skillState.lastResult, skillState.lastErr = skills.ValidateSkillFile(path)
	return nil
}

func anSkillErrorIsReturned() error {
	if skillState.lastErr == nil {
		return fmt.Errorf("expected an error, got nil")
	}
	return nil
}

func aSKILLMdWithLicenseAndCompatibility(license, compat string) error {
	if err := ensureTmpDir(); err != nil {
		return err
	}
	// dir name must match name field
	absPath := filepath.Join(skillState.tmpDir, "test-skill", "SKILL.md")
	extras := map[string]string{
		"license":       license,
		"compatibility": compat,
	}
	if err := createSkillFile(absPath, skillYAML("test-skill", "A test skill", extras)); err != nil {
		return err
	}
	skillState.skillPath = absPath
	return nil
}

func theMetadataLicenseIs(license string) error {
	if skillState.lastResult == nil || skillState.lastResult.Metadata == nil {
		return fmt.Errorf("no metadata available")
	}
	if skillState.lastResult.Metadata.License != license {
		return fmt.Errorf("license=%q want=%q", skillState.lastResult.Metadata.License, license)
	}
	return nil
}

func theMetadataCompatibilityIs(compat string) error {
	if skillState.lastResult == nil || skillState.lastResult.Metadata == nil {
		return fmt.Errorf("no metadata available")
	}
	if skillState.lastResult.Metadata.Compatibility != compat {
		return fmt.Errorf("compatibility=%q want=%q", skillState.lastResult.Metadata.Compatibility, compat)
	}
	return nil
}

// ── FindSkills ────────────────────────────────────────────────────────────────

func validSKILLMdFilesExistAt(table *godog.Table) error {
	if err := ensureTmpDir(); err != nil {
		return err
	}
	for _, row := range table.Rows[1:] { // skip header
		if len(row.Cells) == 0 {
			continue
		}
		relPath := strings.TrimSpace(row.Cells[0].Value)
		// dir name derived from path: e.g. ".opencode/skills/skill-one/SKILL.md" → "skill-one"
		dirName := filepath.Base(filepath.Dir(relPath))
		absPath := filepath.Join(skillState.tmpDir, relPath)
		if err := createSkillFile(absPath, skillYAML(dirName, "A "+dirName+" skill", nil)); err != nil {
			return fmt.Errorf("create skill at %q: %w", relPath, err)
		}
	}
	return nil
}

func findSkillsIsCalledOnProjectRoot() error {
	skillState.findResults, skillState.findErr = skills.FindSkills(skillState.rootDir)
	return nil
}

func nResultsAreReturned(n int) error {
	if len(skillState.findResults) != n {
		return fmt.Errorf("results count=%d want=%d", len(skillState.findResults), n)
	}
	return nil
}

func allNResultsAreValid(n int) error {
	if len(skillState.findResults) != n {
		return fmt.Errorf("results count=%d want=%d", len(skillState.findResults), n)
	}
	for i, r := range skillState.findResults {
		if !r.Valid {
			return fmt.Errorf("result[%d] (%s) is not valid: %v", i, r.Path, r.Errors)
		}
	}
	return nil
}

func aProjectRootWithNoSkillDirs() error {
	if err := ensureTmpDir(); err != nil {
		return err
	}
	// tmpDir has no .opencode/skills, .claude/skills, or .claude/agents — nothing to create.
	skillState.rootDir = skillState.tmpDir
	return nil
}

func aValidSkillAt(relPath string) error {
	if err := ensureTmpDir(); err != nil {
		return err
	}
	dirName := filepath.Base(filepath.Dir(relPath))
	absPath := filepath.Join(skillState.tmpDir, relPath)
	return createSkillFile(absPath, skillYAML(dirName, "A "+dirName+" skill", nil))
}

func anInvalidSkillAtWithName(relPath, name string) error {
	if err := ensureTmpDir(); err != nil {
		return err
	}
	absPath := filepath.Join(skillState.tmpDir, relPath)
	// name deliberately does not match the directory name → ErrNameMismatch
	return createSkillFile(absPath, skillYAML(name, "A skill with mismatched name", nil))
}

func nResultIsValid(n int) error {
	valid := 0
	for _, r := range skillState.findResults {
		if r.Valid {
			valid++
		}
	}
	if valid != n {
		return fmt.Errorf("valid results=%d want=%d", valid, n)
	}
	return nil
}

func nResultIsInvalid(n int) error {
	invalid := 0
	for _, r := range skillState.findResults {
		if !r.Valid {
			invalid++
		}
	}
	if invalid != n {
		return fmt.Errorf("invalid results=%d want=%d", invalid, n)
	}
	return nil
}

func aFileExistsDirectlyUnder(file, dir string) error {
	if err := ensureTmpDir(); err != nil {
		return err
	}
	absDir := filepath.Join(skillState.tmpDir, dir)
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		return fmt.Errorf("mkdir %q: %w", absDir, err)
	}
	return os.WriteFile(filepath.Join(absDir, file), []byte("not a skill"), 0o644)
}

// ── project skills contract ───────────────────────────────────────────────────

func theOcdSmokeAlarmProjectRoot() error {
	skillState.rootDir = realSkillProjectRoot()
	return nil
}

func atLeastNSkillsAreFound(n int) error {
	if len(skillState.findResults) < n {
		return fmt.Errorf("found %d skills, want at least %d", len(skillState.findResults), n)
	}
	return nil
}

func allFoundSkillsAreValid() error {
	for i, r := range skillState.findResults {
		if !r.Valid {
			return fmt.Errorf("skill[%d] (%s) is invalid: %v", i, r.Path, r.Errors)
		}
	}
	return nil
}

func theProjectRootContains(relPath string) error {
	root := realSkillProjectRoot()
	absPath := filepath.Join(root, relPath)
	if _, err := os.Stat(absPath); err != nil {
		return fmt.Errorf("project root does not contain %q: %w", relPath, err)
	}
	skillState.skillPath = absPath
	skillState.rootDir = root
	return nil
}

func theMetadataNameIs(name string) error {
	if skillState.lastResult == nil || skillState.lastResult.Metadata == nil {
		return fmt.Errorf("no metadata available")
	}
	if skillState.lastResult.Metadata.Name != name {
		return fmt.Errorf("name=%q want=%q", skillState.lastResult.Metadata.Name, name)
	}
	return nil
}

func theMetadataDescriptionContains(substr string) error {
	if skillState.lastResult == nil || skillState.lastResult.Metadata == nil {
		return fmt.Errorf("no metadata available")
	}
	desc := skillState.lastResult.Metadata.Description
	if !strings.Contains(strings.ToLower(desc), strings.ToLower(substr)) {
		return fmt.Errorf("description %q does not contain %q (case-insensitive)", desc, substr)
	}
	return nil
}

// ── ValidateProjectConfig ─────────────────────────────────────────────────────

func aProjectRootContainingFiles(a, b, c, d string) error {
	if err := ensureTmpDir(); err != nil {
		return err
	}
	for _, f := range []string{a, b, c, d} {
		if err := os.WriteFile(filepath.Join(skillState.tmpDir, f), []byte(""), 0o644); err != nil {
			return fmt.Errorf("create %q: %w", f, err)
		}
	}
	skillState.rootDir = skillState.tmpDir
	return nil
}

func validateProjectConfigIsCalledOnRoot() error {
	skillState.configResult = skills.ValidateProjectConfig(skillState.rootDir)
	return nil
}

func theResultShowsExists(filename string) error {
	v, ok := skillState.configResult[filename]
	if !ok {
		return fmt.Errorf("%q not in config result (keys: %v)", filename, configResultKeys())
	}
	info, ok := v.(map[string]bool)
	if !ok {
		return fmt.Errorf("unexpected type for %q: %T", filename, v)
	}
	if !info["exists"] {
		return fmt.Errorf("%q shows exists=false in result", filename)
	}
	return nil
}

func configResultKeys() []string {
	keys := make([]string, 0, len(skillState.configResult))
	for k := range skillState.configResult {
		keys = append(keys, k)
	}
	return keys
}

func aProjectRootWithNValidSkills(n int) error {
	if err := ensureTmpDir(); err != nil {
		return err
	}
	for i := range n {
		name := fmt.Sprintf("skill-%d", i)
		relPath := filepath.Join(".opencode", "skills", name, "SKILL.md")
		absPath := filepath.Join(skillState.tmpDir, relPath)
		if err := createSkillFile(absPath, skillYAML(name, "Skill "+name, nil)); err != nil {
			return fmt.Errorf("create skill %d: %w", i, err)
		}
	}
	skillState.rootDir = skillState.tmpDir
	return nil
}

func theSkillsCountInResultIs(n int) error {
	if skillState.configResult == nil {
		return fmt.Errorf("no config result")
	}
	skillsData, ok := skillState.configResult["skills"].(map[string]any)
	if !ok {
		return fmt.Errorf("skills key missing or wrong type in result")
	}
	count, ok := skillsData["count"].(int)
	if !ok {
		return fmt.Errorf("skills.count missing or wrong type: %T", skillsData["count"])
	}
	if count != n {
		return fmt.Errorf("skills count=%d want=%d", count, n)
	}
	return nil
}

// ── GenerateStartHereReport ───────────────────────────────────────────────────

func aProjectRootWithAtLeastOneValidSkill() error {
	if err := ensureTmpDir(); err != nil {
		return err
	}
	relPath := filepath.Join(".opencode", "skills", "test-skill", "SKILL.md")
	absPath := filepath.Join(skillState.tmpDir, relPath)
	if err := createSkillFile(absPath, skillYAML("test-skill", "A test skill", nil)); err != nil {
		return err
	}
	skillState.rootDir = skillState.tmpDir
	return nil
}

func generateStartHereReportIsCalledOnRoot() error {
	skillState.reportOutput = skills.GenerateStartHereReport(skillState.rootDir)
	return nil
}

// theOutputContains is also referenced by warroom_simulator_steps.go for the same
// "the output contains {string}" step pattern. When skillState.reportOutput is empty
// (non-skill scenarios), it falls back to ErrPending so warroom scenarios stay pending.
func theOutputContains(substr string) error {
	if skillState.reportOutput != "" {
		if !strings.Contains(skillState.reportOutput, substr) {
			return fmt.Errorf("output does not contain %q\n--- output ---\n%s", substr, skillState.reportOutput)
		}
		return nil
	}
	return godog.ErrPending
}

func aProjectRootContainingSkill(name, desc string) error {
	if err := ensureTmpDir(); err != nil {
		return err
	}
	relPath := filepath.Join(".opencode", "skills", name, "SKILL.md")
	absPath := filepath.Join(skillState.tmpDir, relPath)
	if err := createSkillFile(absPath, skillYAML(name, desc, nil)); err != nil {
		return err
	}
	skillState.rootDir = skillState.tmpDir
	return nil
}
