# features/known-state.feature
# Canon record — last audited: 2026-03-26
# Exercises: knownstate.Store — sustain-success gate, regression classification,
#            consecutive-failure counting, persistence, reset, and status predicates
# Code: internal/knownstate/store.go
# Step definitions: features/step_definitions/known_state_steps.go
# see: features/engine.feature (engine calls Store.Update and maps IsRegression → StateRegression)
# see: features/alerts.feature (REGRESSION flag on AlertEvent comes from engine, not store)
#
# NOTE: "regression" is NOT a knownstate.Status value.
# Valid Status values: healthy, degraded, outage, failed, unknown.
# IsRegression is a boolean on UpdateResult, set when EverHealthy && IsFailure(status).
# The engine promotes this to targets.StateRegression — that escalation lives in engine.feature.

@known-state @core
Feature: Known-State Regression Detection
  As an operator
  I want ocd-smoke-alarm to distinguish new failures from regressions against a healthy baseline
  So that I am alerted immediately when a previously-healthy target begins failing

  Background:
    Given the ocd-smoke-alarm binary is installed
    And a known-state store is initialized with path "state/known-good.json"

  # ── status predicates ─────────────────────────────────────────────────────

  Scenario Outline: IsHealthy returns true only for the healthy status
    Given a status value "<status>"
    When I call IsHealthy on the status
    Then IsHealthy returns <expected>

    Examples:
      | status   | expected |
      | healthy  | true     |
      | degraded | false    |
      | outage   | false    |
      | failed   | false    |
      | unknown  | false    |

  Scenario Outline: IsFailure returns true for degraded, outage, and failed
    Given a status value "<status>"
    When I call IsFailure on the status
    Then IsFailure returns <expected>

    Examples:
      | status   | expected |
      | degraded | true     |
      | outage   | true     |
      | failed   | true     |
      | healthy  | false    |
      | unknown  | false    |

  # ── sustain-success gate ──────────────────────────────────────────────────

  Scenario: a target is not marked ever-healthy until the sustain threshold is met
    Given sustain_success is set to 2
    When 1 healthy probe result is recorded for target "t1"
    Then the target "t1" ever_healthy is false

  Scenario: a target is marked ever-healthy after reaching the sustain threshold
    Given sustain_success is set to 2
    When 2 consecutive healthy probe results are recorded for target "t1"
    Then the target "t1" ever_healthy is true
    And the target "t1" last_healthy_at is set

  Scenario: a sustain threshold of 1 marks ever-healthy on the first healthy result
    Given sustain_success is set to 1
    When 1 healthy probe result is recorded for target "t1"
    Then the target "t1" ever_healthy is true

  Scenario: an intervening failure resets the success streak
    Given sustain_success is set to 3
    When 2 consecutive healthy probe results are recorded for target "t1"
    And 1 degraded probe result is recorded for target "t1"
    And 1 healthy probe result is recorded for target "t1"
    Then the target "t1" success_streak is 1
    And the target "t1" ever_healthy is false

  # ── regression classification ─────────────────────────────────────────────

  Scenario: a failure after ever-healthy is classified as a regression
    Given the store has target "t1" with ever_healthy true
    When a degraded probe result is recorded for target "t1"
    Then the update result is_regression is true

  Scenario: a failure before ever-healthy is not classified as a regression
    Given the store has target "t1" with ever_healthy false
    When a degraded probe result is recorded for target "t1"
    Then the update result is_regression is false

  Scenario Outline: any failure status after ever-healthy triggers regression
    Given the store has target "t1" with ever_healthy true
    When a "<status>" probe result is recorded for target "t1"
    Then the update result is_regression is <expected>

    Examples:
      | status   | expected |
      | degraded | true     |
      | outage   | true     |
      | failed   | true     |
      | unknown  | false    |

  Scenario: recovering to healthy clears the regression flag
    Given the store has target "t1" with ever_healthy true
    And a degraded probe result has been recorded for target "t1"
    When a healthy probe result is recorded for target "t1"
    Then the update result became_healthy is true
    And the update result is_regression is false

  # ── transition fields ─────────────────────────────────────────────────────

  Scenario: first update for a target has had_previous false
    Given the store has no entry for target "t1"
    When a healthy probe result is recorded for target "t1"
    Then the update result had_previous is false

  Scenario: second update for a target has had_previous true
    Given a healthy probe result has been recorded for target "t1"
    When a healthy probe result is recorded for target "t1"
    Then the update result had_previous is true

  Scenario: transition to unhealthy sets became_unhealthy
    Given the store has target "t1" with ever_healthy true
    When a degraded probe result is recorded for target "t1"
    Then the update result became_unhealthy is true

  Scenario: transition to healthy from unhealthy sets became_healthy
    Given the store has target "t1" with ever_healthy true
    And a degraded probe result has been recorded for target "t1"
    When a healthy probe result is recorded for target "t1"
    Then the update result became_healthy is true

  # ── consecutive failure counting ──────────────────────────────────────────

  Scenario: consecutive_failures increments on each failure
    When 3 degraded probe results are recorded for target "t1"
    Then the target "t1" consecutive_failures is 3

  Scenario: consecutive_failures resets to zero on a healthy result
    Given 3 degraded probe results have been recorded for target "t1"
    When a healthy probe result is recorded for target "t1"
    Then the target "t1" consecutive_failures is 0

  Scenario: unknown status does not increment consecutive_failures
    When a "unknown" probe result is recorded for target "t1"
    Then the target "t1" consecutive_failures is 0

  Scenario: success_streak resets to zero on any non-healthy result
    Given 2 consecutive healthy probe results have been recorded for target "t1"
    When a degraded probe result is recorded for target "t1"
    Then the target "t1" success_streak is 0

  # ── empty target ID guard ──────────────────────────────────────────────────

  Scenario: Update with empty target ID returns an error
    When a probe result is recorded for target ""
    Then an error is returned containing "target id is required"

  # ── empty status normalisation ─────────────────────────────────────────────

  Scenario: Update with empty status treats it as unknown
    When a probe result with no status is recorded for target "t1"
    Then the target "t1" current_status is "unknown"
    And the target "t1" consecutive_failures is 0

  # ── Get ───────────────────────────────────────────────────────────────────

  Scenario: Get returns the current state for a known target
    Given a healthy probe result has been recorded for target "t1"
    When I call Get for target "t1"
    Then the returned state ever_healthy matches the stored value

  Scenario: Get returns not-found for an unknown target
    When I call Get for target "does-not-exist"
    Then the returned found flag is false

  # ── persistence ───────────────────────────────────────────────────────────

  Scenario: Save writes snapshot atomically via temp-file rename
    When a healthy probe result is recorded for target "t1"
    And I call Save explicitly
    Then the file "state/known-good.json" exists
    And no "state/known-good.json.tmp" file remains

  Scenario: auto-persist writes to disk after each Update
    Given the store is initialized with auto_persist true
    When a healthy probe result is recorded for target "t1"
    Then the file "state/known-good.json" exists

  Scenario: Load reads existing snapshot and restores ever_healthy
    Given a snapshot file exists at "state/known-good.json" with target "t1" marked ever_healthy
    When the store loads from disk
    Then the target "t1" ever_healthy is true

  Scenario: Load on missing file succeeds with empty target map
    Given no file exists at "state/known-good.json"
    When the store loads from disk
    Then no error is returned
    And the target map is empty

  Scenario: Snapshot returns a deep copy — mutations do not affect store
    Given a healthy probe result has been recorded for target "t1"
    When I call Snapshot and mutate the returned map
    Then the store's internal target "t1" is unchanged

  # ── reset ─────────────────────────────────────────────────────────────────

  Scenario: Reset with delete_file false clears memory and writes empty snapshot
    Given the store has target "t1" with ever_healthy true
    When I call Reset with delete_file false
    Then the target "t1" ever_healthy is false
    And the file "state/known-good.json" exists with an empty targets map

  Scenario: Reset with delete_file true clears memory and removes the file
    Given the store has target "t1" with ever_healthy true
    And the file "state/known-good.json" exists
    When I call Reset with delete_file true
    Then the target map is empty
    And the file "state/known-good.json" does not exist
