# features/config-validation.feature
# Canon record — last audited: 2026-03-25
# Exercises: validate + check subcommands, schema version, required fields
# Step definitions: features/step_definitions/config_validation_steps.go
# see: features/dynamic-config.feature (config output), features/stdio-mcp.feature (target fields)

@config-validation @core
Feature: Config Validation
  As an operator
  I want the config schema to be validated before any probe runs
  So that misconfigured targets are caught at startup, not at runtime

  Background:
    Given the ocd-smoke-alarm binary is installed

  # ── validate subcommand ───────────────────────────────────────────────────

  Scenario: validate accepts the canonical sample config
    When I run the "validate" subcommand with config "configs/sample.yaml"
    Then the exit code is 0
    And stdout contains no error markers

  Scenario: validate accepts all sample configs
    When I run the "validate" subcommand with config "configs/samples/stdio-mcp-strict.yaml"
    Then the exit code is 0
    When I run the "validate" subcommand with config "configs/samples/llmstxt-auto-discovery.yaml"
    Then the exit code is 0
    When I run the "validate" subcommand with config "configs/samples/sse-remote-mixed.yaml"
    Then the exit code is 0
    When I run the "validate" subcommand with config "configs/samples/oauth-mock-fail.yaml"
    Then the exit code is 0
    When I run the "validate" subcommand with config "configs/samples/oauth-mock-allow.yaml"
    Then the exit code is 0
    When I run the "validate" subcommand with config "configs/samples/hosted-mcp-acp.yaml"
    Then the exit code is 0

  Scenario Outline: validate rejects a config with a missing required field
    Given a config file exists at "<config-path>" with the "<removed-field>" field removed
    When I run the "validate" subcommand with config "<config-path>"
    Then the exit code is non-zero
    And stderr contains "required"

    Examples:
      | config-path                        | removed-field |
      | /tmp/test-missing-version.yaml     | version       |
      | /tmp/test-missing-service.yaml     | service       |
      | /tmp/test-missing-targets.yaml     | targets       |

  Scenario: validate rejects a config that does not exist
    When I run the "validate" subcommand with config "configs/nonexistent.yaml"
    Then the exit code is non-zero
    And stderr contains "no such file"

  Scenario: validate rejects a config with an unknown schema version
    Given a config file exists at "/tmp/test-bad-version.yaml" with version "99"
    When I run the "validate" subcommand with config "/tmp/test-bad-version.yaml"
    Then the exit code is non-zero
    And stderr contains "unsupported version"

  # ── check subcommand ──────────────────────────────────────────────────────

  Scenario: check exits cleanly when all enabled targets are reachable
    Given all enabled targets in "configs/sample.yaml" are reachable
    When I run the "check" subcommand with config "configs/sample.yaml"
    Then the exit code is 0
    And stdout contains no error markers

  Scenario Outline: check reports a non-zero exit when a target is unreachable
    Given the target "<target-id>" in config "<config-path>" is unreachable
    When I run the "check" subcommand with config "<config-path>"
    Then the exit code is non-zero

    Examples:
      | target-id        | config-path             |
      | mcp-local-proxy  | configs/sample.yaml     |

  # ── target field validation ───────────────────────────────────────────────

  Scenario Outline: validate rejects a target with an invalid transport
    Given a config file exists at "/tmp/test-bad-transport.yaml" with target transport "<transport>"
    When I run the "validate" subcommand with config "/tmp/test-bad-transport.yaml"
    Then the exit code is non-zero
    And stderr contains "invalid transport"

    Examples:
      | transport |
      | ftp       |
      | grpc      |
      |           |

  Scenario Outline: validate rejects a target with an invalid protocol
    Given a config file exists at "/tmp/test-bad-protocol.yaml" with target protocol "<protocol>"
    When I run the "validate" subcommand with config "/tmp/test-bad-protocol.yaml"
    Then the exit code is non-zero
    And stderr contains "invalid protocol"

    Examples:
      | protocol |
      | graphql  |
      | unknown  |
