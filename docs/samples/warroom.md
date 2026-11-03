---
title: Warroom & Incident Samples
description: "Sample configs for incident warrooms: A2A handoff, secure context, ephemeral sessions, escalation, observers, and DR drills."
tags:
  - smoke-alarm
  - configs
  - samples
  - warroom
  - incident
source:
  - smoke-alarm/configs/samples/a2a-handoff.yaml
  - smoke-alarm/configs/samples/secure-context-handoff.yaml
  - smoke-alarm/configs/samples/incident-session-ephemeral.yaml
  - smoke-alarm/configs/samples/escalation-chain.yaml
  - smoke-alarm/configs/samples/observer-mode.yaml
  - smoke-alarm/configs/samples/disaster-recovery-drill.yaml
sample_count: 6
---
# Warroom & Incident Samples

Six configs built around an incident **warroom**: who participates, what context they carry, how failures are detected, and how alerts escalate. Several are the target shapes produced by the [Warroom Builder Skill](../features/catalog/warroom-builder.md) and consumed by the [Warroom Guide Skill](../features/catalog/warroom-guide.md).

| Config | Scenario | Participants / targets |
|---|---|---|
| `a2a-handoff.yaml` | agent-to-agent async handoff | planner, executor, reporter, monitor |
| `secure-context-handoff.yaml` | scoped encrypted context | broker, engineering, business, store |
| `incident-session-ephemeral.yaml` | temporary incident bridge | coordinator, engineer, security, ops, store |
| `escalation-chain.yaml` | on-call escalation matrix | L1–L4, pager, incident manager |
| `observer-mode.yaml` | read-only observers | coordinator + 4 observers + audit log |
| `disaster-recovery-drill.yaml` | chaos / DR exercise | region A/B, global config, load balancer |

## a2a-handoff.yaml

Models the Agent-to-Agent **async message-passing** pattern — context plus artifacts carried across handoffs, unlike MCP/ACP request-response. It is expressed as four ACP targets: planner (18801), executor (18802), reporter (18803), and monitor (18804). Alerts are aggressive with immediate regression notification.

## secure-context-handoff.yaml

Shows **secure context injection**: participants submit context that is encrypted in transit, access-scoped, and TTL-bound. Targets are the broker (19801), engineering and business providers (19802–19803), and an encrypted store (19810), each with distinct OAuth scopes; `keystore` and `oauth` are enabled.

## incident-session-ephemeral.yaml

A **temporary** bridge that starts when something breaks and dissolves when resolved: `known_state.persist: false` so no baseline survives the session (with a commented `session_ttl`). Targets are the coordinator (19701), engineer/security/ops participants (19702–19704) with per-role scopes, and a shared context store (19710).

## escalation-chain.yaml

An on-call escalation matrix as targets: L1 primary and backup (19901–19902), L2 lead (19910), L3 manager (19920), L4 executive (19930), plus the pager (19940) and incident manager (19950). Each level is monitored so the chain's reachability and acknowledgment flow can be verified.

## observer-mode.yaml

Read-only participants: a coordinator (20001) plus observers for a new team member, a security auditor, customer success, and a manager (20002–20005), with an audit trail (20010). Alerting is deliberately **non-aggressive** — observers watch without acknowledging.

## disaster-recovery-drill.yaml

A chaos/DR profile with the fastest cadence in the catalog (`poll_interval: 2s`, `timeout: 1s`, `max_workers: 20`, debug logging) and `known_state.persist: false` so each drill starts clean. Targets span region A (19501–19502), region B failover (19511), a global config store (19520), and a load balancer (19530).

## Cross-references

- [Warroom Builder Skill](../features/catalog/warroom-builder.md), [Warroom Guide Skill](../features/catalog/warroom-guide.md), [Warroom Simulator](../features/catalog/warroom-simulator.md)
- [Config-to-Simulation](../features/catalog/config-to-simulation.md) — turning these profiles into exercises
- [Sample Config Catalog](../../configs/samples/README.md)
