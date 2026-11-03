---
title: 42i Trust Rungs
description: "Certification rungs 0-6 mapped to 42i distance ceilings: evidence-derived assignment, InstanceRecord declaration, value-read authorization, JSON-RPC filtering, proxy identity, and distance shifts."
tags:
  - smoke-alarm
  - feature
  - federation-isotopes
  - trust-rungs
  - authorization
  - gherkin
source: smoke-alarm/features/trust-rungs.feature
subsystem: federation-isotopes
gherkin_tags:
  - "@trust-rungs"
  - "@optional"
scenario_count: 25
---
> **Source:** `features/trust-rungs.feature` · **Subsystem:** [Federation & Isotopes](../subsystems/federation-isotopes.md) · **Index:** [Feature catalog](../index.md)

Trust rungs are a coarse bucketing of 42i distance that gate which fields and outputs a caller may read. The server is the authority: it looks up the caller's certified rung in its own registry (by instance ID or connection identity) and filters responses before transmission. The caller never declares its own rung, and never learns that a field was omitted — the response stays structurally valid. This is the encryption stand-in: no crypto yet, authorization gating at the rung boundary does the work (when encryption is added, each rung will map to a key tier; the wire protocol and gating logic do not change).

| Rung | Name | 42i distance ceiling | Condition |
|---|---|---|---|
| 0 | uncertified | ∞ | no isotopes observed |
| 1 | dev-certified | ∞ | plaintext isotopes only |
| 2 | transit-verified | 80 | cryptographic isotopes verified |
| 3 | chaos-certified | 40 | chaos suppression confirmed |
| 4 | peer-certified | 16 | inter-peer registration observed |
| 5 | mesh-certified | 110 | all mesh peers have certified each other |
| 6 | temporally-stable | 220 | transits verified across multiple time windows |

## Scenario map

**Rung assignment** — no isotope activity → rung 0; plaintext flowing → rung 1; verified cryptographic isotopes → rung 2; active chaos window with confirmed suppression → rung 3; observed `probeIsotopeRegistration` → rung 4; decreasing 42i distance promotes; an isotope-variation failure adds 8 distance and can demote.

**Carried in InstanceRecord** — the instance declares its rung in `Meta["trust_rung"]` on introduction and updates it each heartbeat; but the introducer computes the rung from observed evidence, ignoring a declared value that exceeds the evidence (declares 6, stored as 1).

**Value-read authorization** — rung 0 reads `/status` (id, state only; omits `isotope_id`, `isotope_classification`, `message`, `details`); rung 1 adds plaintext `isotope_id`; rung 2 adds canonical `isotope_id`; rung 3 adds `isotope_classification`; rung 4 reads `/membership` with routing-trace metadata and per-peer rungs; a below-rung caller receives the field omitted (HTTP 200, no error/explanation).

**JSON-RPC skill output filtering** — a rung-0 caller invoking a rung-3 skill is rejected with JSON-RPC `-32003` ("insufficient trust rung: need 3, have 0"); a rung-3 caller gets the full result; a mixed-rung skill result returns only the fields at or below the caller's rung.

**Proxying preserves caller identity** — a proxy forwards the original caller's instance ID so the target looks up that caller's rung; an intermediary cannot grant a higher rung; a forged/absent originating-caller header is warned ("proxy forwarded without originating-caller identity") and processed at the lowest known rung.

**Data access shifts distance** — untouched data does not shift distance; reading at or below rung incurs no cost; verifying a transit heals distance (and may promote); a scope-compliance failure adds 8; manipulating a value costs more than reading.

**Thresholds & ceilings** — high rungs have larger ceilings and are disproportionately resilient; a rung-6 entity at 180 + 48 → 228 exceeds 220 and demotes to 5, then further to 4; a rung-2 entity at 70 + 8 stays, + 8 more → 86 demotes to 1.

## Cross-references

- [Isotope ID Transit](isotope-transit.md) — the transits that advance rungs
- [Plain Text Isotope Stand-Ins](plaintext-isotopes.md) — rung 1 evidence
- [Federation Mesh](federation.md) — registry and InstanceRecord
- [Honeypot Nodes](honeypot-nodes.md) — rung revocation signals
- [Federation & Isotopes subsystem](../subsystems/federation-isotopes.md)