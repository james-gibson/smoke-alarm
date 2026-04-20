# features/hosted-server.feature
# Canon record — last audited: 2026-03-25
# Exercises: embedded hosted MCP/ACP HTTP+SSE server, self-probe loop
# Config reference: configs/samples/hosted-mcp-acp.yaml
# Step definitions: features/step_definitions/hosted_server_steps.go
# see: features/config-validation.feature (schema), features/oauth-mock.feature (auth)
# see: features/dynamic-config.feature (dynamic_config output from hosted targets)
@hosted-server @core
Feature: Embedded Hosted MCP/ACP Server
  As an operator running ocd-smoke-alarm in a local or CI environment
  I want the process to serve its own embedded MCP and ACP endpoints
  So that downstream clients can connect without a separate server process

  Background:
    Given the ocd-smoke-alarm binary is installed
    And a valid config file "configs/samples/hosted-mcp-acp.yaml" exists
  # ── server startup ────────────────────────────────────────────────────────

  Scenario: hosted server starts when hosted.enabled is true
    Given "hosted.enabled" is true in config "configs/samples/hosted-mcp-acp.yaml"
    When ocd-smoke-alarm starts with that config
    Then a listener is bound on "localhost:18091"
    And the MCP endpoint "/mcp" is served
    And the ACP endpoint "/acp" is served

  Scenario: hosted server does not start when hosted.enabled is false
    Given a config with "hosted.enabled" set to false
    When ocd-smoke-alarm starts with that config
    Then no listener is bound on the hosted listen address
  # ── HTTP transport ────────────────────────────────────────────────────────

  Scenario: hosted MCP HTTP endpoint responds to an initialize request
    Given the hosted server is running on "localhost:18091"
    When a valid MCP initialize JSON-RPC request is sent to "http://localhost:18091/mcp"
    Then the response is a valid MCP initialize JSON-RPC response
    And the response status code is 200

  Scenario: hosted ACP HTTP endpoint responds to an initialize request
    Given the hosted server is running on "localhost:18091"
    When a valid ACP initialize JSON-RPC request is sent to "http://localhost:18091/acp"
    Then the response is a valid ACP initialize JSON-RPC response
    And the response status code is 200
  # ── SSE transport ─────────────────────────────────────────────────────────

  Scenario: hosted MCP SSE endpoint opens an event stream
    Given the hosted server is running on "localhost:18091"
    When a GET request is sent to "http://localhost:18091/mcp?transport=sse"
    Then the response Content-Type is "text/event-stream"
    And the connection remains open

  Scenario: hosted ACP SSE endpoint opens an event stream
    Given the hosted server is running on "localhost:18091"
    When a GET request is sent to "http://localhost:18091/acp?transport=sse"
    Then the response Content-Type is "text/event-stream"
    And the connection remains open
  # ── self-probe ────────────────────────────────────────────────────────────

  Scenario: hosted-mcp-http target is probed against the embedded server
    Given the hosted server is running on "localhost:18091"
    And the target "hosted-mcp-http" is enabled in config "configs/samples/hosted-mcp-acp.yaml"
    When the probe for target "hosted-mcp-http" completes
    Then the target "hosted-mcp-http" is classified as "HEALTHY"

  Scenario: hosted-acp-http target is probed against the embedded server
    Given the hosted server is running on "localhost:18091"
    And the target "hosted-acp-http" is enabled in config "configs/samples/hosted-mcp-acp.yaml"
    When the probe for target "hosted-acp-http" completes
    Then the target "hosted-acp-http" is classified as "HEALTHY"

  Scenario: hosted-mcp-sse target is probed against the embedded SSE stream
    Given the hosted server is running on "localhost:18091"
    And the target "hosted-mcp-sse" is enabled in config "configs/samples/hosted-mcp-acp.yaml"
    When the probe for target "hosted-mcp-sse" completes
    Then the target "hosted-mcp-sse" is classified as "HEALTHY"

  Scenario: hosted-acp-sse target is probed against the embedded SSE stream
    Given the hosted server is running on "localhost:18091"
    And the target "hosted-acp-sse" is enabled in config "configs/samples/hosted-mcp-acp.yaml"
    When the probe for target "hosted-acp-sse" completes
    Then the target "hosted-acp-sse" is classified as "HEALTHY"
  # ── HURL preflight gate ───────────────────────────────────────────────────

  Scenario: hosted-mcp-http probe runs health-gate HURL test before handshake
    Given the health endpoint "http://localhost:18088/healthz" returns status code 200
    And the target "hosted-mcp-http" has hurl_test "hosted-mcp-health-gate"
    When the probe for target "hosted-mcp-http" runs
    Then the HURL test "hosted-mcp-health-gate" is executed before the MCP handshake
  # ── strict handshake on hosted targets ───────────────────────────────────

  Scenario Outline: hosted HTTP targets require all declared methods under strict profile
    Given the target "<target-id>" has handshake_profile "strict"
    And the hosted server does not respond to method "<method>"
    When the probe for target "<target-id>" runs
    Then the target "<target-id>" is classified as "DEGRADED"

    Examples:
      | target-id       | method         |
      | hosted-mcp-http | tools/list     |
      | hosted-mcp-http | resources/list |
      | hosted-acp-http | session/setup  |
      | hosted-acp-http | prompt/turn    |
  # ── health endpoint dependency ────────────────────────────────────────────

  Scenario: health endpoint /healthz returns 200 while hosted server is running
    Given ocd-smoke-alarm is running with config "configs/samples/hosted-mcp-acp.yaml"
    When a GET request is sent to "http://localhost:18088/healthz"
    Then the response status code is 200

  Scenario: health endpoint /readyz returns 200 after all enabled targets have completed at least one probe
    Given ocd-smoke-alarm is running with config "configs/samples/hosted-mcp-acp.yaml"
    And all enabled targets have completed at least one probe cycle
    When a GET request is sent to "http://localhost:18088/readyz"
    Then the response status code is 200
