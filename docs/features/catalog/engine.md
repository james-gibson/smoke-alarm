---
title: Probe Engine
description: "The continuous probe scheduler: immediate first check, interval probing, worker bounds, state classification, alert emission, ring buffer, snapshots, and stdio transport probing."
tags:
  - smoke-alarm
  - feature
  - engine-simulation
  - engine
  - gherkin
source: smoke-alarm/features/engine.feature
subsystem: engine-simulation
gherkin_tags:
  - "@engine"
  - "@core"
scenario_count: 27
---
> **Source:** `features/engine.feature` · **Subsystem:** [Engine & Simulation](../subsystems/engine-simulation.md) · **Index:** [Feature catalog](../index.md)

The probe engine (`internal/engine/`) is the always-on scheduler at the centre of ocd-smoke-alarm. It performs an immediate first check per target, then re-probes each target on its configured interval, bounds concurrency with `max_workers`, classifies each result into a health state (`healthy`/`unhealthy`/`regression`/`outage`), consults the known-state baseline and the HURL safety gate, emits alert events through registered notifiers, and exposes snapshots (`SnapshotStatuses`, `SnapshotEvents`) to the TUI and health endpoints. It also owns the stdio transport prober for command targets, falling back to HTTP for non-stdio transports.

## Actors & surfaces

| Actor | Interaction |
|---|---|
| Operator | Starts/stops the engine via CLI; observes snapshots in the TUI |
| Notifier | Receives alert events for regression/outage/critical results |
| Known-state store | Supplies the healthy baseline used for regression classification |
| Safety (HURL) gate | Blocks deeper protocol probing when the pre-check fails |
| TUI / health endpoints | Read `SnapshotStatuses` / `SnapshotEvents` |

## Scenario map

**Lifecycle** — immediate first probe before the first tick; ready once every enabled target has at least one result; clean exit within 2000 ms of context cancel; known-state baseline written on graceful stop.

**Scheduling** — probes fire on the configured interval (at least 3 results in 350 ms at a 100 ms interval); concurrency capped by `max_workers` (10 targets, 3 workers → no more than 3 concurrent).

**State classification** — healthy on success; unhealthy on error; regression on a failure after prior success (with known-state and the regression flag set); outage on consecutive failures past the threshold (message contains "consecutive failures"); aggressive alerts elevate a post-healthy failure to `critical`; a failing HURL safety check blocks the protocol probe; a failing probe retries up to the configured count (attempt index recorded as 3 after 2 retries).

**Alert emission** — regression and outage emit alert events; healthy emits none; any `critical` severity emits an alert with state `outage`.

**Event ring buffer** — alert events are recorded in history; oldest are evicted past the configured size; `SnapshotEvents` returns oldest-first.

**Snapshot API** — `SnapshotStatuses` sorts by target ID ascending and carries latency and status code from the probe result.

**Stdio transport probing** — empty command → `unhealthy` / `config` failure class; `handshake_profile=none` skips the subprocess; `base` MCP exercises `initialize` only; `strict` exercises the full method set per protocol (MCP: initialize / tools-list / resources-list; ACP: initialize / session-setup / prompt-turn); `required_methods` overrides the profile; timeout → `timeout`; JSON-RPC error → `protocol`; non-stdio transports fall back to HTTP; stderr is included in failure messages.

## Cross-references

- [Known-State Regression Detection](known-state.md) — baseline and regression classification
- [Alerts & Notification Routing](alerts.md) — severity policy and dispatch
- [HURL Safety Checks](safety-hurl.md) — pre-protocol safety gate
- [Isotope Transit](isotope-transit.md) — probe-result evidence carrier
- [Engine & Simulation subsystem](../subsystems/engine-simulation.md)