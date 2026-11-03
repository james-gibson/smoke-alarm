---
title: Warroom Guide Skill
description: "The warroom-guide skill as automated incident commander: bridge display, participant joining, priority-ordered alarm dismissal, escalation, and resolution summary."
tags:
  - smoke-alarm
  - feature
  - wargame-onboarding
  - warroom
  - skill
  - gherkin
source: smoke-alarm/features/warroom-guide.feature
subsystem: wargame-onboarding
gherkin_tags:
  - "@warroom-guide"
  - "@skill"
scenario_count: 11
---
> **Source:** `features/warroom-guide.feature` · **Subsystem:** [Wargame & Onboarding](../subsystems/wargame-onboarding.md) · **Index:** [Feature catalog](../index.md)

The `warroom-guide` skill acts as an automated incident commander for a training bridge: it tracks participants, enforces alarm priority, and narrates escalation. It is the guided/tutorial variant; [Warroom Simulator Skill](warroom-simulator.md) is the live, trainee-driven variant.

## Scenario map

**Skill invocation** — the agent reads `.opencode/skills/warroom-guide/SKILL.md` and executes the documented steps in order.

**Incident initialisation** — with no scenario supplied the skill prompts for one (`service-outage`, `security-breach`); each run mints a unique incident ID matching `INC-YYYY-MM-DD-NNN`.

**Bridge display** — bridge status shows the severity and a participant table with join codes; all initial participants start `PENDING`.

**Participant joining** — a correct join code flips a participant to `joined` and records the response time; an incorrect code yields an invalid-code message.

**Alarm dismissal order** — alarms are presented in priority order (P1 before P2 before P3); a P2 dismissal is rejected while P1 is unresolved, and a P1 dismissal with a resolution note flips P1 to `resolved` and records the resolution time.

**Escalation** — when a P1 alarm exceeds `max_dismiss_time_minutes` without resolution, the guide escalates to the next role and requests a new participant with an escalation join code.

**Resolution summary** — once all alarms are resolved the guide displays a summary with total duration and per-alarm resolution time.

## Cross-references

- [Warroom Builder Skill](warroom-builder.md) — generates the YAML config an incident runs on
- [Warroom Simulator Skill](warroom-simulator.md) — live simulation rather than guided narration
- [Chaos Window Registration and Isotope Classification](chaos-window.md) — how alarm states are classified upstream
- [Wargame & Onboarding subsystem](../subsystems/wargame-onboarding.md)
