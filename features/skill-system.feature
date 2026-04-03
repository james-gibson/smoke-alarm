# features/skill-system.feature
# Canon record — last audited: 2026-03-25
# Exercises: internal/skills Go package — ValidateSkillFile, FindSkills, ValidateProjectConfig, GenerateStartHereReport
# Code: internal/skills/skills.go
# Step definitions: features/step_definitions/skill_system_steps.go
# see: features/config-validation.feature (project structure checks)
#
# NOTE: The Go package under internal/skills/ is the programmatic enforcement layer
# for SKILL.md contracts. These scenarios are the Gherkin canon for that layer.
# The open-the-pickle-jar skill relies on this package being correct.

@skill-system @core
Feature: Skill System Validation
  As a developer or agent
  I want the skill system to programmatically validate SKILL.md contracts
  So that broken or misconfigured skills are caught before they are invoked

  Background:
    Given the ocd-smoke-alarm binary is installed

  # ── ValidateSkillFile ─────────────────────────────────────────────────────

  Scenario: a valid SKILL.md with required fields passes validation
    Given a SKILL.md file exists at ".opencode/skills/test-skill/SKILL.md" with:
      | field       | value              |
      | name        | test-skill         |
      | description | A test skill       |
    When ValidateSkillFile is called with that path
    Then the result is valid
    And the validation log contains "YAML frontmatter parsed successfully"
    And the validation log contains "name field present"
    And the validation log contains "description field present"
    And the validation log contains "name matches directory"

  Scenario: a SKILL.md missing the name field fails validation
    Given a SKILL.md file exists with description but no name field
    When ValidateSkillFile is called with that path
    Then the result is invalid
    And the error message contains "name"

  Scenario: a SKILL.md missing the description field fails validation
    Given a SKILL.md file exists with name but no description field
    When ValidateSkillFile is called with that path
    Then the result is invalid
    And the error message contains "description"

  Scenario: a SKILL.md whose name does not match its directory name fails validation
    Given a SKILL.md file exists at ".opencode/skills/my-skill/SKILL.md" with name "different-name"
    When ValidateSkillFile is called with that path
    Then the result is invalid
    And the error message contains "name does not match"

  Scenario: a SKILL.md with invalid YAML frontmatter fails validation
    Given a SKILL.md file exists with malformed YAML frontmatter
    When ValidateSkillFile is called with that path
    Then the result is invalid
    And the error message contains "YAML"

  Scenario: calling ValidateSkillFile on a non-existent path returns an error
    When ValidateSkillFile is called with "/nonexistent/path/SKILL.md"
    Then an error is returned
    And the result contains at least one error entry

  Scenario: optional fields (license, compatibility, metadata) are parsed without error
    Given a SKILL.md file with name, description, license "MIT", and compatibility "opencode"
    When ValidateSkillFile is called with that path
    Then the result is valid
    And the metadata license is "MIT"
    And the metadata compatibility is "opencode"

  # ── FindSkills ────────────────────────────────────────────────────────────

  Scenario: FindSkills discovers skills in all three standard directories
    Given valid SKILL.md files exist at:
      | path                                        |
      | .opencode/skills/skill-one/SKILL.md         |
      | .claude/skills/skill-two/SKILL.md           |
      | .claude/agents/skill-three/SKILL.md         |
    When FindSkills is called on the project root
    Then 3 results are returned
    And all 3 results are valid

  Scenario: FindSkills returns zero results when no skill directories exist
    Given a project root with no .opencode/skills, .claude/skills, or .claude/agents directories
    When FindSkills is called on the project root
    Then 0 results are returned
    And no error is returned

  Scenario: FindSkills includes an invalid skill in results without aborting
    Given a valid skill at ".opencode/skills/good-skill/SKILL.md"
    And an invalid skill at ".opencode/skills/bad-skill/SKILL.md" with name "wrong-name"
    When FindSkills is called on the project root
    Then 2 results are returned
    And 1 result is valid
    And 1 result is invalid

  Scenario: FindSkills ignores non-directory entries in the skills folder
    Given a file "not-a-skill.txt" exists directly under ".opencode/skills/"
    When FindSkills is called on the project root
    Then 0 results are returned

  # ── project skills contract ───────────────────────────────────────────────

  Scenario: all project skills in .opencode/skills/ are valid
    Given the ocd-smoke-alarm project root
    When FindSkills is called on the project root
    Then at least 3 skills are found
    And all found skills are valid

  Scenario: the start-here skill exists and its description mentions "client" and "connection"
    Given the project root contains ".opencode/skills/start-here/SKILL.md"
    When ValidateSkillFile is called with that path
    Then the result is valid
    And the metadata name is "start-here"
    And the metadata description contains "client"
    And the metadata description contains "connection"

  Scenario: the demo-capabilities skill exists and its description mentions "demonstrates"
    Given the project root contains ".opencode/skills/demo-capabilities/SKILL.md"
    When ValidateSkillFile is called with that path
    Then the result is valid
    And the metadata description contains "demonstrates"

  Scenario: the opencode-status-report skill exists and is valid
    Given the project root contains ".opencode/skills/opencode-status-report/SKILL.md"
    When ValidateSkillFile is called with that path
    Then the result is valid
    And the metadata name is "opencode-status-report"

  Scenario: the open-the-pickle-jar skill exists and is valid
    Given the project root contains ".opencode/skills/open-the-pickle-jar/SKILL.md"
    When ValidateSkillFile is called with that path
    Then the result is valid
    And the metadata name is "open-the-pickle-jar"

  # ── ValidateProjectConfig ─────────────────────────────────────────────────

  Scenario: ValidateProjectConfig reports presence of required project files
    Given a project root containing "AGENTS.md", "Makefile", ".golangci.yml", ".claude.md"
    When ValidateProjectConfig is called on the project root
    Then the result shows "AGENTS.md" exists
    And the result shows "Makefile" exists
    And the result shows ".golangci.yml" exists
    And the result shows ".claude.md" exists

  Scenario: ValidateProjectConfig includes a skill count in results
    Given a project root with 1 valid skill
    When ValidateProjectConfig is called on the project root
    Then the skills count in the result is 1

  # ── GenerateStartHereReport ───────────────────────────────────────────────

  Scenario: GenerateStartHereReport output contains connection confirmation
    Given a project root with at least one valid skill
    When GenerateStartHereReport is called on the project root
    Then the output contains "Connected"
    And the output contains "Configuration Status"
    And the output contains "make ci"

  Scenario: GenerateStartHereReport lists each discovered skill by name
    Given a project root containing skill "demo-skill" with description "A demonstration skill"
    When GenerateStartHereReport is called on the project root
    Then the output contains "demo-skill"
