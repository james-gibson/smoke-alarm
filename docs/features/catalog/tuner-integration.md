---
title: Tuner Integration
description: "Optional Tuner passive-observer integration: tuner config, mDNS advertisement, audience/caller endpoints with SSE fan-out, MCP tools, CLI, and hosted event log."
tags:
  - smoke-alarm
  - feature
  - engine-simulation
  - tuner
  - integration
  - gherkin
source: smoke-alarm/features/tuner-integration.feature
subsystem: engine-simulation
gherkin_tags:
  - "@tuner-integration"
  - "@optional"
  - "@config"
  - "@mdns"
  - "@audience"
  - "@caller"
  - "@mcp"
  - "@cli"
  - "@events"
scenario_count: 10
---
> **Source:** `features/tuner-integration.feature` · **Subsystem:** [Engine & Simulation](../subsystems/engine-simulation.md) · **Index:** [Feature catalog](../index.md)

Optional integration with the Tuner passive observer (Television model), so operators can watch a smoke-alarm instance as a channel. When enabled, the instance parses the `tuner` config block, advertises `_smoke-alarm._tcp` over mDNS, accepts audience metrics (`POST /tuner/audience`) and caller messages (`POST /tuner/caller/<channel>`) with SSE fan-out, exposes tuner MCP tools, and adds `tuner status` / `tuner audience` CLI subcommands. Tuner events appear in the hosted event log with protocol `tuner`.

## Actors & surfaces

| Actor | Interaction |
|---|---|
| Tuner | Posts audience data; subscribes/posts caller messages |
| Operator | Uses `tuner status` / `tuner audience` CLI; watches the channel |
| MCP client | Calls `smoke.tuner_list_channels`, `smoke.tuner_audience`, `smoke.tuner_caller_messages` |
| mDNS | Advertises `_smoke-alarm._tcp` with version/protocol TXT records |

## Scenario map

**Config** — `tuner.enabled`, `tuner.advertise`, `tuner.service_type` parse and validate; integration becomes active.

**mDNS** — serve mode advertises the service when `advertise` is true (with version/protocol TXT records); none when false.

**Audience** — `POST /tuner/audience` stores metrics (200); `GET /tuner/audience` returns metrics for every channel.

**Caller** — SSE subscribers receive posted messages including channel/from/timestamp; posting with no subscribers returns 200 with `subscribers: 0`.

**MCP tools** — `tools/list` includes the three tuner tools.

**CLI** — `tuner status` shows integration status and audience/caller hook settings; `tuner audience --channel --count` pushes a metric.

**Events** — audience/caller interactions appear in `GET /hosted/events` with protocol `tuner`.

## Cross-references

- [Hosted Server](hosted-server.md) — hosts the tuner endpoints and event log
- [mDNS Service Advertisement](mdns.md) — advertisement mechanics
- [Telemetry & Metrics](telemetry.md) — observability alongside tuner channels
- [Engine & Simulation subsystem](../subsystems/engine-simulation.md)