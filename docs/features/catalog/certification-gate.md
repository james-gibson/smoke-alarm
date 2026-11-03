---
title: Isotope Certification Gate
description: "Excluding uncertified instances from full membership and routing: isotope_certified claims, acceptance policy, membership visibility, revocation, and scope-compliance failures."
tags:
  - smoke-alarm
  - feature
  - skills-certification
  - certification
  - federation
  - gherkin
source: smoke-alarm/features/certification-gate.feature
subsystem: skills-certification
gherkin_tags:
  - "@certification-gate"
  - "@optional"
scenario_count: 17
---
> **Source:** `features/certification-gate.feature` · **Subsystem:** [Skills & Certification](../subsystems/skills-certification.md) · **Index:** [Feature catalog](../index.md)

An instance is "certified" if it can demonstrate isotope transit capability — its probe responses carry isotope IDs verified against their declared feature bindings. Uncertified instances are excluded from full federation membership and are not routed to for skill invocations. Certification status is carried in `InstanceRecord.Meta["isotope_certified"]` and re-evaluated each heartbeat. An instance self-certifies by running the isotope transit pipeline; the introducer trusts the claim in the heartbeat and tombstones instances whose claim is revoked or absent for more than N cycles.

## Scenario map

**Certification claim** — a certified instance sends `iso_certified=true`; an uncertified one sends `false`; the status is included in heartbeat records.

**Introducer acceptance policy** — a certified instance is accepted as role `peer` (status "certified"); an uncertified one is accepted but restricted as `peer-uncertified` ("uncertified") and excluded from federated skill routing; a JSON-RPC call to an uncertified peer is rejected "peer uncertified"; a later certifying heartbeat promotes it to full `peer` and into routing.

**Membership visibility** — each `peers` entry carries `isotope_certified`; `GET /membership?filter=uncertified` returns only the uncertified peers.

**Revocation** — a heartbeat with `iso_certified=false` demotes to `peer-uncertified` ("peer certification revoked"); 3 consecutive heartbeats missing the field also demote ("certification claim absent").

**Scope compliance failures** — a single isotope-variation failure does not revoke (noted in the membership record); 3 within a 5-minute window revoke certification.

**Plaintext isotopes and certification** — a dev-mode instance certified via plaintext isotopes is marked `dev-certified`; a production introducer (with `allow_plaintext_isotopes=false`) restricts dev-certified peers to the uncertified policy ("dev-certified peer restricted in production").

**ADHD-side view** — a self-describing alarm with `iso_certified=true` is a trusted target source eligible for chaos classification; one with `false` shows an "uncertified" annotation and receives no chaos suppression.

## Cross-references

- [Isotope ID Transit](isotope-transit.md) — the capability being certified
- [Federation Mesh](federation.md) — InstanceRecord and membership
- [Chaos Window](chaos-window.md) — classification and suppression
- [Dynamic Skill Certification](skill-certification.md) — skill-level certification
- [Skills & Certification subsystem](../subsystems/skills-certification.md)