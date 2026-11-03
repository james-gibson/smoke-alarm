---
title: Open the Pickle Jar
description: "The open-the-pickle-jar skill: Gherkin authoring rules, targeted feature writing, drift audit, step-definition implementation audit, output rules, and federated context."
tags:
  - smoke-alarm
  - feature
  - skills-certification
  - skills
  - gherkin-authoring
  - drift
  - gherkin
source: smoke-alarm/features/open-the-pickle-jar.feature
subsystem: skills-certification
gherkin_tags:
  - "@open-the-pickle-jar"
  - "@skill"
scenario_count: 28
---
> **Source:** `features/open-the-pickle-jar.feature` · **Subsystem:** [Skills & Certification](../subsystems/skills-certification.md) · **Index:** [Feature catalog](../index.md)

The `open-the-pickle-jar` skill (`.opencode/skills/open-the-pickle-jar/SKILL.md`) has two modes: **direct** (no scope) scans `features/`, `internal/`, and skills to report coverage gaps; **targeted** (with scope) reads domain source and writes a canonical `.feature` file for that domain. Every feature file it produces must conform to the 10 Cucumber step authoring rules documented in `SKILL.md` — this feature is the self-description of those rules. In a federated mesh it is invoked as `[instance-id] open-the-pickle-jar`.

## Scenario map

**SKILL.md validity** — passes `ValidateSkillFile` with the name matching the directory; its description contains "Gherkin" and "drift".

**Direct invocation (no scope)** — produces a coverage summary listing existing features, internal packages without coverage, and the highest-value next feature; flags a skill lacking a feature document; flags an uncovered package; output is stable across repeated calls with no new files.

**Targeted invocation (with scope)** — writes `features/<scope>.feature` beginning with a canon record comment (today's date) and exactly one `Feature:` block; uses `@<scope>` as the primary tag; reads `internal/<scope>/` source first and references exported types; cross-references overlapping feature files with a `see:` header.

**Step authoring rules** — every `When` begins with an imperative verb (not "the"/"a"); variable values use Cucumber expression types (no hardcoded string/integer literals where placeholders fit); no synonymous paraphrases (no "run" vs "execute" for the same concept); every Scenario Outline has an Examples table with a header row.

**Drift audit** — produces a drift table (Scenario, Feature, Code, Tests, Verdict); flags a scenario referencing a non-existent exported symbol as "DRIFT"; flags a test behavior missing from the spec as a gap; emits one `THESIS-FINDING:` per unresolved drift suitable for `TASKS.md`.

**Step definition implementation audit** — classifies each step-definition file "implemented", "partial", or "stub"; fully-stubbed and partially-stubbed files produce `THESIS-FINDING` entries (appended to `TASKS.md`); fully-implemented files produce none; a missing `features/step_definitions/` is noted but not a failure.

**Output location rules** — writes only under `features/`; step-definition stubs are written only when explicitly requested (to `features/step_definitions/<scope>_steps.go`), each step getting a stub function.

**Federated context** — direct invocation reports the local instance's coverage; a targeted `[instance-id]` scope routes the audit to that peer's feature files and skill domain.

## Cross-references

- [Skill System Validation](skill-system.md) — ValidateSkillFile and FindSkills
- [Federated Skill Identity and Discovery](federated-skills.md) — federated invocation
- [Demo Capabilities Skill](demo-capabilities.md) — sibling skill
- [Skills & Certification subsystem](../subsystems/skills-certification.md)

