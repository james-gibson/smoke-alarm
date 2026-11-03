---
title: Deployment & Operations Samples
description: Sample configs for canary/blue-green rollouts, HA failover, cost control, and Tuner observer integration.
tags:
  - smoke-alarm
  - configs
  - samples
  - deployment
  - operations
  - tuner
source:
  - smoke-alarm/configs/samples/deployment-canary.yaml
  - smoke-alarm/configs/samples/deployment-ha.yaml
  - smoke-alarm/configs/samples/cost-control.yaml
  - smoke-alarm/configs/samples/tuner-integration.yaml
sample_count: 4
---
# Deployment & Operations Samples

Four configs aimed at running and operating services: blue/green rollouts, HA failover, cost control, and the [Tuner](../features/catalog/tuner-integration.md) observer integration. They lean on [Known-Good Baseline](../features/catalog/known-state.md), [Alert Routing](../features/catalog/alerts.md), [Lifecycle & Ops](../features/catalog/ops.md), and [Telemetry & Metrics](../features/catalog/telemetry.md).

| Config | Scenario | Signal focus |
|---|---|---|
| `deployment-canary.yaml` | blue/green canary | version + capability gates |
| `deployment-ha.yaml` | HA failover | two primaries, shared store, LB |
| `cost-control.yaml` | spend awareness | retry limits, rate-limit handling |
| `tuner-integration.yaml` | Tuner observer | mDNS, audience, caller hook |

## deployment-canary.yaml

Runs in `background` mode and compares blue (19001, version `>=1.0.0 <2.0.0`) against green (19002, version `>=2.0.0` and an extra `prompts/list` capability). A disabled comparison-helper target (19003) sketches response-consistency checks. `dynamic_config` persists rollout artifacts.

## deployment-ha.yaml

Two primaries — A preferred (19101) and B standby (19102) — plus a shared state store (19110) and a load balancer (19120 accepting `200/204`). Fast cadence (`3s`/`2s`) and short outage threshold make failover visible quickly; federation is enabled on 5700–5702.

## cost-control.yaml

Tracks per-request cost by tiering targets: an expensive GPT-4 proxy (19401), a moderate GPT-3.5 proxy (19402), a free local agent (19403), and a rate-limited external service (19404) whose healthy codes explicitly include `429` and which carries a HURL `rate-limit-check`. Retries are pinned to `0` (each retry costs money) and `telemetry` exports to 19450.

## tuner-integration.yaml

Enables the `tuner` block: mDNS advertisement of `_smoke-alarm._tcp`, audience metrics at `/tuner/audience`, and a caller hook with `mcp_response: true`, so [Tuner](../features/catalog/tuner-integration.md) instances can discover and watch this smoke alarm. It hosts MCP on `localhost:18089` and monitors a single local health target.

## Cross-references

- [Known-Good Baseline](../features/catalog/known-state.md), [Alert Routing](../features/catalog/alerts.md), [Lifecycle & Ops](../features/catalog/ops.md), [Telemetry & Metrics](../features/catalog/telemetry.md), [Tuner Integration](../features/catalog/tuner-integration.md)
- [Sample Config Catalog](../../configs/samples/README.md)
