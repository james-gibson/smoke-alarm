---
title: Config & Discovery
description: "Config validation and target discovery: dynamic/meta config artifacts, llms.txt and mDNS discovery, and the target field contract."
tags:
  - smoke-alarm
  - subsystem
  - config-discovery
feature_count: 7
---
> Part of the [Feature Knowledge Base](../index.md). See the [consolidated catalog](../consolidated.md) for a flat view.

# Config & Discovery

Features covering configuration handling and target discovery: schema validation, two distinct artifact generators (`dynamic_config` vs `meta_config`), remote `llms.txt` auto-discovery, mDNS advertisement/browse, and the target definition/validation contract.

## Features

| Feature | Source | Notes |
|---|---|---|
| [Config Validation](../catalog/config-validation.md) | `features/config-validation.feature` | `validate` / `check` |
| [Dynamic Config Persistence](../catalog/dynamic-config.md) | `features/dynamic-config.feature` | `@optional` |
| [Meta-Config Template Generation](../catalog/meta-config.md) | `features/meta-config.feature` | `@optional` |
| [LLMs.txt Auto-Discovery](../catalog/discovery-llmstxt.md) | `features/discovery-llmstxt.feature` | remote discovery |
| [mDNS Service Advertisement](../catalog/mdns.md) | `features/mdns.feature` | outbound |
| [mDNS Peer Discovery](../catalog/mdns-discovery.md) | `features/mdns-discovery.feature` | inbound |
| [Target Definition and Validation](../catalog/targets.md) | `features/targets.feature` | field contract |

## Cross-cutting notes

- **Two artifact subsystems, do not conflate:** `dynamic_config` writes uniquely-identified probe artifacts for static serving; `meta_config` writes `mcp-add` copy-paste templates with placeholder substitution. Both appear in every sample config.
- **Secrets are never emitted raw** — both generators substitute `${TOKEN}` / `${CLIENT_SECRET}`.
- **Discovery has two independent paths:** remote `llms.txt` fetch (network) and local mDNS browse; both feed the target registry / routing table.
- **[`BindWithRetry`](../catalog/health.md)** hands the *actual* bound port to the mDNS advertiser, which is why advertisement and health share the port story.