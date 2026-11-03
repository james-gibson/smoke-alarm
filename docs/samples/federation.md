---
title: Federation & Topology Samples
description: Sample configs for mesh, chain, star-hub, dependency-graph, and worker-pool federation topologies.
tags:
  - smoke-alarm
  - configs
  - samples
  - federation
  - topology
source:
  - smoke-alarm/configs/samples/federation-mesh.yaml
  - smoke-alarm/configs/samples/federation-chain.yaml
  - smoke-alarm/configs/samples/federation-star-hub.yaml
  - smoke-alarm/configs/samples/federation-dependency.yaml
  - smoke-alarm/configs/samples/federation-pool.yaml
sample_count: 5
---
# Federation & Topology Samples

Five configs that model different multi-agent **topologies** and enable the `federation` block so instances discover and heartbeat one another. The canonical reference is `federation-mesh.yaml`; the feature itself is documented in [Federation Mesh](../features/catalog/federation.md) and [Routing & Traces](../features/catalog/routing.md).

| Config | Topology | Targets | Federation port range |
|---|---|---:|---|
| `federation-mesh.yaml` | self-electing mesh | 0 | 5100–5103 |
| `federation-chain.yaml` | linear chain | 4 | 5300–5304 |
| `federation-star-hub.yaml` | hub + spokes | 4 | 5200–5204 |
| `federation-dependency.yaml` | dependency graph | 8 | 5500–5520 |
| `federation-pool.yaml` | identical pools | 5 | 5400–5405 |

## Slot claiming (shared mechanics)

Each config sets `federation.base_port` / `max_port`. The **first process to bind `base_port` becomes the introducer**; later processes claim `base_port+1`, `base_port+2`, … as followers, then announce to the introducer. The introducer serves `/introductions`, `/heartbeats`, and `/membership`. The cadence knobs are `poll_interval` (status polling), `announce_interval` (re-announce), `heartbeat_interval`, and `heartbeat_timeout` (age-out window). `upstream`/`downstream` can pin an explicit topology; `rank` is `0` to auto-detect from the claimed port.

## federation-mesh.yaml

The canonical mesh: `targets: []` — **the federation itself is the subject**, not any downstream service. Three processes run the same file and self-assign roles on ports 5100–5103. The introducer's membership view is readable at `http://localhost:5100/membership`.

```sh
# run in three terminals, then read the membership view
go run ./cmd/ocd-smoke-alarm serve --mode=foreground --config=configs/samples/federation-mesh.yaml
```

## federation-chain.yaml

A four-link chain — frontend → router → backend → persistence (ports 18501–18504) — all strict MCP targets. It demonstrates cascading failure: if a middle link breaks, downstream links may still answer but the chain is severed. Health binds `0.0.0.0:18088`.

## federation-star-hub.yaml

One coordinator (`hub-coordinator`, 18401) fans out to three specialists — files, code search, and docs (18402–18404). The "incident commander" pattern: the hub verifies each spoke and its own routing.

## federation-dependency.yaml

A non-linear microservice graph: `api-gateway` sits above `auth-service`, `user-service` (→ db-primary, cache-redis), and `payment-service` (→ db-replica, stripe-api). Eight targets on 187xx ports show which failures cascade to dependents.

## federation-pool.yaml

Two identical pools: three code-review workers (18601–18603, quorum 2) and two search workers (18611–18612). The intent is to alarm on **any** worker failure or on the pool dropping below quorum, rather than tracking workers individually.

## Note

Federation is not exclusive to this theme: `deployment-ha.yaml`, `escalation-chain.yaml`, `incident-session-ephemeral.yaml`, and `disaster-recovery-drill.yaml` also enable it with their own port ranges (see [Deployment & operations](deployment-ops.md) and [Warroom & incidents](warroom.md)).

## Cross-references

- [Federation Mesh](../features/catalog/federation.md), [Routing & Traces](../features/catalog/routing.md), [Known-Good Baseline](../features/catalog/known-state.md)
- [Sample Config Catalog](../../configs/samples/README.md)
