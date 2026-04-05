# features/chaos-window.feature
# Next-steps record — chaos isotope prerequisite: window registration and classification
# Dependency: isotope-transit.feature (isotope_id must flow through /status first)
# Step definitions: features/step_definitions/chaos_window_steps.go (to be created)

@chaos @isotope @core
Feature: Chaos Window Registration and Isotope Classification
  As an operator or test harness running intentional failure scenarios
  I want to declare a time window during which a specific isotope is authorised
  to arrive with a failing probe result
  So that the ADHD dashboard can suppress false-alarm red lights during
  planned chaos experiments

  # The smoke-alarm is the authority on isotope classification. ADHD does not
  # consult its own clock or registry — it reads the classification from the
  # /status response. This prevents clock-skew and stale-state errors.

  Background:
    Given the ocd-smoke-alarm binary is installed
    And a target "t1" is configured and being probed

  # ── window registration ────────────────────────────────────────────────────

  Scenario: POST /isotope/register-chaos-window stores a window for a target
    When a POST request is sent to "/isotope/register-chaos-window" with:
      | target_id    | t1                        |
      | isotope_id   | iso-chaos-001             |
      | window_start | <now>                     |
      | window_end   | <now + 10 minutes>        |
    Then the response status is 200
    And the chaos window for "t1" / "iso-chaos-001" is stored

  Scenario: registering a window with a past window_end is rejected
    When a POST request is sent to "/isotope/register-chaos-window" with window_end in the past
    Then the response status is 400
    And the body contains "window_end must be in the future"

  Scenario: registering a window for an unknown target is rejected
    When a POST request is sent to "/isotope/register-chaos-window" for target "does-not-exist"
    Then the response status is 404
    And the body contains "target not found"

  Scenario: a registered window can be listed via GET /isotope/chaos-windows
    Given a chaos window is registered for "t1" / "iso-chaos-001"
    When a GET request is sent to "/isotope/chaos-windows"
    Then the response contains an entry for "t1" / "iso-chaos-001"
    And the entry includes window_start and window_end

  Scenario: expired windows are automatically removed from the registry
    Given a chaos window for "t1" / "iso-chaos-expired" with window_end 1 second ago
    When the smoke-alarm's cleanup cycle runs
    Then the window for "iso-chaos-expired" is no longer in the registry

  # ── isotope classification in /status ─────────────────────────────────────

  Scenario: isotope classified as "registered-chaos" when registered and in-window
    Given chaos window is registered for "t1" / "iso-chaos-001" with an active window
    When the probe for "t1" returns a failing response carrying isotope "iso-chaos-001"
    And a GET request is sent to "/status"
    Then the target "t1" has state "unhealthy"
    And the target "t1" has isotope_id "iso-chaos-001"
    And the target "t1" has isotope_classification "registered-chaos"

  Scenario: isotope classified as "expired-window" when window has elapsed
    Given a chaos window for "t1" / "iso-chaos-002" whose window_end has passed
    When the probe for "t1" returns a failing response carrying isotope "iso-chaos-002"
    And a GET request is sent to "/status"
    Then the target "t1" has isotope_classification "expired-window"
    And the target "t1" has state "unhealthy"

  Scenario: isotope classified as "unregistered" when no window exists
    Given no chaos window is registered for "t1" / "iso-unknown-001"
    When the probe for "t1" returns a failing response carrying isotope "iso-unknown-001"
    And a GET request is sent to "/status"
    Then the target "t1" has isotope_classification "unregistered"

  Scenario: healthy probe responses carry empty isotope_classification
    Given a chaos window is registered for "t1" / "iso-chaos-001"
    When the probe for "t1" returns a healthy response carrying isotope "iso-chaos-001"
    And a GET request is sent to "/status"
    Then the target "t1" has state "healthy"
    And the target "t1" has no isotope_classification field (or empty string)

  Scenario: isotope_classification is absent when no isotope_id was received
    Given no chaos window is registered for "t1"
    When the probe for "t1" returns a response with no isotope field
    And a GET request is sent to "/status"
    Then the target "t1" has no isotope_id field
    And the target "t1" has no isotope_classification field

  # ── classification is authoritative: ADHD must not re-derive it ───────────

  Scenario: classification is determined at the smoke-alarm not forwarded for ADHD to judge
    Given a chaos window registered for "t1" / "iso-chaos-001"
    When ADHD receives a LightUpdate for "t1" with isotope_classification "registered-chaos"
    Then ADHD suppresses the red-light transition without consulting its own window registry
    And ADHD does not send any follow-up request to /isotope to verify the classification

  # ── window isolation ────────────────────────────────────────────────────────

  Scenario: a chaos window for one target does not affect other targets
    Given a chaos window for "t1" / "iso-chaos-001"
    When the probe for "t2" returns a failing response carrying isotope "iso-chaos-001"
    And a GET request is sent to "/status"
    Then the target "t2" has isotope_classification "unregistered"
    And the target "t1"'s window is unaffected
