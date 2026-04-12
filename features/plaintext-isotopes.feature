# features/plaintext-isotopes.feature
# Next-steps record — plain text isotope stand-ins for dev/demo mode
# Dependency: isotope-transit.feature (isotope_id must flow through /status)
# Dependency: isotope-construction.feature (canonical algorithm — this is a dev bypass)
# Step definitions: features/step_definitions/plaintext_isotope_steps.go (to be created)
#
# Plain text stand-ins allow the full isotope transit pipeline to be exercised
# before the cryptographic construction algorithm is implemented or in environments
# where key management is not yet in place (local dev, demo cluster).
#
# Format: "plaintext:<feature-id>:<sequential-id>"
# Example: "plaintext:adhd/light-transitions-dark-to-green:001"
#
# Plain text isotopes are ONLY accepted when the smoke-alarm is configured in
# dev/demo mode (allow_plaintext_isotopes: true). Production instances reject them.

@plaintext-isotopes @dev-mode
Feature: Plain Text Isotope Stand-Ins for Transit Testing
  As a developer testing the isotope transit pipeline
  I want to use human-readable isotope stand-ins instead of cryptographic IDs
  So that I can verify end-to-end plumbing (probe → /status → ADHD) before
  implementing the cryptographic construction algorithm

  Background:
    Given the ocd-smoke-alarm binary is installed
    And the config has "allow_plaintext_isotopes: true"
    And a target "t1" is configured

  # ── format recognition ──────────────────────────────────────────────────────

  Scenario: smoke-alarm accepts a probe response with a plaintext isotope ID in dev mode
    Given the probe endpoint for "t1" returns isotope_id "plaintext:adhd/mdns-discovery:001"
    When the smoke-alarm polls "t1"
    Then no error is logged for the isotope field
    And /status shows isotope_id "plaintext:adhd/mdns-discovery:001" for "t1"

  Scenario: plaintext isotope ID is passed through to /status unchanged
    Given the probe returns isotope_id "plaintext:adhd/light-transitions-dark-to-green:001"
    When /status is requested
    Then the isotope_id field is exactly "plaintext:adhd/light-transitions-dark-to-green:001"
    And no hash computation or verification is attempted

  Scenario: smoke-alarm in production mode rejects plaintext isotope IDs
    Given the config has "allow_plaintext_isotopes: false" (the production default)
    And the probe endpoint returns isotope_id "plaintext:adhd/mdns-discovery:001"
    When the smoke-alarm polls the target
    Then the isotope_id is dropped from the TargetStatus
    And a "warn" log entry is written containing "plaintext isotope rejected in production mode"
    And /status shows no isotope_id for the target

  # ── feature-id extraction ───────────────────────────────────────────────────

  Scenario: the feature-id component is extractable from a plaintext isotope ID
    Given isotope_id "plaintext:adhd/mdns-discovery:001"
    When the feature-id is extracted
    Then the result is "adhd/mdns-discovery"

  Scenario: a plaintext isotope maps to a known Gherkin scenario for coverage tracking
    Given isotope_id "plaintext:adhd/mdns-discovery:001" transits through a health-check
    When ADHD processes the LightUpdate
    Then the scenario in feature "adhd/mdns-discovery" is marked as covered
    And the coverage entry notes the isotope was a plaintext stand-in

  # ── chaos window compatibility ──────────────────────────────────────────────

  Scenario: a chaos window can be registered for a plaintext isotope ID
    When a POST request is sent to "/isotope/register-chaos-window" with:
      | target_id    | t1                                              |
      | isotope_id   | plaintext:adhd/light-transitions-dark-to-green:001 |
      | window_start | <now>                                           |
      | window_end   | <now + 10 minutes>                              |
    Then the response status is 200
    And the chaos window is stored for "t1" / "plaintext:adhd/light-transitions-dark-to-green:001"

  Scenario: a failing probe with a plaintext chaos isotope is classified as "registered-chaos"
    Given a chaos window is registered for "t1" / "plaintext:adhd/light-transitions-dark-to-green:001"
    And the probe for "t1" fails carrying isotope "plaintext:adhd/light-transitions-dark-to-green:001"
    When /status is requested
    Then the target "t1" has isotope_classification "registered-chaos"

  # ── uniqueness within a session ─────────────────────────────────────────────
  # Plain text isotopes use a sequential-id suffix for intra-session uniqueness.
  # They are NOT globally unique across restarts — this is a known dev-mode limitation.

  Scenario: two plaintext isotopes with different sequential IDs are treated as distinct
    Given two probe responses carrying:
      | "plaintext:adhd/mdns-discovery:001" |
      | "plaintext:adhd/mdns-discovery:002" |
    When both are observed in sequence at target "t1"
    Then both transit events are recorded
    And an isotope-variation failure is raised (sequential IDs differ — unexpected rotation)

  Scenario: the same plaintext isotope ID observed twice at the same target is a duplicate
    Given the probe repeatedly returns "plaintext:adhd/mdns-discovery:001" for "t1"
    When two LightUpdates arrive with the same isotope ID and target
    Then only the first transit event is recorded
    And no variation failure is raised

  # ── SSE transport ───────────────────────────────────────────────────────────

  Scenario: SSE event carries the plaintext isotope ID when the probe returned one
    Given a target "t1" configured with use_sse=true in dev mode
    And the probe returns isotope "plaintext:adhd/smoke-alarm-network:001"
    When the smoke-alarm pushes a status event over the SSE stream
    Then the event data contains isotope_id "plaintext:adhd/smoke-alarm-network:001"

  # ── migration path ──────────────────────────────────────────────────────────
  # When the probe switches from a plaintext ID to a canonical cryptographic ID,
  # the smoke-alarm treats it as a new isotope (variation) — not a continuation.

  Scenario: switching from plaintext to canonical isotope ID is treated as a new isotope
    Given "t1" previously emitted "plaintext:adhd/mdns-discovery:001"
    When the probe switches to emitting a canonical 43-character isotope ID
    And a LightUpdate arrives with the canonical ID
    Then a new transit event is recorded for the canonical ID
    And no association is made to the previous plaintext ID
