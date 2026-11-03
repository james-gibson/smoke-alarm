---
title: Stdio MCP Probe
description: Probe that launches npx/uvx/binaries as child processes and validates the strict MCP handshake over stdin/stdout.
tags:
  - smoke-alarm
  - feature
  - transport
  - stdio
  - mcp
source: smoke-alarm/features/stdio-mcp.feature
subsystem: transport
gherkin_tags:
  - "@stdio-mcp"
  - "@core"
scenario_count: 9
---
> **Source:** `features/stdio-mcp.feature` · **Subsystem:** [Transport & Protocol](../subsystems/transport.md) · **Index:** [Feature catalog](../index.md)

The stdio probe launches a configured command (`npx`, `uvx`, or a binary) as a child process and speaks MCP over its stdin/stdout. It is validated with the **strict** handshake profile (`initialize`, `tools/list`, `resources/list`) so locally-spawned servers are monitored without a persistent HTTP port. The probe always tears the child process down afterward.

## Actors & surfaces

| Actor | Interaction |
|---|---|
| Operator | Enables a stdio target in config |
| Child process | Spawned command that answers the MCP handshake |
| Config reference | `configs/samples/stdio-mcp-strict.yaml` |

## Scenario map

**Process lifecycle** — the probe spawns the configured command, sends an MCP `initialize` over stdin, receives a valid `initialize` response over stdout, kills the child after the check (no orphans), and classifies the target `HEALTHY` after a successful strict handshake.

**Strict handshake** — if any declared method (`initialize`, `tools/list`, `resources/list`) is unanswered, the target is `DEGRADED` with "missing required method".

**Failure classification** — `OUTAGE` with "failed to spawn" when the command is not on PATH; `DEGRADED` when the process exits during handshake (codes 1 / 127); `DEGRADED` with "timeout" when the process does not respond inside the configured timeout.

**Env & cwd passthrough** — configured env vars (e.g. `MCP_LOG_LEVEL=warn`) reach the child, and the configured working directory (e.g. `.` = project root) is applied.

## Cross-references

- [Config Validation](config-validation.md) — schema for stdio targets
- [SSE Transport Probe](sse-transport.md) — the remote-streaming counterpart
- [Safety HURL Scanner](safety-hurl.md) — pre-protocol gate
- [Engine](engine.md) — scheduling/classification