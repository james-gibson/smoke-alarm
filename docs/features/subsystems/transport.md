---
title: Transport & Protocol
description: "How the smoke alarm reaches MCP/ACP services: stdio, SSE, embedded hosting, and federation proxying."
tags:
  - smoke-alarm
  - subsystem
  - transport
feature_count: 4
---
> Part of the [Feature Knowledge Base](../index.md). See the [consolidated catalog](../consolidated.md) for a flat view.

# Transport & Protocol

Features covering how `ocd-smoke-alarm` reaches and speaks to MCP/ACP services: local stdio child processes, remote SSE streams, an embedded self-hosted MCP/ACP server, and JSON-RPC proxying across a federation mesh.

## Features

| Feature | Source | Notes |
|---|---|---|
| [Stdio MCP Probe](../catalog/stdio-mcp.md) | `features/stdio-mcp.feature` | local command targets, strict handshake |
| [SSE Transport Probe](../catalog/sse-transport.md) | `features/sse-transport.feature` | remote MCP/ACP event streams |
| [Embedded Hosted MCP/ACP Server](../catalog/hosted-server.md) | `features/hosted-server.feature` | self-hosting + self-probe |
| [MCP JSON-RPC Proxying](../catalog/mcp-proxy.md) | `features/mcp-proxy.feature` | `@optional` next-steps record |

## Cross-cutting notes

- **Handshake profiles** (`none` / `base` / `strict`) and the configured `required_methods` are the common classification contract across stdio, SSE, and hosted targets.
- **HURL preflight** is a shared stage that gates protocol work for all transports.
- **Routing traces** originate here (proxy) and are consumed by the federation/routing features; a trace containing an instance ID twice is a cycle and is rejected.
- SSE is the preferred remote streaming transport; WS/WSS is used only when a provider requires it.