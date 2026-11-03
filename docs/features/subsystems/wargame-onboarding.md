---
title: Wargame & Onboarding
description: Incident-simulation authoring and play-through skills, the first-run welcome check, and the Bubble Tea operator dashboard.
tags:
  - smoke-alarm
  - subsystem
  - wargame-onboarding
feature_count: 5
---
> Part of the [Feature Knowledge Base](../index.md). See the [consolidated catalog](../consolidated.md) for a flat view.

# Wargame & Onboarding

Features that train humans and orient agents: the incident-simulation authoring and play-through skills, the first-run welcome check, and the Bubble Tea operator dashboard.

## Features

| Feature | Source | Type |
|---|---|---|
| [Warroom Builder Skill](../catalog/warroom-builder.md) | `features/warroom-builder.feature` | skill (authoring) |
| [Warroom Guide Skill](../catalog/warroom-guide.md) | `features/warroom-guide.feature` | skill (tutorial) |
| [Warroom Simulator Skill](../catalog/warroom-simulator.md) | `features/warroom-simulator.feature` | skill (live simulation) |
| [Start-Here Welcome Skill](../catalog/start-here.md) | `features/start-here.feature` | skill (onboarding) |
| [Bubble Tea Dashboard](../catalog/tui.md) | `features/tui.feature` | UI (foreground) |

## Cross-cutting notes

- The three warroom skills form a pipeline: **builder** authors the config and script, **guide** narrates a tutorial run, **simulator** runs the live trainee-driven variant with `/join` and `/resolve`.
- Onboarding is split by audience: `start-here` confirms an agent's connection and project layout, while the TUI is the human operator's live view of engine state.
- The TUI is contract-heavy by design — Elm Architecture boundaries (mutations only in `Update`, pure `View`, async via `tea.Cmd`) are themselves asserted as scenarios.

## Cross-references

- [Transport & Protocol](transport.md) — the stdio/SSE surfaces the dashboard probes
- [Engine & Simulation](engine-simulation.md) — config-to-simulation and the simulation validator
- [Feature Knowledge Base](../index.md)
