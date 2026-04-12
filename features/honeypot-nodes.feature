# features/honeypot-nodes.feature
# Next-steps record — honeypot nodes as detection infrastructure
# Dependency: skill-execution-receipts.feature  (isotope = receipt of execution)
# Dependency: chaos-skill-detection.feature     (receipt deviation = detection signal)
# Dependency: trust-rungs.feature               (rung gates routing)
# Step definitions: features/step_definitions/honeypot_steps.go (to be created)
#
# A honeypot node is a target registered in the federation that looks legitimate
# but should never receive skill deliveries or be the source of isotope transits
# in a correctly-operating mesh.
#
# Like counterfeit detection ink: invisible under normal operation, but any contact
# with it leaves a mark. A forger uses the ink — the mark reveals them. An
# untrusted actor routes to the honeypot — the receipt reveals them.
#
# Two honeypot properties:
#
#   Silent honeypot  — a target that exists but should never be contacted.
#                      Any receipt or routing trace touching it is a detection signal.
#
#   Canary honeypot  — a target that responds with a distinctive pre-known receipt.
#                      The receipt is the "counterfeit ink": if it appears anywhere
#                      outside the honeypot itself, a forgery is in progress.
#
# The honeypot isotope is pre-computed and held by the certifying authority.
# It should NEVER appear in /status for any non-honeypot target. Seeing it
# means someone routed to the honeypot and carried the receipt elsewhere.

@honeypot @optional
Feature: Honeypot Nodes — Counterfeit Detection for the Federation Mesh
  As a smoke-alarm certifying a federation mesh
  I want to maintain honeypot nodes that should never receive legitimate traffic
  So that any contact with them reveals routing misbehaviour or untrusted forwarding

  Background:
    Given the ocd-smoke-alarm binary is installed
    And at least one honeypot node is configured in the federation

  # ── honeypot registration ────────────────────────────────────────────────────

  Scenario: a honeypot node is registered as a target that should never receive skill deliveries
    Given a target "honeypot-alpha" is registered with type "honeypot"
    When the federation mesh starts
    Then "honeypot-alpha" appears in GET /membership with role "honeypot"
    And "honeypot-alpha" is excluded from all skill routing decisions
    And "honeypot-alpha" is never returned in tools/list responses to any caller

  Scenario: a honeypot node appears in the target list but not the routing table
    Given "honeypot-alpha" is a registered honeypot
    When any caller requests available targets or skills
    Then "honeypot-alpha" does not appear in the routeable set
    And no legitimate skill invocation can be addressed to "honeypot-alpha"

  Scenario: the honeypot node's isotope is pre-computed and held by the certifying authority
    Given "honeypot-alpha" is configured with:
      | feature_id | ocd/honeypot-alpha                |
      | payload    | <controlled honeypot response>    |
      | nonce      | <pre-selected fixed value>        |
    When the honeypot isotope is constructed
    Then honeypot_isotope = base64url( SHA256( feature_id || ":" || SHA256(payload) || ":" || nonce ) )
    And this value is stored as the canary: it must never appear in a non-honeypot transit

  # ── silent honeypot — no contact expected ────────────────────────────────────

  Scenario: any routing trace mentioning the honeypot node is a detection signal
    Given "honeypot-alpha" should receive zero skill deliveries
    When a skill invocation's routing trace includes "honeypot-alpha"
    Then the smoke-alarm raises: "honeypot contact detected in routing trace: honeypot-alpha"
    And the originating instance has its 42i distance increased by 16
    And the detection event includes: skill_name, routing_trace, originating_instance

  Scenario: a skill delivery addressed directly to the honeypot node is a detection signal
    Given a JSON-RPC call arrives addressed to a skill on "honeypot-alpha"
    When the smoke-alarm's router processes the call
    Then the call is rejected with error code -32000
    And the smoke-alarm raises: "honeypot addressed directly: honeypot-alpha"
    And the caller's certification is flagged for review

  Scenario: a probe response from the honeypot node triggers a detection signal
    Given "honeypot-alpha" is a silent honeypot (should never respond to probes)
    When a probe response is received from "honeypot-alpha"
    Then the smoke-alarm raises: "honeypot responded to probe: honeypot-alpha"
    And the probe response is discarded
    And the instance that caused the probe is flagged

  # ── canary honeypot — receipt reveals contact ────────────────────────────────

  Scenario: the canary honeypot produces a distinctive receipt when contacted
    Given "honeypot-beta" is a canary honeypot that responds with a controlled payload
    When any actor routes a skill to "honeypot-beta" and it responds
    Then the response carries honeypot_isotope as the receipt
    And this isotope is recognisable as the canary receipt by the certifying authority

  Scenario: the canary receipt appearing in /status for any non-honeypot target is a detection signal
    Given honeypot_isotope = "iso-honeypot-001" is the pre-computed canary
    When /status for target "t1" (a legitimate target) shows isotope_id "iso-honeypot-001"
    Then the smoke-alarm raises: "canary receipt observed outside honeypot: t1"
    And this proves an actor contacted "honeypot-beta" and carried the receipt to "t1"
    And the transit records for "t1" are examined to identify the routing path

  Scenario: the canary receipt appearing in a skill response is a detection signal
    Given honeypot_isotope appears in the result of skill "open-the-pickle-jar"
    When the smoke-alarm processes the skill result
    Then the smoke-alarm raises: "canary receipt in skill result: open-the-pickle-jar"
    And the skill's certification is suspended pending investigation
    And the instance hosting the skill has its rung revoked

  # ── honeypot detection in ADHD ───────────────────────────────────────────────

  Scenario: ADHD displays honeypot nodes distinctly in the dashboard
    Given the cluster contains one or more honeypot nodes
    When ADHD renders the dashboard
    Then honeypot nodes are shown with a distinct visual indicator (not a standard light)
    And the indicator communicates "this node should never be active"
    And a green honeypot indicator means "clean — no contact detected"
    And a red honeypot indicator means "contact detected — detection event active"

  Scenario: a honeypot detection event turns the honeypot light red in ADHD
    Given ADHD is monitoring a cluster with honeypot "honeypot-alpha"
    When the smoke-alarm reports a detection event for "honeypot-alpha"
    Then the honeypot light in ADHD turns red
    And the Details field contains the detection event summary
    And the red light is not suppressible by chaos window registration
    And the detection cannot be classified as "registered-chaos"

  Scenario: a clean honeypot cluster certifies the mesh's routing integrity
    Given all honeypots in the cluster are green (no contact detected)
    When ADHD renders the cluster health summary
    Then a "routing integrity" indicator is shown as certified
    And the certification notes: "N honeypot nodes clean"

  # ── honeypot as a test for demo and integration suites ────────────────────────
  # In the demo cluster, at least one honeypot node is always present.
  # A successful demo shows all honeypot nodes green — the demo routing
  # logic never contacts them. This proves the demo cluster's routing is correct.

  Scenario: demo cluster includes a honeypot node in its configuration
    Given the lezz demo cluster is running
    When ADHD connects in demo mode
    Then at least one honeypot node is present in the cluster
    And the honeypot is green (demo routing did not contact it)
    And the demo showcase includes honeypot status in its health summary

  Scenario: integration tests assert the honeypot node is never contacted
    Given the integration test suite runs a full demo cluster scenario
    When all skills are exercised and all features verified
    Then the honeypot node's contact count is zero throughout the test run
    And a final assertion: honeypot_contact_count == 0 is part of the test teardown
