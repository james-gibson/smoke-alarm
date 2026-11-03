---
title: Honeypot Nodes
description: "Counterfeit detection for the mesh: silent and canary honeypots, pre-computed canary isotope, detection signals, ADHD presentation, and demo/integration assertions."
tags:
  - smoke-alarm
  - feature
  - federation-isotopes
  - honeypot
  - detection
  - gherkin
source: smoke-alarm/features/honeypot-nodes.feature
subsystem: federation-isotopes
gherkin_tags:
  - "@honeypot"
  - "@optional"
scenario_count: 14
---
> **Source:** `features/honeypot-nodes.feature` · **Subsystem:** [Federation & Isotopes](../subsystems/federation-isotopes.md) · **Index:** [Feature catalog](../index.md)

A honeypot node is a federation target that looks legitimate but should never receive skill deliveries or be the source of isotope transits in a correctly-operating mesh. Like counterfeit-detection ink: invisible under normal operation, but any contact leaves a mark. Two properties:

- **Silent honeypot** — exists but should never be contacted; any receipt or routing trace touching it is a detection signal.
- **Canary honeypot** — responds with a distinctive pre-known receipt (the "counterfeit ink"); if that receipt appears anywhere outside the honeypot, a forgery is in progress.

The honeypot isotope is pre-computed and held by the certifying authority. It should never appear in `/status` for a non-honeypot target.

## Scenario map

**Registration** — a `type: honeypot` target appears in membership with role `honeypot` but is excluded from all skill routing decisions and never returned in `tools/list`; it is not in the routeable set.

**Canary isotope** — `honeypot_isotope = base64url(SHA256(feature_id || ":" || SHA256(payload) || ":" || nonce))` from a controlled payload and pre-selected nonce, stored as the canary.

**Silent honeypot detection** — a routing trace mentioning the honeypot raises "honeypot contact detected in routing trace" (+16 42i distance, event includes skill_name/routing_trace/originating_instance); a call addressed directly is rejected with `-32000` and flags the caller; a probe response from a silent honeypot raises "honeypot responded to probe", discards the response, and flags the probing instance.

**Canary honeypot detection** — the honeypot returns `honeypot_isotope` as its receipt; that receipt appearing in `/status` for a legitimate target raises "canary receipt observed outside honeypot" and prompts examination of the routing path; appearing in a skill result raises "canary receipt in skill result" and suspends that skill's certification and revokes the host instance's rung.

**Detection in ADHD** — honeypot nodes render with a distinct indicator (green "clean — no contact detected", red "contact detected"); a detection event turns the light red with the event summary in Details; the red light is not suppressible by chaos window registration nor classifiable as `registered-chaos`; an all-green cluster shows a certified "routing integrity" indicator ("N honeypot nodes clean").

**Demo & integration** — the demo cluster always includes at least one honeypot, shown green with status in the health summary; the integration suite asserts `honeypot_contact_count == 0` at teardown.

## Cross-references

- [Skill Execution Receipts](skill-execution-receipts.md) — isotope = receipt of execution
- [Chaos Skill Detection](chaos-skill-detection.md) — receipt deviation = detection
- [Trust Rungs](trust-rungs.md) — rung gates routing and revocation
- [Federation & Isotopes subsystem](../subsystems/federation-isotopes.md)