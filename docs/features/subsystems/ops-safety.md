---
title: Operations & Safety
description: Runtime lifecycle and update orchestration, alert routing, pre-protocol HURL safety, chaos windows, telemetry, MCP/ACP priority routing, and the OAuth mock.
tags:
  - smoke-alarm
  - subsystem
  - ops-safety
feature_count: 7
---
> Part of the [Feature Knowledge Base](../index.md). See the [consolidated catalog](../consolidated.md) for a flat view.

# Operations & Safety

Features covering how ocd-smoke-alarm manages its own lifecycle and protects the systems it probes: runtime/update orchestration, alert routing, pre-protocol safety gating, chaos-window suppression, telemetry export, priority routing with self-healing, and the OAuth mock endpoint.

## Features

| Feature | Source | Type |
|---|---|---|
| [Ops — Runtime Lifecycle and Update Orchestration](../catalog/ops.md) | `features/ops.feature` | lifecycle |
| [Alert Routing and Severity Policy](../catalog/alerts.md) | `features/alerts.feature` | notifications |
| [Pre-Protocol HURL Safety Scanner](../catalog/safety-hurl.md) | `features/safety-hurl.feature` | safety gate |
| [Chaos Window Registration and Isotope Classification](../catalog/chaos-window.md) | `features/chaos-window.feature` | suppression |
| [Telemetry — OTEL Metric Export](../catalog/telemetry.md) | `features/telemetry.feature` | observability |
| [MCP/ACP Priority Routing and Self-Healing](../catalog/routing.md) | `features/routing.feature` | routing |
| [OAuth Mock Redirect Endpoint](../catalog/oauth-mock.md) | `features/oauth-mock.feature` | test harness |

## Cross-cutting notes

- Safety gates run *before* protocol work: HURL checks, the chaos-window classification, and routing admission all keep failing targets out of the active path rather than discovering failure mid-handshake.
- The Ops update workflow is the operator-facing safety net: lock-gated, health-verified, journaled, and rolled back on failure.
- Alerting and routing share the isotope vocabulary — a failure carrying a pre-registered chaos isotope is classified, not paged.
- Telemetry is deliberately best-effort: observability must never degrade the probing it observes.