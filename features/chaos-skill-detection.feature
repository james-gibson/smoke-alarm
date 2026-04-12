# features/chaos-skill-detection.feature
# Next-steps record — chaos skills as detection probes for untrusted or underverified agents
# Dependency: skill-execution-receipts.feature  (isotope = receipt of skill execution)
# Dependency: trust-rungs.feature               (rung gates what agents can receive)
# Dependency: mcp-proxy.feature                 (routing trace through the mesh)
# Dependency: certification-gate.feature        (uncertified agents in the registry)
# Step definitions: features/step_definitions/chaos_skill_detection_steps.go (to be created)
#
# A chaos skill is a skill execution deliberately delivered to an agent that is
# untrusted (rung 0) or underverified (rung 1-2). Because the receipt is
# pre-computable from (feature_id, payload, nonce), the certifying smoke-alarm
# knows in advance what isotope should arrive, from which target, via which route.
#
# Any deviation from the expected receipt is a detection signal:
#
#   Receipt from wrong source    → the proxy forwarded to an untrusted actor
#   Receipt with wrong payload   → the actor tampered with the skill result
#   Receipt via unexpected route → the routing trace was stripped or forged
#   No receipt at all            → the actor swallowed the execution
#   Receipt too early / too late → the actor replayed or delayed the execution
#
# The chaos skill is not a failure simulation here — it is a probe. The "chaos"
# is the controlled uncertainty introduced to observe how untrusted agents handle
# skill deliveries they were not expected to forward.

