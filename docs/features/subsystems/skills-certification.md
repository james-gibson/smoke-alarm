---
title: Skills & Certification
description: Skill validation, dynamic 42i skill certification, execution receipts, chaos detection probes, the isotope certification gate, and the authoring/demo skills.
tags:
  - smoke-alarm
  - subsystem
  - skills-certification
feature_count: 7
---
> Part of the [Feature Knowledge Base](../index.md). See the [consolidated catalog](../consolidated.md) for a flat view.

# Skills & Certification

Features covering how skills are validated, certified, and trusted: the `internal/skills` validation layer, dynamic 42i skill certification, isotopes-as-execution-receipts, chaos detection probes, the instance-level isotope certification gate, and the authoring/demo agent skills.

## Features

| Feature | Source | Type |
|---|---|---|
| [Skill System Validation](../catalog/skill-system.md) | `features/skill-system.feature` | validation layer |
| [Dynamic Skill Certification](../catalog/skill-certification.md) | `features/skill-certification.feature` | certification model |
| [Isotopes as Skill Execution Receipts](../catalog/skill-execution-receipts.md) | `features/skill-execution-receipts.feature` | evidence model |
| [Chaos Skills as Detection Probes](../catalog/chaos-skill-detection.md) | `features/chaos-skill-detection.feature` | detection probes |
| [Isotope Certification Gate](../catalog/certification-gate.md) | `features/certification-gate.feature` | membership gate |
| [Demo Capabilities Skill](../catalog/demo-capabilities.md) | `features/demo-capabilities.feature` | agent skill |
| [Open the Pickle Jar](../catalog/open-the-pickle-jar.md) | `features/open-the-pickle-jar.feature` | agent skill |

## Cross-cutting notes

- Certification is always evidence-derived and externally assessed; neither skills nor instances self-declare their true rung.
- The execution receipt unifies the isotope model: a skill that runs emits a receipt that transits through `/status`, and that same evidence advances instance and skill rungs.
- Chaos detection repurposes the receipt protocol: a pre-computed receipt that arrives from the wrong place, with the wrong payload, or not at all reveals proxy misbehaviour without inspecting traffic.
- The authoring stack (`open-the-pickle-jar`, `demo-capabilities`, `skill-system`) keeps this very knowledge base honest — these features are the canon for the tools that generate features.