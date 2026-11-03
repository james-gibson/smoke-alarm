---
title: Demo Capabilities Skill
description: "The demo-capabilities skill: skill discovery, validation status, related config-file checks, and a stable capabilities report."
tags:
  - smoke-alarm
  - feature
  - skills-certification
  - skills
  - demo
  - gherkin
source: smoke-alarm/features/demo-capabilities.feature
subsystem: skills-certification
gherkin_tags:
  - "@demo-capabilities"
  - "@skill"
scenario_count: 10
---
> **Source:** `features/demo-capabilities.feature` · **Subsystem:** [Skills & Certification](../subsystems/skills-certification.md) · **Index:** [Feature catalog](../index.md)

The `demo-capabilities` skill (`.opencode/skills/demo-capabilities/SKILL.md`) discovers, validates, and reports all available skills, so a developer can understand what agent capabilities exist and confirm they are correctly configured.

## Scenario map

**Invocation** — the agent reads the skill and executes the documented steps in order.

**Skill discovery** — each subdirectory of `.opencode/skills/` is scanned for a `SKILL.md`; the output lists each skill by name and includes a skill inventory table with a validation status per row.

**Skill validation** — a skill with the required name and description fields appears as valid (`start-here`, `opencode-status-report`, `open-the-pickle-jar`); a name/directory mismatch appears as invalid with the failure reason.

**Related config files** — the output includes the presence status of `AGENTS.md` and `Makefile`.

**Output structure** — the output includes a next-steps section containing "make ci" and "opencode-status-report".

**Stability** — repeated invocations with identical inputs produce equivalent output and no duplicate `state/` files.

## Cross-references

- [Skill System Validation](skill-system.md) — the validation layer behind the inventory
- [Open the Pickle Jar](open-the-pickle-jar.md) — sibling skill
- [OpenCode Status Report Skill](opencode-status-report.md) — referenced in next steps
- [Skills & Certification subsystem](../subsystems/skills-certification.md)