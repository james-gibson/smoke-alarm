---
title: SSE Transport Probe
description: Probe for remote MCP/ACP Server-Sent Events streams with bearer auth, handshake profiles, HURL preflight, and remote_agent updates.
tags:
  - smoke-alarm
  - feature
  - transport
  - sse
  - mcp
  - acp
source: smoke-alarm/features/sse-transport.feature
subsystem: transport
gherkin_tags:
  - "@sse-transport"
  - "@core"
scenario_count: 12
---
> **Source:** `features/sse-transport.feature` · **Subsystem:** [Transport & Protocol](../subsystems/transport.md) · **Index:** [Feature catalog](../index.md)

The SSE probe connects to remote MCP and ACP endpoints over Server-Sent Events, receives the event stream, and classifies health. It supports bearer/secret-ref auth, per-target `handshake_profile` (`none` vs `strict`), optional HURL preflight, bounded concurrency, and the `remote_agent` managed-update lifecycle.

## Actors & surfaces

| Actor | Interaction |
|---|---|
| Operator | Monitors remote SSE MCP/ACP services |
| Remote endpoint | Streams MCP/ACP events |
| Secret store | `keychain://...` bearer token refs |
| Config reference | `configs/samples/sse-remote-mixed.yaml` |

## Scenario map

**SSE connection** — a reachable MCP or ACP SSE endpoint yields `HEALTHY`; an unreachable endpoint yields `OUTAGE`; a prematurely-closed stream yields `DEGRADED`.

**Bearer auth** — with `auth type "bearer"`, the connection carries an `Authorization` header and the token value is never logged in plaintext.

**Handshake profile** — `none` skips required-method validation (classification is HTTP-status based); `strict` validates all `required_methods` for a `HEALTHY` result.

**HURL preflight** — a registered HURL test runs *before* the SSE connection opens; a non-200 preflight (e.g. 503) blocks the connection and yields `DEGRADED`.

**Concurrency** — with `max_workers: 8`, all four enabled targets dispatch concurrently and no more than 8 workers run at once.

**remote_agent lifecycle** — a managed update runs `stop_command` → `start_command` → `verify_command` in order; on verify failure with `rollback_on_failure` the previous binary is restored and a `REGRESSION` event is emitted.

## Cross-references

- [mDNS Service Advertisement](mdns.md) / [mDNS Peer Discovery](mdns-discovery.md)
- [OAuth Mock Redirect Endpoint](oauth-mock.md) — auth flows
- [Ops](ops.md) — managed update orchestration