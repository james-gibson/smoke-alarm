# features/start-here.feature
# Canon record — last audited: 2026-03-26
# Exercises: start-here skill — connection confirmation, project validation, welcome output
# Skill: .opencode/skills/start-here/SKILL.md
# Step definitions: features/step_definitions/start_here_steps.go

@start-here @skill
Feature: Start-Here Welcome Skill
  As a developer connecting to this repository for the first time
  I want the start-here skill to confirm my connection and validate the project structure
  So that I know the agent configuration system is operational before doing any work

  Background:
    Given the ocd-smoke-alarm binary is installed
    And a Claude Code session is active in this repository

  # ── connection confirmation ────────────────────────────────────────────────

  Scenario: agent invokes the start-here skill
    When the agent invokes the skill "start-here"
    Then the skill ".opencode/skills/start-here/SKILL.md" is read
    And the agent executes the documented steps in order

  Scenario: start-here output confirms connection is operational
    When the agent invokes the skill "start-here"
    Then the output contains "Connected"
    And the output contains "ocd-smoke-alarm"

  # ── project structure validation ───────────────────────────────────────────

  Scenario: start-here validates that AGENTS.md is present
    Given the project root contains "AGENTS.md"
    When the agent invokes the skill "start-here"
    Then the output shows "AGENTS.md" is present

  Scenario: start-here validates that Makefile is present
    Given the project root contains "Makefile"
    When the agent invokes the skill "start-here"
    Then the output shows "Makefile" is present

  Scenario: start-here reports skill count from .opencode/skills
    Given the project root contains ".opencode/skills/start-here/SKILL.md"
    When the agent invokes the skill "start-here"
    Then the output contains a skill inventory with at least 1 skill

  # ── skill listing ──────────────────────────────────────────────────────────

  Scenario: start-here lists available skills by name
    When the agent invokes the skill "start-here"
    Then the output lists "opencode-status-report" as an available skill
    And the output lists "demo-capabilities" as an available skill

  # ── quick start commands ───────────────────────────────────────────────────

  Scenario: start-here includes make ci in next steps
    When the agent invokes the skill "start-here"
    Then the output contains "make ci"

  # ── stability ──────────────────────────────────────────────────────────────

  Scenario: start-here output is stable across repeated invocations
    Given the skill "start-here" has completed successfully once
    When the agent invokes the skill "start-here" again with identical inputs
    Then the output is equivalent to the first run
    And no duplicate state files are created under "state/"
