---
title: Known-State Regression Detection
description: "The knownstate store: sustain-success gate, regression classification, counters, persistence/reset, isotope transit, 42i boundary authorization, and the chaos isotope guard."
tags:
  - smoke-alarm
  - feature
  - engine-simulation
  - known-state
  - regression
  - gherkin
source: smoke-alarm/features/known-state.feature
subsystem: engine-simulation
gherkin_tags:
  - "@known-state"
  - "@core"
scenario_count: 48
---
> **Source:** `features/known-state.feature` · **Subsystem:** [Engine & Simulation](../subsystems/engine-simulation.md) · **Index:** [Feature catalog](../index.md)

The known-state store (`internal/knownstate/store.go`) maintains a persisted healthy baseline per target so the engine can distinguish a brand-new failure from a regression of a previously-healthy target. It tracks `ever_healthy` (behind a `sustain_success` gate), `consecutive_failures`, `success_streak`, `last_healthy_at`, and transition flags (`had_previous`, `became_healthy`, `became_unhealthy`), and sets `is_regression` when a failure follows an ever-healthy state. Valid status values are `healthy`, `degraded`, `outage`, `failed`, `unknown` — "regression" is not a store status; the engine promotes `is_regression` to `targets.StateRegression`.

## Actors & surfaces

| Actor | Interaction |
|---|---|
| Engine | Calls `Store.Update` per probe result; maps `IsRegression` → `StateRegression` |
| Operator | Relies on accurate regression vs. new-failure distinction |
| Snapshot file | `state/known-good.json` persisted atomically via temp-file rename |

## Scenario map

**Status predicates** — `IsHealthy` true only for `healthy`; `IsFailure` true for `degraded`, `outage`, `failed`.

**Sustain-success gate** — `ever_healthy` only after `sustain_success` consecutive healthy results; `1` marks on the first success; an intervening failure resets the streak.

**Regression classification** — failure after ever-healthy → `is_regression` true; before ever-healthy → false; any failure status after ever-healthy triggers it (`unknown` does not); recovering to healthy sets `became_healthy` and clears the flag.

**Transition fields** — first update → `had_previous false`; subsequent → true; failure → `became_unhealthy`; recovery → `became_healthy`.

**Failure counting** — `consecutive_failures` increments per failure, resets on healthy, and `unknown` does not increment; `success_streak` resets on any non-healthy result.

**Guards & normalisation** — an empty target ID errors "target id is required"; an empty status normalises to `unknown`.

**Persistence** — `Save` writes atomically (no `.tmp` remains); `auto_persist` writes after each `Update`; `Load` restores `ever_healthy` and tolerates a missing file; `Snapshot` returns a deep copy; `Reset` clears memory (`delete_file false`) or removes the file (`delete_file true`).

**Isotope transit & feature mapping** — recording a result emits a transit event (isotope at point `known-state`) carrying no payload; isotope-tagged results can mark declared scenarios as covered; untagged results create no coverage entry.

**42i boundary authorization** — a transit clearing its declared boundary passes scope-compliance; an undeclared boundary is a scope-compliance failure (+20 42i distance); a replayed isotope ID is an isotope-variation failure (+8); probe payload in the transit event is a secret-flow-violation (+24).

**Isotope ID construction** — `isotope_id = base64url(SHA256(feature_id || ":" || SHA256(payload) || ":" || nonce))`; two IDs for the same feature+payload differ and are 43 base64url chars; verification needs feature_id + payload + nonce; the wrong feature fails; payload is not recoverable from the ID alone.

**Chaos isotope guard** — a chaos-tagged failure within an active window preserves `ever_healthy`, `last_healthy_at`, `consecutive_failures`, `success_streak`, and `is_regression`; an unregistered or expired-window isotope is treated as a real failure.

## Cross-references

- [Probe Engine](engine.md) — caller of the store; regression escalation
- [Alerts & Notification Routing](alerts.md) — regression severity policy
- [Isotope Transit](isotope-transit.md) — transit model and evidence
- [Chaos Window](chaos-window.md) — chaos isotope registration and windows
- [Engine & Simulation subsystem](../subsystems/engine-simulation.md)