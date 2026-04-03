# features/config-to-simulation.feature
# Canon record — last audited: 2026-03-26
# Exercises: config-to-simulation skill — config parsing, scenario generation, warroom voice, mentor voice
# Skill: .opencode/skills/config-to-simulation/SKILL.md
# see: features/warroom-builder.feature (builds config; this skill consumes it)
# see: features/warroom-simulator.feature (runs the simulation output)
# Step definitions: features/step_definitions/config_to_simulation_steps.go

@config-to-simulation @skill
Feature: Config to Simulation Skill
  As a devops trainer or educator
  I want the config-to-simulation skill to transform a YAML config file into an interactive incident simulation
  So that I can immediately run training sessions from existing configs without manual scenario construction

  Background:
    Given the ocd-smoke-alarm binary is installed
    And a Claude Code session is active in this repository

  # ── skill invocation ───────────────────────────────────────────────────────

  Scenario: agent invokes the config-to-simulation skill
    When the agent invokes the skill "config-to-simulation"
    Then the skill ".opencode/skills/config-to-simulation/SKILL.md" is read
    And the agent executes the documented steps in order

  Scenario: skill surfaces a missing required file
    Given the required file "configs/samples/federation-star-hub.yaml" does not exist
    When the agent invokes the skill "config-to-simulation"
    Then the output identifies the missing file "configs/samples/federation-star-hub.yaml"
    And the output suggests a remediation action

  # ── config analysis ────────────────────────────────────────────────────────

  Scenario: skill parses targets as simulation participants
    Given a valid config file "configs/samples/federation-star-hub.yaml" exists
    When the agent invokes "config-to-simulation" with config "configs/samples/federation-star-hub.yaml"
    Then each target in the config is listed as a simulation participant
    And each participant has a join code

  Scenario: skill derives escalation speed from poll_interval
    Given a config with poll_interval "2s"
    When the agent invokes "config-to-simulation" with that config
    Then the simulation escalation speed reflects the "2s" poll interval

  Scenario: skill identifies federation topology from federation config block
    Given a config with federation.enabled true
    When the agent invokes "config-to-simulation" with that config
    Then the simulation scenario includes a federation topology description

  # ── warroom voice ──────────────────────────────────────────────────────────

  Scenario: warroom voice output uses urgent command language
    Given a valid config file "configs/samples/federation-star-hub.yaml" exists
    When the agent invokes "config-to-simulation" with config and voice "warroom"
    Then the output uses urgent imperative language
    And the output contains a participant join-code table

  Scenario: warroom voice sets aggressive alerts when config has alerts.aggressive true
    Given a config with "alerts.aggressive: true"
    When the agent invokes "config-to-simulation" with that config and voice "warroom"
    Then the simulation is labelled as "SEV1-level urgency"

  # ── mentor voice ───────────────────────────────────────────────────────────

  Scenario: mentor voice output uses explanatory language
    Given a valid config file "configs/samples/federation-star-hub.yaml" exists
    When the agent invokes "config-to-simulation" with config and voice "mentor"
    Then the output uses explanatory question-based language
    And the output contains at least one learning question

  Scenario: mentor voice presents multiple failure scenarios to choose from
    Given a valid config file "configs/samples/federation-star-hub.yaml" exists
    When the agent invokes "config-to-simulation" with config and voice "mentor"
    Then the output presents at least 2 named failure scenarios for the user to choose

  # ── voice selection ────────────────────────────────────────────────────────

  Scenario: skill prompts for voice mode when not specified
    Given a valid config file "configs/sample.yaml" exists
    When the agent invokes "config-to-simulation" with config and no voice specified
    Then the output prompts for voice selection
    And the output offers "warroom" and "mentor" as options

  Scenario: skill outputs both voices when "both" is requested
    Given a valid config file "configs/sample.yaml" exists
    When the agent invokes "config-to-simulation" with config and voice "both"
    Then the output contains a warroom section
    And the output contains a mentor section

  # ── auto-detection ─────────────────────────────────────────────────────────

  Scenario Outline: config pattern auto-detects the incident type
    Given a valid config file "<config>" exists
    When the agent invokes "config-to-simulation" with that config and no voice specified
    Then the detected scenario type is "<scenario>"

    Examples:
      | config                                           | scenario           |
      | configs/samples/federation-star-hub.yaml         | federation failure |
      | configs/samples/oauth-mock-fail.yaml             | auth incident      |

  # ── stability ──────────────────────────────────────────────────────────────

  Scenario: config-to-simulation output is stable across repeated invocations
    Given the skill "config-to-simulation" has completed successfully once
    When the agent invokes the skill "config-to-simulation" again with identical inputs
    Then the output is equivalent to the first run
    And no duplicate state files are created under "state/"
