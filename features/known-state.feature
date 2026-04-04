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

  # ── isotope transit and feature mapping ──────────────────────────────────
  # Probe results carry isotopes. Recording a result records the isotope transit —
  # never the response payload. Each isotope maps to the Gherkin scenario it
  # represents; the transit is live evidence of scenario coverage.

  Scenario: recording a probe result emits an isotope transit event, not payload data
    Given a probe result for target "t1" carrying isotope "isotope-probe-healthy-001"
    When the result is recorded
    Then a transit event is emitted: isotope "isotope-probe-healthy-001" at point "known-state"
    And no response body or payload data from the probe appears in the transit event

  Scenario: an isotope-tagged probe result contributes to feature coverage
    Given isotope "isotope-probe-healthy-001" is declared as corresponding to the scenario "sustain threshold met"
    When a healthy probe result tagged with that isotope is recorded
    Then the scenario "sustain threshold met" is marked as covered in the live coverage report

  Scenario: probe results without isotopes do not contribute to feature coverage
    Given a probe result for target "t1" with no isotope attached
    When the result is recorded
    Then the result updates the known-state store normally
    And no feature coverage entry is created

  # ── isotope transit as 42i boundary authorization ─────────────────────────
  # A transit declaration (feature_id → component) is a boundary authorization.
  # Violations raise the agent's 42i distance via smoke-alarm test dimensions.

  Scenario: an isotope transiting its declared boundary clears scope-compliance for that dimension
    Given isotope "isotope-probe-healthy-001" is declared for feature "known-state/sustain-threshold-met"
    When the isotope transits the known-state component
    Then the scope-compliance test passes for this dimension
    And no 42i distance is added

  Scenario: an isotope arriving at an undeclared boundary is a scope-compliance failure
    Given isotope "isotope-probe-healthy-001" is declared for feature "known-state/sustain-threshold-met"
    When the isotope is observed at the routing component instead
    Then a scope-compliance failure is recorded
    And the agent's 42i distance increases by 20 units

  Scenario: a replayed isotope ID in the known-state layer is an isotope-variation failure
    Given isotope "isotope-probe-healthy-001" has already been recorded in this window
    When the same isotope ID is submitted a second time
    Then an isotope-variation failure is recorded
    And the agent's 42i distance increases by 8 units

  Scenario: probe payload data appearing in the transit event is a secret-flow-violation
    Given a transit event is emitted for isotope "isotope-probe-healthy-001"
    When the event contains any field from the probe response body
    Then a secret-flow-violation is recorded
    And the agent's 42i distance increases by 24 units

  # ── isotope ID construction properties ────────────────────────────────────
  # isotope_id = base64url( SHA256( feature_id || ":" || SHA256(payload) || ":" || nonce ) )

  Scenario: two isotope IDs for the same feature and payload are not equal
    Given feature "known-state/sustain-threshold-met" and a fixed probe payload
    When two isotope IDs are constructed for the same feature and payload
    Then the two IDs are different
    And each is 43 base64url characters

  Scenario: a holder of feature_id, payload, and nonce can verify the isotope ID
    Given isotope "isotope-probe-healthy-001" was constructed from feature "known-state/sustain-threshold-met", a payload, and a nonce
    When verification is performed with those three inputs
    Then verification succeeds

  Scenario: an isotope ID cannot be verified against a different feature
    Given isotope "isotope-probe-healthy-001" was constructed for feature "known-state/sustain-threshold-met"
    When verification is attempted with feature "known-state/regression-detected"
    Then verification fails

  Scenario: no payload information is recoverable from the isotope ID alone
    Given isotope "isotope-probe-healthy-001"
    When an attempt is made to extract the probe payload from the ID
    Then no payload information is recoverable

  # ── chaos isotope guard ───────────────────────────────────────────────────
  #
  # A chaos-isotope-tagged probe result is an expected test event.
  # It must not alter the known-good baseline: ever_healthy, last_healthy_at,
  # consecutive_failures, and the regression flag must all be preserved.
  # The proof that it is safe to discard: a real failure cannot produce a
  # pre-registered chaos isotope.

  Scenario: a chaos-isotope-tagged failure does not increment consecutive_failures
    Given the store has target "t1" with ever_healthy true and consecutive_failures 0
    And isotope "isotope-chaos-001" is registered as a chaos isotope with an active window
    When a degraded probe result tagged with isotope "isotope-chaos-001" is recorded for "t1"
    Then the target "t1" consecutive_failures is 0

  Scenario: a chaos-isotope-tagged failure does not set is_regression
    Given the store has target "t1" with ever_healthy true
    And isotope "isotope-chaos-001" is registered as a chaos isotope with an active window
    When a degraded probe result tagged with isotope "isotope-chaos-001" is recorded for "t1"
    Then the update result is_regression is false

  Scenario: a chaos-isotope-tagged failure does not update last_healthy_at
    Given the store has target "t1" with ever_healthy true and a known last_healthy_at
    And isotope "isotope-chaos-001" is registered as a chaos isotope with an active window
    When a degraded probe result tagged with isotope "isotope-chaos-001" is recorded for "t1"
    Then the target "t1" last_healthy_at is unchanged

  Scenario: a chaos-isotope-tagged failure does not reset the sustain-success streak
    Given the store has target "t1" with success_streak 2
    And isotope "isotope-chaos-001" is registered as a chaos isotope with an active window
    When a degraded probe result tagged with isotope "isotope-chaos-001" is recorded for "t1"
    Then the target "t1" success_streak is 2

  Scenario: an unregistered isotope on a failure is treated as a real failure regardless
    Given the store has target "t1" with ever_healthy true
    When a degraded probe result tagged with an unregistered isotope is recorded for "t1"
    Then the target "t1" consecutive_failures is 1
    And the update result is_regression is true

  Scenario: a chaos isotope arriving after its window has closed is treated as a real failure
    Given the store has target "t1" with ever_healthy true
    And "isotope-chaos-002" was registered with a window that has now expired
    When a degraded probe result tagged with isotope "isotope-chaos-002" is recorded for "t1"
    Then the target "t1" consecutive_failures is 1
    And the update result is_regression is true

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
