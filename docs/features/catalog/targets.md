---
title: Target Definition and Validation
description: "Full target field contract: required fields, endpoint rules, check policy, handshake, HURL, auth rules, and CheckResult semantics."
tags:
  - smoke-alarm
  - feature
  - config-discovery
  - targets
  - validation
source: smoke-alarm/features/targets.feature
subsystem: config-discovery
gherkin_tags:
  - "@targets"
  - "@core"
scenario_count: 30
---
> **Source:** `features/targets.feature` · **Subsystem:** [Config & Discovery](../subsystems/config-discovery.md) · **Index:** [Feature catalog](../index.md)

Target definitions are validated before any probe is scheduled. This feature enumerates the required-field, endpoint, check-policy, handshake, HURL, and auth rules, plus the `CheckResult` classification helpers and the canonical type vocabulary.

## Scenario map

**Required fields** — `id`, `protocol`, and `transport` are required (`target id is required`, `protocol is required`, `transport is required`).

**Endpoint rules by transport** — HTTP: empty (`endpoint is required`) or malformed (`endpoint is invalid`) fails; SSE: must be http/https (`sse endpoint must use http/https`); stdio: a command is required (`stdio command is required`).

**Check policy** — `timeout must be > 0`, `interval must be > 0`, `retries must be >= 0`.

**Handshake** — `handshake_profile` must be one of `none`/`base`/`strict`; `required_methods` must not contain empty entries.

**HURL tests** — each test requires exactly one of `file` or `endpoint` (mutually exclusive); the HTTP method must be supported.

**Auth** — `bearer` requires `secret_ref`; `apikey` requires `key_name` and `secret_ref`; `oauth` requires at least `client_id` and `token_url`; unknown auth types fail.

**CheckResult semantics** — `IsFailure` is true for `unhealthy`/`outage`/`regression`, false for `healthy`/`degraded`/`unknown`; `IsEscalated` is true for critical severity, regression, or an alert-worthy combination.

**Type vocabulary** — protocols `mcp`/`acp`/`http`/`tcp`; transports `http`/`websocket`/`sse`/`stdio`/`grpc`/`tcp`; failure classes `none`/`network`/`timeout`/`auth`/`protocol`/`config`/`tls`/`rate_limited`/`unknown`.

## Cross-references

- [Config Validation](config-validation.md) — the `validate`/`check` subcommands
- [Engine](engine.md) — scheduling and classification
- [Routing](routing.md) — priority-based selection