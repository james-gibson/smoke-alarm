# features/warroom-simulator.feature
# Canon record — last audited: 2026-03-26
# Exercises: warroom-simulator skill — interactive simulation, /join /resolve commands, escalation timers, post-sim feedback
# Skill: .opencode/skills/warroom-simulator/SKILL.md
# see: features/warroom-guide.feature (tutorial vs. live simulation distinction)
# Step definitions: features/step_definitions/warroom_simulator_steps.go

@warroom-simulator @skill
Feature: Warroom Simulator Skill
  As an incident response trainee
  I want the warroom-simulator skill to run a live interactive simulation
  So that I can practice incident response under realistic time pressure

  Background:
    Given the ocd-smoke-alarm binary is installed
    And a Claude Code session is active in this repository

  # ── skill invocation ───────────────────────────────────────────────────────

  Scenario: agent invokes the warroom-simulator skill
    When the agent invokes the skill "warroom-simulator"
    Then the skill ".opencode/skills/warroom-simulator/SKILL.md" is read
    And the agent executes the documented steps in order

  # ── simulation startup ─────────────────────────────────────────────────────

  Scenario: warroom-simulator presents a ready prompt before the simulation starts
    Given a warroom-simulator session is configured with scenario "security-breach" and severity "SEV1"
    When the simulation is loaded but not yet started
    Then the output contains "INCIDENT SIMULATION READY"
    And the output contains a "/start" prompt

  Scenario: warroom-simulator starts the simulation when the /start command is issued
    Given a warroom-simulator session is configured and ready
    When the user issues the "/start" command
    Then the simulation status changes to "ACTIVE"
    And the incident elapsed timer begins
    And participants are called with join codes

  # ── /join command ──────────────────────────────────────────────────────────

  Scenario: /join with a valid code adds the participant to the bridge
    Given a warroom-simulator session is active with "L1-OnCall" code "JOIN-L1-7K2M9"
    When the user issues "/join L1-7K2M9"
    Then the participant "L1-OnCall" is marked as joined
    And the response time for "L1-OnCall" is recorded

  Scenario: /join with an invalid code returns an error
    Given a warroom-simulator session is active
    When the user issues "/join INVALID-CODE"
    Then the simulator responds with an invalid code message
    And no participant is marked as joined

  # ── /resolve command ───────────────────────────────────────────────────────

  Scenario: /resolve with a valid alarm ID resolves the alarm
    Given a warroom-simulator session with P1 alarm "p1-unauthorized-access" active
    When the user issues "/resolve p1-unauthorized-access Blocked IPs at firewall"
    Then the alarm "p1-unauthorized-access" is marked as resolved
    And the resolution time is recorded
    And P2 is unblocked

  Scenario: /resolve is rejected for a P2 alarm when P1 is still active
    Given a warroom-simulator session with P1 and P2 alarms both active
    When the user issues "/resolve p2-database-latency"
    Then the simulator rejects the command
    And the response states that P1 must be resolved first

  # ── /status command ────────────────────────────────────────────────────────

  Scenario: /status displays current incident elapsed time and alarm state
    Given a warroom-simulator session is active
    When the user issues "/status"
    Then the output includes elapsed time
    And the output includes a participant join table
    And the output includes an alarm status table

  # ── escalation ────────────────────────────────────────────────────────────

  Scenario: escalation is triggered when a participant join timeout expires
    Given a warroom-simulator session with "L1-OnCall" join timeout of 10 seconds
    When 10 seconds pass without "L1-OnCall" joining
    Then the simulator triggers an escalation
    And a new join code is issued for the escalated role

  Scenario: escalation is triggered when an alarm dismiss timeout expires
    Given a warroom-simulator session with P1 alarm and "max_dismiss_time" of 5 minutes
    When the P1 alarm has been active for 5 minutes without resolution
    Then the simulator escalates to the next tier
    And the escalation event is recorded

  # ── /pause and /resume ────────────────────────────────────────────────────

  Scenario: /pause halts escalation timers
    Given a warroom-simulator session is active with an alarm timer running
    When the user issues "/pause"
    Then the simulation status changes to "PAUSED"
    And alarm timers stop advancing

  Scenario: /resume restarts timers from where they paused
    Given a warroom-simulator session is paused
    When the user issues "/resume"
    Then the simulation status changes to "ACTIVE"
    And alarm timers resume from their paused values

  # ── post-simulation feedback ───────────────────────────────────────────────

  Scenario: simulator generates a performance summary when all alarms are resolved
    Given a warroom-simulator session where all alarms have been resolved
    Then the simulator displays a performance summary
    And the summary includes time-to-first-participant
    And the summary includes per-alarm resolution time
    And the summary includes improvement suggestions

  # ── /abort ────────────────────────────────────────────────────────────────

  Scenario: /abort ends the simulation immediately
    Given a warroom-simulator session is active
    When the user issues "/abort"
    Then the simulation ends immediately
    And a partial summary is displayed
