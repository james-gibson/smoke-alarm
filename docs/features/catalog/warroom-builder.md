---
title: Warroom Builder Skill
description: "The warroom-builder skill: incident parameter collection, severity-driven YAML config generation, and incident-script generation with unique join codes."
tags:
  - smoke-alarm
  - feature
  - wargame-onboarding
  - warroom
  - skill
  - gherkin
source: smoke-alarm/features/warroom-builder.feature
subsystem: wargame-onboarding
gherkin_tags:
  - "@warroom-builder"
  - "@skill"
scenario_count: 11
---
> **Source:** `features/warroom-builder.feature` · **Subsystem:** [Wargame & Onboarding](../subsystems/wargame-onboarding.md) · **Index:** [Feature catalog](../index.md)

The `warroom-builder` skill turns structured incident parameters into a complete warroom setup: a valid `ocd-smoke-alarm` YAML config plus an incident script for the role-play. It is the authoring half of the wargame suite — [Warroom Guide Skill](warroom-guide.md) narrates a tutorial run and [Warroom Simulator Skill](warroom-simulator.md) runs the live simulation.

## Scenario map

**Skill invocation** — the agent reads `.opencode/skills/warroom-builder/SKILL.md` and executes the documented steps in order.

**Parameter collection** — with no incident parameters supplied, the skill asks for the incident type (`service-outage`, `security-incident`) and severity (`SEV1`, `SEV2`, `SEV3`).

**YAML config generation** — `security-breach`/`SEV1` parameters render a config with `version: "1"`, a `targets` block containing one entry per participant, and an `alerts` block with `aggressive: true`.

**Severity-driven polling** — `poll_interval` is `2s` for `SEV1` and `10s` for `SEV3`; `service-outage`/`SEV2` parameters produce `persist: false` under `known_state` (an ephemeral run).

**Incident script generation** — alongside the YAML the skill emits a script with a participant join-code table and an alarm-sequence section; join codes differ on every run, even with identical parameters.

**Output location** — generated configs land under `configs/samples/warroom-…`; when a required file such as `configs/samples/federation-star-hub.yaml` is missing, the skill names it and suggests a remediation.

## Cross-references

- [Warroom Guide Skill](warroom-guide.md) — tutorial incident-commander flow over a generated config
- [Warroom Simulator Skill](warroom-simulator.md) — live `/join`/`/resolve` simulation over a generated config
- [Config to Simulation Skill](config-to-simulation.md) — the reverse direction: config to simulation artefacts
- [Wargame & Onboarding subsystem](../subsystems/wargame-onboarding.md)
