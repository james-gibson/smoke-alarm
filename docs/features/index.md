---
title: Feature Knowledge Base
description: Navigation hub for the breakdown of the 45 smoke-alarm Gherkin feature files into catalog docs and subsystem rollups.
tags:
  - smoke-alarm
  - features
  - gherkin
  - knowledge-base
---
# Feature Knowledge Base

This knowledge base breaks down the 45 Gherkin `.feature` documents under `smoke-alarm/features/` into browsable markdown. Every feature gets a catalog doc, related features are grouped into subsystem rollups, and the flat view lives in the consolidated catalog.

> **Source of truth:** the `.feature` files are authoritative. These docs are a derived breakdown — when a feature changes, re-sync its catalog doc.

## Subsystems

| Subsystem | Features | Scope |
|---|---:|---|
| [Health & Status](subsystems/health-status.md) | 3 | Observability surfaces: health/readiness/status endpoints, the self-description document, and the OpenCode status-report skill. |
| [Transport & Protocol](subsystems/transport.md) | 4 | How the smoke alarm reaches MCP/ACP services: stdio, SSE, embedded hosting, and federation proxying. |
| [Config & Discovery](subsystems/config-discovery.md) | 7 | Config validation and target discovery: dynamic/meta config artifacts, llms.txt and mDNS discovery, and the target field contract. |
| [Engine & Simulation](subsystems/engine-simulation.md) | 5 | The probe engine, known-state baseline, simulation authoring/validation skills, and optional Tuner integration. |
| [Federation & Isotopes](subsystems/federation-isotopes.md) | 7 | The federation mesh, federated skill identity, canonical isotope construction and transit, dev-mode plaintext isotopes, 42i trust rungs, and honeypot detection. |
| [Skills & Certification](subsystems/skills-certification.md) | 7 | Skill validation, dynamic 42i skill certification, execution receipts, chaos detection probes, the isotope certification gate, and the authoring/demo skills. |
| [Operations & Safety](subsystems/ops-safety.md) | 7 | Runtime lifecycle and update orchestration, alert routing, pre-protocol HURL safety, chaos windows, telemetry, MCP/ACP priority routing, and the OAuth mock. |
| [Wargame & Onboarding](subsystems/wargame-onboarding.md) | 5 | Incident-simulation authoring and play-through skills, the first-run welcome check, and the Bubble Tea operator dashboard. |

## How to navigate

- **One feature?** Open its doc from the [consolidated catalog](consolidated.md), which links every feature.
- **A theme?** Start from a subsystem rollup above.
- **Everything at once?** The [consolidated catalog](consolidated.md) is the flat, one-section-per-feature view.
- **How a request moves?** [Client Request Flow](../request-flow.md) — the four ways a request meets the alarm — then [Request-Pathing Blast Radius](../pathing-blast-radius.md) on how each pathing style fails and why they mesh.

## Provenance

Each catalog doc carries `source:` (the `.feature` it derives from), `subsystem`, `gherkin_tags`, and `scenario_count` in its frontmatter. Subsystem rollups carry `feature_count`. Feature text is extracted from the `.feature` scenarios; no external sources are involved.
