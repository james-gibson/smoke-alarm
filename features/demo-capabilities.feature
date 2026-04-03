# features/demo-capabilities.feature
# Canon record — last audited: 2026-03-26
# Exercises: demo-capabilities skill — skill discovery, validation, capabilities report
# Skill: .opencode/skills/demo-capabilities/SKILL.md
# Step definitions: features/step_definitions/demo_capabilities_steps.go

@demo-capabilities @skill
Feature: Demo Capabilities Skill
  As a developer exploring this repository
  I want the demo-capabilities skill to discover, validate, and report all available skills
  So that I can understand what agent capabilities are available and confirm they are correctly configured

  Background:
    Given the ocd-smoke-alarm binary is installed
    And a Claude Code session is active in this repository

  # ── skill invocation ───────────────────────────────────────────────────────

  Scenario: agent invokes the demo-capabilities skill
    When the agent invokes the skill "demo-capabilities"
    Then the skill ".opencode/skills/demo-capabilities/SKILL.md" is read
    And the agent executes the documented steps in order

  # ── skill discovery ────────────────────────────────────────────────────────

  Scenario: demo-capabilities discovers all skills in .opencode/skills
    When the agent invokes the skill "demo-capabilities"
    Then each subdirectory of ".opencode/skills/" is scanned for a SKILL.md file
    And the output lists each discovered skill by name

  Scenario: demo-capabilities includes validation status for each skill
    When the agent invokes the skill "demo-capabilities"
    Then the output contains a skill inventory table
    And each row in the inventory table includes a validation status

  # ── skill validation ───────────────────────────────────────────────────────

  Scenario Outline: demo-capabilities reports a skill as valid when it has required fields
    Given a SKILL.md at ".opencode/skills/<skill>/SKILL.md" with name and description fields
    When the agent invokes the skill "demo-capabilities"
    Then the skill "<skill>" appears in the inventory as valid

    Examples:
      | skill                  |
      | start-here             |
      | opencode-status-report |
      | open-the-pickle-jar    |

  Scenario: demo-capabilities reports a skill as invalid when name does not match directory
    Given a SKILL.md exists with a name field that does not match its directory
    When the agent invokes the skill "demo-capabilities"
    Then that skill appears in the inventory as invalid
    And the output identifies the validation failure reason

  # ── related config files ───────────────────────────────────────────────────

  Scenario: demo-capabilities checks for AGENTS.md
    When the agent invokes the skill "demo-capabilities"
    Then the output includes the presence status of "AGENTS.md"

  Scenario: demo-capabilities checks for Makefile
    When the agent invokes the skill "demo-capabilities"
    Then the output includes the presence status of "Makefile"

  # ── output structure ───────────────────────────────────────────────────────

  Scenario: demo-capabilities output includes a next steps section
    When the agent invokes the skill "demo-capabilities"
    Then the output contains "make ci"
    And the output contains "opencode-status-report"

  # ── stability ──────────────────────────────────────────────────────────────

  Scenario: demo-capabilities output is stable across repeated invocations
    Given the skill "demo-capabilities" has completed successfully once
    When the agent invokes the skill "demo-capabilities" again with identical inputs
    Then the output is equivalent to the first run
    And no duplicate state files are created under "state/"
