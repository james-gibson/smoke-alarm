---
title: LLMs.txt Auto-Discovery
description: Fetch remote llms.txt documents and auto-register MCP/ACP endpoints (and OAuth config) as probe targets.
tags:
  - smoke-alarm
  - feature
  - config-discovery
  - discovery
  - llms-txt
source: smoke-alarm/features/discovery-llmstxt.feature
subsystem: config-discovery
gherkin_tags:
  - "@discovery-llmstxt"
  - "@core"
scenario_count: 10
---
> **Source:** `features/discovery-llmstxt.feature` · **Subsystem:** [Config & Discovery](../subsystems/config-discovery.md) · **Index:** [Feature catalog](../index.md)

`ocd-smoke-alarm` fetches registered authoritative `llms.txt` documents and automatically registers discovered MCP/ACP endpoints as probe targets, optionally with OAuth scaffolding — so new services are monitored without manual config edits.

## Scenario map

**fetch behavior** — each configured `llms.txt` URI is fetched exactly once per interval over HTTPS; a non-HTTPS URI is skipped with a `require_https` warning; a slow URI is abandoned at `fetch_timeout` with a "fetch timeout" warning.

**target auto-registration** — a discovered MCP endpoint becomes a probe target with protocol `mcp`; repeat discovery never duplicates an endpoint; nothing is registered when `auto_register_as_targets` is false.

**oauth auto-registration** — an endpoint declaring OAuth metadata gets an OAuth config when `auto_register_oauth` is true; skipped otherwise.

**interval** — discovery re-runs at the configured interval (e.g. 30s) with no busy polling between runs.

**self-health** — a `self-health` target is probed independently of discovery and classifies `HEALTHY` against `http://localhost:18088/healthz`.

## Cross-references

- [mDNS Peer Discovery](mdns-discovery.md) — the local-network discovery complement
- [OAuth Mock Redirect Endpoint](oauth-mock.md) — OAuth registration flow
- [Config Validation](config-validation.md) — schema
- [Targets](targets.md) — the registry these entries land in