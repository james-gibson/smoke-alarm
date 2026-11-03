---
title: Config Validation
description: validate/check subcommands, schema-version and required-field checks, and target transport/protocol vocabulary enforcement.
tags:
  - smoke-alarm
  - feature
  - config-discovery
  - validation
source: smoke-alarm/features/config-validation.feature
subsystem: config-discovery
gherkin_tags:
  - "@config-validation"
  - "@core"
scenario_count: 9
---
> **Source:** `features/config-validation.feature` · **Subsystem:** [Config & Discovery](../subsystems/config-discovery.md) · **Index:** [Feature catalog](../index.md)

The config schema is validated **before any probe runs**, so misconfiguration is caught at startup rather than at runtime. This feature covers the `validate` and `check` subcommands, schema-version checks, required fields, and target transport/protocol vocabulary enforcement.

## Subcommands

| Command | Purpose | Success | Failure |
|---|---|---|---|
| `validate` | Schema + field validation, no runtime side effects | exit 0, no error markers | non-zero, stderr explains |
| `check` | One-shot probe of enabled targets | exit 0 when all reachable | non-zero when a target is unreachable |

## Scenario map

**validate accepts** — the canonical `configs/sample.yaml` and every sample under `configs/samples/` (stdio-mcp-strict, llmstxt-auto-discovery, sse-remote-mixed, oauth-mock-fail/allow, hosted-mcp-acp) validate with exit 0.

**validate rejects** — a missing required field (`version` / `service` / `targets`) → "required"; a nonexistent config → "no such file"; an unknown schema version (99) → "unsupported version".

**check** — exits 0 when all enabled targets are reachable; exits non-zero when a target (e.g. `mcp-local-proxy`) is unreachable.

**target field vocabulary** — invalid transports (`ftp`, `grpc`, empty) → "invalid transport"; invalid protocols (`graphql`, `unknown`) → "invalid protocol".

## Cross-references

- [Target Definition and Validation](targets.md) — the per-target field rules
- [Dynamic Config Persistence](dynamic-config.md) — config output
- [Meta-Config Template Generation](meta-config.md) — schema fields
- [Stdio MCP Probe](stdio-mcp.md) — target field usage