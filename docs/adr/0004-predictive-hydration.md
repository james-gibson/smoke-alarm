# ADR 0004 — Predictive hydration: the surge arrives on time

- **Status:** Proposed
- **Date:** 2026-10-22
- **Related:** [ADR 0001](0001-alarm-membership-tiers.md), [ADR 0002](0002-golden-is-a-derived-reference.md), [ADR 0003](0003-lagged-tick-window.md)

## Context

[ADR 0002](0002-golden-is-a-derived-reference.md) establishes that golden serves nothing,
so the correctness surge has to be provisioned. The naive way to hold it is idle spares,
and the naive way to avoid idle cost is to spawn on demand — which loses the race:

```
hydration for 1 000 000 entries: 550ms
cold path (restore + hydrate):   5m 0.5s  -> 60 ticks late
```

A node that becomes ready 60 ticks late is not surge capacity; it is a post-mortem
resource. What makes the surge affordable is knowing the *next* tick's state before the
tick begins, which [ADR 0002](0002-golden-is-a-derived-reference.md)'s functor provides
for free.

## Decision

**Spawn surge nodes hydrated with the next tick's derived state, ahead of the boundary.**

1. Hydration uses `F(epoch+1)`, evaluated locally. No transfer, no restore.
2. The spawn lead time is the hydration cost — 550 ms for 10⁶ entries — which must fit
   inside one epoch. It does, by more than an order of magnitude.
3. A node exists for the tick it was hydrated for. Idle cost per spawn falls from 5
   minutes to 550 ms, a factor of ~545.
4. Golden's role in the surge is therefore **timing, not hardware**: it does not add
   nodes, it makes nodes arrive on schedule.

## Consequences

**Positive**

- The surge becomes provisionable at herd timescales: 0.55 s of lead instead of 5
  minutes of restore.
- Idle cost per spawn is 545× lower, which is what turns "hold spares" into "spawn
  ahead".
- Hydration is deterministic, so two factories hydrating the same epoch produce identical
  nodes — the property that makes the surge pool interchangeable.

**Negative and costs**

- About 4.4 s of the 5 s epoch remains for ledger adoption and chain join after
  hydration; the budget is real but not generous.
- The factory must hang over the boundary: it must decide before it is needed, which
  requires a load forecast.
- Surge nodes are only correct for the tick they were hydrated for; a node hydrated for
  `epoch+1` and used at `epoch+2` is stale by construction.

**Risks**

- **The forecast is not supplied by this design.** Hydration answers "how do I make a
  node ready", not "when should one exist". Without a trigger, the factory either idles
  (cost) or misses the herd (worse).
- Hydrating ahead concentrates risk: a defect in `F(epoch+1)` lands in every node spawned
  for that tick simultaneously, rather than one node at a time.

## Alternatives considered

**Hold warm spares.** Rejected on cost: 16 nodes idle indefinitely at 400 req/s, for a
resource needed for seconds.

**Cold spawn on demand.** Rejected: 60 ticks late.

**Pre-warm one tick ahead but without deriving state.** Rejected: the node would still
need the state transferred, and transfer was 37 % of a warm rebuild before the cold
restore even starts.

## What this does not decide

- The spawn trigger and the load forecast behind it.
- Whether the factory is the golden cluster, a scheduler, or the mirror.
- How a spawned node joins the chain without re-registering (ledger adoption is assumed
  available).

## Verifying the numbers in this ADR

```sh
go test ./internal/mcp_faults/ -run 'GoldenSpawns|PredictiveSurge|DerivedGolden' -v
```

| claim | test |
| --- | --- |
| 550 ms hydration, 60 ticks late if cold | `TestGoldenSpawnsHydratedNodes` |
| the surge is 16 real nodes; golden adds none | `TestPredictiveSurgeIsNotFreeCapacity` |
| derivation removes transfer and cold restore | `TestDerivedGoldenNeedsNoTransfer` |
