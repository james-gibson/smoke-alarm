---
title: Auth, Secrets & Compliance Samples
description: Sample configs covering OAuth mock allow/fail paths, multi-tenant isolation, secrets rotation, and compliance audit.
tags:
  - smoke-alarm
  - configs
  - samples
  - auth
  - oauth
  - secrets
  - compliance
source:
  - smoke-alarm/configs/samples/oauth-mock-allow.yaml
  - smoke-alarm/configs/samples/oauth-mock-fail.yaml
  - smoke-alarm/configs/samples/compliance-audit.yaml
  - smoke-alarm/configs/samples/multitenancy-isolation.yaml
  - smoke-alarm/configs/samples/secrets-rotation.yaml
sample_count: 5
---
# Auth, Secrets & Compliance Samples

Five configs about **behaving correctly** rather than merely being up: OAuth callback handling, tenant boundaries, secret rotation, and audit. They exercise [OAuth Mock & Callback](../features/catalog/oauth-mock.md), [HURL Safety Checks](../features/catalog/safety-hurl.md), and the target auth contract in [Target Registry & Contract](../features/catalog/targets.md).

| Config | Focus | Key mechanism |
|---|---|---|
| `oauth-mock-allow.yaml` | OAuth success path | mock redirect in `allow` mode |
| `oauth-mock-fail.yaml` | OAuth failure path | mock redirect in `fail` mode |
| `compliance-audit.yaml` | audit & redaction | OAuth scopes, telemetry, debug logging |
| `multitenancy-isolation.yaml` | tenant boundaries | distinct client IDs and scopes per tenant |
| `secrets-rotation.yaml` | credential rotation | short-lived tokens, rotating API keys |

## oauth-mock-allow.yaml

Runs the OAuth mock redirect in `allow` mode on `localhost:28877` and validates the success path against a hosted ACP target on `:28191`. Useful targets are a mock-status endpoint and `hosted-acp-oauth-validated`; a HURL test (`oauth-mock-ready`) gates the probe. `known_state` is disabled so each run is independent, and the client secret is read from `env://OAUTH_CLIENT_SECRET`.

## oauth-mock-fail.yaml

The negative twin: the mock redirect on `localhost:28877` is set to `fail`, and the single ACP target (`acp-oauth-fail-sample`, 28191) intentionally routes through a failing callback. It exists to verify the smoke alarm classifies and surfaces OAuth failures correctly (HURL test `oauth-mock-redirect-callback-fail`).

## compliance-audit.yaml

An audit-focused profile at `log_level: debug` with slow polling (`30s`) and a 1000-entry event history. Targets are a strict production MCP (19301) and an ACP session target (19302), both OAuth with audit scopes such as `audit.log`. `telemetry` exports OTEL to 19401, and `redaction` plus a keystore are enabled.

## multitenancy-isolation.yaml

Three tenants — corporate, startup, and enterprise (19601–19603) — each with a distinct OAuth `client_id` and scope set (`mcp.full`, `mcp.limited`, `mcp.readonly`), plus a shared logging target (19610). The goal is to confirm cross-tenant data and tool access do not leak.

## secrets-rotation.yaml

Verifies credentials **refresh**, not just exist. Targets cover a short-lived OAuth token (19201, `keychain://` secret ref), a rotating API key sent as the `X-API-Key` bearer header (19202), and a disabled mTLS placeholder (19203). A longer `token_grace_period` (60s), `retries: 2`, and non-aggressive alerting accommodate the transient failures expected during rotation.

## Cross-references

- [OAuth Mock & Callback](../features/catalog/oauth-mock.md), [HURL Safety Checks](../features/catalog/safety-hurl.md), [Target Registry & Contract](../features/catalog/targets.md)
- [Sample Config Catalog](../../configs/samples/README.md)
