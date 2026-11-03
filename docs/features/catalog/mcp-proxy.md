---
title: MCP JSON-RPC Proxying Across Federation Mesh
description: "Optional capability: proxy JSON-RPC calls for federated skills to the hosting instance, with routing-trace cycle rejection."
tags:
  - smoke-alarm
  - feature
  - transport
  - federation
  - proxy
source: smoke-alarm/features/mcp-proxy.feature
subsystem: transport
gherkin_tags:
  - "@mcp-proxy"
  - "@optional"
scenario_count: 14
---
> **Source:** `features/mcp-proxy.feature` · **Subsystem:** [Transport & Protocol](../subsystems/transport.md) · **Index:** [Feature catalog](../index.md) · **Status:** `@optional` next-steps record (step definitions pending)

The smoke-alarm can act as an **MCP proxy**: a JSON-RPC call addressed to a federated skill (`[instance-id] skill-name`) is forwarded to the instance that hosts it, and the response is proxied back transparently. This lets an agent or test harness reach any skill in the mesh through a single endpoint without knowing the topology. Routing traces (from [Federated Skills](federated-skills.md)) carry through the proxy layer, and cycles are rejected.

## Actors & surfaces

| Actor | Interaction |
|---|---|
| AI agent / harness (e.g. ADHD) | Connects to one instance; invokes skills mesh-wide |
| Introducer / gateway instance | Hosts the MCP endpoint, proxies calls |
| Peer instances | Host skills, receive forwarded calls |

## Scenario map

**Transparent proxying** — a call for a local skill is handled directly (no outbound request); a call for `[bbbb000000000002] start-here` is POSTed to the hosting peer and its response returned unchanged; the original JSON-RPC `id` is preserved; an `X-Routing-Trace` header carrying the caller's instance ID is added; a trace that already contains the local instance ID is rejected with `-32600` "routing cycle detected".

**Multi-hop** — a 3-instance linear mesh (`inst-a → inst-b → inst-c`) reaches a skill on `inst-c` via two hops with all IDs in the final trace; a directly-known peer is reached without an intermediate hop.

**Error handling** — unreachable peer → `-32001` "peer unreachable" (+ ID); unknown federated skill → `-32601`; JSON-RPC errors from the target are propagated to the caller unchanged.

**tools/list namespacing** — the response merges local skills and federated `[instance-id] skill` methods, so one `tools/list` call discovers the whole mesh.

**Demo cluster** — ADHD connected to `alarm-b` reaches `adhd.chaos.register-window` on `alarm-a` by proxy, including when `alarm-a` joins later, with no ADHD reconfiguration.

## Cross-references

- [Federated Skill Identity and Discovery](federated-skills.md) — routing infrastructure
- [Federation Mesh](federation.md) — slot election, `InstanceRecord`, membership
- [Routing](routing.md) — priority routing and self-healing