---
title: Dynamic Skill Certification
description: "42i certification of skills by observed evidence: existence, equality, and isotope levels, plus hop-distance penalties and dynamic re-evaluation on heartbeats."
tags:
  - smoke-alarm
  - feature
  - skills-certification
  - certification
  - skills
  - gherkin
source: smoke-alarm/features/skill-certification.feature
subsystem: skills-certification
gherkin_tags:
  - "@skill-certification"
  - "@optional"
scenario_count: 21
---
> **Source:** `features/skill-certification.feature` · **Subsystem:** [Skills & Certification](../subsystems/skills-certification.md) · **Index:** [Feature catalog](../index.md)

Skills are managed like isotopes: certified externally by the smoke-alarm, not self-declared by the hosting instance. A skill has a certified rung determined by observed evidence at three levels — **existence** (reachable and responds, rung ≥ 1), **equality** (equal output for equal inputs, rung ≥ 2), and **isotope** (execution traceable via an isotope transit to a Gherkin scenario, rung ≥ 3). Skills also carry a **distance** equal to the number of proxy hops between the skill and the certifying alarm; each hop adds to its 42i distance. A skill-variation failure (equal inputs, non-equal outputs) adds 8 to its distance; sufficient failures demote it. Certification is re-evaluated on each heartbeat.

## Scenario map

**Existence certification (rung 1)** — a skill that responds to an empty-params probe within the timeout is rung 1 and reachable in `GET /membership`; a non-responding skill stays rung 0 and is excluded from routing; re-evaluated each heartbeat (demotion logs "skill existence probe failed"); a rung-0 skill is absent from `tools/list`.

**Equality certification (rung 2)** — two identical probe calls with byte-equal results promote to rung 2; non-equal results record a skill-variation failure (+8 distance) and stay at rung 1; four failures in a 5-minute window demote to rung 0 ("skill variation failures exceeded threshold"); equality can lapse (demote if distance exceeds the rung-2 ceiling of 80).

**Isotope certification (rung 3+)** — an invocation carrying a verifiable isotope promotes to rung 3 and records the certifying isotope as the feature binding; the isotope is treated as a transit event mapped to the declared scenario; 3 consecutive invocations without an isotope demote to rung 2 ("skill isotope transit lapsed").

**Skill distance** — directly hosted → distance 0 (no penalty); one proxy hop → distance 1 (+8); N hops → N (N × 8 penalty, reduced effective ceiling); when two instances host the same skill, the closer one is preferred for routing.

**Rung in tools/list and routing** — `tools/list` includes the certified rung; a rung-1 caller invoking a rung-3 skill is rejected `-32003` ("insufficient trust rung: skill requires 3, caller has 1"); rung gates response content, not just invocability (mixed-rung fields filtered).

**Dynamic certification** — rung advances (1→2) or retreats (3→2) during a heartbeat cycle; rapid alternation within a window is smoothed to the minimum observed rung.

## Cross-references

- [42i Trust Rungs](trust-rungs.md) — rung assignment and distance ceilings
- [Federated Skill Identity and Discovery](federated-skills.md) — skill identity and routing
- [Skill Execution Receipts](skill-execution-receipts.md) — execution evidence
- [Certification Gate](certification-gate.md) — instance-level baseline
- [Skills & Certification subsystem](../subsystems/skills-certification.md)