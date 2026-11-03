---
title: Federation & Isotopes
description: The federation mesh, federated skill identity, canonical isotope construction and transit, dev-mode plaintext isotopes, 42i trust rungs, and honeypot detection.
tags:
  - smoke-alarm
  - subsystem
  - federation-isotopes
feature_count: 7
---
> Part of the [Feature Knowledge Base](../index.md). See the [consolidated catalog](../consolidated.md) for a flat view.

# Federation & Isotopes

Features covering multi-instance meshing and the isotope evidence model that underpins trust: mesh slot election and membership, federated skill identity and routing, canonical isotope construction and transit, dev-mode plaintext stand-ins, 42i trust rungs, and honeypot counterfeit detection.

## Features

| Feature | Source | Type |
|---|---|---|
| [Federation Mesh](../catalog/federation.md) | `features/federation.feature` | runtime mesh |
| [Federated Skill Identity and Discovery](../catalog/federated-skills.md) | `features/federated-skills.feature` | skill discovery |
| [Canonical Isotope ID Construction](../catalog/isotope-construction.md) | `features/isotope-construction.feature` | algorithm spec |
| [Isotope ID Transit](../catalog/isotope-transit.md) | `features/isotope-transit.feature` | evidence plumbing |
| [Plain Text Isotope Stand-Ins](../catalog/plaintext-isotopes.md) | `features/plaintext-isotopes.feature` | dev-mode bypass |
| [42i Trust Rungs](../catalog/trust-rungs.md) | `features/trust-rungs.feature` | authorization model |
| [Honeypot Nodes](../catalog/honeypot-nodes.md) | `features/honeypot-nodes.feature` | detection |

## Cross-cutting notes

- Isotope IDs are the shared currency: constructed once ([`isotope-construction`](../catalog/isotope-construction.md)), transited through probe responses ([`isotope-transit`](../catalog/isotope-transit.md)), and consumed by rungs and honeypots.
- Trust rungs are evidence-derived, never self-declared — the server is the authority, which is what makes the model resist forged certification.
- Honeypots turn routing misbehaviour into a visible mark without requiring traffic inspection: a canary isotope outside its honeypot proves contact.
- Fan-out is valid and cycles are forbidden across both federation polling and skill routing; every routed message carries a trace.