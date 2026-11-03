---
title: Federated Skill Identity and Discovery
description: Skill inventory announced via InstanceRecord, '[instance-id] skill-name' namespacing, membership skill lists, lookup, routed invocation with traces, peer departure, and self-description fields.
tags:
  - smoke-alarm
  - feature
  - federation-isotopes
  - federation
  - skills
  - gherkin
source: smoke-alarm/features/federated-skills.feature
subsystem: federation-isotopes
gherkin_tags:
  - "@federated-skills"
  - "@optional"
scenario_count: 20
---
> **Source:** `features/federated-skills.feature` · **Subsystem:** [Federation & Isotopes](../subsystems/federation-isotopes.md) · **Index:** [Feature catalog](../index.md)

Each federation instance announces the skills it hosts as part of its membership identity, so skills can be discovered, routed, and invoked across the mesh with unambiguous namespacing. A remote skill is referred to as `[instance-id] skill-name` (instance-id is the 16-hex identity from `ClaimSlot`); a local skill keeps its unqualified name. Announcement rides on the `InstanceRecord` extension point (`Meta["skills"]` or a dedicated field — an implementation decision; this feature specs behaviour regardless of wire representation).

## Actors & surfaces

| Actor | Interaction |
|---|---|
| Instance | Announces its skill inventory on introduction and heartbeat |
| Introducer | Aggregates inventories; serves them in `GET /membership` |
| Agent | Looks up and invokes federated skills by namespaced ID |
| Self-description | Exposes `skills` and `federated_skills` |

## Scenario map

**Skill announcement** — the `InstanceRecord` on introduction and heartbeat carries the inventory; adding a skill appears in the next heartbeat.

**Namespacing** — a remote skill is `[instance-id] skill-name`; a local skill is unqualified; two peers sharing a name yield distinct federated IDs.

**Membership API** — each peer entry includes its skill inventory (empty list when none).

**Federated lookup** — resolves a federated ID to its hosting instance and local skill name; unknown IDs return not-found.

**Skill routing & traces** — invoking a federated skill routes to the host and the trace contains the source→target IDs; a 3-hop fan-out carries the full hop trace; an invocation whose trace already contains the local instance ID is rejected as a cycle ("routing cycle detected"); a direct A→B→A cycle is rejected and names both IDs.

**open-the-pickle-jar in federated context** — invoked on an instance summarises that instance's skills and notes peer overlap; targeted at `[instance-id]` routes the audit to that peer; audit output distinguishes local vs federated coverage.

**Peer departure** — skills are removed from the federated registry on age-out (with a `removed` event) and on explicit removal (count decreases).

**Self-description reflects federated skills** — the document lists local `skills` and a `federated_skills` array of `[instance-id] skill-name` entries.

## Cross-references

- [Federation Mesh](federation.md) — InstanceRecord, membership, slot election
- [Open the Pickle Jar](open-the-pickle-jar.md) — the skill exercised in federated context
- [Self-Description Endpoint](selfdescription.md) — `skills` / `federated_skills`
- [Federation & Isotopes subsystem](../subsystems/federation-isotopes.md)