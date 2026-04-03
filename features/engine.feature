# features/engine.feature
# Canon record — last audited: 2026-03-26
# Step definitions: features/step_definitions/engine_steps.go

@engine @core
Feature: Probe Engine
  As an operator running ocd-smoke-alarm
  I want the engine to continuously probe configured targets and classify their health state
  So that regressions and outages are surfaced before they affect users

  Background:
    Given the ocd-smoke-alarm binary is installed
    And a valid config file "configs/sample.yaml" exists

  # ── lifecycle ─────────────────────────────────────────────────────────────

  Scenario: Engine starts and performs an immediate first check
    Given a target "t1" is configured with a 500 ms poll interval
    When the engine starts with that config
    Then a probe result for target "t1" is recorded before the first poll tick

  Scenario: Engine becomes ready once all enabled targets have at least one result
    Given 3 enabled targets are configured
    When the engine starts with that config
    And all targets have been probed at least once
    Then the engine reports ready

  Scenario: Engine stops cleanly when context is canceled
    Given the engine is running with target "t1"
    When the engine context is canceled
    Then the engine exits without error within 2000 ms

  Scenario: Engine saves known-state baseline on graceful stop
    Given the engine is running with known-state enabled
    When the engine context is canceled
    Then the baseline file is written to disk

  # ── probe scheduling ──────────────────────────────────────────────────────

  Scenario: Engine probes each target on its configured interval
    Given a target "t1" is configured with a 100 ms poll interval
    When the engine runs for 350 ms
    Then at least 3 probe results are recorded for target "t1"

  Scenario: Engine bounds concurrent probes by max_workers
    Given 10 targets are configured
    And max_workers is set to 3
    When all targets are due for a probe simultaneously
    Then no more than 3 probes run concurrently

  # ── state classification ──────────────────────────────────────────────────

  Scenario: Engine classifies a successful probe as healthy
    Given the engine is running with target "t1"
    When the prober returns a healthy result for "t1"
    Then the status for "t1" is "healthy"

  Scenario: Engine classifies a probe error as unhealthy
    Given the engine is running with target "t1"
    When the prober returns an unhealthy result for "t1"
    Then the status for "t1" is "unhealthy"

  Scenario: Engine classifies a failure after prior success as regression
    Given the engine is running with target "t1" and known-state enabled
    And the prober returns a healthy result for "t1"
    When the prober returns an unhealthy result for "t1"
    Then the status for "t1" is "regression"
    And the result has regression flag set

  Scenario: Engine classifies consecutive failures past threshold as outage
    Given a target "t1" is configured with outage threshold 3
    And the prober returns a healthy result for "t1"
    When the prober returns 3 consecutive unhealthy results for "t1"
    Then the status for "t1" is "outage"
    And the status message contains "consecutive failures"

  Scenario: Engine applies aggressive policy to elevate post-healthy failures
    Given the engine is running with aggressive alerts enabled for target "t1"
    And the prober returns a healthy result for "t1"
    When the prober returns an unhealthy result for "t1"
    Then the result severity is "critical"

  Scenario: Engine blocks deeper probing when HURL safety check fails
    Given a target "t1" has a HURL safety check configured
    When the safety check for "t1" fails
    Then no protocol probe is executed for "t1"
    And the status for "t1" reflects the safety failure

  Scenario: Engine retries a failing probe up to the configured retry count
    Given a target "t1" is configured with 2 retries
    When the first 2 probes return failures and the final attempt succeeds
    Then the recorded result for "t1" is "healthy"
    And the attempt index recorded is 3

  # ── alert emission ────────────────────────────────────────────────────────

  Scenario: Engine emits an alert when a regression is detected
    Given the engine is running with a notifier registered
    When the prober returns a regression result for target "t1"
    Then an alert event is dispatched with state "regression"
    And the alert event target id is "t1"

  Scenario: Engine emits an alert when an outage is detected
    Given the engine is running with a notifier registered
    When the prober returns an outage result for target "t1"
    Then an alert event is dispatched with state "outage"

  Scenario: Engine does not emit an alert for a healthy result
    Given the engine is running with a notifier registered
    When the prober returns a healthy result for "t1"
    Then no alert event is dispatched

  Scenario: Engine emits an alert for any critical severity result
    Given the engine is running with a notifier registered
    When the prober returns a result with severity "critical" for target "t1"
    Then an alert event is dispatched with state "outage"

  # ── event ring buffer ─────────────────────────────────────────────────────

  Scenario: Engine records alert events in its history ring buffer
    Given the engine is running with a notifier registered
    When 5 regression events occur for target "t1"
    Then the event history contains 5 entries

  Scenario: Engine evicts oldest events when the ring buffer is full
    Given the engine is running with event history size 10
    When 15 alert events are emitted
    Then the event history size does not exceed 10
    And the most recent event is last in the history

  Scenario: SnapshotEvents returns events in chronological order
    Given the engine has recorded 4 alert events
    When I call SnapshotEvents
    Then the returned slice length is 4
    And events are ordered oldest-first

  # ── snapshot API ──────────────────────────────────────────────────────────

  Scenario: SnapshotStatuses returns entries sorted by target ID
    Given 3 targets with IDs "z1", "a2", "m3" are configured
    When the engine has a status for each target
    Then SnapshotStatuses returns entries in ascending target ID order

  Scenario: SnapshotStatuses includes latency and status code from probe result
    Given the engine is running with target "t1"
    When the prober returns a result with latency 250 ms and status code 200
    Then the snapshot for "t1" includes latency 250 ms
    And the snapshot for "t1" includes status code 200

  # ── stdio transport probing ───────────────────────────────────────────────

  Scenario: Stdio prober returns config failure when command is empty
    Given a stdio target "s1" with an empty command field
    When the stdio prober probes "s1"
    Then the result state is "unhealthy"
    And the failure class is "config"
    And the message contains "stdio.command is required"

  Scenario: Stdio prober skips handshake when profile is "none"
    Given a stdio target "s1" with handshake_profile "none"
    When the stdio prober probes "s1"
    Then the result state is "healthy"
    And the message contains "handshake_profile=none"
    And no subprocess is launched

  Scenario: Stdio prober uses "initialize" only for base profile MCP
    Given a stdio target "s1" with protocol "mcp" and handshake_profile "base"
    When the stdio prober probes "s1"
    Then the exercised methods list is ["initialize"]

  Scenario Outline: Stdio prober uses full method set for strict profile
    Given a stdio target "s1" with protocol "<protocol>" and handshake_profile "strict"
    When the stdio prober probes "s1"
    Then the exercised methods list is <expected-methods>

    Examples:
      | protocol | expected-methods                                      |
      | mcp      | ["initialize", "tools/list", "resources/list"]        |
      | acp      | ["initialize", "session/setup", "prompt/turn"]        |

  Scenario: Stdio prober respects required_methods override over profile
    Given a stdio target "s1" with required_methods ["initialize", "ping"]
    When the stdio prober probes "s1"
    Then the exercised methods list is ["initialize", "ping"]

  Scenario: Stdio prober classifies timeout as failure class "timeout"
    Given a stdio target "s1" whose subprocess does not respond within the timeout
    When the stdio prober probes "s1"
    Then the result state is "unhealthy"
    And the failure class is "timeout"

  Scenario: Stdio prober classifies JSON-RPC error response as failure class "protocol"
    Given a stdio target "s1" whose subprocess returns a JSON-RPC error for "initialize"
    When the stdio prober probes "s1"
    Then the result state is "unhealthy"
    And the failure class is "protocol"

  Scenario: Stdio prober falls back to HTTP prober for non-stdio transport
    Given a target "h1" with transport "http"
    When the stdio prober probes "h1"
    Then the HTTP fallback prober is invoked for "h1"

  Scenario: Stdio prober includes stderr in failure message when present
    Given a stdio target "s1" whose subprocess writes to stderr before crashing
    When the stdio prober probes "s1"
    Then the result message contains the stderr output
