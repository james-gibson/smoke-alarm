---
title: Chaos Skills as Detection Probes
description: "Delivering pre-computed chaos skills to underverified agents and detecting misbehaviour: wrong source, tampering, suppression, replay, and blind forwarding."
tags:
  - smoke-alarm
  - feature
  - skills-certification
  - chaos
  - detection
  - isotope
  - gherkin
source: smoke-alarm/features/chaos-skill-detection.feature
subsystem: skills-certification
gherkin_tags:
  - "@chaos-detection"
  - "@isotope"
  - "@optional"
scenario_count: 16
---
> **Source:** `features/chaos-skill-detection.feature` · **Subsystem:** [Skills & Certification](../subsystems/skills-certification.md) · **Index:** [Feature catalog](../index.md)

A chaos skill is delivered to an agent that is untrusted (rung 0) or underverified (rung 1-2). Because the receipt is pre-computable from (`feature_id`, payload, nonce), the certifying smoke-alarm knows in advance what isotope should arrive, from which target, via which route. Any deviation is a detection signal: wrong source (proxy forwarded to an untrusted actor), wrong payload (tampering), unexpected route (trace stripped/forged), no receipt (suppression), or a duplicated receipt (replay). The chaos skill here is not a failure simulation — it is a probe, and the controlled uncertainty reveals how untrusted agents handle deliveries they were not expected to forward.

## Scenario map

**Pre-computing the expected receipt** — constructed before delivery as `base64url(SHA256(feature_id || ":" || SHA256(payload) || ":" || nonce))`; distinct nonces make each expected receipt unique and non-replayable.

**Delivery to an underverified agent** — delivered to a rung-0 proxy as a normal JSON-RPC invocation the proxy cannot distinguish from a legitimate call (routing trace populated normally, no special marker).

**Receipt arrives as expected** — correct forwarding yields the expected receipt from the expected target, the trace includes the proxy, no detection fires, and the proxy's rung may advance.

**Detection: wrong source** — a receipt from an unexpected target raises "receipt from unexpected source"; the proxy's rung decreases; the event logs expected_target/actual_target/isotope_id/routing_trace; a trace omitting the proxy indicates stripping ("routing trace stripped by proxy", +8 distance).

**Detection: tampered payload** — a mismatched payload with the correct `feature_id` raises "receipt payload mismatch", recorded as a skill-variation failure (+8); a wrong `feature_id` raises "unexpected feature_id" and is held unverified.

**Detection: suppressed receipt** — no receipt before the deadline raises "chaos probe receipt suppressed" (+8, possible demotion); a late receipt is recorded but flagged "receipt arrived outside detection window" (possible buffering/replay).

**Detection: replay** — the same receipt twice raises "chaos probe replay detected", is not recorded as a new transit, and the delivering agent's distance rises by 8.

**The untrusted actor pattern** — a blind forwarder is revealed by a receipt from an unexpected source and has its certification revoked; a selectively-suppressing proxy is revealed by receipt gaps between a proxied probe and a direct probe ("selective suppression detected").

**No chaos window for detection probes** — detection is a mesh health check, not a planned failure; no window is registered, and a red light from the probe is itself a signal. A clean probe (receipt matches expected source/route) raises no red light and is recorded as a clean certification event.

## Cross-references

- [Skill Execution Receipts](skill-execution-receipts.md) — isotope = receipt of execution
- [42i Trust Rungs](trust-rungs.md) — rung gating of agents
- [MCP Proxy](mcp-proxy.md) — routing trace through the mesh
- [Certification Gate](certification-gate.md) — uncertified agents in the registry
- [Skills & Certification subsystem](../subsystems/skills-certification.md)