# features/health.feature
# Canon record — last audited: 2026-03-26 (updated: added BindWithRetry + mDNS-on-actual-port scenarios; fixed {int} in non-Outline steps)
# Step definitions: features/step_definitions/health_steps.go

@health @core
Feature: Health and Status Endpoints
  As an operator or orchestration system
  I want deterministic HTTP endpoints for liveness, readiness, status, and self-description
  So that I can observe and gate on the service's health without parsing logs

  Background:
    Given the ocd-smoke-alarm binary is installed
    And a valid config file "configs/sample.yaml" exists
    And the health server is started on an available port

  # ── /healthz ──────────────────────────────────────────────────────────────

  Scenario: /healthz returns 200 when the service is live
    Given the service is live
    When a GET request is sent to "/healthz"
    Then the response status code is 200
    And the response body contains "ok"

  Scenario: /healthz returns 503 when liveness is set to false
    Given liveness is set to false
    When a GET request is sent to "/healthz"
    Then the response status code is 503
    And the response body contains "unhealthy"

  Scenario: /healthz response includes the service name
    Given the service is live
    When a GET request is sent to "/healthz"
    Then the response body contains the configured service name

  # ── /readyz ───────────────────────────────────────────────────────────────

  Scenario: /readyz returns 503 when ready flag is false
    Given the ready flag is false
    When a GET request is sent to "/readyz"
    Then the response status code is 503
    And the response body contains "not_ready"

  Scenario: /readyz returns 200 when ready flag is true and all components are healthy
    Given the ready flag is true
    And all registered components are healthy
    When a GET request is sent to "/readyz"
    Then the response status code is 200
    And the response body contains "ready"

  Scenario: /readyz returns 503 when any component is unhealthy even with ready flag true
    Given the ready flag is true
    And a component "probe-engine" is registered as unhealthy
    When a GET request is sent to "/readyz"
    Then the response status code is 503
    And the response body contains "not_ready"
    And the response body contains "one or more components are unhealthy"

  Scenario: /readyz response includes target summary counts
    Given the ready flag is true
    And a target "t1" has state "healthy"
    And a target "t2" has state "unhealthy"
    When a GET request is sent to "/readyz"
    Then the response body includes a summary with 2 total targets

  Scenario: /readyz exposes ready_error when set
    Given the service is set not-ready with reason "waiting for initial probe"
    When a GET request is sent to "/readyz"
    Then the response body contains "waiting for initial probe"

  # ── /status ───────────────────────────────────────────────────────────────

  Scenario: /status always returns 200
    When a GET request is sent to "/status"
    Then the response status code is 200

  Scenario: /status response includes service name and uptime
    When a GET request is sent to "/status"
    Then the response body contains the configured service name
    And the response body uptime_sec is at least 0

  Scenario: /status response includes live and ready flags
    Given the service is live
    And the ready flag is true
    When a GET request is sent to "/status"
    Then the response body live flag is true
    And the response body ready flag is true

  Scenario: /status components are sorted alphabetically by name
    Given components "zebra", "alpha", "mango" are registered as healthy
    When a GET request is sent to "/status"
    Then the components list in the response is ordered "alpha", "mango", "zebra"

  Scenario: /status targets are sorted alphabetically by ID
    Given targets "z1", "a2", "m3" are registered with state "healthy"
    When a GET request is sent to "/status"
    Then the targets list in the response is ordered "a2", "m3", "z1"

  Scenario: /status summary counts reflect registered target states
    Given a target "t1" has state "healthy"
    And a target "t2" has state "unhealthy"
    And a target "t3" has state "regression"
    When a GET request is sent to "/status"
    Then the summary healthy count is 1
    And the summary unhealthy count is 1
    And the summary regression count is 1
    And the summary total is 3

  Scenario: /status treats "failed" state as unhealthy in summary counts
    Given a target "t1" has state "failed"
    When a GET request is sent to "/status"
    Then the summary unhealthy count is 1

  Scenario: /status summary treats unrecognized state as unknown
    Given a target "t1" has state "mystery"
    When a GET request is sent to "/status"
    Then the summary unknown count is 1

  # ── target status management ──────────────────────────────────────────────

  Scenario: UpsertTargetStatus normalizes state to lowercase
    Given a target status is upserted with id "t1" and state "HEALTHY"
    When a GET request is sent to "/status"
    Then the target "t1" state in the response is "healthy"

  Scenario: UpsertTargetStatus defaults empty state to unknown
    Given a target status is upserted with id "t1" and an empty state
    When a GET request is sent to "/status"
    Then the target "t1" state in the response is "unknown"

  Scenario: UpsertTargetStatus ignores entries with empty ID
    Given a target status is upserted with an empty id
    When a GET request is sent to "/status"
    Then no target with empty id appears in the response

  Scenario: RemoveTarget removes a target from status output
    Given a target "t1" has state "healthy"
    When target "t1" is removed
    And a GET request is sent to "/status"
    Then the target "t1" does not appear in the response

  # ── component management ──────────────────────────────────────────────────

  Scenario: SetComponent with empty name is a no-op
    Given a component "" is registered as healthy
    When a GET request is sent to "/status"
    Then no component with empty name appears in the response

  Scenario: RemoveComponent removes a component from status output
    Given a component "probe-engine" is registered as healthy
    When component "probe-engine" is removed
    And a GET request is sent to "/status"
    Then component "probe-engine" does not appear in the response

  Scenario: Readiness composite: component becoming healthy restores readiness
    Given the ready flag is true
    And a component "probe-engine" is registered as unhealthy
    When component "probe-engine" is updated to healthy
    And a GET request is sent to "/readyz"
    Then the response status code is 200

  # ── /.well-known/smoke-alarm.json ─────────────────────────────────────────

  Scenario: Self-description endpoint returns 404 when no factory is configured
    Given no self-description factory is registered
    When a GET request is sent to "/.well-known/smoke-alarm.json"
    Then the response status code is 404
    And the response body contains "self-description not configured"

  Scenario: Self-description endpoint returns 200 with factory registered
    Given a self-description factory returning service name "test-service" is registered
    When a GET request is sent to "/.well-known/smoke-alarm.json"
    Then the response status code is 200
    And the response body contains "test-service"

  # ── /federation/report ────────────────────────────────────────────────────

  Scenario: POST to /federation/report accepts target status and updates snapshot
    Given a target "remote-t1" is not in the local status
    When a POST is sent to "/federation/report" with targets including "remote-t1" in state "healthy"
    Then the response status code is 200
    And a GET request is sent to "/status"
    And the target "remote-t1" state in the response is "healthy"

  Scenario: GET to /federation/report returns 405
    When a GET request is sent to "/federation/report"
    Then the response status code is 405

  Scenario: POST to /federation/report with malformed JSON returns 400
    When a POST is sent to "/federation/report" with body "not json"
    Then the response status code is 400

  # ── port binding (BindWithRetry) ──────────────────────────────────────────

  Scenario: BindWithRetry binds on the configured address when the port is free
    Given the configured listen address has a free port
    When BindWithRetry is called with 10 retries
    Then the returned address matches the configured listen address

  Scenario: BindWithRetry yields to the next available port when the configured port is occupied
    Given the configured listen port is occupied by another process
    When BindWithRetry is called with 10 retries
    Then the returned address differs from the configured listen address
    And the returned port is higher than the configured port

  Scenario: the mDNS advertiser is registered on the actual bound port after BindWithRetry
    Given the configured listen port is occupied by another process
    And BindWithRetry is called with 10 retries
    When the mDNS advertiser is started with the actual bound address
    Then the advertiser registers the service on the actual bound port
    And the advertiser does not register on the configured port

  # ── shutdown ───────────────────────────────────────────────────────────────

  Scenario: Shutdown sets liveness to false
    Given the service is live
    When the health server is shut down
    Then the service is no longer live
