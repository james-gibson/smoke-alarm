# ADR 0009 — The side channel: three filters, three polarities

- **Status:** Proposed
- **Date:** 2026-10-22
- **Matrices:** [all numbers in one place](../design-matrices.md)
- **Related:** [ADR 0001](0001-alarm-membership-tiers.md), [ADR 0002](0002-golden-is-a-derived-reference.md), [ADR 0005](0005-punish-floods-reward-derivation.md), [ADR 0007](0007-conveniences-are-attack-surfaces.md), [ADR 0008](0008-rotation-as-a-helix-of-residues.md)

## Context

[ADR 0008](0008-rotation-as-a-helix-of-residues.md) splits the wire into a fast side channel
and a slow authoritative path: bits per turn for "certainly absent or maybe present", the
exact lift for "where it actually is".

The side channel gets asked three different questions, and they need **three different
filters**, because the key and the polarity of the one-sided error differ for each:

| question | key | false positive means | may it decide? |
| --- | --- | --- | --- |
| is this rotation admissible for this tick? | `(tick, rotation)` | admit a stranger | only with the golden repair |
| has this rotation been used at all? | `rotation` | accuse an honest node | **never** - flag only |
| is this route under abnormal volume? | `route` | - (saturation *is* the signal) | yes, it is a volume alarm |

The first is the existing parental filter, rebuilt every epoch. The second is the replay
detector. The third is a canary. Collapsing them into one filter is the failure this ADR
forbids, because one key cannot answer two questions: a replay presented at a new tick under
a tick-keyed filter is a **new key**, and reads as fresh.

Measured:

```
tick-keyed:  replay at tick 18 reads fresh
value-keyed: replay at any tick reads seen
```

## Decision

**Run the side channel as three filters with three distinct keys and three distinct
polarities. Never let one answer a question keyed for another, and never let the replay
filter decide anything.**

1. **Admission filter** - key `(tick, rotation)`, rebuilt per tick. Its false positive admits
   a stranger, so it is only safe where the error is repaired by derivation
   ([ADR 0002](0002-golden-is-a-derived-reference.md)) and bounded by the saturation budget.
2. **Replay filter** - key `rotation` alone, persisted across ticks. Its false positive is an
   accusation, so it may flag for review and never evict
   ([ADR 0007](0007-conveniences-are-attack-surfaces.md), rule one).
3. **Canary filters** - one per route, sized at `1/k` of the route's expected per-epoch
   volume. Saturation is the alarm, so its false-positive rate is the *signal*, not an error.

## The alarm gaps it closes, and the ones it does not

### Closes: saturation is invisible until it is damage

A shared filter shows nothing until it is breached: 2 000 insertions into a 4 096-entry filter
leaves the false-positive rate at essentially zero, because saturation needs roughly `4 × n`
insertions. A canary is the same filter with a smaller `n`, so it crosses at the same *ratio*
but far fewer insertions:

| filter | crosses 40 % at | design FP |
| --- | --- | --- |
| full, `n = 1000` | 2 998 insertions | 0.82 % |
| canary, `n = 62` | **186 insertions** | 0.82 % |

**16.1× earlier, at no design cost** - the false-positive rate at design load is identical,
because it is a property of bits per item, not of size.

### Closes: the unexamined backlog

Every item either resolves as *certainly absent* locally or escalates. Over 100 000 items at
10 bits/item:

```
escalated 1 048 (1.05%)      certainly absent 98.95%
```

So the slow path and the mirror see 1 % of traffic instead of all of it. That is
[ADR 0005](0005-punish-floods-reward-derivation.md)'s `unexamined` verdict being drained at
the source rather than audited later.

### Closes, partially: localising a flood

Per-route canaries name the noisy route without the slow path, and a flooded route's canary
saturates long before any shared filter notices:

| route | load | FP |
| --- | --- | --- |
| the flooded route | 364 | **87.7 %** |
| every other route | 64 | 0.8 % |
| a shared 4 096-entry filter after the same traffic | 2 000 | 0.000 |

