---
title: Self-Description Endpoint
description: Machine-readable /.well-known/smoke-alarm.json identity document generated from the running config and validated against JSON schema.
tags:
  - smoke-alarm
  - feature
  - health-status
  - self-description
  - schema
source: smoke-alarm/features/selfdescription.feature
subsystem: health-status
gherkin_tags:
  - "@selfdescription"
  - "@core"
scenario_count: 15
---
> **Source:** `features/selfdescription.feature` · **Subsystem:** [Health & Status](../subsystems/health-status.md) · **Index:** [Feature catalog](../index.md)

`SelfDescription` is the machine-readable service identity document served at `/.well-known/smoke-alarm.json`. It lets a downstream agent or orchestrator discover the instance's protocols, transports, capabilities, permissions, endpoints, and enabled targets, and it is validated against `configs/schema/self-description.schema.json`. The document is generated from the running config, so its capability flags mirror config values.

## Actors & surfaces

| Actor | Interaction |
|---|---|
| Downstream agent / orchestrator | GETs the document to discover capabilities |
| Schema validator | Validates the document against the JSON schema |

## Document structure

Required top-level fields: `version`, `service`, `health`, `capabilities`, `permissions`, `mcp`, `targets`.

- `service.name` = `ocd-smoke-alarm`, `service.mode`, `service.environment`, `service.started_at` (RFC3339).
- `health.endpoints` = `healthz`/`readyz`/`status`/`self_description`; `health.listen_addr` = configured address.
- `capabilities.monitoring.protocols` lists `mcp`/`acp` per active targets.
- `capabilities.discovery.llms_txt`, `capabilities.hosted`, `capabilities.dynamic_config`, `capabilities.meta_config` mirror their config enables.

## Scenario map

- Served with `200` and `Content-Type: application/json` when `health.enabled` is true; endpoint path is listed in the health endpoints block.
- Document contains all required top-level fields; `service` block reflects running config values with a valid RFC3339 `started_at`.
- Capability flags reflect config: protocols list, `llms_txt` true/false per config, `hosted`, `dynamic_config`, `meta_config`.
- `targets` list includes enabled targets (e.g. `hosted-mcp-http`, `hosted-acp-http`) and excludes disabled ones (e.g. `mcp-cloud-primary`).
- Document conforms to `configs/schema/self-description.schema.json` with no validation errors.

## Cross-references

- [Health and Status Endpoints](health.md) — the endpoint is wired into `/healthz`/`/readyz`/`/status`
- [Config Validation](config-validation.md) — `health.enabled` config block
- [Hosted Server](hosted-server.md) — hosted capabilities reflected here
- [LLMs.txt Auto-Discovery](discovery-llmstxt.md) — `discovery.llms_txt` capability flag