# ADR 0015 — The dashboard sees rates, not identities

- **Status:** Proposed
- **Date:** 2026-10-22
- **Related:** [ADR 0009](0009-the-side-channel-three-filters.md), [ADR 0010](0010-trust-rotates-with-the-layout.md), [ADR 0011](0011-redundancy-is-a-route-property.md), [ADR 0013](0013-the-stream-is-dispersed-not-filtered.md), [ADR 0014](0014-polarity-is-a-correlation-channel.md)

## Context

[ADR 0014](0014-polarity-is-a-correlation-channel.md) ends with an uncomfortable sentence: a
dashboard is a correlator, and `lezz.sea` is the component best placed to join what the design
keeps apart. This ADR decides what it may join, which turns out to be almost nothing — not
because the view is untrustworthy, but because **a rate is not an identity**.

The observation that makes this practical: the dashboard does not need real data to be useful.
Relative flows between nodes are enough to see the cluster working, and a share of a tick
carries no key. So the question is not "what may we display" but "what survives being
displayed as a share".

Monitoring the golden cluster specifically, three questions have to be answered together:

1. What would `lezz.sea` see?
2. What would it hide?
3. What is lost when it is hidden?

They are one question. Every item in (2) is an item in (3), and the design is a choice about
which losses are acceptable.

## Decision

**The dashboard renders per-tick relative flows: shares, rates, phases and invariants. It
never renders keys, payloads, bit arrays or a history, and its safety property is that it is
stateless — it recomputes from the current tick and overwrites.**

### 1. Rates are safe; identities are not

A relative flow is a quantity divided by another quantity from the same tick. It discloses
*shape* — this member is writing, that shard is hot, this mechanism is deflecting — and it
discloses nothing that can be matched against a route.

That is the same move as [ADR 0013](0013-the-stream-is-dispersed-not-filtered.md): the fan
shows where a wavelength lands, not what it is. The dashboard shows where the work lands, not
whose it is.

It also revives a figure that was rejected on different grounds. [ADR 0012](0012-the-isotope-view-is-the-chart-of-the-nuclides.md)
rejected a Sankey *from threat to daughter* because those widths would be invented volumes.
A Sankey *between nodes* is not the same object: the widths are measured counts — manifest
writes, object copies, block reads, escalations — divided by the tick's total. Measured rates
are exactly what a flow diagram is for.

### 2. Show the ADC's position, not its bits

[ADR 0014](0014-polarity-is-a-correlation-channel.md) established that a Bloom filter is a
one-bit ADC whose bias grows with occupancy. A dashboard that hides occupancy cannot warn
before clipping, and a dashboard that shows the bit array hands out a membership oracle.

So the rule is precise: **occupancy and its estimated false-positive rate are visible; the
bits are not.** The operator sees the transfer curve's current point (`1M` of `2.9M` design,
`0.82%` FP) and can see it move toward `54%`; nobody can query a bit and learn whether a
specific item is in the filter.

The same logic applies to parity: a stripe's XOR is a block, so parity *contents* are hidden
while parity *health* (does the stripe have its share, is the holder excluded from serving) is
shown.

### 3. The view has no history

This is the load-bearing property, and it is structural rather than a permission:
`lezz.sea` computes from the current tick and overwrites. There is no store, no append, no
export. A view with no history cannot join across turns, whatever it is allowed to see.

It is also why the tuple nonce from [ADR 0014](0014-polarity-is-a-correlation-channel.md) §5 is
compatible with a live dashboard: the dashboard's own statelessness and the nonce's temporal
salt are the same policy at two layers.

## What the golden cluster shows

The golden cluster is the reference-holding side of the system: `n` members tracking a million
routes, two replicas per route, parity per stripe, canaries per route group, and a five-second
tick. Monitoring it, the visible surface is:

| visible | why it is a rate, not an identity |
| --- | --- |
| topology: member count, shard count, replicas, parity holders | computed from `(key, n)`, no directory to disclose |
| tick budget: derive, hash, commit, object copy, total | four durations and a margin |
| rebuild events and their count per tick | 870 ms each, five absorbed per tick |
| hydration lead vs cold spawn | 550 ms against 60 ticks |
| fast-path share, slow-path escalation | 99% and 1.05% |
| occupancy and estimated FP | the ADC's position on its own curve |
| canary lead and saturation per route group | 16.1x; 4.7 s at 40/s |
| deflections per mechanism | counts of priced, windowed, re-derived, attributed, localised |
| trust duty and rung changes | `1/n`; demotions counted, not named |
| invariant checks: parity exclusion, residence, twice-over | 0% self-served; 1 tick; 2.87x |

The last row is the one that justifies the whole view. A golden cluster can be *up* and
*wrong*: twice-over can have slipped below 2, parity can be served by its own holder, residence
can exceed a tick. Those are all visible as rates and none of them requires a key.

## What it hides

| hidden | because |
| --- | --- |
| payloads, isotope preimages, nonces | content, and the preimage is the content |
| route keys and their owners | there is no directory; displaying one would create it |
| filter bit arrays | a membership oracle — query a bit, learn membership |
| parity contents | XOR of a stripe is the block |
| golden derivation inputs | precomputation, and a poisoned rebuild |
| per-route polarity across ticks | the tuple nonce rotates; the view could not hold it anyway |
| which actor caused a demotion | the attribution delta names a path, not a person |

