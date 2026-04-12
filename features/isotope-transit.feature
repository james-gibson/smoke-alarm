# features/isotope-transit.feature
# Next-steps record — chaos isotope prerequisite: isotope IDs must flow through /status
# Dependency: none — this is the foundational prerequisite for all chaos isotope work
# Step definitions: features/step_definitions/isotope_transit_steps.go (to be created)

@isotope @core
Feature: Isotope ID Transit Through Health Check Responses
  As an ADHD dashboard consuming smoke-alarm /status
  I want each target's health-check result to carry the isotope ID that arrived
  with that probe response
  So that ADHD can match the isotope against a registered chaos window or flag
  an unregistered isotope as a scope-compliance failure

  # This is the foundational prerequisite for chaos isotopes.
  # Until isotope_id appears in TargetStatus, no downstream classification
  # or window-matching logic can function.

  Background:
    Given the ocd-smoke-alarm binary is installed
    And a target is configured with a health-check endpoint that returns isotope IDs

  # ── isotope field on TargetStatus ─────────────────────────────────────────

  Scenario: health-check response carrying an isotope ID is reflected in /status
    Given a target "t1" whose probe endpoint returns an isotope ID "iso-abc-001"
    When the smoke-alarm polls "t1" and receives a healthy response
    And a GET request is sent to "/status"
    Then the target "t1" in the response has "isotope_id" equal to "iso-abc-001"

  Scenario: /status omits isotope_id when the probe response carries none
    Given a target "t2" whose probe endpoint returns no isotope field
    When the smoke-alarm polls "t2"
    And a GET request is sent to "/status"
    Then the target "t2" in the response has no "isotope_id" field
    And no empty string is emitted for "isotope_id"

  Scenario: isotope_id is updated on each probe cycle
    Given a target "t1" whose probe endpoint cycles through different isotope IDs
    When the smoke-alarm polls "t1" and receives isotope "iso-abc-001"
    Then /status shows isotope_id "iso-abc-001" for "t1"
    When the smoke-alarm polls "t1" again and receives isotope "iso-abc-002"
    Then /status shows isotope_id "iso-abc-002" for "t1"

  Scenario: isotope_id from a failing probe is still recorded
    Given a target "t1" whose probe endpoint returns state "unhealthy" and isotope "iso-fail-001"
    When the smoke-alarm polls "t1"
    And a GET request is sent to "/status"
    Then the target "t1" has state "unhealthy"
    And the target "t1" has isotope_id "iso-fail-001"

  # ── isotope payload is not stored ─────────────────────────────────────────
  # Only the isotope ID transits — not the probe response body that produced it.
  # This is a privacy boundary: ADHD must not be able to recover the payload.

  Scenario: the probe response body is not present in /status
    Given a target "t1" whose probe endpoint returns a rich JSON body with an isotope ID
    When the smoke-alarm polls "t1"
    And a GET request is sent to "/status"
    Then the /status response contains "isotope_id" for "t1"
    And the /status response does not contain any field from the probe response body
    And no probe response field appears in "message", "details", or any other TargetStatus field

  # ── isotope_id construction ────────────────────────────────────────────────
  # isotope_id = base64url(SHA256(feature_id || ":" || SHA256(payload) || ":" || nonce))
  # This is the canonical construction algorithm. The smoke-alarm does not
  # construct isotopes — it receives them and records their transit.

  Scenario: smoke-alarm can verify a received isotope against its declared feature binding
    Given isotope "iso-abc-001" was constructed from feature "adhd/light-transitions-dark-to-green", a payload hash, and a nonce
    When the smoke-alarm receives that isotope in a probe response for target "t1"
    Then the smoke-alarm records: isotope "iso-abc-001" observed at target "t1"
    And the recorded transit does not include the payload or nonce

  Scenario: two isotope IDs for the same feature and payload are different
    Given feature "adhd/light-transitions-dark-to-green" and a fixed probe payload
    When two isotope IDs are constructed for that feature and payload
    Then the two IDs are not equal
    And each ID is 43 base64url characters

  # ── SSE transport carries isotope_id ──────────────────────────────────────

  Scenario: SSE status event includes isotope_id when the probe returned one
    Given a target "t1" configured with use_sse=true
    And the probe endpoint returns isotope "iso-sse-001"
    When the smoke-alarm pushes a status event over the SSE stream
    Then the event data contains "isotope_id" equal to "iso-sse-001"

  Scenario: SSE event omits isotope_id when the probe carried none
    Given a target "t2" configured with use_sse=true
    And the probe endpoint returns no isotope field
    When the smoke-alarm pushes a status event
    Then the SSE event data has no "isotope_id" field
