---
title: Discovery & Generated Configs
description: The llms.txt auto-discovery sample and a generated mcp-add meta-config fragment.
tags:
  - smoke-alarm
  - configs
  - samples
  - discovery
  - meta-config
source:
  - smoke-alarm/configs/samples/llmstxt-auto-discovery.yaml
  - smoke-alarm/configs/samples/mcp-add-meta-20260408-045454.yaml
sample_count: 2
---
# Discovery & Generated Configs

Two configs covering how the smoke alarm **finds** services and how it **emits** config: remote llms.txt discovery that auto-registers targets, and a generated `mcp-add` meta-config fragment. See [llms.txt Discovery](../features/catalog/discovery-llmstxt.md), [Meta-Config Generation](../features/catalog/meta-config.md), and [Dynamic Config Artifacts](../features/catalog/dynamic-config.md).

| Config | Kind | Notes |
|---|---|---|
| `llmstxt-auto-discovery.yaml` | hand-written runtime config | discovery on, auto-registration on |
| `mcp-add-meta-20260408-045454.yaml` | generated fragment | `source: discovery`, timestamped |

## llmstxt-auto-discovery.yaml

Discovery is the point: `discovery.llms_txt` lists three authoritative URIs (Model Context Protocol, Agent Client Protocol, and llmstxt.org) with `require_https: true` and both `auto_register_as_targets` and `auto_register_oauth` enabled, so discovered services become live targets. It also runs a self-health control target, a disabled ACP OAuth *reference* target that documents the full auth field shape, an OAuth mock in `allow` mode, hosted MCP/ACP/A2A on 18091, and `dynamic_config` persistence.

```sh
go run ./cmd/ocd-smoke-alarm discover --config=configs/samples/llmstxt-auto-discovery.yaml --json=false
```

## mcp-add-meta-20260408-045454.yaml

A **generated** artifact, not a runtime profile: its filename is timestamped, it carries `generated_at` and `source: discovery`, and its top-level shape is `entries[]` rather than `targets[]`. The single entry is an npx filesystem stdio target labelled `generated_by: ocd-smoke-alarm` with `intent: mcp-add-partial`, plus `confidence` and `provenance`, and a `notes:` list warning that entries are partial until placeholders are replaced. It is the output shape produced by [Meta-Config Generation](../features/catalog/meta-config.md).

## Cross-references

- [llms.txt Discovery](../features/catalog/discovery-llmstxt.md), [Meta-Config Generation](../features/catalog/meta-config.md), [Dynamic Config Artifacts](../features/catalog/dynamic-config.md)
- [Sample Config Catalog](../../configs/samples/README.md)
