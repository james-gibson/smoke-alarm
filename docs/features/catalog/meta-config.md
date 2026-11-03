---
title: Meta-Config Template Generation
description: Generator producing partial-but-valid mcp-add configs with placeholder substitution, confidence, and provenance.
tags:
  - smoke-alarm
  - feature
  - config-discovery
  - artifacts
  - mcp-add
source: smoke-alarm/features/meta-config.feature
subsystem: config-discovery
gherkin_tags:
  - "@meta-config"
  - "@optional"
scenario_count: 15
---
> **Source:** `features/meta-config.feature` · **Subsystem:** [Config & Discovery](../subsystems/config-discovery.md) · **Index:** [Feature catalog](../index.md) · **Status:** `@optional`

The meta-config generator (`internal/meta/generator.go`) produces **partial-but-valid configs** from discovered targets, suitable for copy-paste into `mcp-add` with placeholder substitution. Distinct from [Dynamic Config](dynamic-config.md): `meta_config` is `mcp-add` templates; `dynamic_config` is probe artifacts.

## Defaults

| Setting | Default |
|---|---|
| `output_dir` | `./state/meta-config` |
| `formats` | `["yaml", "json"]` |
| `placeholders.token` | `${TOKEN}` |

## Scenario map

**output** — YAML output contains `version`, `generated_at`, and an `entries` list; JSON output is valid JSON; every sample config with `meta_config.enabled` writes at least one file; nothing is written when disabled.

**placeholder substitution** — a bearer `secret_ref` becomes `${TOKEN}`; an OAuth `client_secret` becomes `${CLIENT_SECRET}`; when `include_provenance` is true the original endpoint value is retained and a provenance note records its source.

**confidence & provenance** — each entry includes a `confidence` in [0.0, 1.0] when `include_confidence` is true, and a `provenance` identifying the source config when `include_provenance` is true; both fields are absent when their flags are false.

**document structure** — the document has `version`, RFC3339 `generated_at`, `source`, and a non-empty `entries` list; each entry has `id` and `name`, and IDs are unique.

## Cross-references

- [Dynamic Config Persistence](dynamic-config.md) — the distinct artifact subsystem
- [Config Validation](config-validation.md) — `meta_config` schema fields
- [LLMs.txt Auto-Discovery](discovery-llmstxt.md) — a source of discovered entries