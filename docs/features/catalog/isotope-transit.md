---
title: Isotope ID Transit Through Health Check Responses
description: "Carrying the received isotope_id on TargetStatus: per-cycle updates, failure recording, the no-payload privacy boundary, verification, and SSE transport."
tags:
  - smoke-alarm
  - feature
  - federation-isotopes
  - isotope
  - transit
  - gherkin
source: smoke-alarm/features/isotope-transit.feature
subsystem: federation-isotopes
gherkin_tags:
  - "@isotope"
  - "@core"
scenario_count: 11
---
> **Source:** `features/isotope-transit.feature` · **Subsystem:** [Federation & Isotopes](../subsystems/federation-isotopes.md) · **Index:** [Feature catalog](../index.md)

Each target's health-check result carries the isotope ID that arrived with the probe response, so an ADHD dashboard consuming `/status` can match the isotope against a registered chaos window or flag an unregistered isotope as a scope-compliance failure. This is the foundational prerequisite for chaos isotopes: until `isotope_id` appears in `TargetStatus`, no downstream classification or window-matching logic can function.

## Scenario map

**Isotope field on TargetStatus** — a healthy response carrying `iso-abc-001` is reflected as `isotope_id` on the target; `isotope_id` is omitted (no empty string) when the probe carries none; the value updates on each probe cycle; it is still recorded on a failing probe (`unhealthy` + `iso-fail-001`).

**Payload is not stored** — only the isotope ID transits, never the probe response body; the `/status` response must not contain any probe response field in `isotope_id`, `message`, `details`, or any other field. This is a privacy boundary (ADHD must not be able to recover the payload).

**Isotope construction** — the canonical algorithm is `base64url(SHA256(feature_id || ":" || SHA256(payload) || ":" || nonce))`; the smoke-alarm does not construct isotopes, it receives and records their transit; a received isotope is recorded as "observed at target" without the payload or nonce; two IDs for the same feature+payload differ and are 43 base64url chars.

**SSE transport** — an SSE status event includes `isotope_id` when the probe returned one; the event omits the field when the probe carried none.

## Cross-references

- [Canonical Isotope ID Construction](isotope-construction.md) — the pinned algorithm
- [Known-State Regression Detection](known-state.md) — transit events on record
- [Trust Rungs](trust-rungs.md) — isotope-classification read gating
- [Chaos Window](chaos-window.md) — window matching against transits
- [Federation & Isotopes subsystem](../subsystems/federation-isotopes.md)