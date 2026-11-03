---
title: Dynamic Config Persistence
description: dynamic-config persist subcommand generating uniquely-identified JSON/Markdown probe artifacts with overwrite and uniqueness controls.
tags:
  - smoke-alarm
  - feature
  - config-discovery
  - artifacts
source: smoke-alarm/features/dynamic-config.feature
subsystem: config-discovery
gherkin_tags:
  - "@dynamic-config"
  - "@optional"
scenario_count: 11
---
> **Source:** `features/dynamic-config.feature` · **Subsystem:** [Config & Discovery](../subsystems/config-discovery.md) · **Index:** [Feature catalog](../index.md) · **Status:** `@optional`

The `dynamic-config persist` subcommand generates **uniquely-identified config artifacts** (JSON and Markdown) from discovered/validated configurations so they can be statically served or consumed by downstream tools. Not to be confused with [Meta-Config](meta-config.md): `dynamic_config` is probe-result artifacts; `meta_config` is `mcp-add` templates.

## Scenario map

**persist** — generates JSON *and* Markdown artifacts for the llmstxt config, and a JSON artifact for every sample config, under the config's `dynamic_config.directory`; creates the output directory if missing.

**uniqueness** — all JSON artifacts in an output directory have distinct `id` fields; with `require_unique_ids: true` a would-be duplicate ID fails with "duplicate id".

**overwrite** — `allow_overwrite: true` replaces an existing artifact with updated content; `allow_overwrite: false` fails with "already exists".

**content** — artifacts include `confidence` and `provenance` when `meta_config.include_confidence`/`include_provenance` are true; secrets are placeholdered (no raw keychain secrets; `${TOKEN}` and `${CLIENT_SECRET}` patterns); Markdown artifacts contain at least one target ID as a heading and are valid CommonMark.

**formats** — the `dynamic_config.formats` list (`json`, `markdown`) controls which artifacts are written.

## Cross-references

- [Meta-Config Template Generation](meta-config.md) — the distinct template subsystem
- [Config Validation](config-validation.md) — schema
- [Hosted Server](hosted-server.md) — `serve_base_url`