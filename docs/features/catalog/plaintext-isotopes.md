---
title: Plain Text Isotope Stand-Ins
description: "Dev/demo-mode 'plaintext:<feature-id>:<seq>' isotope stand-ins: format recognition, feature-id extraction, chaos-window compatibility, uniqueness, SSE, and the migration path."
tags:
  - smoke-alarm
  - feature
  - federation-isotopes
  - isotope
  - dev-mode
  - gherkin
source: smoke-alarm/features/plaintext-isotopes.feature
subsystem: federation-isotopes
gherkin_tags:
  - "@plaintext-isotopes"
  - "@dev-mode"
scenario_count: 12
---
> **Source:** `features/plaintext-isotopes.feature` · **Subsystem:** [Federation & Isotopes](../subsystems/federation-isotopes.md) · **Index:** [Feature catalog](../index.md)

Plain text stand-ins allow the full isotope transit pipeline to be exercised before the cryptographic construction algorithm is implemented, or in environments where key management is not yet in place (local dev, demo cluster). Format: `plaintext:<feature-id>:<sequential-id>` (e.g. `plaintext:adhd/mdns-discovery:001`). They are accepted only when the instance is configured `allow_plaintext_isotopes: true`; production instances reject them.

## Scenario map

**Format recognition** — in dev mode a plaintext `isotope_id` produces no error and appears unchanged in `/status`; in production mode it is dropped from `TargetStatus`, a warning "plaintext isotope rejected in production mode" is logged, and `/status` shows no `isotope_id`. No hash computation or verification is attempted on plaintext values.

**Feature-id extraction** — the feature-id component is extractable (`plaintext:adhd/mdns-discovery:001` → `adhd/mdns-discovery`); a transiting plaintext isotope marks the mapped Gherkin scenario as covered, noting it was a plaintext stand-in.

**Chaos window compatibility** — a chaos window can be registered for a plaintext isotope via `POST /isotope/register-chaos-window`; a failing probe carrying a registered plaintext chaos isotope is classified `registered-chaos`.

**Uniqueness within a session** — plaintext isotopes use a sequential-id suffix for intra-session uniqueness (not globally unique across restarts — a known dev-mode limitation); two different sequential IDs raise an isotope-variation failure (unexpected rotation); the same ID observed twice at the same target is a duplicate — only the first transit is recorded and no variation failure is raised.

**SSE transport** — the SSE event carries the plaintext isotope ID when the probe returned one.

**Migration path** — switching from a plaintext ID to a canonical cryptographic ID is treated as a new isotope (variation), with no association to the previous plaintext ID.

## Cross-references

- [Isotope ID Transit](isotope-transit.md) — the pipeline being exercised
- [Canonical Isotope ID Construction](isotope-construction.md) — the algorithm this bypasses
- [Trust Rungs](trust-rungs.md) — plaintext isotopes confer rung 1
- [Chaos Window](chaos-window.md) — registration and classification
- [Federation & Isotopes subsystem](../subsystems/federation-isotopes.md)