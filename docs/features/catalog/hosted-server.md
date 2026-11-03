---
title: Embedded Hosted MCP/ACP Server
description: Embedded MCP/ACP HTTP+SSE server on localhost:18091 with self-probing hosted targets and HURL-gated strict handshakes.
tags:
  - smoke-alarm
  - feature
  - transport
  - hosted
  - mcp
  - acp
source: smoke-alarm/features/hosted-server.feature
subsystem: transport
gherkin_tags:
  - "@hosted-server"
  - "@core"
scenario_count: 14
---
> **Source:** `features/hosted-server.feature` · **Subsystem:** [Transport & Protocol](../subsystems/transport.md) · **Index:** [Feature catalog](../index.md)

When `hosted.enabled` is true, the process serves its **own embedded MCP and ACP endpoints** (HTTP + SSE) on `localhost:18091` (`/mcp`, `/acp`) so downstream clients can connect without a separate server. The hosted endpoints are themselves monitored as targets (`hosted-mcp-http`, `hosted-acp-http`, `hosted-mcp-sse`, `hosted-acp-sse`), making the process self-probing and useful as a known-good baseline in local/CI runs.

## Actors & surfaces

| Actor | Interaction |
|---|---|
| Downstream MCP/ACP client | Connects to the embedded endpoints |
| Smoke alarm itself | Probes its own hosted targets |
| Config reference | `configs/samples/hosted-mcp-acp.yaml` |

| Endpoint | Transport |
|---|---|
| `http://localhost:18091/mcp` | HTTP JSON-RPC |
| `http://localhost:18091/acp` | HTTP JSON-RPC |
| `.../mcp?transport=sse` | SSE event stream |
| `.../acp?transport=sse` | SSE event stream |

## Scenario map

**Startup** — a listener binds `localhost:18091` and serves `/mcp` and `/acp` when enabled; nothing binds when disabled.

**HTTP transport** — `initialize` over HTTP returns a valid MCP/ACP JSON-RPC response with status 200.

**SSE transport** — `GET ...?transport=sse` returns `Content-Type: text/event-stream` and keeps the connection open.

**Self-probe** — all four hosted targets classify `HEALTHY` when probed against the embedded server.

**HURL preflight** — a `hosted-mcp-health-gate` HURL test runs against `/healthz` before the MCP handshake.

**Strict handshake** — under `strict`, missing methods (`tools/list`, `resources/list`, `session/setup`, `prompt/turn`) degrade the hosted target.

**Health dependency** — `/healthz` returns 200 while running; `/readyz` returns 200 once all enabled targets have completed at least one probe cycle.

## Cross-references

- [Health and Status Endpoints](health.md) — `/healthz`, `/readyz`
- [Self-Description Endpoint](selfdescription.md) — `capabilities.hosted`
- [Dynamic Config Persistence](dynamic-config.md) — artifacts from hosted targets