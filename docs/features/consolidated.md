---
title: Consolidated Feature Catalog
description: Flat, one-section-per-feature view of all 45 smoke-alarm Gherkin features, grouped by subsystem and linked to their catalog docs.
tags:
  - smoke-alarm
  - features
  - gherkin
  - consolidated
---
# Consolidated Feature Catalog

A flat, one-section-per-feature view of all 45 smoke-alarm Gherkin features. For thematic grouping see the [subsystem rollups](../../../../smoke-alarm/docs/features/subsystems); for orientation see the [knowledge base index](index.md).

## Health & Status

_Observability surfaces: health/readiness/status endpoints, the self-description document, and the OpenCode status-report skill._

### [Health and Status Endpoints](catalog/health.md)

Deterministic /healthz, /readyz, /status endpoints plus federation report intake, BindWithRetry, and shutdown semantics.

`features/health.feature` · [subsystem](subsystems/health-status.md)

### [Self-Description Endpoint](catalog/selfdescription.md)

Machine-readable /.well-known/smoke-alarm.json identity document generated from the running config and validated against JSON schema.

`features/selfdescription.feature` · [subsystem](subsystems/health-status.md)

### [OpenCode Status Report Skill](catalog/opencode-status-report.md)

Skill that audits agent/config files (AGENTS.md, .opencode.json, skills, sample configs) into a validation report.

`features/opencode-status-report.feature` · [subsystem](subsystems/health-status.md)

## Transport & Protocol

_How the smoke alarm reaches MCP/ACP services: stdio, SSE, embedded hosting, and federation proxying._

### [Stdio MCP Probe](catalog/stdio-mcp.md)

Probe that launches npx/uvx/binaries as child processes and validates the strict MCP handshake over stdin/stdout.

`features/stdio-mcp.feature` · [subsystem](subsystems/transport.md)

### [SSE Transport Probe](catalog/sse-transport.md)

Probe for remote MCP/ACP Server-Sent Events streams with bearer auth, handshake profiles, HURL preflight, and remote_agent updates.

`features/sse-transport.feature` · [subsystem](subsystems/transport.md)

### [MCP JSON-RPC Proxying Across Federation Mesh](catalog/mcp-proxy.md)

Optional capability: proxy JSON-RPC calls for federated skills to the hosting instance, with routing-trace cycle rejection.

`features/mcp-proxy.feature` · [subsystem](subsystems/transport.md)

### [Embedded Hosted MCP/ACP Server](catalog/hosted-server.md)

Embedded MCP/ACP HTTP+SSE server on localhost:18091 with self-probing hosted targets and HURL-gated strict handshakes.

`features/hosted-server.feature` · [subsystem](subsystems/transport.md)

## Config & Discovery

_Config validation and target discovery: dynamic/meta config artifacts, llms.txt and mDNS discovery, and the target field contract._

### [Config Validation](catalog/config-validation.md)

validate/check subcommands, schema-version and required-field checks, and target transport/protocol vocabulary enforcement.

`features/config-validation.feature` · [subsystem](subsystems/config-discovery.md)

### [Dynamic Config Persistence](catalog/dynamic-config.md)

dynamic-config persist subcommand generating uniquely-identified JSON/Markdown probe artifacts with overwrite and uniqueness controls.

`features/dynamic-config.feature` · [subsystem](subsystems/config-discovery.md)

### [Meta-Config Template Generation](catalog/meta-config.md)

Generator producing partial-but-valid mcp-add configs with placeholder substitution, confidence, and provenance.

`features/meta-config.feature` · [subsystem](subsystems/config-discovery.md)

### [LLMs.txt Auto-Discovery](catalog/discovery-llmstxt.md)

Fetch remote llms.txt documents and auto-register MCP/ACP endpoints (and OAuth config) as probe targets.

`features/discovery-llmstxt.feature` · [subsystem](subsystems/config-discovery.md)

### [mDNS Service Advertisement](catalog/mdns.md)

Advertiser publishing _smoke-alarm._tcp over mDNS, with ParsePort/ServiceID helpers, TXT records, and bounded port registration.

`features/mdns.feature` · [subsystem](subsystems/config-discovery.md)

### [mDNS Peer Discovery](catalog/mdns-discovery.md)

Continuous mDNS browser that maintains the cluster routing table, honouring TXT priorities and peer departure/TTL expiry.

`features/mdns-discovery.feature` · [subsystem](subsystems/config-discovery.md)

### [Target Definition and Validation](catalog/targets.md)

Full target field contract: required fields, endpoint rules, check policy, handshake, HURL, auth rules, and CheckResult semantics.

`features/targets.feature` · [subsystem](subsystems/config-discovery.md)

## Engine & Simulation

_The probe engine, known-state baseline, simulation authoring/validation skills, and optional Tuner integration._

### [Probe Engine](catalog/engine.md)

The continuous probe scheduler: immediate first check, interval probing, worker bounds, state classification, alert emission, ring buffer, snapshots, and stdio transport probing.

