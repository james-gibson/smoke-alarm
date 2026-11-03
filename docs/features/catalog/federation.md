---
title: Federation Mesh
description: Slot election, instance identity, introducer endpoints, membership age-out, peer cap, follower client, registry snapshots/events, and downstream status fan-out with cycle detection.
tags:
  - smoke-alarm
  - feature
  - federation-isotopes
  - federation
  - mesh
  - gherkin
source: smoke-alarm/features/federation.feature
subsystem: federation-isotopes
gherkin_tags:
  - "@federation"
  - "@optional"
scenario_count: 32
---
> **Source:** `features/federation.feature` · **Subsystem:** [Federation & Isotopes](../subsystems/federation-isotopes.md) · **Index:** [Feature catalog](../index.md)

The federation subsystem (`internal/federation/`) forms a local mesh of ocd-smoke-alarm instances. The first instance to bind `base_port` wins the introducer role; others claim the next free port and become followers. It covers slot election with lock and identity persistence, the introducer HTTP endpoints (`POST /introductions`, `POST /heartbeats`, `GET /membership`), age-out of silent peers, a peer cap, the follower announce/heartbeat client, atomic registry snapshots, registry change events, and downstream status fan-out with cycle detection. Architecture constraints: fan-out is valid, cycles are forbidden, and every routed message carries a routing trace.

## Actors & surfaces

| Actor | Interaction |
|---|---|
| Introducer | Serves introductions, heartbeats, and membership |
| Follower | Announces itself, heartbeats, applies returned membership |
| Poller | Fetches downstream `/status` and aggregates namespaced targets |
| Registry | Tracks peers, version, and change events |

| Surface | Purpose |
|---|---|
| `POST /introductions` | Register a peer; 400 on empty id; 405 non-POST |
| `POST /heartbeats` | Refresh `last_seen_at` |
| `GET /membership` | Registry snapshot (`self`, `peers`) |

## Scenario map

**Slot election** — first instance binds `base_port` as introducer and persists identity to `<state_dir>/federation/identity.json`; the next claims `base_port + 1` as follower; `ClaimSlot` returns "federation: no free local slots available" when full; defaults to 5100 with 4 slots.

**Instance identity** — deterministic for the same hostname/service/state_dir/port; changes when the port changes; a persisted identity is tried first for slot stability across restarts.

**Slot lock** — concurrent claims never double-claim; a stale lock from a dead PID is recovered.

**Introducer server** — binds the claimed listener; `POST /introductions` adds the peer and returns membership, rejects empty `record.id` (400) and non-POST (405); `POST /heartbeats` refreshes `last_seen_at`; `GET /membership` returns `self` and `peers`.

**Age-out** — a peer silent past `heartbeat_timeout` is removed with reason `heartbeat_timeout` and disappears from membership; a timely heartbeat keeps it.

**Peer cap** — `Upsert` silently drops a new peer at `MaxPeers`; the local instance's own ID is ignored with no event.

**Follower client** — introduces immediately on `Start`; only heartbeats after a successful introduction; upserts returned membership and saves a snapshot; warns on failed introduction ("federation introduction failed") and heartbeat ("federation heartbeat failed").

**Registry snapshot persistence** — written atomically via temp-file rename; contains `self`, `introducer_id`, `peers`, `version`, `generated_at`; version increments on each Upsert/Remove.

**Registry events** — Upsert fires `added`/`updated`; Remove fires `removed` via `OnChange`.

**Poller fan-out** — fetches `/status` from each downstream; namespaces target IDs as `[endpoint] id`; reports a `federation-error` unhealthy target for unreachable endpoints; invokes `updateFn` with all aggregated targets; `SortEndpoints` orders by port ascending.

**Routing constraints** — a downstream that is also a peer introduces no cycle and every message carries a trace; a downstream referencing an ancestor is rejected with a cycle error.

**Full lifecycle (integration)** — three instances form introducer + two followers, all visible in membership; stopping a follower removes it within 5 s while the other remains.

## Cross-references

- [Federated Skill Identity and Discovery](federated-skills.md) — skill inventory over federation
- [Self-Description Endpoint](selfdescription.md) — `capabilities.federation`
- [Known-State Regression Detection](known-state.md) — status consumed by the poller
- [Message Routing](routing.md) — fan-out and cycle rules
- [Federation & Isotopes subsystem](../subsystems/federation-isotopes.md)