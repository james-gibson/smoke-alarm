---
title: MCP/ACP Priority Routing and Self-Healing
description: "The internal/routing priority table: admission and eviction, independent MCP/ACP paths, failover and recovery, isotope transit logging, 42i boundary authorization, and chaos isotope registration."
tags:
  - smoke-alarm
  - feature
  - ops-safety
  - routing
  - self-healing
  - isotope
  - gherkin
source: smoke-alarm/features/routing.feature
subsystem: ops-safety
gherkin_tags:
  - "@routing"
  - "@core"
scenario_count: 32
---
> **Source:** `features/routing.feature` · **Subsystem:** [Operations & Safety](../subsystems/ops-safety.md) · **Index:** [Feature catalog](../index.md)

The routing layer (`internal/routing/table.go`) routes MCP and ACP traffic through priority-ordered healthy paths, shifting automatically when instances fail and reclaiming when they recover. Invariants: admission requires a passing health check (state != OUTAGE); eviction is immediate on health-check failure or mDNS deregistration; MCP and ACP maintain independent priority lists; an OUTAGE instance is never in the active paths; an empty table holds traffic and fires an alert — never silently routes to dead instances; a failure carrying a pre-registered chaos isotope within its window is not a real failure (the isotope is the proof).

## Scenario map

**Admission** — a discovered peer enters the active paths only after its first health check returns HEALTHY; a first OUTAGE keeps it out (state "outage"); active paths never contain an OUTAGE instance.

**MCP routing** — routes to the highest-priority healthy instance; fails over to the next priority on OUTAGE, continuing down the list; holds and fires "no healthy MCP paths" when all are OUTAGE.

**ACP routing** — routes independently of MCP; an ACP-only OUTAGE fails ACP over while MCP still routes to the same instance.

**Shared priority fallback** — a TXT `priority=2` with no traffic-specific field sets both MCP and ACP priority to 2; two instances at the same priority distribute load.

**Self-healing: failure** — traffic shifts within one probe interval on a health failure; immediate eviction on mDNS deregistration without waiting for a probe timeout.

**Self-healing: recovery** — a recovered primary re-announces, passes its health check, and reclaims its original priority slot (secondary returns to standby); a re-announcing instance is not admitted until its health check completes.

**Isotope transit logging** — every routing decision emits a transit event (isotope at point "routing", result "routed", no payload); a transit maps back to its declared scenario and marks coverage; transit events over a window form a coverage report; a "held" decision still emits a transit event.

**42i boundary authorization** — transiting the declared boundary clears scope-compliance; an undeclared boundary → scope-compliance failure (+20); a replayed isotope → isotope-variation failure (+8); payload in a transit event → secret-flow-violation (+24); uncovered scenarios are the 42i gap map.

**Isotope ID construction properties** — same feature+payload with different nonces differ and are 43 base64url chars; verification fails on a different feature or payload and succeeds with feature_id+payload+nonce; the payload is not recoverable from the ID.

**Chaos isotope registration** — a chaos test registers its isotope and window before running (auto-closes); a failure carrying a registered, in-window chaos isotope does not evict the instance; an unregistered isotope evicts; a registered isotope arriving after its window closes is a real failure ("chaos isotope arrived outside declared window") and evicts; an expiring registration with no failure clears cleanly; simultaneous tests on different instances do not interfere; a non-chaos instance's failure is real even while another is in a chaos window.

**Routing table invariants** — the table is rebuilt from mDNS state on restart with no stale entries.

## Cross-references

- [mDNS Discovery](mdns-discovery.md) — peer discovery that populates the table
- [Federation Mesh](federation.md) — fan-out topology above the routing layer
- [Chaos Window](chaos-window.md) — window registration and classification
- [Operations & Safety subsystem](../subsystems/ops-safety.md)