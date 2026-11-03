---
title: Bubble Tea Dashboard
description: "The foreground Bubble Tea dashboard's Elm Architecture contract: Init/Update/View purity, keyboard navigation, status/details/events/topology panes, SnapshotProvider, and Run."
tags:
  - smoke-alarm
  - feature
  - wargame-onboarding
  - tui
  - ui
  - bubbletea
  - gherkin
source: smoke-alarm/features/tui.feature
subsystem: wargame-onboarding
gherkin_tags:
  - "@tui"
  - "@optional"
scenario_count: 33
---
> **Source:** `features/tui.feature` · **Subsystem:** [Wargame & Onboarding](../subsystems/wargame-onboarding.md) · **Index:** [Feature catalog](../index.md)

`internal/ui/` implements the Bubble Tea dashboard shown in foreground mode. It follows the Elm Architecture strictly: state changes only in `Update()`, `View()` is a pure function of the model, and async work travels as `tea.Cmd`. This file is the agentic dashboard only — the planned stdio-proxy message-flow UI is a separate concern.

## Scenario map

**`Init`** — returns a `tickCmd` and a `loadCmd` without mutating the model.

**`Update` — `tickMsg`** — schedules a `loadCmd` (`SnapshotProvider.Snapshot()`) and the next `tickCmd`; target/event state is unchanged until the `loadMsg` arrives.

**`Update` — `loadMsg`** — replaces the model snapshot and refreshes `last_updated`; a `loadMsg` with zero targets yields an empty status table; a nil snapshot is retained rather than panicking.

**`Update` — keyboard** — `q`/`esc` return `tea.Quit`; `down`/`j` increment and `up`/`k` decrement the selection within bounds (never below 0 or past the last row); `tab` toggles the events pane.

**`Update` — resize** — `tea.WindowSizeMsg` updates width/height, and `View` reflows so no line exceeds the terminal width.

**`View` contract** — identical model state renders identical output; no file reads, network calls, or goroutine launches occur during `View`.

**`View` — panes** — the status table renders one row per target with id, state, severity, failure count, and latency, and the selected row is visually distinct; the details pane renders the selected target's endpoint, protocol, transport, and last-checked time (empty with no selection); the events pane lists timestamped alerts when visible, is absent when hidden, and truncates to `Options.MaxEvents`; the topology pane aggregates protocol/transport counts (e.g. `MCP: 2`, `ACP: 1`); the demo state machine renders only when `Options.Demo` is true.

**`SnapshotProvider`** — the dashboard accepts any provider; the model matches a mock provider's snapshot after a `loadMsg` cycle.

**`Run`** — blocks until `q`/`esc` and returns without error; returns a non-nil error when no TTY is available.

## Cross-references

- [Ops — Runtime Lifecycle and Update Orchestration](ops.md) — `runForeground` dispatches to `ui.Run`
- [Known-State Regression Detection](known-state.md) — target states surfaced in the status table
- [Alert Routing and Severity Policy](alerts.md) — events rendered in the events pane
- [Probe Engine](engine.md) — supplies snapshots via `SnapshotProvider`
- [Wargame & Onboarding subsystem](../subsystems/wargame-onboarding.md)
