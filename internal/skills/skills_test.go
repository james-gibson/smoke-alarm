package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateSkillFile_Valid(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	skillPath := filepath.Join(dir, "test-skill", "SKILL.md")
	_ = os.MkdirAll(filepath.Dir(skillPath), 0o755)

	content := `---
name: test-skill
description: A test skill for validation
---

# Test Skill
This is a test skill.`

	if err := os.WriteFile(skillPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ValidateSkillFile(skillPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Valid {
		t.Errorf("expected valid, got errors: %v", result.Errors)
	}

	if result.Metadata == nil {
		t.Fatal("expected metadata to be parsed")
	}

	if result.Metadata.Name != "test-skill" {
		t.Errorf("expected name 'test-skill', got %q", result.Metadata.Name)
	}

	if result.Metadata.Description != "A test skill for validation" {
		t.Errorf("expected description, got %q", result.Metadata.Description)
	}

	if len(result.Validations) == 0 {
		t.Error("expected validations to be recorded")
	}
}

func TestValidateSkillFile_NameMismatch(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	skillPath := filepath.Join(dir, "my-skill", "SKILL.md")
	_ = os.MkdirAll(filepath.Dir(skillPath), 0o755)

	content := `---
name: different-name
description: A skill with mismatched name
---

# SKILL
`

	if err := os.WriteFile(skillPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ValidateSkillFile(skillPath)
	if err == nil {
		t.Fatal("expected error for name mismatch")
	}

	if result == nil || !strings.Contains(result.Errors[0], "name does not match") {
		t.Errorf("expected name mismatch error, got: %v", result.Errors)
	}
}

func TestValidateSkillFile_MissingName(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	skillPath := filepath.Join(dir, "skill", "SKILL.md")
	_ = os.MkdirAll(filepath.Dir(skillPath), 0o755)

	content := `---
description: A skill without name
---

# SKILL
`

	if err := os.WriteFile(skillPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ValidateSkillFile(skillPath)
	if err == nil {
		t.Fatal("expected error for missing name")
	}

	if result == nil || !strings.Contains(result.Errors[0], "name") {
		t.Errorf("expected missing name error, got: %v", result.Errors)
	}
}

func TestValidateSkillFile_MissingDescription(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	skillPath := filepath.Join(dir, "skill", "SKILL.md")
	_ = os.MkdirAll(filepath.Dir(skillPath), 0o755)

	content := `---
name: test-skill
---

# SKILL
`

	if err := os.WriteFile(skillPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ValidateSkillFile(skillPath)
	if err == nil {
		t.Fatal("expected error for missing description")
	}

	if result == nil || !strings.Contains(result.Errors[0], "description") {
		t.Errorf("expected missing description error, got: %v", result.Errors)
	}
}

func TestValidateSkillFile_InvalidYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	skillPath := filepath.Join(dir, "skill", "SKILL.md")
	_ = os.MkdirAll(filepath.Dir(skillPath), 0o755)

	content := `---
name: test
  invalid yaml: - this
---

# SKILL
`

	if err := os.WriteFile(skillPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ValidateSkillFile(skillPath)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}

	if result == nil || !strings.Contains(result.Errors[0], "YAML") {
		t.Errorf("expected YAML parse error, got: %v", result.Errors)
	}
}

func TestValidateSkillFile_FileNotFound(t *testing.T) {
	t.Parallel()

	result, err := ValidateSkillFile("/nonexistent/path/SKILL.md")
	if err == nil {
		t.Fatal("expected error for missing file")
	}

	if result != nil && len(result.Errors) == 0 {
		t.Error("expected errors to be recorded")
	}
}

func TestFindSkills_EmptyDir(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	results, err := FindSkills(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 skills, got %d", len(results))
	}
}

func TestFindSkills_WithValidSkills(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	skillDirs := []string{
		filepath.Join(".opencode", "skills", "test-skill-1"),
		filepath.Join(".opencode", "skills", "test-skill-2"),
		filepath.Join(".claude", "skills", "claude-skill"),
	}

	for _, d := range skillDirs {
		path := filepath.Join(dir, d, "SKILL.md")
		_ = os.MkdirAll(filepath.Dir(path), 0o755)

		dirName := filepath.Base(d)
		content := `---
name: ` + dirName + `
description: A test skill
---

# Content
`
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	results, err := FindSkills(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("expected 3 skills, got %d", len(results))
	}

	for _, r := range results {
		if !r.Valid {
			t.Errorf("expected all skills to be valid, got errors: %v", r.Errors)
		}
	}
}

func TestFindSkills_InvalidSkillIgnored(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	path := filepath.Join(dir, ".opencode", "skills", "bad-skill", "SKILL.md")
	_ = os.MkdirAll(filepath.Dir(path), 0o755)

	content := `---
name: wrong-dir-name
description: Invalid skill
---

# Content
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	results, err := FindSkills(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 result (with error), got %d", len(results))
	}

	if results[0].Valid {
		t.Error("expected invalid skill to have errors")
	}
}

func TestValidateProjectConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	_ = os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Agents"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "Makefile"), []byte("all:\n\techo done"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, ".golangci.yml"), []byte("run:\n  timeout: 5m"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, ".claude.md"), []byte("# Claude"), 0o644)

	_ = os.MkdirAll(filepath.Join(dir, ".opencode", "skills", "test-skill"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, ".opencode", "skills", "test-skill", "SKILL.md"), []byte(`---
name: test-skill
description: Test skill
---

# Test`), 0o644)

	status := ValidateProjectConfig(dir)

	checkFile := func(name string, shouldExist bool) {
		if info, ok := status[name].(map[string]bool); !ok {
			t.Errorf("expected status for %s", name)
		} else if info["exists"] != shouldExist {
			t.Errorf("expected exists=%v for %s", shouldExist, name)
		}
	}

	checkFile("AGENTS.md", true)
	checkFile("Makefile", true)
	checkFile(".golangci.yml", true)
	checkFile(".claude.md", true)

	skillsInfo, ok := status["skills"].(map[string]any)
	if !ok {
		t.Fatal("expected skills info in status")
	}

	count := skillsInfo["count"].(int) //nolint:errcheck
	if count != 1 {
		t.Errorf("expected 1 skill, got %d", count)
	}
}

func TestGenerateStartHereReport(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	_ = os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("# Agents"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "Makefile"), []byte("all:"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, ".claude.md"), []byte("# Claude"), 0o644)

	_ = os.MkdirAll(filepath.Join(dir, ".opencode", "skills", "demo-skill"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, ".opencode", "skills", "demo-skill", "SKILL.md"), []byte(`---
name: demo-skill
description: A demonstration skill
---

# Demo`), 0o644)

	report := GenerateStartHereReport(dir)

	if !strings.Contains(report, "Connected") {
		t.Error("expected connection confirmation in report")
	}

	if !strings.Contains(report, "Configuration Status") {
		t.Error("expected configuration status section")
	}

	if !strings.Contains(report, "demo-skill") {
		t.Error("expected skill name in report")
	}

	if !strings.Contains(report, "make ci") {
		t.Error("expected quick start commands in report")
	}
}

func TestValidateSkillFile_WithOptionalFields(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	skillPath := filepath.Join(dir, "full-skill", "SKILL.md")
	_ = os.MkdirAll(filepath.Dir(skillPath), 0o755)

	content := `---
name: full-skill
description: A skill with all optional fields
license: MIT
compatibility: opencode
metadata:
  audience: developers
  workflow: testing
---

# Full Skill
This skill demonstrates all optional fields.`

	if err := os.WriteFile(skillPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := ValidateSkillFile(skillPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.Valid {
		t.Errorf("expected valid, got errors: %v", result.Errors)
	}

	if result.Metadata.License != "MIT" {
		t.Errorf("expected license MIT, got %q", result.Metadata.License)
	}

	if result.Metadata.Compatibility != "opencode" {
		t.Errorf("expected compatibility opencode, got %q", result.Metadata.Compatibility)
	}

	if result.Metadata.Metadata["audience"] != "developers" {
		t.Errorf("expected metadata audience developers, got %q", result.Metadata.Metadata["audience"])
	}
}

func TestFindSkills_NonDirectoryEntries(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()

	_ = os.MkdirAll(filepath.Join(dir, ".opencode", "skills"), 0o755)
	_ = os.WriteFile(filepath.Join(dir, ".opencode", "skills", "not-a-skill.txt"), []byte("not a skill"), 0o644)

	results, err := FindSkills(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("expected 0 skills, got %d", len(results))
	}
}

func TestStartHereSkillExistsAndValid(t *testing.T) {
	t.Parallel()

	dir := findProjectRoot()
	skillPath := filepath.Join(dir, ".opencode", "skills", "start-here", "SKILL.md")

	result, err := ValidateSkillFile(skillPath)
	if err != nil {
		t.Fatalf("start-here skill validation failed: %v", err)
	}

	if !result.Valid {
		t.Errorf("start-here skill should be valid, errors: %v", result.Errors)
	}

	if result.Metadata == nil {
		t.Fatal("expected metadata to be parsed")
	}

	if result.Metadata.Name != "start-here" {
		t.Errorf("expected name 'start-here', got %q", result.Metadata.Name)
	}

	if result.Metadata.Description == "" {
		t.Error("expected description to be present")
	}

	if !strings.Contains(result.Metadata.Description, "client") || !strings.Contains(result.Metadata.Description, "connection") {
		t.Errorf("expected description to mention client/connection, got %q", result.Metadata.Description)
	}

	for _, v := range result.Validations {
		t.Logf("validation: %s", v)
	}
}

func TestDemoCapabilitiesSkillExistsAndValid(t *testing.T) {
	t.Parallel()

	dir := findProjectRoot()
	skillPath := filepath.Join(dir, ".opencode", "skills", "demo-capabilities", "SKILL.md")

	result, err := ValidateSkillFile(skillPath)
	if err != nil {
		t.Fatalf("demo-capabilities skill validation failed: %v", err)
	}

	if !result.Valid {
		t.Errorf("demo-capabilities skill should be valid, errors: %v", result.Errors)
	}

	if result.Metadata == nil {
		t.Fatal("expected metadata to be parsed")
	}

	if result.Metadata.Name != "demo-capabilities" {
		t.Errorf("expected name 'demo-capabilities', got %q", result.Metadata.Name)
	}

	if !strings.Contains(strings.ToLower(result.Metadata.Description), "demonstrates") {
		t.Errorf("expected description to mention demonstrates, got %q", result.Metadata.Description)
	}
}

func TestOpencodeStatusReportSkillExistsAndValid(t *testing.T) {
	t.Parallel()

	dir := findProjectRoot()
	skillPath := filepath.Join(dir, ".opencode", "skills", "opencode-status-report", "SKILL.md")

	result, err := ValidateSkillFile(skillPath)
	if err != nil {
		t.Fatalf("opencode-status-report skill validation failed: %v", err)
	}

	if !result.Valid {
		t.Errorf("opencode-status-report skill should be valid, errors: %v", result.Errors)
	}

	if result.Metadata == nil {
		t.Fatal("expected metadata to be parsed")
	}

	if result.Metadata.Name != "opencode-status-report" {
		t.Errorf("expected name 'opencode-status-report', got %q", result.Metadata.Name)
	}
}

func TestAllProjectSkills(t *testing.T) {
	t.Parallel()

	dir := findProjectRoot()
	results, err := FindSkills(dir)
	if err != nil {
		t.Fatalf("FindSkills failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("expected at least one skill in project")
	}

	validCount := 0
	for _, r := range results {
		if r.Valid {
			validCount++
			t.Logf("valid skill: %s - %s", r.Metadata.Name, r.Metadata.Description)
		} else {
			t.Logf("invalid skill: %s - errors: %v", r.Path, r.Errors)
		}
	}

	t.Logf("Total skills: %d, Valid: %d", len(results), validCount)

	if validCount < 3 {
		t.Errorf("expected at least 3 valid skills, got %d", validCount)
	}
}
