---
title: Start-Here Welcome Skill
description: "The start-here skill: connection confirmation, AGENTS.md/Makefile and skills-inventory validation, welcome output, and repeat-run stability."
tags:
  - smoke-alarm
  - feature
  - wargame-onboarding
  - onboarding
  - skill
  - gherkin
source: smoke-alarm/features/start-here.feature
subsystem: wargame-onboarding
gherkin_tags:
  - "@start-here"
  - "@skill"
scenario_count: 8
---
> **Source:** `features/start-here.feature` · **Subsystem:** [Wargame & Onboarding](../subsystems/wargame-onboarding.md) · **Index:** [Feature catalog](../index.md)

The `start-here` skill is the first-run check for an agent newly connected to this repository: it confirms the connection, validates the project layout, and produces a stable welcome report.

## Scenario map

**Skill invocation** — the agent reads `.opencode/skills/start-here/SKILL.md` and executes the documented steps in order.

**Connection confirmation** — output contains `Connected` and `ocd-smoke-alarm`.

**Project structure validation** — the report confirms `AGENTS.md` and `Makefile` are present, reports a skill inventory of at least one skill from `.opencode/skills/`, and lists at least `opencode-status-report` and `demo-capabilities` by name.

**Quick-start** — the next-steps section includes `make ci`.

**Stability** — running start-here twice with identical inputs yields equivalent output and creates no duplicate state files under `state/`.

## Cross-references

- [OpenCode Status Report Skill](opencode-status-report.md) — deeper audit of the same agent/config layout
- [Demo Capabilities Skill](demo-capabilities.md) — capability discovery over the skills inventory
- [Skill System Validation](skill-system.md) — the `SKILL.md` contract this skill assumes
- [Wargame & Onboarding subsystem](../subsystems/wargame-onboarding.md)
