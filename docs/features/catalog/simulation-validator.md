---
title: Simulation Validator
description: "The simulation-validator skill: config extraction, feature mapping, secure-context verification, bidirectional gap analysis, TUI guidance, and reports."
tags:
  - smoke-alarm
  - feature
  - engine-simulation
  - skill
  - simulation
  - validation
  - gherkin
source: smoke-alarm/features/simulation-validator.feature
subsystem: engine-simulation
gherkin_tags:
  - "@simulation-validator"
  - "@skill"
scenario_count: 26
---
> **Source:** `features/simulation-validator.feature` · **Subsystem:** [Engine & Simulation](../subsystems/engine-simulation.md) · **Index:** [Feature catalog](../index.md)

The `simulation-validator` skill (`.opencode/skills/simulation-validator`) checks that a warroom simulation aligns with the Gherkin feature specs — "what we train matches what we spec matches what runs". It reads a config, counts targets, detects federation and alert settings, discovers relevant feature files, maps targets to feature steps, verifies secure-context (OAuth/scope/redaction) configuration, performs gap analysis in both directions, emits TUI integration guidance, and produces a validation report. It supports warroom and mentor voices and integrates with `config-to-simulation` output.

## Actors & surfaces

| Actor | Interaction |
|---|---|
| Agent | Invokes the skill; reads config and feature files |
| Trainer | Consumes the validation report and gap analysis |
| Config file | Input: targets, `federation`, `alerts`, OAuth blocks |
| Feature files | Ground truth for coverage and gap analysis |

## Scenario map

**Config input** — reads a config and extracts service parameters; counts targets; detects federation (`base_port 5100`) and alert aggressiveness.

**Feature mapping** — discovers relevant features per config type (federation → federation / tui / alerts; deployment → tui / ops); maps targets to federation steps with matching-scenario counts.

**Secure context verification** — checks OAuth providers (OK / WARNING), scope assignments, and the redaction mask (`****`), with recommendations for weak scopes.

**Gap analysis** — identifies simulation→feature gaps (e.g. "malformed JSON error handling not simulated"), feature→simulation gaps, and skills lacking feature documents.

**TUI integration** — generates a TUI command containing `--mode=foreground` and the config path, documents shortcuts, and maps elements to scenarios.

**Validation report** — covers feature coverage, security verification, and TUI observation; overall status (`PASSED`, `PASSED WITH GAPS`, `NO TARGETS`); passed/failed validation counts.

**Voice modes** — warroom (urgent command language) and mentor (explanatory language with pauses).

**Edge cases** — config with 0 targets → `NO TARGETS`; no matching features → 0 feature→simulation gaps; missing feature files → 1 gap named "no feature files found for this config type".

## Cross-references

- [Config to Simulation Skill](config-to-simulation.md) — upstream simulation generator
- [Federation Mesh](federation.md) — federation config under validation
- [Warroom Builder](warroom-builder.md) — skill gap example
- [Engine & Simulation subsystem](../subsystems/engine-simulation.md)