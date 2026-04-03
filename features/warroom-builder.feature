# features/warroom-builder.feature
# Canon record — last audited: 2026-03-26
# Exercises: warroom-builder skill — incident parameter collection, YAML config generation, incident script generation
# Skill: .opencode/skills/warroom-builder/SKILL.md
# Step definitions: features/step_definitions/warroom_builder_steps.go

@warroom-builder @skill
Feature: Warroom Builder Skill
  As a devops trainer or incident commander
  I want the warroom-builder skill to generate complete warroom configuration YAML and incident scripts from structured input
  So that I can quickly set up realistic incident training scenarios

  Background:
    Given the ocd-smoke-alarm binary is installed
    And a Claude Code session is active in this repository

  # ── skill invocation ───────────────────────────────────────────────────────

  Scenario: agent invokes the warroom-builder skill
    When the agent invokes the skill "warroom-builder"
    Then the skill ".opencode/skills/warroom-builder/SKILL.md" is read
    And the agent executes the documented steps in order

  # ── parameter collection ────────────────────────────────────────────────────

  Scenario: warroom-builder prompts for incident type when not provided
    Given no incident parameters have been provided
    When the agent invokes the skill "warroom-builder"
    Then the output includes a questionnaire requesting incident type
    And the output lists "service-outage" as an incident type option
    And the output lists "security-incident" as an incident type option

  Scenario: warroom-builder prompts for severity level
    Given no incident parameters have been provided
    When the agent invokes the skill "warroom-builder"
    Then the output includes severity options "SEV1", "SEV2", and "SEV3"

  # ── YAML config generation ─────────────────────────────────────────────────

  Scenario: warroom-builder generates a valid YAML config from SEV1 parameters
    Given incident parameters with type "security-breach" and severity "SEV1"
    And participants "L1-OnCall", "Security", and "L2-Lead" are required
    When the agent invokes the skill "warroom-builder" with those parameters
    Then a YAML config is generated with version "1"
    And the YAML config contains a "targets" block with one entry per participant
    And the YAML config contains an "alerts" block with "aggressive: true"

  Scenario: warroom-builder sets poll_interval based on severity
    Given incident parameters with severity "SEV1"
    When the agent invokes the skill "warroom-builder" with those parameters
    Then the generated YAML config has a poll_interval of "2s"

  Scenario: warroom-builder sets poll_interval based on severity
    Given incident parameters with severity "SEV3"
    When the agent invokes the skill "warroom-builder" with those parameters
    Then the generated YAML config has a poll_interval of "10s"

  Scenario: warroom-builder generates an ephemeral state config
    Given incident parameters with type "service-outage" and severity "SEV2"
    When the agent invokes the skill "warroom-builder" with those parameters
    Then the generated YAML config has "persist: false" under known_state

  # ── incident script generation ─────────────────────────────────────────────

  Scenario: warroom-builder generates an incident script alongside the YAML config
    Given incident parameters with type "service-outage" and severity "SEV1"
    When the agent invokes the skill "warroom-builder" with those parameters
    Then an incident script is generated with a participant join-code table
    And the script contains an alarm sequence section

  Scenario: warroom-builder generates unique join codes per simulation run
    Given incident parameters with type "security-breach" and severity "SEV1"
    When the skill "warroom-builder" is invoked twice with identical parameters
    Then the join codes in the second run differ from the first run

  # ── output file locations ───────────────────────────────────────────────────

  Scenario: warroom-builder places generated YAML under configs/samples
    Given incident parameters with type "service-outage" and severity "SEV2"
    When the agent invokes the skill "warroom-builder" with those parameters
    Then the output identifies the generated config path as starting with "configs/samples/warroom-"

  # ── stability ──────────────────────────────────────────────────────────────

  Scenario: warroom-builder surfaces a missing required file
    Given the required file "configs/samples/federation-star-hub.yaml" does not exist
    When the agent invokes the skill "warroom-builder"
    Then the output identifies the missing file "configs/samples/federation-star-hub.yaml"
    And the output suggests a remediation action
