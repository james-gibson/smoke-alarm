# features/warroom-guide.feature
# Canon record — last audited: 2026-03-26
# Exercises: warroom-guide skill — incident bridge setup, participant management, alarm dismissal order, escalation
# Skill: .opencode/skills/warroom-guide/SKILL.md
# Step definitions: features/step_definitions/warroom_guide_steps.go

@warroom-guide @skill
Feature: Warroom Guide Skill
  As a devops trainer running an incident simulation
  I want the warroom-guide skill to act as an automated incident commander
  So that participants experience realistic bridge management, alarm sequencing, and escalation protocols

  Background:
    Given the ocd-smoke-alarm binary is installed
    And a Claude Code session is active in this repository

  # ── skill invocation ───────────────────────────────────────────────────────

  Scenario: agent invokes the warroom-guide skill
    When the agent invokes the skill "warroom-guide"
    Then the skill ".opencode/skills/warroom-guide/SKILL.md" is read
    And the agent executes the documented steps in order

  # ── incident initialisation ────────────────────────────────────────────────

  Scenario: warroom-guide prompts for incident scenario when none is provided
    Given no incident scenario has been provided
    When the agent invokes the skill "warroom-guide"
    Then the output includes a scenario selection prompt
    And the output lists "service-outage" as an option
    And the output lists "security-breach" as an option

  Scenario: warroom-guide generates a unique incident ID on each run
    When the agent invokes the skill "warroom-guide" with scenario "service-outage" and severity "SEV1"
    Then the output contains an incident ID matching the pattern "INC-YYYY-MM-DD-NNN"

  # ── bridge display ─────────────────────────────────────────────────────────

  Scenario: warroom-guide displays bridge status with severity and participant table
    Given a warroom-guide session is active with scenario "service-outage" and severity "SEV1"
    Then the bridge status shows severity "SEV1"
    And the bridge status shows a participant table with join codes
    And all initial participants have status "PENDING"

  # ── participant joining ────────────────────────────────────────────────────

  Scenario: warroom-guide marks a participant as joined when they provide a valid code
    Given a warroom-guide session is active with participant "L1-OnCall" requested
    When the participant provides the join code for "L1-OnCall"
    Then the participant "L1-OnCall" status changes to "joined"
    And the join response time is recorded

  Scenario: warroom-guide rejects an invalid join code
    Given a warroom-guide session is active with participant "L1-OnCall" requested
    When a participant provides an incorrect join code
    Then the warroom-guide responds with an invalid code message

  # ── alarm dismissal order ──────────────────────────────────────────────────

  Scenario: warroom-guide presents alarms in priority order
    Given a warroom-guide session with alarms P1, P2, and P3 defined
    Then the P1 alarm is presented before P2
    And the P2 alarm is presented before P3

  Scenario: warroom-guide blocks P2 dismissal until P1 is resolved
    Given a warroom-guide session with P1 and P2 alarms active
    When a participant attempts to resolve P2 before P1
    Then the warroom-guide rejects the P2 dismissal
    And the response states P1 must be resolved first

  Scenario: warroom-guide accepts a P1 dismissal with a resolution note
    Given a warroom-guide session with P1 alarm active
    When a participant resolves P1 with resolution note "Root cause identified and fixed"
    Then the P1 alarm status changes to "resolved"
    And the resolution time is recorded

  # ── escalation ────────────────────────────────────────────────────────────

  Scenario: warroom-guide escalates when an alarm exceeds its dismiss timeout
    Given a warroom-guide session with P1 alarm and "max_dismiss_time_minutes" of 5
    When the P1 alarm has been active for 5 minutes without resolution
    Then the warroom-guide triggers escalation to the next role
    And a new participant is requested with an escalation join code

  # ── resolution summary ─────────────────────────────────────────────────────

  Scenario: warroom-guide generates a resolution summary when all alarms are dismissed
    Given a warroom-guide session with P1 and P2 alarms
    When both P1 and P2 are resolved by participants
    Then the warroom-guide displays an incident summary
    And the summary includes total duration
    And the summary includes per-alarm resolution time
