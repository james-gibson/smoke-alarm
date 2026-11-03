---
title: Dev & QA Harness
description: The local observer fleet under configs/samples/dev and the QA scratch config.
tags:
  - smoke-alarm
  - configs
  - samples
  - dev
  - qa
  - harness
source:
  - smoke-alarm/configs/samples/dev/config.yaml
  - smoke-alarm/configs/samples/dev/1.yaml
  - smoke-alarm/configs/samples/dev/2.yaml
  - smoke-alarm/configs/samples/dev/3.yaml
  - smoke-alarm/configs/samples/qa/config.yaml
sample_count: 5
---
# Dev & QA Harness

Scratch configs used to exercise the smoke alarm itself locally rather than to model a customer scenario. The `dev/` fleet sets up a small **observer topology** on loopback; `qa/` is a general scratch profile.

| Config | Service name | Health | Mode |
|---|---|---|---|
| `dev/config.yaml` | `fire-marshal` | `localhost:18088` | foreground |
| `dev/1.yaml` | `observer-1` | `localhost:28088` | background |
| `dev/2.yaml` | `observer-2` | `localhost:38088` | foreground |
| `dev/3.yaml` | `observer-3` | `localhost:48088` | foreground |
| `qa/config.yaml` | `qa` | `localhost:18088` | foreground |

## dev/config.yaml

The hub: service `fire-marshal` runs the `observer-mode` environment with discovery on, hosting enabled, and federation enabled. Its targets health-check itself (`:18088`), an audit trail, and `observers-observer.1` (`:28088`). All state lands under `configs/samples/dev/dist/state/observer-mode`.

## dev/1.yaml, dev/2.yaml, dev/3.yaml

Three fleet members that watch the hub and each other. `dev/1.yaml` also carries an npx memory target and a **disabled** known-failure GitHub target (documented inline as a package that intentionally does not exist). `dev/2.yaml` and `dev/3.yaml` include a deliberately flaky health target to exercise regression classification. `dev/3.yaml` adds an `upstream-stdio-mcp` target; its `dev/3/Dockerfile` is currently an empty placeholder.

## Generated run state

`dev/dist/` holds per-instance runtime state plus empty JSONL traffic logs (`adhd.traffic.jsonl`, `adhd.1.jsonl`); these are generated artifacts, not configs, and are safe to ignore when reading the profiles.

## qa/config.yaml

A scratch QA profile using the `serviceConfig`-wrapped layout (`service.name: qa`) with `dynamic_config` emitting Markdown, hosted HTTP+SSE for MCP/ACP/A2A, and federation disabled (upstream/downstream values are left commented out as examples).

## Cross-references

- [Known-Good Baseline](../features/catalog/known-state.md), [Alert Routing](../features/catalog/alerts.md), [Health & Status Endpoints](../features/catalog/health.md)
- [Sample Config Catalog](../../configs/samples/README.md)
