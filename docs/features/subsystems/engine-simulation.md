---
title: Engine & Simulation
description: The probe engine, known-state baseline, simulation authoring/validation skills, and optional Tuner integration.
tags:
  - smoke-alarm
  - subsystem
  - engine-simulation
feature_count: 5
---
> Part of the [Feature Knowledge Base](../index.md). See the [consolidated catalog](../consolidated.md) for a flat view.

# Engine & Simulation

Features covering the scheduling and classification core of ocd-smoke-alarm and the simulation tooling built on top of its configs: the probe engine, the known-state regression baseline, the config-to-simulation and simulation-validator skills, and optional Tuner channel integration.

## Features

| Feature | Source | Type |
|---|---|---|
| [Probe Engine](../catalog/engine.md) | `features/engine.feature` | runtime core |
| [Known-State Regression Detection](../catalog/known-state.md) | `features/known-state.feature` | baseline store |
| [Config to Simulation Skill](../catalog/config-to-simulation.md) | `features/config-to-simulation.feature` | agent skill |
| [Simulation Validator](../catalog/simulation-validator.md) | `features/simulation-validator.feature` | agent skill |
| [Tuner Integration](../catalog/tuner-integration.md) | `features/tuner-integration.feature` | optional integration |

## Cross-cutting notes

- The engine is the only writer of probe results; everything downstream (known-state, alerts, snapshots, health endpoints) consumes its output.
- Regression semantics live in the known-state store (`is_regression`) but are promoted to a runtime state by the engine — keep the two in sync.
- simulation-validator is the consistency check across the simulation stack: it compares simulations against the Gherkin features that this knowledge base catalogues.