---
title: Telemetry — OTEL Metric Export and System Sampling
description: "The telemetry Exporter: check latency histogram, failure counter, target-state gauge, system gauges, export interval, endpoint resilience, and graceful close."
tags:
  - smoke-alarm
  - feature
  - ops-safety
  - telemetry
  - otel
  - gherkin
source: smoke-alarm/features/telemetry.feature
subsystem: ops-safety
gherkin_tags:
  - "@telemetry"
  - "@optional"
scenario_count: 18
---
> **Source:** `features/telemetry.feature` · **Subsystem:** [Operations & Safety](../subsystems/ops-safety.md) · **Index:** [Feature catalog](../index.md)

The telemetry subsystem (`internal/telemetry/telemetry.go`) exports probe results and system health as OTEL metrics. The Exporter wraps an OpenTelemetry PeriodicReader and Meter Provider; all metric recording is additive (no state held between calls); `Close()` flushes the final batch and shuts down the meter provider. Telemetry is best-effort — failures must not affect probing.

Canonical metric names: `check_latency_ms` (histogram), `check_failures` (counter), `target_state` (gauge: 0=healthy … 3=outage), `system_memory_bytes`, `system_goroutines`, `system_gc_runs` (gauges).

## Scenario map

**Exporter construction** — enabled → an OTEL HTTP exporter at `telemetry.endpoint` with the `export_interval` PeriodicReader; disabled → a no-op exporter (no network); `service_name` becomes the OTEL `service.name` resource attribute.

**Check latency** — `RecordCheckLatency` adds histogram data points per target; distinct targets produce distinct points.

**Check failures** — `RecordCheckFailure` increments the counter per target+failure_class and accumulates across calls.

**Target state gauge** — `RecordTargetState` sets the numeric state (`healthy` → 0; `degraded` > 0) and updates on state change.

**System metrics** — `RecordSystemMetrics` registers gauges for memory, goroutines, and GC; values reflect the live process (memory > 0 within a plausible range, goroutines > 0).

**Export interval** — metrics export at `export_interval` (≥1 request after 6 s at 5 s); none before the first interval (no request after 5 s at 30 s).

**Endpoint unavailability** — an unreachable endpoint does not fail the probe loop; a "warn" log entry captures the export failure.

**Graceful close** — `Close` flushes the final batch before returning and shuts down the meter provider (subsequent recordings are no-ops, no further exports).

## Cross-references

- [Known-State Regression Detection](known-state.md) — target states recorded as gauges
- [Config Validation](config-validation.md) — telemetry config block validation
- [Ops — Runtime Lifecycle](ops.md) — runtime bootstrap wires telemetry
- [Operations & Safety subsystem](../subsystems/ops-safety.md)