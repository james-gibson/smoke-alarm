# features/stdio-mcp.feature
# Canon record — last audited: 2026-03-25
# Exercises: stdio transport probe (npx/uvx), strict handshake profile
# Config reference: configs/samples/stdio-mcp-strict.yaml
# Step definitions: features/step_definitions/stdio_mcp_steps.go
# see: features/config-validation.feature (schema), features/dynamic-config.feature (state output)

@stdio-mcp @core
Feature: Stdio MCP Probe
  As an operator monitoring a locally-spawned MCP server
  I want ocd-smoke-alarm to launch the server process, complete the MCP handshake, and classify health
  So that stdio-transport servers are monitored without a persistent HTTP port

  Background:
    Given the ocd-smoke-alarm binary is installed
    And a valid config file "configs/samples/stdio-mcp-strict.yaml" exists
    And the "npx" command is available on PATH

  # ── process lifecycle ─────────────────────────────────────────────────────

  Scenario: probe spawns the configured command and establishes stdio transport
    Given the target "mcp-stdio-filesystem-strict" is enabled in config "configs/samples/stdio-mcp-strict.yaml"
    When the probe for target "mcp-stdio-filesystem-strict" runs
    Then a child process is spawned with command "npx"
    And the process receives an MCP initialize request over stdin
    And the process responds with a valid MCP initialize response over stdout

  Scenario: probe terminates the child process after the check completes
    Given the target "mcp-stdio-filesystem-strict" is enabled in config "configs/samples/stdio-mcp-strict.yaml"
    When the probe for target "mcp-stdio-filesystem-strict" completes
    Then no orphan child processes remain for target "mcp-stdio-filesystem-strict"

  Scenario: probe classifies target as HEALTHY after a successful strict handshake
    Given the target "mcp-stdio-filesystem-strict" responds to all required methods
    When the probe for target "mcp-stdio-filesystem-strict" completes
    Then the target "mcp-stdio-filesystem-strict" is classified as "HEALTHY"

  # ── strict handshake profile ──────────────────────────────────────────────

  Scenario Outline: strict handshake requires all declared methods
    Given the target "mcp-stdio-filesystem-strict" does not respond to method "<method>"
    When the probe for target "mcp-stdio-filesystem-strict" runs
    Then the target "mcp-stdio-filesystem-strict" is classified as "DEGRADED"
    And the probe result contains "missing required method"

    Examples:
      | method         |
      | initialize     |
      | tools/list     |
      | resources/list |

  # ── failure classification ────────────────────────────────────────────────

  Scenario: probe classifies target as OUTAGE when process fails to start
    Given the stdio command "npx" is not available on PATH
    When the probe for target "mcp-stdio-filesystem-strict" runs
    Then the target "mcp-stdio-filesystem-strict" is classified as "OUTAGE"
    And the probe result contains "failed to spawn"

  Scenario Outline: probe classifies target as DEGRADED when process exits before handshake completes
    Given the target "mcp-stdio-filesystem-strict" process exits with code <exit-code> during handshake
    When the probe for target "mcp-stdio-filesystem-strict" runs
    Then the target "mcp-stdio-filesystem-strict" is classified as "DEGRADED"

    Examples:
      | exit-code |
      | 1         |
      | 127       |

  Scenario: probe respects the configured timeout
    Given the target "mcp-stdio-filesystem-strict" has timeout "5s"
    And the spawned process does not respond within 6 seconds
    When the probe for target "mcp-stdio-filesystem-strict" runs
    Then the target "mcp-stdio-filesystem-strict" is classified as "DEGRADED"
    And the probe result contains "timeout"

  # ── env and cwd passthrough ───────────────────────────────────────────────

  Scenario: probe passes configured env vars to the child process
    Given the target "mcp-stdio-filesystem-strict" has env var "MCP_LOG_LEVEL" set to "warn"
    When the probe for target "mcp-stdio-filesystem-strict" spawns the process
    Then the child process environment contains "MCP_LOG_LEVEL=warn"

  Scenario: probe sets the working directory for the child process
    Given the target "mcp-stdio-filesystem-strict" has cwd "."
    When the probe for target "mcp-stdio-filesystem-strict" spawns the process
    Then the child process working directory is the project root
