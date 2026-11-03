---
title: Health & Status
description: "Observability surfaces: health/readiness/status endpoints, self-description document, and the OpenCode status-report skill."
tags:
  - smoke-alarm
  - subsystem
  - health-status
feature_count: 3
---
> Part of the [Feature Knowledge Base](../index.md). See the [consolidated catalog](../consolidated.md) for a flat view.

# Health & Status

Features covering the operator-facing observability surfaces of `ocd-smoke-alarm`: liveness/readiness/status HTTP endpoints, the machine-readable self-description document, and a skill that audits agent/config files.

## Features

| Feature | Source | Type |
|---|---|---|
| [Health and Status Endpoints](../catalog/health.md) | `features/health.feature` | runtime endpoints |
| [Self-Description Endpoint](../catalog/selfdescription.md) | `features/selfdescription.feature` | identity document |
| [OpenCode Status Report Skill](../catalog/opencode-status-report.md) | `features/opencode-status-report.feature` | agent skill |

## Cross-cutting notes

- The self-description document is generated from the same config that drives the health server; capability flags must stay in sync with config enables.
- `/status` is the aggregation point: it merges local targets, federation report intake, and component health into a sorted snapshot with summary counts.
- These docs link to [Health and Status Endpoints](../catalog/health.md) as the canonical endpoint contract.