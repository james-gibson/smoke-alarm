---
title: Skill System Validation
description: "internal/skills enforcement of SKILL.md contracts: ValidateSkillFile, FindSkills, ValidateProjectConfig, and GenerateStartHereReport."
tags:
  - smoke-alarm
  - feature
  - skills-certification
  - skills
  - validation
  - gherkin
source: smoke-alarm/features/skill-system.feature
subsystem: skills-certification
gherkin_tags:
  - "@skill-system"
  - "@core"
scenario_count: 20
---
> **Source:** `features/skill-system.feature` · **Subsystem:** [Skills & Certification](../subsystems/skills-certification.md) · **Index:** [Feature catalog](../index.md)

The `internal/skills` Go package is the programmatic enforcement layer for `SKILL.md` contracts — the Gherkin canon for that layer. It validates individual skill files, discovers skills across the standard directories, checks project structure, and generates a start-here report. The `open-the-pickle-jar` skill relies on this package being correct.

## Actors & surfaces

| Actor | Interaction |
|---|---|
| Developer / agent | Runs validation before invoking a skill |
| `ValidateSkillFile` | Validates one `SKILL.md` |
| `FindSkills` | Discovers skills under standard dirs |
| `ValidateProjectConfig` | Checks required project files and skill count |
| `GenerateStartHereReport` | Emits a connection/status report |

## Scenario map

**ValidateSkillFile** — a valid file passes (log shows "YAML frontmatter parsed successfully", "name field present", "description field present", "name matches directory"); missing `name` or `description`, a name/directory mismatch, or malformed YAML fails with a matching error; a non-existent path returns an error with at least one error entry; optional `license`/`compatibility` fields parse without error.

**FindSkills** — discovers skills in `.opencode/skills/`, `.claude/skills/`, and `.claude/agents/`; returns zero (no error) when none exist; includes an invalid skill without aborting (mix of valid/invalid); ignores non-directory entries.

**Project skills contract** — all project skills validate; the `start-here` description mentions "client" and "connection"; `demo-capabilities` mentions "demonstrates"; `opencode-status-report` and `open-the-pickle-jar` exist and are valid.

**ValidateProjectConfig** — reports presence of `AGENTS.md`, `Makefile`, `.golangci.yml`, `.claude.md`; includes a skill count.

**GenerateStartHereReport** — output contains "Connected", "Configuration Status", and "make ci"; lists each discovered skill by name.

## Cross-references

- [Config Validation](config-validation.md) — project structure checks
- [Open the Pickle Jar](open-the-pickle-jar.md) — the skill that depends on this package
- [Demo Capabilities Skill](demo-capabilities.md) — skill inventory reporting
- [Skills & Certification subsystem](../subsystems/skills-certification.md)