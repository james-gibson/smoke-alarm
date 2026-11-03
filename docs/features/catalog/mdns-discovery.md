---
title: mDNS Peer Discovery
description: Continuous mDNS browser that maintains the cluster routing table, honouring TXT priorities and peer departure/TTL expiry.
tags:
  - smoke-alarm
  - feature
  - config-discovery
  - mdns
  - discovery
source: smoke-alarm/features/mdns-discovery.feature
subsystem: config-discovery
gherkin_tags:
  - "@mdns"
  - "@core"
scenario_count: 11
---
> **Source:** `features/mdns-discovery.feature` · **Subsystem:** [Config & Discovery](../subsystems/config-discovery.md) · **Index:** [Feature catalog](../index.md)

The mDNS browser (`internal/mdns/browser.go`) continuously browses `_smoke-alarm._tcp` for peer instances and keeps the cluster routing table current without manual configuration. This is the inbound complement to [mDNS Service Advertisement](mdns.md).

## Scenario map

**initial browse** — a peer already advertising at startup is added to the routing table with health `unknown`; `priority` is read from the TXT record (default `99` when absent); independent `mcp-priority` and `acp-priority` are honoured; multiple peers are ordered by priority ascending.

**continuous browse** — a peer coming online after startup is added without restarting the browser; the browse loop stays open for the process lifetime (still discovers a peer after 60s); the loop does not close after the first result batch.

**peer departure** — a peer that deregisters its mDNS record is removed immediately; a peer whose TTL expires without renewal transitions to `unreachable` and is removed from active routing paths.

**cancellation** — cancelling the context exits the browse loop cleanly; no peers are added afterward.

## Cross-references

- [mDNS Service Advertisement](mdns.md) — outbound advertisement
- [Routing](routing.md) — routing table populated from discovered peers