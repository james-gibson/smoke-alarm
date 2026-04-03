# features/sse-transport.feature
# Canon record — last audited: 2026-03-25
# Exercises: SSE transport for MCP and ACP, mixed HTTP+SSE target set, remote_agent lifecycle
# Config reference: configs/samples/sse-remote-mixed.yaml
# Step definitions: features/step_definitions/sse_transport_steps.go
# see: features/config-validation.feature (schema), features/oauth-mock.feature (bearer/oauth auth)

@sse-transport @core
Feature: SSE Transport Probe
  As an operator monitoring remote MCP and ACP servers over SSE
  I want ocd-smoke-alarm to connect, receive the event stream, and classify health
  So that SSE-transport services are monitored alongside HTTP-transport ones

  Background:
    Given the ocd-smoke-alarm binary is installed
    And a valid config file "configs/samples/sse-remote-mixed.yaml" exists

  # ── SSE connection ────────────────────────────────────────────────────────

  Scenario: probe connects to an MCP SSE endpoint and receives an event stream
    Given the SSE endpoint "https://mcp.example.com/stream?transport=sse" is reachable
    And the endpoint streams valid MCP events
    When the probe for target "mcp-remote-sse-primary" runs
    Then the probe establishes an SSE connection
    And the target "mcp-remote-sse-primary" is classified as "HEALTHY"

  Scenario: probe connects to an ACP SSE endpoint and receives an event stream
    Given the SSE endpoint "https://agent.example.com/events?transport=sse" is reachable
    And the endpoint streams valid ACP events
    When the probe for target "acp-remote-sse-primary" runs
    Then the probe establishes an SSE connection
    And the target "acp-remote-sse-primary" is classified as "HEALTHY"

  Scenario: probe classifies an SSE target as OUTAGE when the endpoint is unreachable
    Given the SSE endpoint "https://mcp.example.com/stream?transport=sse" is unreachable
    When the probe for target "mcp-remote-sse-primary" runs
    Then the target "mcp-remote-sse-primary" is classified as "OUTAGE"

  Scenario: probe classifies an SSE target as DEGRADED when the stream closes unexpectedly
    Given the SSE endpoint "https://mcp.example.com/stream?transport=sse" closes the stream prematurely
    When the probe for target "mcp-remote-sse-primary" runs
    Then the target "mcp-remote-sse-primary" is classified as "DEGRADED"

  # ── bearer auth with SSE ──────────────────────────────────────────────────

  Scenario: probe attaches a bearer token to SSE connection headers
    Given the target "mcp-remote-sse-primary" has auth type "bearer"
    And the secret ref "keychain://ocd-smoke-alarm/mcp-remote-sse-primary/token" resolves to a valid token
    When the probe for target "mcp-remote-sse-primary" runs
    Then the SSE connection request contains an "Authorization" header
    And the token value is not logged in plaintext

  # ── strict vs none handshake profile ─────────────────────────────────────

  Scenario: a target with handshake_profile "none" skips method validation
    Given the target "mcp-remote-sse-primary" has handshake_profile "none"
    When the probe for target "mcp-remote-sse-primary" completes
    Then no required_methods check is performed
    And classification is based solely on HTTP status code

  Scenario: a target with handshake_profile "strict" validates required methods
    Given the target "mcp-remote-https-strict" has handshake_profile "strict"
    And the endpoint responds to all methods in "required_methods"
    When the probe for target "mcp-remote-https-strict" completes
    Then the target "mcp-remote-https-strict" is classified as "HEALTHY"

  # ── HURL preflight with SSE targets ──────────────────────────────────────

  Scenario: probe runs HURL preflight before establishing SSE connection
    Given the target "mcp-remote-sse-primary" has a hurl_test named "mcp-sse-preflight"
    And the preflight endpoint "https://mcp.example.com/healthz" returns status code 200
    When the probe for target "mcp-remote-sse-primary" runs
    Then the HURL preflight "mcp-sse-preflight" is executed before the SSE connection
    And the SSE connection is only attempted after preflight passes

  Scenario: probe aborts when HURL preflight returns a non-200 status code
    Given the preflight endpoint "https://mcp.example.com/healthz" returns status code 503
    When the probe for target "mcp-remote-sse-primary" runs
    Then the SSE connection is not attempted
    And the target "mcp-remote-sse-primary" is classified as "DEGRADED"

  # ── mixed target set ──────────────────────────────────────────────────────

  Scenario: all four remote targets are probed concurrently within max_workers limit
    Given the config "configs/samples/sse-remote-mixed.yaml" has "max_workers" set to 8
    And all four targets are enabled
    When a probe cycle runs
    Then all four probes are dispatched concurrently
    And no more than 8 worker goroutines are active at once

  # ── remote_agent lifecycle ────────────────────────────────────────────────

  Scenario: remote_agent managed update runs stop, start, and verify commands in order
    Given "remote_agent.managed_updates" is true in config "configs/samples/sse-remote-mixed.yaml"
    When a managed update is triggered
    Then the "stop_command" is executed first
    And the "start_command" is executed after stop succeeds
    And the "verify_command" is executed after start
    And on verify failure the previous version is restored

  Scenario: remote_agent update is rolled back when verify_command reports failure
    Given the verify_command exits with a non-zero code
    And "rollback_on_failure" is true
    When the managed update completes
    Then the previous binary version is restored
    And a REGRESSION event is emitted
