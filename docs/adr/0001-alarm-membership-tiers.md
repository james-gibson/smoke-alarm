# ADR 0001 — Non-members get the fire alarm; members get the fire station

- **Status:** Proposed
- **Date:** 2026-10-22
- **Related:** [ADR 0002](0002-golden-is-a-derived-reference.md), [ADR 0003](0003-lagged-tick-window.md), [ADR 0004](0004-predictive-hydration.md), [ADR 0005](0005-punish-floods-reward-derivation.md), [ADR 0006](0006-the-name-space-is-isotropic.md), [smoke-alarm#52][52], [lab-safety#9][9], `features/rotation-commit.feature`, `features/skill-pads.feature`, `internal/mcp_faults/`, [design matrices](../design-matrices.md)

## Context

The cluster is layered chains of trust: groups of three, each with an ingress, a
backup and a stamper, observed by a mirror node that commits decisions. Alarms travel
on a sidechannel: a fire alarm forces a rotation announcement, republishes the
parental filter, and burns the skills stamped on the route.

That makes the alarm the most expensive event in the system and the cheapest thing to
send — the wrong way round. Everything below is a measured consequence of that
asymmetry, taken from `internal/mcp_faults` (the numbers and the test that produces
each one are listed at the end).

**The alarm pathway amplifies.** Each accepted alarm rotates a chain and republishes
the summary. A filter sized for `n` insertions per epoch does not degrade gracefully:
at 4× its design insertions the measured false-admission rate is 54 %, at 8× it is
94 %. Thirty rotations are enough to evict an honest cohort, because a burned stamp
costs 2 against an eviction threshold of 60. So an alarm flood does not need to
overwhelm bandwidth; it only needs to fill the filter, after which the sidechannel
admits everyone.

**Rotation must be conditional.** An alarm that is refused must change nothing — no
rotation, no republish, no burn. With acceptance gated on following the tumble chain,
a flood of stale references plateaus at `tolerance` rotations regardless of its size:
one million stale alarms cost exactly what ten cost. Pipelining is inherently stale,
because acceptance advances the chain and alarms sent without waiting all reference
the same outdated position. Plain-alias alarms cannot lag at all (no provenance);
isotoped alarms may lag one rotation because they carry their parent. The per-claimant
ceiling is therefore `(tolerance + 1)` per round trip — 20/s, or 40/s for the isotoped
form at a 50 ms round trip — and it does not depend on the size of the cluster.

**Budgets must match filters.** With one shared filter and per-ingress budgets, one
attacker identity lands alarms on every ingress and the identities needed to saturate
the summary *fall* as the cluster grows: 133 % of the membership at three ingresses,
4 % at one hundred. With a filter per group and a budget per filter, the cost adds
across groups instead of multiplying: 1.33× the membership at every cluster size.

**Capacity is not the same as permission.** Under a 4× herd, a class-blind queue
serves the critical class 5 % of the time while the aggregate reads 25 % success,
because bulk dominates the count. Per-item verification of the same traffic costs
`μ·v` — five times the fleet at a 50 ms verification latency — whereas a shared
parental filter query costs a handful of hashes and returns the correct path to the
serving path's capacity. Correctness is cheap; the *decision to be correct on this
particular alarm* is what is scarce.

**Independence is bounded by the bridge.** Groups of three tolerate one fault (a
quorum of 2 of 3), and a compromised group is `1/G` of the system. If nothing crosses
a boundary there is no global budget at all. The moment 1 % of claims cross, the
bridge sees them and inherits the old exposure — 0.04 of the membership in the
hundred-group case — because it is a shared surface again.

## Decision

**Split the population, and split the pathway with it. Non-members may raise the
alarm. Members hold the station.**

1. **Non-members get the fire alarm.** Anyone may present an alarm. What they receive
   is conveyance: the alarm is carried, priced, and answered on a best-effort path
   subject to the conditional tumble. They receive no reserved capacity, no priority
   class, and no vote in rotation. An alarm that does not follow the chain is refused
   without advancing anything, and the refusal is attributable to the claimant.

2. **Members get the fire station.** Membership is the only path to reserved capacity,
   to the critical class, to the stamper path, and to participation in the chain. A
   group's chain advances only on a quorum of 2 of 3, so no single member can move it,
   and no single group can move the cluster.

3. **Rotation is conditional on acceptance**, per the deal: behave and your alarm is
   processed. Refusal is free to the cluster and expensive to the claimant. The
   claimant's ceiling is `(tolerance + 1)` per round trip.

4. **The unit of the alarm budget is the unit of the filter.** Filters are per group
   of three; budgets are per filter. A global filter with per-ingress budgets is
   rejected — it hands the attacker an N-fold multiplier as the cluster grows.

5. **A global rotation requires a quorum that tracks the membership.** Quorum is not a
   constant, because a constant turns into "one identity per rotation" as scale
   increases.

6. **Criticality is a property of the declared route**, part of what the address
   promises, and it is acted on at admission: a critical arrival displaces bulk rather
   than being refused behind it. The health metric is the worst class, never the
   aggregate.

## Consequences

**Positive**

- Alarm spam cannot force rotations, republish a filter, or burn a skill. The cluster's
  exposure to a flood is bounded by `tolerance`, not by the flood's size.
- **Joining buys a local write path.** A member writes its own slice to its own store,
  uncontended; a non-member's records go through the shared path or are not tracked. At a
  million records per cycle that is the difference between an 11.3 s pipeline (2.26 ticks,
  fails) and a 1.0 s one (0.2 ticks, twice over): 4 shards at 300k rows/s each, or 2 if you
  accept half-shares. Being *tracked* is what makes the slice addressable - the perk and the
  obligation are the same fact ([ADR 0002](0002-golden-is-a-derived-reference.md)).
- Saturation of the sidechannel requires ~1.33× the membership at any scale, and
  forcing a global rotation requires a quorum of the membership — 51 identities at 100
  members. Both are fractions of the membership, which is the point: the rational move
  for a serious actor is to join and behave, because scale buys the attacker nothing
  proportional.
- A single member cannot move a chain (2 of 3), a single group cannot move the cluster
  (`1/G` blast radius), and a compromised group does not carry across an independent
  chain.
- The fire is never refused at admission while bulk exists, and the aggregate cannot
  hide a class failure.
- Correctness costs about a filter query instead of `μ·v` fleet, so correct serving and
  the herd stop competing for capacity.

**Negative and costs**

- Two product surfaces, not one: an alarm path with no guarantees and a member path with
  guarantees. This must be visible in the API, not inferred.
- Members carry a maintenance cost: they must follow the chain, or their alarms are
  refused. The isotoped form buys one rotation of tolerance for that effort; the plain
  alias form buys none.
- The filter costs 20–34 bits per item to hold a 1 % per-epoch admission budget,
  depending on how many queries the epoch sees.
- Grace becomes load-bearing rather than a formality: it is the window in which two
  filters (or two chain positions) are both acceptable.
- The bridge between groups is a shared budget and inherits the exposure the groups
  avoid. Cross-group claims must be rare and separately budgeted.
- Identity acquisition cost is the gate on every quorum argument. If identities are
  free, a quorum only multiplies the attacker's bill by `q` and nothing more.

**Risks**

- Per-ingress budgets with a shared filter re-introduce the N-fold multiplier; the
  architecture must not be read as permitting that.
- Strict priority starves bulk when critical traffic is a large share. A weighted floor
  is the intended refinement; unweighted strict priority is not.
- Nothing in `internal/engine` currently drives rotation. The mechanics are proven at
  unit level only; the mirror/watcher that would drive them is [lab-safety#9][9].

## Alternatives considered

**Rotate on arrival.** Every alarm advances the chain. Rejected: 1:1 amplification, so
30 alarms evict an honest cohort and 4× the design insertions takes the filter to 54 %
false admissions. This is the arrangement the decision exists to replace.

**One global filter with per-ingress budgets.** Rejected: the identities needed to
saturate fall as ingresses multiply — 4 % of the membership at one hundred ingresses.
Scale becomes an attacker's ally, which is exactly what must not happen.

**Per-item verification instead of a shared parental filter.** Rejected on cost: `μ·v`
is five times the fleet at a 50 ms verification latency, and the correct path then
competes with the herd for capacity. Retained as the fallback for items the filter
cannot resolve.

**A single global budget for alarms.** Rejected as the primary mechanism: it restores
scale invariance but collapses the independence of groups and makes one shared counter
a single point of contention.

**Free identity with a quorum only.** Rejected: it multiplies the attacker's cost by
`q` without requiring membership, so a determined non-member can still buy the effect.
The decision accepts that an alarm may be raised by anyone; it does not accept that
raising enough of them should be cheap.

## What this does not decide

- The bridge protocol: how a parental chain is validated across a group boundary that
  never shared a parent. This is the one place where "descent is provable" must work
  between chains that did not agree.
- The mechanism and cost of identity registration — the quorum arguments rest on it.
- Whether the parental filter is strictly a hash chain over parent rotations (so
  membership proves descent) or a flat Bloom filter over rotation identities (so
  membership proves presence). The saturation numbers above hold for either; the
  strict form additionally defeats invented parents.
- The weight of the bulk floor under weighted priority.
- Quarantine and eviction rules, which are per route and out of scope here.
- Replay accounting for one-time skills: a stamp currently resolves on every use.

## Related decisions

Four follow-ups were split out rather than folded in, because each is a decision with its
own consequences and its own numbers:

- **[ADR 0002](0002-golden-is-a-derived-reference.md)** — golden is a derived reference,
  not capacity. A functor of the epoch, zero serving and stamping, content agreement for
  free and position agreement via the shared tumble channel.
- **[ADR 0003](0003-lagged-tick-window.md)** — one lagged tick, so the skew budget is
  10 s rather than 5, monitored for a consistent breach. Two live epochs double the
  attacker's work.
- **[ADR 0004](0004-predictive-hydration.md)** — the correctness surge is provisioned by
  spawning nodes hydrated with the next tick's derived state: 550 ms of lead instead of
  a 5-minute cold restore.
- **[ADR 0005](0005-punish-floods-reward-derivation.md)** — the principle the four
  decisions apply: the cluster punishes floods and rewards derivation. The churn-yield
  policy at a 50% work share is its calibration.

## Verifying the numbers in this ADR

All figures come from unit tests in `internal/mcp_faults`. Run:

```sh
go test ./internal/mcp_faults/ -run 'Alarm|Filter|Scale|Groups|Priority|Sla|SLA|Inverse|Flood|Stack|Verdict' -v
```

| claim | test |
| --- | --- |
| filter saturates at 4× insertions (54 %) | `TestAlarmFloodSaturatesTheFilter` |
| a stale flood plateaus at `tolerance` rotations | `TestStaleFloodCannotForceRotations` |
| pipelining is caught by chain skew | `TestPipelinedFloodIsCaughtBySkew` |
| plain alias cannot lag; isotoped may lag one | `TestPlainAliasCannotLag` |
| 30 rotations evict an honest cohort | `TestAlarmRotationsEvictTheCohort` |
| per-ingress budgets degrade with scale | `TestIngressParallelismMultipliesTheAttacker` |
| grouped filters are scale-invariant at 1.33× | `TestGroupsOfThreeRestoreScaleInvariance` |
| a shared filter degrades with scale | `TestGlobalFilterStillDegrades` |
| a triple tolerates exactly one fault | `TestGroupsOfThreeTolerateOneFault` |
| blast radius is `1/G` | `TestIndependenceIsTheBlastRadius` |
| the bridge inherits the shared exposure | `TestCrossGroupIsTheOnlySharedSurface` |
| the critical class is 5 % under a class-blind herd | `TestHerdThatIgnoresTheFireIsNotHealthy` |
| `μ·v` is the correctness fleet multiplier | `TestCorrectnessFleetMultiplierIsMuV` |
| a filter query replaces `μ·v` | `TestSidechannelReplacesTheVerificationBottleneck` |
| false admission follows bits per item | `TestBitsPerItemFollowsTheEpochBudget` |
| an unexamined item is not innocent | `TestUnexaminedIsNotInnocence` |

[52]: https://github.com/james-gibson/smoke-alarm/issues/52
[9]: https://github.com/james-gibson/lab-safety/issues/9
