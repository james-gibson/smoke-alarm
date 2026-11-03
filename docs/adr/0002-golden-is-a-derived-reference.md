# ADR 0002 — Golden is a derived reference, not capacity

- **Status:** Proposed
- **Date:** 2026-10-22
- **Related:** [ADR 0001](0001-alarm-membership-tiers.md), [ADR 0003](0003-lagged-tick-window.md), [ADR 0004](0004-predictive-hydration.md), [smoke-alarm#52][52], [lab-safety#9][9]

## Context

An "offline golden cluster" reads like spare capacity: it exists, it runs, and it can be
pointed at. The measurements say otherwise — it contributes **0 serving nodes and 0
stamping nodes**, and the correctness surge it was suspected of absorbing is untouched
by its existence. At 400 req/s, μ = 100/s and v = 50 ms the surge is 16 real nodes
(4 serving → 20 correct); a golden cluster makes that 20 into 20.

What it *does* contribute is a reference, and the measurements separate that into two
resolutions with different costs:

```
4 sub-clusters agreed on 16 expected rotations using 0 messages
shared channel, 3 sub-clusters: agreed=true after 2 comparisons
```

Content agreement is free because it is derived. Position agreement is linear in the
number of sub-clusters because it has to be shared. The two are not substitutes.

Treating golden as a snapshot also produces the failure it was meant to prevent: a
rebuilt filter only knows what golden knew. At five minutes of lag that is **300
legitimate rotations refused** — a false negative, which refuses a fire, the one error
this design must not make.

## Decision

**Golden state is a functor of the epoch, not a stored snapshot, and it is a reference
rather than a resource.**

1. `golden(epoch) = F(mod(UTC, 5))`. It is evaluated at call time, identical on every
   node that evaluates it, and never shipped, restored or queried.
2. It is the **expected** set, not the observed set. Expectations cannot be stale;
   observations age, and stay bounded by the lagged tick ([ADR 0003](0003-lagged-tick-window.md)).
3. It contributes no serving and no stamping capacity, and must never be counted as
   surge. The surge is provisioned per [ADR 0004](0004-predictive-hydration.md).
4. The third role of a golden triple — *reference* — is a value, not an action. It is
   derived, which is what stops the reference regressing: a reference cluster would
   need its own reference.

## The durable path carries the derivation, not the records

Put a golden cluster in front of a million tracked routes and add the obvious durability
path - process each record, duplicate it through SQLite, ship it to object storage - and the
tick disappears. The budget, at 10⁶ records per cycle:

| stage | derived path | materialized path |
| --- | --- | --- |
| derive records | - | 100 ms |
| hash into the filter | - | 550 ms |
| SQLite commit | 0.1 ms (one manifest row) | **10.0 s** |
| object store, 2 copies | 65 ms (3.25 MB filter) | 640 ms (32 MB) |
| **total per tick** | **65 ms** | **11.3 s = 2.26 ticks** |

So a materialized pipeline does not fit one tick, let alone twice over: it needs a store that
sustains **400 000 rows/s** for the twice-over claim, which is above a batched SQLite
transaction. The dominant term is not the network - it is the row store, 10.0 s of the 11.3 s.

The durable artifact of a derived state is the **derivation and the filter**, not the rows:
a manifest row plus 3.25 MB at 26 bits per item, which is 65 ms and a 38x twice-over margin.
The million records are recomputed on read, which is the whole point of
[ADR 0002](0002-golden-is-a-derived-reference.md)'s functor.

The asymmetry that follows is acceptable only because the paths are separate:

| | cost | relation |
| --- | --- | --- |
| live repair from derivation | 870 ms | fits a tick, twice over |
| durable write per tick | 65 ms | 77x inside a tick |
| cold restore from the object store | 10.3 s | 2.06 ticks, 12x the live repair |

### Membership is what makes the durable path fit

The materialized pipeline above is what a *single* shared writer costs. Partition it across
tracked members and the rows fit, because the critical path becomes the slowest member rather
than the sum:

| configuration | rows per writer | write time | with shipping | ticks |
| --- | --- | --- | --- | --- |
| one shared store | 1 000 000 | 10.0 s | 11.3 s | 2.26 - fails |
| 2 members, half-share each | 500 000 | 1.67 s | 1.99 s | 0.40 |
| 4 members, quarter-share each | 250 000 | 0.83 s | 0.99 s | **0.20** |

Two effects compound: partitioning divides the rows, and an uncontended single-writer local
store is ~3× faster than a coordinated shared one (300k vs 100k rows/s). Twice-over holds at
**two** members on local stores, or **four** if every member wrote through the shared path.
Total bytes shipped are unchanged - 640 ms of object store either way - so the gain is
parallelism and locality, not less work.

That is the operational content of [ADR 0001](0001-alarm-membership-tiers.md)'s tiers:
joining buys the local write path, and being tracked is what makes the slice addressable. A
non-member keeps the alarm and the shared path, which is 11× slower and does not fit twice
over.

A cold restore misses two ticks and that is fine, because **nothing on the repair path may
read from the durable store**. The store lags by design; it exists for cold starts, audit and
replication. A design that let repair wait on it would convert a two-tick warm-up into an
outage, which is the failure this split prevents.

## Consequences

**Positive**

- Staleness disappears where it mattered: 300 missed rotations at five minutes of lag
  becomes 0, because the expectation is computed rather than recalled.
- **The tick was chosen to fit the repair twice over.** A full rebuild at 10⁶ entries is
  870 ms, so `2 x 870 ms = 1.74 s` against a 5 s epoch is a 2.87x margin, and a single tick
  absorbs five rebuilds. The budget holds up to 2.9M entries; beyond that the claim fails and
  the epoch has to grow. A collapse that must first be *detected* spends a turn before repair
  starts and no longer fits, which is why saturation is made observable and the replacement is
  pre-built (`TestRepairFitsTwiceInATick`, `TestObservableCollapseSkipsDetection`).
- Rebuild gets cheaper and loses its worst case: 870 ms warm (550 ms hashing + 37 %
  transfer) becomes 550 ms with no transfer, and the 5-minute cold restore — 346× the
  rebuild itself — is eliminated.
- Saturation stops being an integrity loss. The attacker still fills the filter at 4×
  insertions (unchanged), but the filter is discarded and re-derived, so the payload
  falls from 64 % false admission to 0.82 % — a factor of 79, and temporary.
- Sub-clusters can disagree on position while agreeing on content, which is what lets
  them stay independent.

**Negative and costs**

- A public function is computable by the attacker. They can derive future epochs and
  pre-date alarms; `0 accepted, 40 refused` — the future chain is refused, so knowing
  what should exist is not the same as being able to spend it. This holds because chain
  positions are **possessed and never derived** — a property of state, not of secrecy.
  Treating it as a secret would be security by obscurity, which this design does not
  claim.
- The reference is only as shared as the clocks: sub-5 s skew keeps one anchor, 6 s
  splits it into two internally consistent ones. NTP becomes a correctness dependency,
  mitigated to 10 s by the lagged tick.
- Whatever publishes golden becomes the trust root. A poisoned derivation poisons every
  rebuild, so the published state must carry the parental chain and be re-derived on
  receipt rather than trusted.

**Risks**

- Any plan that funds the surge "in golden" is 16 nodes short and will fail at the
  first herd.
- A golden cluster implemented as *three clusters* is compatible with this decision only
  if the two live roles hold state and the reference role does not. Three stateful roles
  reintroduce the regress.

## Alternatives considered

**A stored golden snapshot.** Rejected: it ages, it must be transferred, and a cold
restore costs 5 minutes of critical unavailability. It also converts a saturation attack
into fire refusals rather than into a rebuild.

**Golden as warm standby capacity.** Rejected on the numbers: it serves nothing while
being pointed at, so it adds idle cost without throughput, and the surge is unaffected.

**Golden as a fourth quorum member.** Rejected: it cannot observe, so it cannot witness
traversal; a reference that cannot witness is not a member, it is a value.

## What this does not decide

- Whether the parental filter is a strict hash chain over parent rotations or a flat
  Bloom filter over identities. The saturation numbers hold for both; the strict form
  additionally defeats invented parents.
- Who forecasts load, since hydration ahead of time needs a spawn trigger that this
  design does not supply.
- The publication path for golden state and its verification.

## Verifying the numbers in this ADR

```sh
go test ./internal/mcp_faults/ -run 'Golden|Functor|Anchor|SharedTumble' -v
```

| claim | test |
| --- | --- |
| the functor cannot be stale (300 → 0) | `TestFunctorCannotBeStale` |
| derivation removes transfer and cold restore | `TestDerivedGoldenNeedsNoTransfer` |
| a public schedule is safe while the future is refused | `TestPublicScheduleIsSafeOnlyBecauseTheFutureIsRefused` |
| the reference role must be derived, not delegated | `TestGoldenTripleIsWellFoundedByTheFunctor` |
| content agreement costs zero messages | `TestGoldenAnchorGivesContentAgreementForFree` |
| position agreement needs the shared channel | `TestSharedTumbleChannelGivesPositionAgreement` |
| golden contributes no surge capacity | `TestGoldenContributesNoSurgeCapacity` |
| 6 s of skew splits the anchor | `TestAnchorIsOnlyAsSharedAsTheClocks` |
| saturation payload drops 64 % → 0.82 % after rebuild | `TestGoldenChangesThePayoffNotTheArithmetic` |
| stale snapshot refuses 300 legitimate rotations | `TestGoldenStalenessRefusesTheFire` |
| the defender wins the rebuild race by 1149× | `TestGoldenRebuildWinsTheRace` |

[52]: https://github.com/james-gibson/smoke-alarm/issues/52
[9]: https://github.com/james-gibson/lab-safety/issues/9
