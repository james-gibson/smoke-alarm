---
title: Transport & Protocol Samples
description: Sample configs exercising stdio, SSE, and embedded hosted MCP/ACP transports, including npx and uvx package probes.
tags:
  - smoke-alarm
  - configs
  - samples
  - transport
  - stdio
  - sse
  - hosted
source:
  - smoke-alarm/configs/samples/stdio-mcp-strict.yaml
  - smoke-alarm/configs/samples/npm-mcp-targets.yaml
  - smoke-alarm/configs/samples/npm-sse.yaml
  - smoke-alarm/configs/samples/uvx-sse.yaml
  - smoke-alarm/configs/samples/sse-remote-mixed.yaml
  - smoke-alarm/configs/samples/hosted-mcp-acp.yaml
sample_count: 6
---
# Transport & Protocol Samples

These six configs exercise the ways [ocd-smoke-alarm](../../README.md) reaches MCP/ACP services: **stdio** child processes, **SSE** streams, and its own **embedded hosted** server. They back the transport catalog docs [Stdio MCP Probe](../features/catalog/stdio-mcp.md), [SSE Transport Probe](../features/catalog/sse-transport.md), and [Embedded Hosted MCP/ACP Server](../features/catalog/hosted-server.md).

| Config | Transport | Protocol | Targets |
|---|---|---|---|
| `stdio-mcp-strict.yaml` | stdio | MCP | 1 npx filesystem server |
| `npm-mcp-targets.yaml` | stdio | MCP | 3 npx packages |
| `npm-sse.yaml` | stdio | MCP | 4 npx packages, env-token auth |
| `uvx-sse.yaml` | stdio | MCP | 4 uvx packages |
| `sse-remote-mixed.yaml` | SSE + HTTP | MCP, ACP | 4 remote targets, disabled by default |
| `hosted-mcp-acp.yaml` | HTTP + SSE | MCP, ACP, A2A | 4 self-monitored endpoints |

## stdio-mcp-strict.yaml

One `npx -y @modelcontextprotocol/server-filesystem .` target (`mcp-stdio-filesystem-strict`) under a **strict** handshake: `initialize`, `tools/list`, and `resources/list` are all required. Health listens on `localhost:18188`; `hosted` is disabled, and `dynamic_config` writes JSON/Markdown artifacts with `allow_overwrite: false`.

```sh
go run ./cmd/ocd-smoke-alarm check --config=configs/samples/stdio-mcp-strict.yaml
```

## npm-mcp-targets.yaml

A minimal npx matrix: three stdio targets (filesystem, memory, github) run as `npx -y <package>` with no extra args, all requiring `initialize` + `tools/list`. Health on `localhost:18189`; `meta_config` emits YAML and JSON.

## npm-sse.yaml

Despite the filename this profile is **stdio, not SSE**: four npx package targets — filesystem (with a `.` path arg and `resources/list`), GitHub (`GITHUB_PERSONAL_ACCESS_TOKEN`), memory, and Brave Search (`BRAVE_API_KEY`). It is the reference for injecting per-package environment variables into stdio probes.

## uvx-sse.yaml

The Python-side mirror of the npx matrix: four `uvx` stdio targets — `mcp-server-fetch`, `mcp-server-git --repository .`, `mcp-server-sqlite --db-path ./state/uvx-mcp/test.db`, and `mcp-server-time`. Here too the name says SSE but the transport is stdio. Discovery is enabled.

## sse-remote-mixed.yaml

A remote-first profile. All four targets are **disabled by default** and show the two remote transport styles: SSE (`mcp-remote-sse-primary`, `acp-remote-sse-primary`) and strict HTTPS JSON-RPC (`mcp-remote-https-strict`, `acp-remote-https-strict-oauth`). Targets carry `bearer`/`oauth` auth backed by `keychain://` secret references with HURL preflights. `remote_agent.managed_updates` is on with concrete `ops stop` / `serve` / `check` commands, and the process also hosts MCP/ACP/A2A on `localhost:18091`.

## hosted-mcp-acp.yaml

The self-contained hosting demo: the process embeds an MCP/ACP HTTP+SSE server (`hosted.listen_addr: ":0"`, ephemeral port) and then **records four targets against its own hosted endpoints** on `:18091`. Discovery is fully on (local paths, env vars, a cloud catalog, and llms.txt auto-registration), the OAuth mock redirect runs in `allow` mode on `0.0.0.0:8877`, and `dynamic_config` persists JSON/Markdown. Run it with `serve --mode=foreground` or the `demo` subcommand.

```sh
go run ./cmd/ocd-smoke-alarm serve --config=configs/samples/hosted-mcp-acp.yaml --mode=foreground
```

## Cross-references

- [Stdio MCP Probe](../features/catalog/stdio-mcp.md), [SSE Transport Probe](../features/catalog/sse-transport.md), [Embedded Hosted MCP/ACP Server](../features/catalog/hosted-server.md)
- [Target Registry & Contract](../features/catalog/targets.md) — the `targets[]` fields every sample uses
- [Sample Config Catalog](../../configs/samples/README.md)
