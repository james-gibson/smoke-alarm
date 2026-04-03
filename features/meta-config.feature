# features/meta-config.feature
# Canon record — last audited: 2026-03-25
# Exercises: meta_config block, internal/meta Generator — partial config generation for mcp-add workflows
# Code: internal/meta/generator.go
# Step definitions: features/step_definitions/meta_config_steps.go
# see: features/dynamic-config.feature (different: dynamic_config is probe artifacts; meta_config is mcp-add templates)
# see: features/config-validation.feature (meta_config schema fields)
#
# IMPORTANT: meta_config and dynamic_config are DISTINCT subsystems.
# meta_config  → partial/template configs suitable for copy-paste into mcp-add, with placeholder substitution
# dynamic_config → uniquely-identified probe result artifacts for static serving
# Both are present in all sample configs. Do not conflate them.

@meta-config @optional
Feature: Meta-Config Template Generation
  As an operator or agent using mcp-add
  I want ocd-smoke-alarm to generate partial-but-valid meta configs from discovered targets
  So that I can copy-paste them into mcp-add without manually reconstructing endpoint and auth fields

  Background:
    Given the ocd-smoke-alarm binary is installed

  # ── generator defaults ────────────────────────────────────────────────────

  Scenario: generator uses default output_dir when not specified in config
    Given a meta_config block with no output_dir field
    When the meta-config generator is initialised
    Then the output directory defaults to "./state/meta-config"

  Scenario: generator uses default formats when not specified in config
    Given a meta_config block with no formats field
    When the meta-config generator is initialised
    Then the formats default to ["yaml", "json"]

  Scenario: generator uses default token placeholder when not specified
    Given a meta_config block with no placeholders.token field
    When the meta-config generator is initialised
    Then the token placeholder defaults to "${TOKEN}"

  # ── output formats ────────────────────────────────────────────────────────

  Scenario: generator writes YAML output when "yaml" is in formats
    Given "meta_config.formats" includes "yaml" in config "configs/sample.yaml"
    When the meta-config generator runs
    Then a YAML file is written under "state/meta-config"
    And the YAML file contains a "version" field
    And the YAML file contains a "generated_at" field
    And the YAML file contains an "entries" list

  Scenario: generator writes JSON output when "json" is in formats
    Given "meta_config.formats" includes "json" in config "configs/sample.yaml"
    When the meta-config generator runs
    Then a JSON file is written under "state/meta-config"
    And the JSON file is valid JSON

  Scenario Outline: generator writes output for all sample configs
    Given a valid config file "<config-path>" with meta_config.enabled true
    When the meta-config generator runs with that config
    Then at least one output file is written under the meta_config.output_dir

    Examples:
      | config-path                                      |
      | configs/samples/stdio-mcp-strict.yaml            |
      | configs/samples/llmstxt-auto-discovery.yaml      |
      | configs/samples/sse-remote-mixed.yaml            |
      | configs/samples/oauth-mock-fail.yaml             |
      | configs/samples/oauth-mock-allow.yaml            |
      | configs/samples/hosted-mcp-acp.yaml              |

  # ── placeholder substitution ──────────────────────────────────────────────

  Scenario: bearer token secret_ref is replaced with the token placeholder
    Given a target with auth type "bearer" and secret_ref "keychain://ocd-smoke-alarm/mcp-cloud-primary/token"
    When the meta-config generator produces an entry for that target
    Then the entry auth secret_ref value is "${TOKEN}"

  Scenario: OAuth client secret is replaced with the client_secret placeholder
    Given a target with auth type "oauth" and secret_ref "keychain://ocd-smoke-alarm/acp-remote-agent/client-secret"
    When the meta-config generator produces an entry for that target
    Then the entry auth secret_ref value is "${CLIENT_SECRET}"

  Scenario: endpoint value is replaced with the endpoint placeholder when include_provenance is true
    Given "meta_config.include_provenance" is true
    And a target with endpoint "https://mcp.example.com/v1"
    When the meta-config generator produces an entry for that target
    Then the entry retains the original endpoint value
    And a provenance note records the original source

  # ── confidence and provenance ─────────────────────────────────────────────

  Scenario: each entry includes a confidence field when include_confidence is true
    Given "meta_config.include_confidence" is true in config "configs/sample.yaml"
    When the meta-config generator produces entries
    Then each entry contains a "confidence" field
    And the confidence value is between 0.0 and 1.0

  Scenario: each entry includes a provenance field when include_provenance is true
    Given "meta_config.include_provenance" is true in config "configs/sample.yaml"
    When the meta-config generator produces entries
    Then each entry contains a "provenance" field
    And the provenance field identifies the source config file

  Scenario: confidence and provenance fields are absent when their flags are false
    Given "meta_config.include_confidence" is false
    And "meta_config.include_provenance" is false
    When the meta-config generator produces entries
    Then no entry contains a "confidence" field
    And no entry contains a "provenance" field

  # ── document structure ────────────────────────────────────────────────────

  Scenario: generated document contains version, generated_at, source, and entries
    When the meta-config generator runs with config "configs/sample.yaml"
    Then the document "version" field is present
    And the document "generated_at" field is a valid RFC3339 timestamp
    And the document "source" field identifies the config file
    And the document "entries" list is non-empty

  Scenario: each entry contains id and name fields
    When the meta-config generator runs with config "configs/sample.yaml"
    Then each entry contains an "id" field
    And each entry contains a "name" field
    And no two entries share the same "id"

  # ── disabled meta_config ──────────────────────────────────────────────────

  Scenario: generator does not write output when meta_config.enabled is false
    Given a config with meta_config.enabled set to false
    When the meta-config generator runs
    Then no files are written to the meta_config output_dir
