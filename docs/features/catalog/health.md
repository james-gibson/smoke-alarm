---
title: Health and Status Endpoints
description: Deterministic /healthz, /readyz, /status endpoints plus federation report intake, BindWithRetry, and shutdown semantics.
tags:
  - smoke-alarm
  - feature
  - health-status
  - endpoints
  - gherkin
source: smoke-alarm/features/health.feature
subsystem: health-status
gherkin_tags:
  - "@health"
  - "@core"
scenario_count: 32
---
> **Source:** `features/health.feature` · **Subsystem:** [Health & Status](../subsystems/health-status.md) · **Index:** [Feature catalog](../index.md)

The health subsystem exposes deterministic HTTP surfaces so an operator or orchestrator can observe and gate on service state without parsing logs. `/healthz` reports liveness, `/readyz` reports readiness as a composite of registered components, and `/status` returns the full snapshot (service identity, uptime, components, targets, summary counts). The same server also exposes the self-description document (`/.well-known/smoke-alarm.json`) and accepts peer status via `POST /federation/report`. Port binding uses `BindWithRetry` so a busy port yields the next free one, and the mDNS advertiser registers on the actually-bound port.

## Actors & surfaces

| Actor | Interaction |
|---|---|
| Operator / orchestrator | Polls `/healthz`, `/readyz`, `/status` for gating |
| Federation peer | `POST /federation/report` injects remote target states |
| mDNS advertiser | Registers the service on the actually-bound port |

| Surface | Purpose |
|---|---|
| `GET /healthz` | Liveness (`200 ok` / `503 unhealthy`) |
| `GET /readyz` | Readiness (`200 ready` / `503 not_ready`) |
| `GET /status` | Full snapshot, always `200` |
| `GET /.well-known/smoke-alarm.json` | Self-description (delegated) |
| `POST /federation/report` | Peer target-status intake (`400` malformed, `405` on GET) |

## Scenario map

**Liveness (`/healthz`)** — 200 when live; 503 when liveness false; body contains the configured service name.

**Readiness (`/readyz`)** — 503 when the ready flag is false; 200 when ready and all components healthy; 503 when any component is unhealthy even with ready true; body includes a target summary (counts); `ready_error` (e.g. "waiting for initial probe") is surfaced when set.

**Status snapshot (`/status`)** — always 200; includes service name and non-negative uptime; reports live and ready flags; components and targets are sorted alphabetically; summary counts reflect target states (healthy / unhealthy / regression), treat `failed` as unhealthy, and count unrecognized states as `unknown`.

**Target & component management** — `UpsertTargetStatus` lowercases state, defaults empty state to `unknown`, and ignores empty IDs; `RemoveTarget` drops a target; `SetComponent` with an empty name is a no-op; `RemoveComponent` drops a component; a component recovering to healthy restores composite readiness.

**Self-description + federation report** — 404 "self-description not configured" with no factory registered, 200 with a factory; `POST /federation/report` accepts target status and updates the snapshot, GET returns 405, malformed JSON returns 400.

**Binding & shutdown** — `BindWithRetry` binds the configured address on a free port, yields a higher port when occupied, and the mDNS advertiser registers on the actual bound port (not the configured one); shutdown clears liveness.

## Cross-references

- [Self-Description Endpoint](selfdescription.md) — the `/.well-known/smoke-alarm.json` document in depth
- [Federation Mesh](federation.md) — fan-out and report intake
- [mDNS Service Advertisement](mdns.md) — advertiser behaviour
- [Hosted Server](hosted-server.md) — embedded endpoints that reuse these health surfaces