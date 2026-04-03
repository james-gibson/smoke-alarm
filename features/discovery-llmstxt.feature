# features/discovery-llmstxt.feature
# Canon record — last audited: 2026-03-25
# Exercises: llms.txt remote fetch, auto-register targets, auto-register oauth
# Config reference: configs/samples/llmstxt-auto-discovery.yaml
# Step definitions: features/step_definitions/discovery_llmstxt_steps.go
# see: features/config-validation.feature (schema), features/oauth-mock.feature (oauth registration)

@discovery-llmstxt @core
Feature: LLMs.txt Auto-Discovery
  As an operator
  I want ocd-smoke-alarm to fetch llms.txt documents and automatically register discovered endpoints as probe targets
  So that new MCP/ACP services are monitored without manual config changes

  Background:
    Given the ocd-smoke-alarm binary is installed
    And a valid config file "configs/samples/llmstxt-auto-discovery.yaml" exists
    And network access to llms.txt URIs is available

  # ── fetch behavior ───────────────────────────────────────────────────────

  Scenario: discovery fetches each configured llms.txt URI
    Given discovery is enabled with llms_txt URIs:
      | uri                                         |
      | https://modelcontextprotocol.io/llms.txt    |
      | https://agentclientprotocol.com/llms.txt    |
      | https://llmstxt.org/llms.txt                |
    When discovery runs once
    Then each URI is fetched exactly once per discovery interval
    And fetch requests use HTTPS

  Scenario: discovery rejects a non-HTTPS llms.txt URI when require_https is true
    Given discovery is configured with llms_txt URI "http://insecure.example.com/llms.txt"
    And "require_https" is set to true
    When discovery runs once
    Then the URI "http://insecure.example.com/llms.txt" is not fetched
    And a warning is logged containing "require_https"

  Scenario: discovery respects the configured fetch timeout
    Given an llms.txt URI that does not respond within 7 seconds
    And "fetch_timeout" is set to "6s"
    When discovery runs once
    Then the fetch is abandoned after 6 seconds
    And a warning is logged containing "fetch timeout"

  # ── target auto-registration ──────────────────────────────────────────────

  Scenario: discovery registers a valid MCP endpoint from llms.txt as a probe target
    Given an llms.txt document at "https://modelcontextprotocol.io/llms.txt" lists an MCP endpoint "https://mcp.example.com/v1"
    And "auto_register_as_targets" is set to true
    When discovery runs once
    Then a probe target is registered for endpoint "https://mcp.example.com/v1"
    And the target protocol is "mcp"

  Scenario: discovery does not register duplicate targets for the same endpoint
    Given an llms.txt endpoint "https://mcp.example.com/v1" was registered in a previous discovery run
    When discovery runs again
    Then the target registry contains exactly 1 entry for endpoint "https://mcp.example.com/v1"

  Scenario: discovery does not auto-register when auto_register_as_targets is false
    Given "auto_register_as_targets" is set to false
    When discovery runs once
    Then no new targets are added to the target registry

  # ── oauth auto-registration ───────────────────────────────────────────────

  Scenario: discovery registers OAuth config for an endpoint that declares it
    Given an llms.txt document lists an endpoint "https://acp.example.com/v1" with OAuth metadata
    And "auto_register_oauth" is set to true
    When discovery runs once
    Then an OAuth config is registered for endpoint "https://acp.example.com/v1"

  Scenario: discovery skips OAuth registration when auto_register_oauth is false
    Given an llms.txt endpoint "https://acp.example.com/v1" declares OAuth metadata
    And "auto_register_oauth" is set to false
    When discovery runs once
    Then no OAuth config is registered for endpoint "https://acp.example.com/v1"

  # ── discovery interval ────────────────────────────────────────────────────

  Scenario: discovery re-runs at the configured interval
    Given "interval" is set to "30s"
    When 31 seconds elapse after the first discovery run
    Then a second discovery run is initiated
    And no busy polling occurs between runs

  # ── self-health control target ────────────────────────────────────────────

  Scenario: self-health target is probed independently of discovery
    Given the "self-health" target is enabled in config "configs/samples/llmstxt-auto-discovery.yaml"
    When the probe for target "self-health" completes
    Then the target "self-health" is classified as "HEALTHY"
    And the health endpoint "http://127.0.0.1:18088/healthz" returned status code 200
