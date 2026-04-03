# features/selfdescription.feature
# Canon record — last audited: 2026-03-25
# Exercises: /.well-known/smoke-alarm.json endpoint, SelfDescription schema
# Code: internal/health/selfdescription.go, internal/health/server.go
# Step definitions: features/step_definitions/selfdescription_steps.go
# see: features/hosted-server.feature (/healthz, /readyz coverage)
# see: features/config-validation.feature (health.enabled config block)
#
# SelfDescription is the machine-readable service identity document.
# Schema reference: configs/schema/self-description.schema.json
# Served at: /.well-known/smoke-alarm.json (distinct from /healthz, /readyz, /status)

@selfdescription @core
Feature: Self-Description Endpoint
  As a downstream agent or orchestration tool
  I want to GET /.well-known/smoke-alarm.json and receive a machine-readable capability document
  So that I can discover what protocols, transports, and endpoints this instance supports

  Background:
    Given the ocd-smoke-alarm binary is installed
    And health.enabled is true in the config

  # ── endpoint availability ─────────────────────────────────────────────────

  Scenario: self-description endpoint is served when health is enabled
    Given ocd-smoke-alarm is running with config "configs/sample.yaml"
    When a GET request is sent to "http://127.0.0.1:8088/.well-known/smoke-alarm.json"
    Then the response status code is 200
    And the Content-Type is "application/json"

  Scenario: self-description endpoint is listed in health endpoints block
    Given the SelfDescription document for config "configs/sample.yaml"
    Then the document health.endpoints.self_description field is "/.well-known/smoke-alarm.json"

  # ── document structure ────────────────────────────────────────────────────

  Scenario: self-description document contains required top-level fields
    Given the SelfDescription document for config "configs/sample.yaml"
    Then the document contains field "version"
    And the document contains field "service"
    And the document contains field "health"
    And the document contains field "capabilities"
    And the document contains field "permissions"
    And the document contains field "mcp"
    And the document contains field "targets"

  Scenario: service block reflects the running config values
    Given the SelfDescription document for config "configs/sample.yaml"
    Then the document service.name is "ocd-smoke-alarm"
    And the document service.mode is "foreground"
    And the document service.environment is "dev"
    And the document service.started_at is a valid RFC3339 timestamp

  # ── capabilities reflect config ───────────────────────────────────────────

  Scenario: capabilities.monitoring.protocols lists mcp and acp when targets use both
    Given the SelfDescription document for config "configs/sample.yaml"
    Then the document capabilities.monitoring.protocols contains "mcp"
    And the document capabilities.monitoring.protocols contains "acp"

  Scenario: capabilities.discovery.llms_txt reflects the config discovery.llms_txt.enabled value
    Given ocd-smoke-alarm is running with config "configs/samples/llmstxt-auto-discovery.yaml"
    When a GET request is sent to the self-description endpoint
    Then the document capabilities.discovery.llms_txt is true

  Scenario: capabilities.discovery.llms_txt is false when llms_txt.enabled is false
    Given ocd-smoke-alarm is running with config "configs/samples/stdio-mcp-strict.yaml"
    When a GET request is sent to the self-description endpoint
    Then the document capabilities.discovery.llms_txt is false

  Scenario: capabilities.hosted reflects hosted.enabled from config
    Given ocd-smoke-alarm is running with config "configs/samples/hosted-mcp-acp.yaml"
    When a GET request is sent to the self-description endpoint
    Then the document capabilities.hosted.enabled is true

  Scenario: capabilities.dynamic_config reflects dynamic_config.enabled from config
    Given ocd-smoke-alarm is running with config "configs/samples/hosted-mcp-acp.yaml"
    When a GET request is sent to the self-description endpoint
    Then the document capabilities.dynamic_config.enabled is true

  Scenario: capabilities.meta_config reflects meta_config.enabled from config
    Given ocd-smoke-alarm is running with config "configs/sample.yaml"
    When a GET request is sent to the self-description endpoint
    Then the document capabilities.meta_config.enabled is true

  # ── targets list ─────────────────────────────────────────────────────────

  Scenario: targets list includes enabled targets from config
    Given ocd-smoke-alarm is running with config "configs/samples/hosted-mcp-acp.yaml"
    When a GET request is sent to the self-description endpoint
    Then the document targets list contains an entry with id "hosted-mcp-http"
    And the document targets list contains an entry with id "hosted-acp-http"

  Scenario: targets list does not include disabled targets
    Given ocd-smoke-alarm is running with config "configs/sample.yaml"
    When a GET request is sent to the self-description endpoint
    Then the document targets list does not contain an entry with id "mcp-cloud-primary"

  # ── health endpoints cross-reference ──────────────────────────────────────

  Scenario: health block lists all configured endpoint paths
    Given the SelfDescription document for config "configs/sample.yaml"
    Then the document health.endpoints.healthz is "/healthz"
    And the document health.endpoints.readyz is "/readyz"
    And the document health.endpoints.status is "/status"
    And the document health.listen_addr is "127.0.0.1:8088"

  # ── schema validation ─────────────────────────────────────────────────────

  Scenario: self-description document conforms to the JSON schema
    Given the schema at "configs/schema/self-description.schema.json"
    And the SelfDescription document for config "configs/sample.yaml"
    When the document is validated against the schema
    Then validation passes with no errors
