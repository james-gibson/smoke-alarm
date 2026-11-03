---
title: Sample Config Catalog
description: Index of the 33 scenario YAML configs under smoke-alarm/configs/samples, grouped by theme with links to per-theme documentation.
tags:
  - smoke-alarm
  - configs
  - samples
  - index
  - catalog
---
# Sample Config Catalog

`configs/samples/` holds **33 ready-to-run scenario configs** for [ocd-smoke-alarm](../../README.md) — the minimal-footprint Go monitor for MCP/ACP environments. Each YAML is a self-contained profile you can open in an editor and feed straight to the CLI for a smoke test.

> **Source of truth:** the YAML files themselves. This catalog and its per-theme docs are derived; when a sample changes, re-sync its write-up. Some samples are generated, not hand-written — see [Discovery & generated configs](../../docs/samples/discovery.md).

Every sample shares the same top-level shape: `version: "1"` followed by `service`, `health`, `runtime`, `discovery`, `alerts`, `auth`, `targets`, `known_state`, `meta_config`, `hosted`, and `federation`. The samples differ in **which blocks they enable and how their targets are wired**. Validation rules live in [Config Validation](../../docs/features/catalog/config-validation.md).

## Themes

| Theme | Samples | What it covers |
|---|---:|---|
| [Transport & protocol](../../docs/samples/transports.md) | 6 | stdio probes, SSE remotes, embedded hosting, npx/uvx packages |
| [Federation & topology](../../docs/samples/federation.md) | 5 | mesh, chain, star-hub, dependency graph, worker pool |
| [Warroom & incidents](../../docs/samples/warroom.md) | 6 | A2A handoff, secure context, escalation, observers, DR drill |
| [Auth, secrets & compliance](../../docs/samples/auth-security.md) | 5 | OAuth mock, tenant isolation, secret rotation, audit |
| [Deployment & operations](../../docs/samples/deployment-ops.md) | 4 | canary, HA failover, cost control, Tuner integration |
| [Discovery & generated configs](../../docs/samples/discovery.md) | 2 | llms.txt auto-discovery and a generated mcp-add artifact |
| [Dev & QA harness](../../docs/samples/dev-qa.md) | 5 | local observer fleet and a QA scratch config |

## Sample index

### Transport & protocol — [details](../../docs/samples/transports.md)

| Config | Scenario |
|---|---|
| `stdio-mcp-strict.yaml` | npx filesystem server over a strict stdio MCP handshake |
| `npm-mcp-targets.yaml` | npx MCP package matrix: filesystem, memory, github |
| `npm-sse.yaml` | npx packages incl. GitHub and Brave Search with env-token auth |
| `uvx-sse.yaml` | uvx package matrix: fetch, git, sqlite, time |
| `sse-remote-mixed.yaml` | remote SSE and strict HTTPS MCP/ACP targets (disabled by default) |
| `hosted-mcp-acp.yaml` | embedded hosted MCP/ACP HTTP+SSE server, self-monitored |

### Federation & topology — [details](../../docs/samples/federation.md)

| Config | Scenario |
|---|---|
| `federation-mesh.yaml` | 3-instance self-electing mesh (no external targets) |
| `federation-chain.yaml` | linear frontend → router → backend → persistence chain |
| `federation-star-hub.yaml` | hub coordinator fanning out to specialist spokes |
| `federation-dependency.yaml` | non-linear microservice dependency graph |
| `federation-pool.yaml` | identical worker pools with quorum semantics |

### Warroom & incidents — [details](../../docs/samples/warroom.md)

| Config | Scenario |
|---|---|
| `a2a-handoff.yaml` | agent-to-agent async handoff (planner/executor/reporter/monitor) |
| `secure-context-handoff.yaml` | scoped, encrypted context injection with TTL |
| `incident-session-ephemeral.yaml` | temporary incident bridge with per-participant scopes |
| `escalation-chain.yaml` | L1→L4 on-call escalation matrix plus pager |
| `observer-mode.yaml` | read-only observers watching an incident |
| `disaster-recovery-drill.yaml` | chaos/DR exercise with fast timeouts |

### Auth, secrets & compliance — [details](../../docs/samples/auth-security.md)

| Config | Scenario |
|---|---|
| `oauth-mock-allow.yaml` | OAuth mock redirect in `allow` mode |
| `oauth-mock-fail.yaml` | OAuth mock redirect in `fail` mode |
| `compliance-audit.yaml` | audit logging, redaction, capability enforcement |
| `multitenancy-isolation.yaml` | tenant A/B/C isolation with distinct scopes |
| `secrets-rotation.yaml` | short-lived OAuth tokens and rotating API keys |

### Deployment & operations — [details](../../docs/samples/deployment-ops.md)

| Config | Scenario |
|---|---|
| `deployment-canary.yaml` | blue/green canary rollout with version gates |
| `deployment-ha.yaml` | two primaries plus shared store and load balancer |
| `cost-control.yaml` | per-request cost awareness and rate-limit handling |
| `tuner-integration.yaml` | Tuner mDNS advertisement, audience, caller hook |

### Discovery & generated configs — [details](../../docs/samples/discovery.md)

| Config | Scenario |
|---|---|
| `llmstxt-auto-discovery.yaml` | authoritative llms.txt discovery with auto-registration |
| `mcp-add-meta-20260408-045454.yaml` | generated `mcp-add` meta-config fragment |

### Dev & QA harness — [details](../../docs/samples/dev-qa.md)

| Config | Scenario |
|---|---|
| `dev/config.yaml` | observer "fire-marshal" hub |
| `dev/1.yaml`, `dev/2.yaml`, `dev/3.yaml` | observer fleet members |
| `qa/config.yaml` | QA scratch config |

## How to run

Run from the repository root:

```sh
# Validate every sample
for file in configs/samples/*.yaml; do
  go run ./cmd/ocd-smoke-alarm validate --config="$file"
done

# One-shot check of a single profile
go run ./cmd/ocd-smoke-alarm check --config=configs/samples/stdio-mcp-strict.yaml

# Foreground serve (embedded hosted endpoints)
go run ./cmd/ocd-smoke-alarm serve --config=configs/samples/hosted-mcp-acp.yaml --mode=foreground

# Discovery + dynamic-config artifacts
go run ./cmd/ocd-smoke-alarm discover --config=configs/samples/llmstxt-auto-discovery.yaml --json=false
```

## Cross-references

- [Config Validation](../../docs/features/catalog/config-validation.md) — how `validate`/`check` treat these files
- [Feature Knowledge Base](../../docs/features/index.md) — the subsystem docs each sample exercises
- [ocd-smoke-alarm README](../../README.md) — build, CLI, and architecture
