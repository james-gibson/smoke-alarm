---
title: Warroom Simulator Skill
description: "The warroom-simulator skill: interactive /start /join /resolve /status /pause /resume /abort commands, escalation timers, and post-simulation performance feedback."
tags:
  - smoke-alarm
  - feature
  - wargame-onboarding
  - warroom
  - simulator
  - skill
  - gherkin
source: smoke-alarm/features/warroom-simulator.feature
subsystem: wargame-onboarding
gherkin_tags:
  - "@warroom-simulator"
  - "@skill"
scenario_count: 14
---
> **Source:** `features/warroom-simulator.feature` · **Subsystem:** [Wargame & Onboarding](../subsystems/wargame-onboarding.md) · **Index:** [Feature catalog](../index.md)

The `warroom-simulator` skill runs a live, interactive incident simulation driven by chat commands (`/start`, `/join`, `/resolve`, `/status`, `/pause`, `/resume`, `/abort`). Unlike [Warroom Guide Skill](warroom-guide.md), the trainee drives the pace under time pressure and receives a performance summary.

## Scenario map

**Skill invocation** — the agent reads `.opencode/skills/warroom-simulator/SKILL.md` and executes the documented steps in order.

**Startup** — a loaded-but-not-started session prints `INCIDENT SIMULATION READY` and a `/start` prompt; issuing `/start` moves status to `ACTIVE`, begins the elapsed timer, and calls participants with join codes.

**`/join`** — a valid code (e.g. `L1-7K2M9` for `L1-OnCall`) marks the participant joined and records response time; an invalid code returns an error and marks no one.

**`/resolve`** — resolving `p1-unauthorized-access` with a note marks it resolved, records the time, and unblocks P2; a P2 `/resolve` is rejected while P1 is active ("P1 must be resolved first").

**`/status`** — prints elapsed time, a participant join table, and an alarm status table.

**Escalation** — a join timeout (e.g. 10 s for `L1-OnCall`) or an alarm dismiss timeout (e.g. 5 min for P1) triggers escalation to the next tier and issues a new join code; escalation events are recorded.

**`/pause` and `/resume`** — `/pause` sets status `PAUSED` and stops alarm timers advancing; `/resume` returns to `ACTIVE` and resumes timers from their paused values.

**Post-simulation feedback** — resolving all alarms produces a performance summary with time-to-first-participant, per-alarm resolution time, and improvement suggestions; `/abort` ends immediately with a partial summary.

## Cross-references

- [Warroom Guide Skill](warroom-guide.md) — tutorial vs. live-simulation distinction
- [Warroom Builder Skill](warroom-builder.md) — produces the config the simulation loads
- [Alert Routing and Severity Policy](alerts.md) — severity vocabulary shared with alarm states
- [Wargame & Onboarding subsystem](../subsystems/wargame-onboarding.md)
