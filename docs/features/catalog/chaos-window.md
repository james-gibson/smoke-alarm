---
title: Chaos Window Registration and Isotope Classification
description: Registering authorised chaos windows and classifying isotopes as registered-chaos, expired-window, or unregistered, with the smoke-alarm as the authoritative classifier.
tags:
  - smoke-alarm
  - feature
  - ops-safety
  - chaos
  - isotope
  - gherkin
source: smoke-alarm/features/chaos-window.feature
subsystem: ops-safety
gherkin_tags:
  - "@chaos"
  - "@isotope"
  - "@core"
scenario_count: 12
---
> **Source:** `features/chaos-window.feature` · **Subsystem:** [Operations & Safety](../subsystems/ops-safety.md) · **Index:** [Feature catalog](../index.md)

An operator or test harness declares a time window during which a specific isotope is authorised to arrive with a failing probe result, so ADHD can suppress false-alarm red lights during planned chaos experiments. The smoke-alarm is the **authority** on isotope classification: ADHD does not consult its own clock or registry — it reads the classification from the `/status` response, avoiding clock-skew and stale-state errors.

## Scenario map

**Window registration** — `POST /isotope/register-chaos-window` stores a window (target_id, isotope_id, window_start, window_end) and returns 200; a past `window_end` → 400 "window_end must be in the future"; an unknown target → 404 "target not found"; `GET /isotope/chaos-windows` lists entries with their start/end; expired windows are removed by the cleanup cycle.

**Isotope classification in /status** — a failing probe carrying a registered, in-window isotope → `isotope_classification` "registered-chaos"; an elapsed window → "expired-window"; no window → "unregistered"; a healthy response → empty/no classification; no isotope received → neither `isotope_id` nor `isotope_classification`.

**Classification is authoritative** — ADHD receives the classification in the LightUpdate and suppresses the red light without consulting its own registry or sending a follow-up request to `/isotope`.

**Window isolation** — a window for one target does not classify another target's failure (the other is "unregistered") and is unaffected.

## Cross-references

- [Isotope ID Transit](isotope-transit.md) — isotope_id must flow first
- [Message Routing](routing.md) — chaos isotope registration in the routing layer
- [Certification Gate](certification-gate.md) — chaos classification eligibility
- [Operations & Safety subsystem](../subsystems/ops-safety.md)