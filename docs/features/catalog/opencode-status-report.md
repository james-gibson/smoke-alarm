---
title: OpenCode Status Report Skill
description: Skill that audits agent/config files (AGENTS.md, .opencode.json, skills, sample configs) into a validation report.
tags:
  - smoke-alarm
  - feature
  - health-status
  - skill
  - opencode
source: smoke-alarm/features/opencode-status-report.feature
subsystem: health-status
gherkin_tags:
  - "@opencode-status-report"
  - "@skill"
scenario_count: 9
---
> **Source:** `features/opencode-status-report.feature` · **Subsystem:** [Health & Status](../subsystems/health-status.md) · **Index:** [Feature catalog](../index.md)

The `opencode-status-report` skill generates a comprehensive audit report of the project's agent/config files so a developer can confirm the configuration is complete and valid. The report covers agent instruction files (e.g. `AGENTS.md`), `.opencode.json`, discovered skills under `.opencode/skills/`, and sample configs under `configs/samples/`, ending in a validation summary.

## Actors & surfaces

| Actor | Interaction |
|---|---|
| Developer | Invokes the skill from an OpenCode session |
| Skill bundle | `.opencode/skills/opencode-status-report/SKILL.md` (documented steps) |

## Scenario map

- Invoking the skill reads `SKILL.md` and executes the documented steps in order.
- Report includes the status of `AGENTS.md`, `.opencode.json` (when present), and each skill under `.opencode/skills/` with its validation status.
- Report includes at least one sample config entry from `configs/samples/` and shows `configs/sample.yaml` as valid (YAML syntax checked).
- Report includes a validation summary with "Total files checked", "Passed", and "Failed"; a missing required file (e.g. `AGENTS.md`) is flagged as missing and increments the failed count.
- Output is stable across repeated invocations and creates no duplicate state files under `state/`.

## Cross-references

- [Feature Knowledge Base](../index.md)
- [Start-Here Welcome Skill](start-here.md) — sibling onboarding skill