# features/telemetry.feature
# Canon record — last audited: 2026-03-26
# Exercises: telemetry subsystem — OTEL metric export, system metrics sampling
# Code: internal/telemetry/telemetry.go
# Step definitions: features/step_definitions/telemetry_steps.go
# see: features/known-state.feature (target states recorded as gauges)
# see: features/config-validation.feature (telemetry config block validation)
# see: features/ops.feature (runtime bootstrap wires telemetry)
#
# ARCHITECTURE NOTE:
#   Exporter wraps an OpenTelemetry PeriodicReader and Meter Provider.
#   All metric recording is additive (no state held between calls).
#   Close() flushes the final batch and shuts down the meter provider.
#   Telemetry is a best-effort subsystem — failures must not affect probing.
#
# METRIC NAMES (canonical):
#   check_latency_ms    — histogram of probe round-trip time per target
#   check_failures      — counter of probe failures per target + failure class
#   target_state        — observable gauge: state value (0=healthy … 3=outage)
#   system_memory_bytes — observable gauge: runtime.MemStats.Alloc
#   system_goroutines   — observable gauge: runtime.NumGoroutine()
#   system_gc_runs      — observable gauge: runtime.MemStats.NumGC

@telemetry @optional
Feature: Telemetry — OTEL Metric Export and System Sampling
  As an operator running ocd-smoke-alarm in a monitored environment
  I want probe results and system health to be exported as OTEL metrics
  So that dashboards, alerts, and SLOs can be derived from structured metric data

  Background:
    Given the ocd-smoke-alarm binary is installed
    And a config with telemetry.enabled true and telemetry.endpoint "http://localhost:4318/v1/metrics"

  # ── exporter construction ──────────────────────────────────────────────────

  Scenario: NewExporter returns a configured Exporter when telemetry is enabled
    Given telemetry.enabled is true
    And telemetry.endpoint is "http://localhost:4318/v1/metrics"
    When NewExporter is called
    Then the Exporter holds an OTEL HTTP exporter pointed at that endpoint
    And the PeriodicReader is configured with the export_interval from config

  Scenario: NewExporter returns a no-op Exporter when telemetry is disabled
    Given telemetry.enabled is false
    When NewExporter is called
    Then the returned Exporter is a no-op
    And no network connection is attempted

  Scenario: NewExporter uses the service_name from config as the OTEL resource attribute
    Given telemetry.service_name is "ocd-smoke-alarm"
    When NewExporter is called
    Then the OTEL resource has a service.name attribute equal to that value

  # ── check latency ─────────────────────────────────────────────────────────

  Scenario: RecordCheckLatency increments the latency histogram for the named target
    Given an Exporter is running
    When RecordCheckLatency is called with target "mcp-primary" and latency 120 milliseconds
    Then the check_latency_ms histogram has a data point for that target
    And the data point value is 120

  Scenario: RecordCheckLatency records distinct data points for different targets
    Given an Exporter is running
    When RecordCheckLatency is called for target "mcp-primary" with 120ms
    And RecordCheckLatency is called for target "acp-remote" with 340ms
    Then the histogram contains separate data points for each target

  # ── check failures ────────────────────────────────────────────────────────

  Scenario: RecordCheckFailure increments the failure counter for the named target and class
    Given an Exporter is running
    When RecordCheckFailure is called with target "mcp-primary" and failure_class "timeout"
    Then the check_failures counter is incremented by 1
    And the counter attributes include target and failure_class

  Scenario: RecordCheckFailure accumulates across multiple calls for the same target
    Given an Exporter is running
    When RecordCheckFailure is called 3 times for target "mcp-primary"
    Then the check_failures counter for that target is 3

  # ── target state gauge ────────────────────────────────────────────────────

  Scenario: RecordTargetState sets the observable gauge to the numeric state value
    Given an Exporter is running
    When RecordTargetState is called with target "mcp-primary" and state "healthy"
    Then the target_state gauge for that target is 0

  Scenario: RecordTargetState reflects degraded state as a non-zero gauge value
    Given an Exporter is running
    When RecordTargetState is called with target "mcp-primary" and state "degraded"
    Then the target_state gauge for that target is greater than 0

  Scenario: RecordTargetState updates the gauge when the target state changes
    Given RecordTargetState was previously called with state "healthy"
    When RecordTargetState is called again for the same target with state "outage"
    Then the target_state gauge reflects the new state value

  # ── system metrics ────────────────────────────────────────────────────────

  Scenario: RecordSystemMetrics registers observable gauges for memory, goroutines, and GC
    Given an Exporter is running
    When RecordSystemMetrics is called
    Then the meter has an observable gauge named "system_memory_bytes"
    And the meter has an observable gauge named "system_goroutines"
    And the meter has an observable gauge named "system_gc_runs"

  Scenario: system_memory_bytes gauge reflects the current heap allocation
    Given RecordSystemMetrics has been called
    When an export cycle runs
    Then the system_memory_bytes value is greater than 0
    And the value is within a plausible range for a running Go process

  Scenario: system_goroutines gauge reflects the current goroutine count
    Given RecordSystemMetrics has been called
    When an export cycle runs
    Then the system_goroutines value is greater than 0

  # ── export interval ───────────────────────────────────────────────────────

  Scenario: metrics are exported at the configured export_interval
    Given telemetry.export_interval is "5s"
    And the Exporter is running
    When 6 seconds elapse
    Then at least one export request has been sent to the telemetry endpoint

  Scenario: metrics are not exported before the first export_interval elapses
    Given telemetry.export_interval is "30s"
    And the Exporter is running
    When 5 seconds elapse
    Then no export request has been sent to the telemetry endpoint

  # ── endpoint unavailability ───────────────────────────────────────────────

  Scenario: an unreachable telemetry endpoint does not cause the probe loop to fail
    Given telemetry.endpoint points to an unreachable address
    And the Exporter is running
    When a probe cycle runs and RecordCheckLatency is called
    Then no error is propagated to the caller
    And a "warn" log entry is written containing the export failure

  # ── graceful close ────────────────────────────────────────────────────────

  Scenario: Close flushes the final metric batch before shutdown
    Given the Exporter has recorded metrics that have not yet been exported
    When Close is called
    Then a final export request is sent before the function returns

  Scenario: Close shuts down the meter provider
    When Close is called
    Then subsequent RecordCheckLatency calls are no-ops
    And no further export requests are sent
