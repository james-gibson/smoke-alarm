---
title: Isotopes as Skill Execution Receipts
description: "The isotope-as-receipt model: receipt structure, transit through /status, cross-alarm certification, verification, and cumulative evidence."
tags:
  - smoke-alarm
  - feature
  - skills-certification
  - isotope
  - receipts
  - gherkin
source: smoke-alarm/features/skill-execution-receipts.feature
subsystem: skills-certification
gherkin_tags:
  - "@skill-receipts"
  - "@isotope"
  - "@core"
scenario_count: 18
---
> **Source:** `features/skill-execution-receipts.feature` · **Subsystem:** [Skills & Certification](../subsystems/skills-certification.md) · **Index:** [Feature catalog](../index.md)

Conceptual unification: isotopes are receipts of skill execution on remote targets. The isotope ID construction algorithm already encodes this — `feature_id` is the Gherkin scenario describing the skill, `payload` is the skill's execution result, and `nonce` makes each receipt unique. Seeing an isotope in transit proves the named skill executed, produced the committed payload, and ran at least once. Receipts travel back through the target's `/status` response automatically — there is no separate reporting step. **The isotope IS the receipt. Transit IS the proof.**

## Scenario map

**Receipt structure** — an isotope committed from skill execution encodes the skill's Gherkin scenario; the receipt commits to the result without exposing it (one-way); two executions with the same result produce different receipts (distinct nonces, no replay); two executions producing different results produce different receipts (evidence of non-determinism).

**Receipt transit** — a skill executed by a remote agent produces a receipt that transits through `/status` with no separate reporting call; multiple agents executing the same skill on the same target produce distinct, independently recorded receipts.

**Smoke-alarms testing other smoke-alarms** — A executes a skill against B and records the receipt, advancing B's skill rung; a receipt from a cross-alarm execution advances the target's rung toward 3; B's `/status` carrying the receipt proves B received, processed, and committed the result (no acknowledgement required).

**Receipt verification** — verifies when `feature_id`, payload, and nonce all match; a forged receipt (unknown nonce) fails; a mismatched `feature_id` fails and indicates a different skill.

**Chain of receipts as evidence** — each verified receipt reduces the skill's 42i distance (and can advance it to rung 3); a receipt for a never-seen skill creates a new transit record and enters certification at rung 1; a receipt from a rung-0 agent is recorded but does not advance the rung ("receipt from uncertified agent: rung advancement withheld").

**Receipts in chaos scenarios** — a chaos skill execution produces the chaos receipt (constructed from the failure payload), classified `registered-chaos` when it transits with a failing probe; observing it proves the chaos skill ran intentionally, distinguishing it from an unregistered failure.

**No separate confirmation protocol** — proof of execution is established by the receipt appearing in `/status`; no callback, acknowledgement, or reporting endpoint is required.

## Cross-references

- [Canonical Isotope ID Construction](isotope-construction.md) — the receipt format
- [Isotope ID Transit](isotope-transit.md) — the delivery channel
- [Dynamic Skill Certification](skill-certification.md) — receipts certify skills
- [Chaos Skills as Detection Probes](chaos-skill-detection.md) — receipts as detection
- [Skills & Certification subsystem](../subsystems/skills-certification.md)