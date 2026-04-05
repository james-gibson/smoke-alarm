# features/isotope-construction.feature
# Next-steps record — canonical isotope ID construction algorithm
# Dependency: none — this is a pure algorithm spec, no runtime dependencies
# Step definitions: features/step_definitions/isotope_construction_steps.go (to be created)
#
# This algorithm must be agreed and pinned before any isotope work proceeds.
# Changing it after deployment invalidates all existing isotope IDs.

@isotope @core
Feature: Canonical Isotope ID Construction
  As a system producing or verifying isotope IDs
  I want a canonical, deterministic construction algorithm
  So that any component can independently construct or verify an isotope ID
  without coordination

  # Algorithm:
  #   isotope_id = base64url( SHA256( feature_id || ":" || SHA256(payload) || ":" || nonce ) )
  #
  # - feature_id: the Gherkin scenario identifier, e.g. "adhd/light-transitions-dark-to-green"
  # - payload:    the probe response body (never stored; only its hash is used)
  # - nonce:      a per-isotope random value ensuring uniqueness across invocations
  # - ||:         byte concatenation
  # - SHA256:     standard SHA-256, output as raw bytes before outer hash
  # - base64url:  RFC 4648 §5 unpadded URL-safe base64 encoding
  # - Result:     always 43 characters (256 bits → 32 bytes → 43 base64url chars)

  # ── construction properties ────────────────────────────────────────────────

  Scenario: isotope ID is always 43 base64url characters
    Given any feature_id, payload, and nonce
    When an isotope ID is constructed
    Then the result is exactly 43 characters
    And every character is in the base64url alphabet [A-Za-z0-9_-]

  Scenario: two IDs for the same feature and payload are not equal due to different nonces
    Given feature "adhd/light-transitions-dark-to-green" and a fixed probe payload
    When two isotope IDs are constructed with different nonces
    Then the two IDs are not equal
    And each is 43 characters

  Scenario: same inputs produce the same ID (deterministic given nonce)
    Given feature_id "adhd/light-transitions-dark-to-green", a fixed payload, and a fixed nonce
    When the isotope ID is constructed twice with identical inputs
    Then both results are equal

  Scenario: changing feature_id produces a different ID
    Given a fixed payload and nonce
    When one ID is constructed for feature "adhd/light-transitions-dark-to-green"
    And another is constructed for feature "adhd/light-transitions-dark-to-red"
    Then the two IDs are not equal

  Scenario: changing the payload produces a different ID
    Given a fixed feature_id and nonce
    When one ID is constructed with payload A
    And another is constructed with payload B (A ≠ B)
    Then the two IDs are not equal

  # ── verification ───────────────────────────────────────────────────────────

  Scenario: an isotope can be verified against its declared feature binding
    Given isotope "iso-abc-001" was constructed from feature "adhd/light-transitions-dark-to-green",
      payload P, and nonce N
    When verification is attempted with feature "adhd/light-transitions-dark-to-green", P, and N
    Then verification succeeds

  Scenario: verification fails when the feature_id does not match
    Given isotope "iso-abc-001" was constructed for feature "adhd/light-transitions-dark-to-green"
    When verification is attempted against feature "adhd/light-transitions-dark-to-red"
    Then verification fails

  Scenario: verification fails when the payload does not match
    Given isotope "iso-abc-001" was constructed with payload P
    When verification is attempted with a different payload P'
    Then verification fails

  Scenario: no payload information is recoverable from the ID alone
    Given isotope "iso-abc-001" and the feature_id used to construct it
    When an attempt is made to extract the payload from the ID and feature_id
    Then no payload information is recoverable
    And the ID is a one-way commitment to the payload, not an encoding of it

  # ── encoding requirements ──────────────────────────────────────────────────

  Scenario: ID uses unpadded URL-safe base64 (no + / or = characters)
    Given any constructed isotope ID
    Then the ID contains no "+" characters
    And the ID contains no "/" characters
    And the ID contains no "=" padding characters

  Scenario: ID is safe to use in URL path segments without percent-encoding
    Given a constructed isotope ID
    When the ID is embedded in a URL path such as "/isotope/<id>/verify"
    Then no percent-encoding is required

  # ── cross-component agreement ──────────────────────────────────────────────
  # Both ocd-smoke-alarm and adhd must produce identical results for the same
  # inputs. This scenario exists to catch any divergence between implementations.

  Scenario: ocd-smoke-alarm and adhd produce identical IDs for the same inputs
    Given feature_id "adhd/light-transitions-dark-to-green", payload P, and nonce N
    When ocd-smoke-alarm constructs an isotope ID with those inputs
    And adhd constructs an isotope ID with the same inputs
    Then the two IDs are equal
