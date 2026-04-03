# features/dynamic-config.feature
# Canon record — last audited: 2026-03-25
# Exercises: dynamic-config persist subcommand, JSON+Markdown artifact generation, allow_overwrite, require_unique_ids
# Config reference: all sample configs (dynamic_config block present in each)
# Step definitions: features/step_definitions/dynamic_config_steps.go
# see: features/config-validation.feature (schema), features/hosted-server.feature (serve_base_url)

@dynamic-config @optional
Feature: Dynamic Config Persistence
  As an operator or agent managing MCP/ACP targets
  I want ocd-smoke-alarm to generate uniquely-identified config artifacts in JSON and Markdown formats
  So that discovered and validated configurations can be statically served or consumed by downstream tools

  Background:
    Given the ocd-smoke-alarm binary is installed

  # ── persist subcommand ────────────────────────────────────────────────────

  Scenario: dynamic-config persist generates JSON and Markdown artifacts for the llmstxt config
    Given a valid config file "configs/samples/llmstxt-auto-discovery.yaml" exists
    And the dynamic config directory "state/llmstxt-discovery/dynamic-config" is empty
    When I run the "dynamic-config" subcommand with args "persist --config=configs/samples/llmstxt-auto-discovery.yaml"
    Then the exit code is 0
    And a JSON file exists under "state/llmstxt-discovery/dynamic-config"
    And a Markdown file exists under "state/llmstxt-discovery/dynamic-config"

  Scenario Outline: persist generates artifacts for all sample configs
    Given a valid config file "<config-path>" exists
    When I run the "dynamic-config" subcommand with args "persist --config=<config-path>"
    Then the exit code is 0
    And a JSON file exists under the config's dynamic_config.directory

    Examples:
      | config-path                                      |
      | configs/samples/stdio-mcp-strict.yaml            |
      | configs/samples/llmstxt-auto-discovery.yaml      |
      | configs/samples/sse-remote-mixed.yaml            |
      | configs/samples/oauth-mock-fail.yaml             |
      | configs/samples/oauth-mock-allow.yaml            |
      | configs/samples/hosted-mcp-acp.yaml              |

  # ── artifact uniqueness ───────────────────────────────────────────────────

  Scenario: each generated artifact has a unique ID
    Given the dynamic config directory is empty
    When I run the "dynamic-config" subcommand with args "persist --config=configs/samples/hosted-mcp-acp.yaml"
    Then all JSON artifacts under the output directory have distinct "id" fields

  Scenario: persist fails when require_unique_ids is true and a duplicate ID would be written
    Given a JSON artifact with id "duplicate-target" already exists in the output directory
    And "require_unique_ids" is true
    When I run the "dynamic-config" subcommand with args "persist --config=configs/samples/hosted-mcp-acp.yaml"
    Then the exit code is non-zero
    And stderr contains "duplicate id"

  # ── overwrite behavior ───────────────────────────────────────────────────

  Scenario: persist overwrites existing artifacts when allow_overwrite is true
    Given an artifact already exists at the expected output path
    And "allow_overwrite" is true
    When I run the "dynamic-config" subcommand with args "persist --config=configs/samples/hosted-mcp-acp.yaml"
    Then the exit code is 0
    And the existing artifact is replaced with updated content

  Scenario: persist fails when allow_overwrite is false and an artifact already exists
    Given an artifact already exists at the expected output path
    And "allow_overwrite" is false in config "configs/samples/stdio-mcp-strict.yaml"
    When I run the "dynamic-config" subcommand with args "persist --config=configs/samples/stdio-mcp-strict.yaml"
    Then the exit code is non-zero
    And stderr contains "already exists"

  # ── artifact content ──────────────────────────────────────────────────────

  Scenario: generated JSON artifact contains confidence and provenance fields
    Given "meta_config.include_confidence" is true
    And "meta_config.include_provenance" is true
    When I run the "dynamic-config" subcommand with args "persist --config=configs/samples/hosted-mcp-acp.yaml"
    Then each JSON artifact contains a "confidence" field
    And each JSON artifact contains a "provenance" field

  Scenario: generated JSON artifact uses placeholder values for secrets
    When I run the "dynamic-config" subcommand with args "persist --config=configs/sample.yaml"
    Then no JSON artifact contains a raw keychain secret
    And token placeholders match the pattern "${TOKEN}"
    And client_secret placeholders match the pattern "${CLIENT_SECRET}"

  Scenario: generated Markdown artifact is human-readable and contains target IDs
    Given a valid config file "configs/samples/hosted-mcp-acp.yaml" exists
    When I run the "dynamic-config" subcommand with args "persist --config=configs/samples/hosted-mcp-acp.yaml"
    Then each Markdown artifact contains at least one target ID as a heading
    And the Markdown is valid CommonMark

  # ── output directory ──────────────────────────────────────────────────────

  Scenario: persist creates the output directory if it does not exist
    Given the dynamic config directory "state/fresh-test/dynamic-config" does not exist
    When I run the "dynamic-config" subcommand with args "persist --config=configs/samples/hosted-mcp-acp.yaml"
    Then the directory "state/fresh-test/dynamic-config" is created
    And the exit code is 0

  # ── formats ───────────────────────────────────────────────────────────────

  Scenario Outline: persist respects the formats list
    Given a config with dynamic_config.formats set to ["<format>"]
    When I run the "dynamic-config" subcommand with args "persist --config=<config-path>"
    Then only "<format>" format artifacts are written to the output directory

    Examples:
      | format   | config-path                                 |
      | json     | configs/samples/hosted-mcp-acp.yaml         |
      | markdown | configs/samples/hosted-mcp-acp.yaml         |
