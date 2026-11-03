---
title: Canonical Isotope ID Construction
description: "The pinned isotope_id algorithm: base64url(SHA256(feature_id || ':' || SHA256(payload) || ':' || nonce)), its properties, verification, encoding, and cross-component agreement."
tags:
  - smoke-alarm
  - feature
  - federation-isotopes
  - isotope
  - algorithm
  - gherkin
source: smoke-alarm/features/isotope-construction.feature
subsystem: federation-isotopes
gherkin_tags:
  - "@isotope"
  - "@core"
scenario_count: 13
---
> **Source:** `features/isotope-construction.feature` · **Subsystem:** [Federation & Isotopes](../subsystems/federation-isotopes.md) · **Index:** [Feature catalog](../index.md)

A pure algorithm spec, with no runtime dependencies, pinning the canonical isotope ID so any component can independently construct or verify an ID without coordination. The algorithm must be agreed and pinned before any isotope work proceeds; changing it after deployment invalidates all existing isotope IDs.

```text
isotope_id = base64url( SHA256( feature_id || ":" || SHA256(payload) || ":" || nonce ) )
```

- `feature_id` — the Gherkin scenario identifier (e.g. `adhd/light-transitions-dark-to-green`)
- `payload` — the probe response body (never stored; only its hash is used)
- `nonce` — a per-isotope random value ensuring uniqueness across invocations
- `||` — byte concatenation
- `SHA256` — standard SHA-256, raw bytes before the outer hash
- `base64url` — RFC 4648 §5 unpadded URL-safe base64
- Result — always 43 characters (256 bits → 32 bytes → 43 base64url chars)

## Scenario map

**Construction properties** — always 43 base64url chars (`[A-Za-z0-9_-]`); two IDs for the same feature+payload differ by nonce; identical inputs (incl. nonce) are deterministic; changing `feature_id` or payload changes the ID.

**Verification** — succeeds with the declared feature binding; fails on a mismatched `feature_id` or payload; no payload information is recoverable from the ID (a one-way commitment, not an encoding).

**Encoding requirements** — unpadded URL-safe base64 (no `+`, `/`, or `=`); safe in URL path segments (e.g. `/isotope/<id>/verify`) without percent-encoding.

**Cross-component agreement** — ocd-smoke-alarm and adhd must produce identical IDs for the same inputs (guards against implementation divergence).

## Cross-references

- [Isotope ID Transit](isotope-transit.md) — where isotope IDs flow in probe results
- [Known-State Regression Detection](known-state.md) — transit and verification in the store
- [Honeypot Nodes](honeypot-nodes.md) — a pre-computed canary isotope
- [Federation & Isotopes subsystem](../subsystems/federation-isotopes.md)