`features/engine.feature` · [subsystem](subsystems/engine-simulation.md)

### [Config to Simulation Skill](catalog/config-to-simulation.md)

The config-to-simulation skill: parses a YAML config into participants, derives escalation and topology, and renders warroom/mentor voices.

`features/config-to-simulation.feature` · [subsystem](subsystems/engine-simulation.md)

### [Simulation Validator](catalog/simulation-validator.md)

The simulation-validator skill: config extraction, feature mapping, secure-context verification, bidirectional gap analysis, TUI guidance, and reports.

`features/simulation-validator.feature` · [subsystem](subsystems/engine-simulation.md)

### [Known-State Regression Detection](catalog/known-state.md)

The knownstate store: sustain-success gate, regression classification, counters, persistence/reset, isotope transit, 42i boundary authorization, and the chaos isotope guard.

`features/known-state.feature` · [subsystem](subsystems/engine-simulation.md)

### [Tuner Integration](catalog/tuner-integration.md)

Optional Tuner passive-observer integration: tuner config, mDNS advertisement, audience/caller endpoints with SSE fan-out, MCP tools, CLI, and hosted event log.

`features/tuner-integration.feature` · [subsystem](subsystems/engine-simulation.md)

## Federation & Isotopes

_The federation mesh, federated skill identity, canonical isotope construction and transit, dev-mode plaintext isotopes, 42i trust rungs, and honeypot detection._

### [Federation Mesh](catalog/federation.md)

Slot election, instance identity, introducer endpoints, membership age-out, peer cap, follower client, registry snapshots/events, and downstream status fan-out with cycle detection.

`features/federation.feature` · [subsystem](subsystems/federation-isotopes.md)

### [Federated Skill Identity and Discovery](catalog/federated-skills.md)

Skill inventory announced via InstanceRecord, '[instance-id] skill-name' namespacing, membership skill lists, lookup, routed invocation with traces, peer departure, and self-description fields.

`features/federated-skills.feature` · [subsystem](subsystems/federation-isotopes.md)

### [Canonical Isotope ID Construction](catalog/isotope-construction.md)

The pinned isotope_id algorithm: base64url(SHA256(feature_id || ':' || SHA256(payload) || ':' || nonce)), its properties, verification, encoding, and cross-component agreement.

`features/isotope-construction.feature` · [subsystem](subsystems/federation-isotopes.md)

### [Isotope ID Transit Through Health Check Responses](catalog/isotope-transit.md)

Carrying the received isotope_id on TargetStatus: per-cycle updates, failure recording, the no-payload privacy boundary, verification, and SSE transport.

`features/isotope-transit.feature` · [subsystem](subsystems/federation-isotopes.md)

### [Plain Text Isotope Stand-Ins](catalog/plaintext-isotopes.md)

Dev/demo-mode 'plaintext:<feature-id>:<seq>' isotope stand-ins: format recognition, feature-id extraction, chaos-window compatibility, uniqueness, SSE, and the migration path.

`features/plaintext-isotopes.feature` · [subsystem](subsystems/federation-isotopes.md)

### [42i Trust Rungs](catalog/trust-rungs.md)

Certification rungs 0-6 mapped to 42i distance ceilings: evidence-derived assignment, InstanceRecord declaration, value-read authorization, JSON-RPC filtering, proxy identity, and distance shifts.

`features/trust-rungs.feature` · [subsystem](subsystems/federation-isotopes.md)

### [Honeypot Nodes](catalog/honeypot-nodes.md)

Counterfeit detection for the mesh: silent and canary honeypots, pre-computed canary isotope, detection signals, ADHD presentation, and demo/integration assertions.

`features/honeypot-nodes.feature` · [subsystem](subsystems/federation-isotopes.md)

## Skills & Certification

_Skill validation, dynamic 42i skill certification, execution receipts, chaos detection probes, the isotope certification gate, and the authoring/demo skills._

### [Skill System Validation](catalog/skill-system.md)

internal/skills enforcement of SKILL.md contracts: ValidateSkillFile, FindSkills, ValidateProjectConfig, and GenerateStartHereReport.

`features/skill-system.feature` · [subsystem](subsystems/skills-certification.md)

### [Dynamic Skill Certification](catalog/skill-certification.md)

42i certification of skills by observed evidence: existence, equality, and isotope levels, plus hop-distance penalties and dynamic re-evaluation on heartbeats.

`features/skill-certification.feature` · [subsystem](subsystems/skills-certification.md)

### [Isotopes as Skill Execution Receipts](catalog/skill-execution-receipts.md)

The isotope-as-receipt model: receipt structure, transit through /status, cross-alarm certification, verification, and cumulative evidence.

`features/skill-execution-receipts.feature` · [subsystem](subsystems/skills-certification.md)

### [Chaos Skills as Detection Probes](catalog/chaos-skill-detection.md)

Delivering pre-computed chaos skills to underverified agents and detecting misbehaviour: wrong source, tampering, suppression, replay, and blind forwarding.