## What is lost

Five losses, and they are real:

- **Attribution.** You see a demotion and a rung change. You do not see the actor. Even the
  delta (`15/20/6`) names a path, and the dashboard does not get the delta.
- **Campaigns.** With a per-tick salt and no history, "this route has been probing for 29
  ticks" is unanswerable — not only by the dashboard, but by anyone who did not reconstruct it
  inside the cluster. Cohort eviction at 30 rotations is a cluster fact, not a dashboard fact.
- **Causality of repair.** You see that a rebuild took 870 ms repeatedly. You do not see why
  this one differed. The timing is a rate; the reason is an input you hid.
- **Cross-shard joins.** Each view is an aggregate over `1/n` of the picture, so a distributed
  flood looks like a uniform rise in escalation rather than one actor spread thin. That is the
  price of not being a correlator, and it is the same price [ADR 0009](0009-the-side-channel-three-filters.md)
  already pays for locality.
- **Correctness of the reference arm.** The fast path's 99% says the path was taken, not that
  the answer was right. A dashboard can show a healthy reference and a wrong one identically.

The sharpest form: **you can see that the cluster is working, and you cannot see whether it is
wrong in the same way twice.** That is the loss, and it is deliberate — the alternative is a
dashboard that is a surveillance surface.

## Consequences

**Positive**

- The dashboard is useful with no identifiers, so the correlator risk is bounded by design
  rather than by policy.
- Occupancy is displayable while bits are not, which is the only way to warn before clipping.
- The tick budget becomes a live gauge with a hard number: 65 ms against 5 s, and the failure
  mode at 11.3 s is visible as a bar, not an incident.
- "Rate, not identity" is a testable rendering contract: no key, no bit, no parity byte, no
  append.

**Negative and costs**

- **It cannot answer the questions an incident review asks.** Who, when, how long — all three
  are outside the view. The cluster can answer them; the dashboard deliberately cannot.
- **The view is less impressive than it could be.** A live per-route map would be a better
  demo and a worse system.
- **Relative flows still leak magnitude.** A flood is visible as a flood. That is intended
  (saturation is the signal), and it means the dashboard is not a way to hide an attack from
  an operator — only from a correlator.

**Risks**

- **History creeps back in.** A "last 5 minutes" sparkline is a store. If one is added, the
  join returns and this ADR is void; the rule has to be enforced at the data layer, not the
  view layer.
- **Aggregates can be differenced.** Two views of overlapping groups let a patient reader
  subtract one from the other. Mitigation is coarse grouping, not finer data.
- **A rate view reads as a content view.** "99% local" and "0.82% FP" look like measurements of
  traffic; they are measurements of the system's own behaviour.

## Alternatives considered

**Show real routes, filtered by permission.** Rejected: a permission is a policy, and the point
of the ADR is that no one — including the operator — should be able to build the join from the
dashboard. Policy can be changed; a missing store cannot.

**Show per-route polarity live.** Rejected by [ADR 0014](0014-polarity-is-a-correlation-channel.md):
it is the correlator, and it survives neither the salt nor the tick.

**No dashboard at all.** Rejected: observability is how a twice-over margin of 2.87x is
noticed before it is 1.9x. The answer is a rate view, not blindness.

**A pull-based query API for operators.** Deferred: it is the right shape for the incident
questions the dashboard cannot answer, and it needs its own audit boundary.

## What this does not decide

- What the golden cluster exposes as an aggregate endpoint, and how it is authenticated. The
  view is only as safe as that surface.
- Whether the dashboard displays anything per route at all under an explicit, audited operator
  action. If it does, the join is back and needs its own record — the declared debug role is
  decided separately in [ADR 0016](0016-observation-is-a-protocol-role.md).
- Whether the tick budget gauge is a bar, a ratio, or a set of four durations; all three are
  rates.
- Whether a "last tick vs this tick" delta is a history. It is one tick of memory, which is the
  same budget the side channel already spends.

## Verifying

| claim | test |
| --- | --- |
| data residence is one tick, so there is no history to show | `TestDataResidenceIsOneTick` |
| a route's server is not its parity holder, so parity health is safe to show | `TestSelfServeConflictAtBlockGranularity` |
| the durable path fits the tick twice over | `TestDerivedPathFitsTheTickTwiceOver` |
| escalation is 1.05%, not a per-route signal | `TestSideChannelDrainsTheUnexaminedBacklog` |
| the fast path is an exclusion channel | `TestFastPathIsAnExclusionChannel` |
| the filter's bias is a function of occupancy | `TestAlarmFloodSaturatesTheFilter` |

```sh
go test ./internal/mcp_faults/ -run 'DataResidence|SelfServeConflict|DerivedPath|SideChannel|FastPath|Saturates' -count=1
```

The view's own contract — no store, no key, no bit, no parity byte — is not yet tested. It is
held by construction in `LezzSea/Models/NuclideChart.swift` (static data only) and by the
absence of persistence in the view; a test that fails on the first `@AppStorage` or file write
in that target would be the right guard.
