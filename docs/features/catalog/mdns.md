---
title: mDNS Service Advertisement
description: Advertiser publishing _smoke-alarm._tcp over mDNS, with ParsePort/ServiceID helpers, TXT records, and bounded port registration.
tags:
  - smoke-alarm
  - feature
  - config-discovery
  - mdns
source: smoke-alarm/features/mdns.feature
subsystem: config-discovery
gherkin_tags:
  - "@mdns"
  - "@core"
scenario_count: 13
---
> **Source:** `features/mdns.feature` · **Subsystem:** [Config & Discovery](../subsystems/config-discovery.md) · **Index:** [Feature catalog](../index.md)

The `Advertiser` (`internal/mdns/advertiser.go`) publishes the instance over mDNS (via `grandcat/zeroconf`) as `_smoke-alarm._tcp`, so Tuner instances can discover it passively. This is the outbound complement to [mDNS Peer Discovery](mdns-discovery.md).

## Scenario map

**defaults & helpers** — the domain defaults to `local`; `ServiceID` formats `_smoke-alarm._tcp.local:9090`; `ParsePort` extracts the port from `host:port` and returns 0 for no-port or empty input.

**Start** — registers the service on the configured port and type; includes TXT records (e.g. `version=1.0`); returns a non-nil "mdns: register" error when zeroconf registration fails.

**Shutdown & cancellation** — Shutdown deregisters and allows a fresh registration on restart; Shutdown before Start is safe (no panic); cancelling the context stops the advertisement.

**config integration** — the advertiser is not started when `tuner.advertise` is false; after `BindWithRetry` the advertiser registers on the **actual** bound port, not the configured one.

## Cross-references

- [mDNS Peer Discovery](mdns-discovery.md) — inbound browse
- [Health and Status Endpoints](health.md) — `BindWithRetry` port hand-off
- [Tuner Integration](tuner-integration.md) — consumer of the advertisement