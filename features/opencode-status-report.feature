# features/opencode-status-report.feature
# Canon record — last audited: 2026-03-26
# Exercises: opencode-status-report skill — config analysis, dynamic config validation, markdown report
# Skill: .opencode/skills/opencode-status-report/SKILL.md
# Step definitions: features/step_definitions/opencode_status_report_steps.go

@opencode-status-report @skill
Feature: OpenCode Status Report Skill
  As a developer
  I want the opencode-status-report skill to generate a comprehensive status report of all configuration files
  So that I can quickly audit whether the project configuration is complete and valid

  Background:
    Given the ocd-smoke-alarm binary is installed
    And a Claude Code session is active in this repository

  # ── skill invocation ───────────────────────────────────────────────────────

  Scenario: agent invokes the opencode-status-report skill
    When the agent invokes the skill "opencode-status-report"
    Then the skill ".opencode/skills/opencode-status-report/SKILL.md" is read
    And the agent executes the documented steps in order

  # ── agent config file analysis ─────────────────────────────────────────────

  Scenario: report includes AGENTS.md status
    When the agent invokes the skill "opencode-status-report"
    Then the report includes the status of "AGENTS.md"

  Scenario: report includes .opencode.json status when file exists
    Given ".opencode.json" exists in the project root
    When the agent invokes the skill "opencode-status-report"
    Then the report includes the status of ".opencode.json"

  Scenario: report includes all discovered skills
    When the agent invokes the skill "opencode-status-report"
    Then the report lists each skill found under ".opencode/skills/"
    And each skill entry includes its validation status

  # ── dynamic configuration analysis ────────────────────────────────────────

  Scenario: report includes sample config file status
    Given sample config files exist under "configs/samples/"
    When the agent invokes the skill "opencode-status-report"
    Then the report includes at least one sample config entry

  Scenario: report validates sample config YAML syntax
    Given the file "configs/sample.yaml" exists
    When the agent invokes the skill "opencode-status-report"
    Then the report shows "configs/sample.yaml" as valid

  # ── validation summary ─────────────────────────────────────────────────────

  Scenario: report includes a validation summary section
    When the agent invokes the skill "opencode-status-report"
    Then the report contains "Total files checked"
    And the report contains "Passed"
    And the report contains "Failed"

  Scenario: report flags a missing required config file
    Given "AGENTS.md" does not exist in the project root
    When the agent invokes the skill "opencode-status-report"
    Then the report shows "AGENTS.md" as missing
    And the validation summary failed count is at least 1

  # ── stability ──────────────────────────────────────────────────────────────

  Scenario: opencode-status-report output is stable across repeated invocations
    Given the skill "opencode-status-report" has completed successfully once
    When the agent invokes the skill "opencode-status-report" again with identical inputs
    Then the output is equivalent to the first run
    And no duplicate state files are created under "state/"