`features/chaos-skill-detection.feature` · [subsystem](subsystems/skills-certification.md)

### [Isotope Certification Gate](catalog/certification-gate.md)

Excluding uncertified instances from full membership and routing: isotope_certified claims, acceptance policy, membership visibility, revocation, and scope-compliance failures.

`features/certification-gate.feature` · [subsystem](subsystems/skills-certification.md)

### [Demo Capabilities Skill](catalog/demo-capabilities.md)

The demo-capabilities skill: skill discovery, validation status, related config-file checks, and a stable capabilities report.

`features/demo-capabilities.feature` · [subsystem](subsystems/skills-certification.md)

### [Open the Pickle Jar](catalog/open-the-pickle-jar.md)

The open-the-pickle-jar skill: Gherkin authoring rules, targeted feature writing, drift audit, step-definition implementation audit, output rules, and federated context.

`features/open-the-pickle-jar.feature` · [subsystem](subsystems/skills-certification.md)

## Operations & Safety

_Runtime lifecycle and update orchestration, alert routing, pre-protocol HURL safety, chaos windows, telemetry, MCP/ACP priority routing, and the OAuth mock._

### [Ops — Runtime Lifecycle and Update Orchestration](catalog/ops.md)

Runtime construction, lock file, mode dispatch, bootstrap, health sync, graceful shutdown, and the LifecycleController lock-stop-deploy-restart-verify-commit workflow with rollback and journal.

`features/ops.feature` · [subsystem](subsystems/ops-safety.md)

### [Alert Routing and Severity Policy](catalog/alerts.md)

LogNotifier, DesktopNotifier, dedupe and severity filtering, secret redaction, NotifierGroup fan-out, and config-level per-state severity policy.

`features/alerts.feature` · [subsystem](subsystems/ops-safety.md)

### [Pre-Protocol HURL Safety Scanner](catalog/safety-hurl.md)

The internal/safety Scanner: two HURL modes (endpoint, file), registration validation, failure classification, and report aggregation.

`features/safety-hurl.feature` · [subsystem](subsystems/ops-safety.md)

### [Chaos Window Registration and Isotope Classification](catalog/chaos-window.md)

Registering authorised chaos windows and classifying isotopes as registered-chaos, expired-window, or unregistered, with the smoke-alarm as the authoritative classifier.

`features/chaos-window.feature` · [subsystem](subsystems/ops-safety.md)

### [Telemetry — OTEL Metric Export and System Sampling](catalog/telemetry.md)

The telemetry Exporter: check latency histogram, failure counter, target-state gauge, system gauges, export interval, endpoint resilience, and graceful close.

`features/telemetry.feature` · [subsystem](subsystems/ops-safety.md)

### [MCP/ACP Priority Routing and Self-Healing](catalog/routing.md)

The internal/routing priority table: admission and eviction, independent MCP/ACP paths, failover and recovery, isotope transit logging, 42i boundary authorization, and chaos isotope registration.

`features/routing.feature` · [subsystem](subsystems/ops-safety.md)

### [OAuth Mock Redirect Endpoint](catalog/oauth-mock.md)

The mock OAuth redirect endpoint in allow and fail modes: startup, callback handling, auth-failure classification, HURL preflight, token redaction, and unattended flows.

`features/oauth-mock.feature` · [subsystem](subsystems/ops-safety.md)

## Wargame & Onboarding

_Incident-simulation authoring and play-through skills, the first-run welcome check, and the Bubble Tea operator dashboard._

### [Warroom Builder Skill](catalog/warroom-builder.md)

The warroom-builder skill: incident parameter collection, severity-driven YAML config generation, and incident-script generation with unique join codes.

`features/warroom-builder.feature` · [subsystem](subsystems/wargame-onboarding.md)

### [Warroom Guide Skill](catalog/warroom-guide.md)

The warroom-guide skill as automated incident commander: bridge display, participant joining, priority-ordered alarm dismissal, escalation, and resolution summary.

`features/warroom-guide.feature` · [subsystem](subsystems/wargame-onboarding.md)

### [Warroom Simulator Skill](catalog/warroom-simulator.md)

The warroom-simulator skill: interactive /start /join /resolve /status /pause /resume /abort commands, escalation timers, and post-simulation performance feedback.

`features/warroom-simulator.feature` · [subsystem](subsystems/wargame-onboarding.md)

### [Start-Here Welcome Skill](catalog/start-here.md)

The start-here skill: connection confirmation, AGENTS.md/Makefile and skills-inventory validation, welcome output, and repeat-run stability.

`features/start-here.feature` · [subsystem](subsystems/wargame-onboarding.md)

### [Bubble Tea Dashboard](catalog/tui.md)

The foreground Bubble Tea dashboard's Elm Architecture contract: Init/Update/View purity, keyboard navigation, status/details/events/topology panes, SnapshotProvider, and Run.

`features/tui.feature` · [subsystem](subsystems/wargame-onboarding.md)