Attribution stops at the route: a route is not an identity, so naming the *sender* still costs
the slow path.

### Does not close

- **A stale flood.** The plateau at `tolerance` is arithmetic, not membership; the side channel
  changes nothing about it ([ADR 0005](0005-punish-floods-reward-derivation.md)).
- **Cohort eviction.** Eviction is caused by rotations advancing, not by membership, so the
  fast path cannot prevent it - only hydration repairs it
  ([ADR 0002](0002-golden-is-a-derived-reference.md)).
- **Cross-tick replay.** A tick-keyed filter cannot see it by construction; only the
  value-keyed replay filter can, and only as a flag.
- **Identity cost.** Every quorum argument still rests on it, and three filters make it
  neither cheaper nor dearer.

## Consequences

**Positive**

- Three cheap filters, each answering one question exactly as well as its error permits, and
  the polarity of each error is named rather than assumed.
- Saturation becomes an **early warning** instead of a late symptom, at no cost in the design
  false-positive rate.
- The slow path and the mirror shrink to the escalation rate, which is a property of bits per
  item rather than of traffic.
- Flood localisation is available at route granularity for the price of one small filter per
  route.

**Negative and costs**

- Three filters to size, rotate and monitor, each with its own load and its own budget.
- Per-route canaries multiply the filter count; at many routes the canary fleet can exceed
  the cost of the filter it is guarding, so the sizing needs a floor.
- The replay filter must persist across ticks, which means it has state the admission filter
  does not - and persistence invites the "when do we rebuild it" question this ADR does not
  answer.
- A canary that saturates on legitimate growth becomes a false alarm; the per-route expected
  volume has to track seasonality.

**Risks**

- **One filter, three questions** is the recurring implementation mistake: it looks like a
  simplification and silently inverts a polarity. The tick-keyed replay blindness above is
  what it costs.
- A canary tuned to the attacker's advantage - too large and it warns late, too small and it
  cries wolf - is a tuning surface an attacker can exploit by holding volume just below the
  alarm.
- The replay filter's false positives are accusations; any path that acts on them without the
  slow path converts the fast path into an authority.

## Alternatives considered

**One filter keyed on `(tick, route, rotation)`.** Rejected: it answers all three questions
badly. It cannot see replay (new tick, new key), it cannot localise a route (the key mixes
them), and its saturation is not attributable to any one of the three.

**No canary; watch the real filter's rate.** Rejected after measurement: the real filter shows
nothing until roughly `4 × n` insertions, which is the moment the damage is done.

**Let the replay filter evict to save the slow path.** Rejected: its false positive is a false
accusation, so eviction there turns a probabilistic structure into an authority
([ADR 0007](0007-conveniences-are-attack-surfaces.md)).

## What this does not decide

- The canary's `k`, and whether it is per route, per group, or per class.
- The replay filter's retention: how long a rotation is remembered, and whether that window is
  the grace period or longer.
- Whether saturation of a canary escalates to the slow path, sheds the route, or is only
  reported.
- How the three filters are monitored without themselves becoming a side channel - the
  saturation level of a canary is exactly the kind of diagnostic
  [ADR 0007](0007-conveniences-are-attack-surfaces.md) says not to publish per probe.

## Verifying the numbers in this ADR

```sh
go test ./internal/mcp_faults/ -run 'TickKeyed|CanaryFilter|SideChannelDrains|PerRouteCanaries' -v
```

| claim | test |
| --- | --- |
| a tick-keyed filter cannot see a replay | `TestTickKeyedFilterAnswersFreshnessNotReplay` |
| a `1/16` canary warns 16× earlier at the same design FP | `TestCanaryFilterWarnsSixteenTimesEarlier` |
| the slow path sees ~1 % of items | `TestSideChannelDrainsTheUnexaminedBacklog` |
| per-route canaries localize a flood; a shared filter does not | `TestPerRouteCanariesLocalizeAFlood` |