@chaos-detection @isotope @optional
Feature: Chaos Skills as Detection Probes for Untrusted Agent Behaviour
  As a smoke-alarm maintaining a certified federation mesh
  I want to deliver chaos skills to underverified agents and observe the resulting receipts
  So that I can detect agents that blindly forward skill executions to untrusted actors
  or tamper with, suppress, or replay skill results

  Background:
    Given the ocd-smoke-alarm binary is installed
    And a federation mesh is running with at least one underverified peer

  # ── pre-computing the expected receipt ──────────────────────────────────────
  # Because isotope construction is deterministic given (feature_id, payload, nonce),
  # the certifying smoke-alarm can construct the expected receipt before delivery.

  Scenario: the smoke-alarm pre-computes the expected receipt before delivering a chaos skill
    Given a chaos skill with:
      | feature_id | adhd/chaos-skill-detection          |
      | payload    | <controlled probe result>           |
      | nonce      | <pre-selected random value>         |
    When the expected receipt is constructed
    Then expected_isotope = base64url( SHA256( feature_id || ":" || SHA256(payload) || ":" || nonce ) )
    And this value is recorded before the skill is delivered

  Scenario: the expected receipt uniquely identifies this execution
    Given the same chaos skill is delivered twice with different nonces
    Then the two expected receipts are distinct
    And neither can be replayed as proof of the other execution

  # ── delivery to an underverified agent ──────────────────────────────────────

  Scenario: the chaos skill is delivered to a rung-0 proxy as a normal skill invocation
    Given inst-c is registered at rung 0 (uncertified)
    And inst-c is acting as a proxy between the smoke-alarm and target "t1"
    When the chaos skill is delivered to inst-c as a JSON-RPC call
    Then inst-c receives a valid skill invocation
    And inst-c does not know the invocation is a detection probe

  Scenario: the delivery is indistinguishable from a legitimate skill call to the proxy
    Given the chaos skill invocation carries:
      | method | adhd/chaos-skill-detection |
      | params | <controlled probe params>  |
    Then the routing trace is populated normally
    And no special marker indicates this is a chaos probe
    And an untrusted actor cannot distinguish it from a real skill call

  # ── receipt arrives as expected ─────────────────────────────────────────────

  Scenario: correct forwarding produces the expected receipt from the expected target
    Given the chaos skill is delivered to inst-c for forwarding to target "t1"
    When inst-c forwards correctly and "t1" processes the skill
    And the receipt appears in "t1"'s /status via the normal polling loop
    Then the observed isotope matches expected_isotope
    And the routing trace includes inst-c's instance ID
    And no detection signal is raised
    And inst-c's trust rung may advance based on correct handling

  # ── detection: receipt from wrong source ────────────────────────────────────
  # If the proxy forwarded to an untrusted actor instead of the expected target,
  # the receipt appears in /status from the wrong target or with the wrong routing trace.

  Scenario: receipt arriving from an unexpected target indicates proxy forwarded to wrong actor
    Given the chaos skill was intended for forwarding to "t1"
    When the receipt appears in /status for target "t2" instead
    Then the smoke-alarm raises a detection event: "receipt from unexpected source: t2 (expected t1)"
    And inst-c's trust rung decreases by at least one level
    And the detection event is logged with: expected_target, actual_target, isotope_id, routing_trace

  Scenario: receipt arriving with a routing trace that omits the proxy indicates trace stripping
    Given the chaos skill was routed through inst-c
    When the receipt arrives in /status but the routing trace does not include inst-c's instance ID
    Then the smoke-alarm raises a detection event: "routing trace stripped by proxy"
    And inst-c's 42i distance increases by 8
    And a "warn" log entry is written: "trace integrity violation detected via chaos probe"

  # ── detection: tampered payload ─────────────────────────────────────────────
  # If the actor modified the skill result before producing the receipt,
  # the receipt's payload hash will not match the expected value.

  Scenario: receipt with a mismatched payload indicates the actor tampered with the result
    Given the expected receipt was pre-computed from the controlled payload
    When the receipt arrives but does not match expected_isotope
    And the feature_id in the receipt is correct (same skill, different result)
    Then the smoke-alarm raises: "receipt payload mismatch: skill result was tampered"
    And the tampered receipt is recorded as a skill-variation failure
    And the skill's 42i distance increases by 8

  Scenario: a completely forged receipt (wrong feature_id) indicates a different skill was run
    Given the receipt arrives with feature_id "adhd/some-other-skill"
    When the smoke-alarm attempts to match it to the expected chaos probe
    Then no match is found
    And the smoke-alarm raises: "unexpected feature_id in receipt: expected chaos-skill-detection"
    And the receipt is held as unverified pending further investigation

  # ── detection: suppressed receipt ───────────────────────────────────────────
  # If the proxy or actor swallowed the execution, no receipt arrives.
  # The smoke-alarm detects this via receipt timeout.

  Scenario: no receipt within the detection window indicates the actor suppressed the execution
    Given the chaos skill was delivered to inst-c with a detection_deadline of T+30s
    When no matching receipt appears in /status before T+30s
    Then the smoke-alarm raises: "chaos probe receipt suppressed: no transit observed"
    And inst-c's 42i distance increases by 8
    And if inst-c's distance exceeds the rung ceiling, inst-c is demoted

  Scenario: a late receipt (after the detection window) is still recorded but flagged
    Given the receipt arrives at T+45s (past the T+30s deadline)
    When the smoke-alarm processes the late receipt
    Then the receipt is recorded as a transit event
    And a detection flag is set: "receipt arrived outside detection window"
    And the late arrival may indicate the actor buffered or replayed the execution

  # ── detection: replay ───────────────────────────────────────────────────────
  # Because nonces make each receipt unique, a replayed receipt is detectable:
  # the same isotope ID appearing twice is a replay, not a second execution.

  Scenario: the same receipt arriving twice indicates replay
    Given receipt "iso-chaos-001" was already recorded as a transit for "t1"
    When "iso-chaos-001" appears again in /status for "t1"
    Then the smoke-alarm raises: "chaos probe replay detected: iso-chaos-001"
    And the replay is not recorded as a new transit event
    And the agent that delivered the replay has its 42i distance increased by 8

  # ── the untrusted actor pattern ─────────────────────────────────────────────
  # If a rung-0 proxy blindly forwards all skill invocations to an unknown actor,
  # the chaos probe reveals this by producing a receipt from an unexpected source.
  # The pattern: expected source ≠ actual source of receipt.

  Scenario: blind forwarding by an untrusted proxy is revealed by the receipt source
    Given inst-c (rung 0) receives all skill invocations
    And inst-c blindly forwards them to "untrusted-actor" without inspection
    When the chaos skill is delivered to inst-c
    And "untrusted-actor" processes it and produces a receipt
    And that receipt appears in /status for "untrusted-actor"
    Then the smoke-alarm detects: receipt from unknown actor "untrusted-actor"
    And inst-c is flagged as a blind forwarder
    And inst-c's certification is revoked pending investigation

  Scenario: a partially-trusted proxy that selectively forwards is detected by receipt gaps
    Given inst-c forwards some skill invocations but suppresses others
    When a chaos probe targeted at "t1" via inst-c produces no receipt
    But a separate probe directly to "t1" produces the expected receipt
    Then the smoke-alarm determines inst-c is selectively suppressing forwarding
    And a detection event is raised: "selective suppression detected via inst-c"

  # ── chaos detection does not require a chaos window ─────────────────────────
  # Chaos windows are for suppressing false alarm red lights during planned failures.
  # Detection probes are NOT planned failures — they are health checks on the mesh.
  # No chaos window is registered for detection probe isotopes.
  # If the probe causes a red light, that is the correct and expected outcome
  # when the probe reveals misbehaviour.

  Scenario: a chaos detection probe does not register a chaos window
    Given a chaos skill detection probe is initiated for inst-c
    When the probe is delivered
    Then no POST is sent to /isotope/register-chaos-window for the probe's isotope
    And if a red light results from the probe, it is not suppressed
    And the red light is itself a signal that something unexpected occurred

  Scenario: a successful detection probe (no misbehaviour found) produces no red light
    Given the chaos skill is delivered and the receipt arrives correctly
    And the receipt matches expected_isotope from the expected source via the expected route
    When the probe is evaluated
    Then no red light is raised (the probe passed — the proxy forwarded correctly)
    And the probe result is recorded as a clean certification event for inst-c
