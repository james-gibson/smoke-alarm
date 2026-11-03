---
title: Config to Simulation Skill
description: "The config-to-simulation skill: parses a YAML config into participants, derives escalation and topology, and renders warroom/mentor voices."
tags:
  - smoke-alarm
  - feature
  - engine-simulation
  - skill
  - simulation
  - gherkin
source: smoke-alarm/features/config-to-simulation.feature
subsystem: engine-simulation
gherkin_tags:
  - "@config-to-simulation"
  - "@skill"
scenario_count: 15
---
> **Source:** `features/config-to-simulation.feature` · **Subsystem:** [Engine & Simulation](../subsystems/engine-simulation.md) · **Index:** [Feature catalog](../index.md)

The `config-to-simulation` skill (`.opencode/skills/config-to-simulation/SKILL.md`) turns an existing YAML config into an interactive incident simulation, so a trainer can run a session from a config without hand-authoring scenarios. It parses targets into participants with join codes, derives escalation speed from `poll_interval`, detects federation topology, and renders output in one of two voices (`warroom` or `mentor`), or `both`. It pairs with `warroom-builder` (which builds the config) and `warroom-simulator` (which runs the output).

## Actors & surfaces

| Actor | Interaction |
|---|---|
| Agent | Invokes the skill by name; reads and executes `SKILL.md` |
| Trainer / educator | Chooses voice mode and consumes the simulation |
| Config file | Input: targets, `poll_interval`, `federation`, `alerts` |

## Scenario map

**Invocation** — reads `SKILL.md` and executes documented steps in order; a missing required file is reported with a remediation suggestion.

**Config analysis** — each target becomes a participant with a join code; escalation speed reflects `poll_interval` (e.g. `2s`); `federation.enabled: true` produces a federation topology description.

**Warroom voice** — urgent imperative language, a participant join-code table, and `alerts.aggressive: true` labelled "SEV1-level urgency".

**Mentor voice** — explanatory, question-based language with at least one learning question and at least two named failure scenarios to choose from.

**Voice selection** — prompts when unspecified (offering `warroom` and `mentor`); `both` emits a warroom section and a mentor section.

**Auto-detection** — config pattern infers incident type (e.g. star-hub → federation failure; oauth-mock-fail → auth incident).

**Stability** — repeated invocations with identical inputs produce equivalent output and no duplicate files under `state/`.

## Cross-references

- [Warroom Builder](warroom-builder.md) — produces the config this skill consumes
- [Warroom Simulator](warroom-simulator.md) — runs the generated simulation
- [Simulation Validator](simulation-validator.md) — checks simulation ↔ feature alignment
- [Engine & Simulation subsystem](../subsystems/engine-simulation.md)