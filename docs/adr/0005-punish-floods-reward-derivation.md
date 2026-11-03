# ADR 0005 — The cluster punishes floods and rewards derivation

- **Status:** Proposed
- **Date:** 2026-10-22
- **Related:** [ADR 0001](0001-alarm-membership-tiers.md), [ADR 0002](0002-golden-is-a-derived-reference.md), [ADR 0003](0003-lagged-tick-window.md), [ADR 0004](0004-predictive-hydration.md), [ADR 0006](0006-the-name-space-is-isotropic.md), [ADR 0007](0007-conveniences-are-attack-surfaces.md)

## Context

The preceding four ADRs describe separate mechanisms — conditional rotation, grouped
budgets, a derived golden reference, predictive hydration. Measuring them one at a time
made them look like four answers. They are one answer applied four times.

Every measured mechanism charges the path that did *not* do the work that makes its claim
checkable, and discounts the path that did:

| mechanism | non-deriving path pays | deriving path gets |
| --- | --- | --- |
| conditional rotation | flood plateaus at `tolerance`; 10⁶ stale alarms cost what 10 cost | one rotation per accepted alarm |
| per-claimant cap | pipelining is stale by shape | `(tolerance+1)` per round trip, sequencing |
| grouped budgets | saturating costs 1.33× the membership, per filter | per-group filters are scale-invariant |
| parental filter | 4× insertions ⇒ 64 % false admission | a rebuild from derivation: 0.82 %, 870 ms |
| golden reference | a snapshot ages into 300 refused rotations | the functor: 0 messages, 0 staleness |
| hydration | a cold spawn arrives 60 ticks late | 550 ms lead from `F(epoch+1)` (hashing only, no transfer) |
| priority at admission | bulk is evicted for critical | critical is never refused behind bulk |

The common quantity is not volume. It is **whether the client did the work that lets its
claim be checked**: following the chain, staying inside the epoch, carrying its parent,
arriving when it said it would. Volume from a client that did that work is served. Volume
from a client that skipped it is refused, priced, or made inert.

That is what "a flood isn't behaving" means precisely: a flood is not merely large, it is
*pipelined*, and pipelining is stale by construction because acceptance advances the
chain.

## Decision

**Discriminate by derivation, not by volume. Every mechanism must charge the
non-deriving path and/or discount the deriving one, and no mechanism may price by
throughput alone.**

Corollaries that follow directly, and that the ADRs above already implement:

1. **Cost attaches to the un-checkable claim.** A refusal must be free to the cluster and
   expensive to the claimant ([ADR 0001](0001-alarm-membership-tiers.md)).
2. **Discount attaches to work done in advance.** Derivable state is expected, not
   recalled; predicted hydration is a reward for having a schedule
   ([ADR 0002](0002-golden-is-a-derived-reference.md), [ADR 0004](0004-predictive-hydration.md)).
3. **A wider tolerance is paid for by the same principle.** The lagged tick
   ([ADR 0003](0003-lagged-tick-window.md)) is a discount on being one tick behind, which
   only a client following the chain can claim.
4. **Churn is a maintained base load, calibrated at 50 % work share.** Churn is derived
   work — it is the rotation machinery's own clock — so it is planned, and planned work
   yields to urgent work rather than the reverse.

   | work | churn | share | epoch | window | replay-safe |
   | --- | --- | --- | --- | --- | --- |
   | 50 | 100 | 0.33 | 5 s | 10 s | yes |
   | 100 | 100 | **0.50** | 5 s | 10 s | yes — churn equals work |
   | 150 | 50 | 0.75 | 10 s | 20 s | **no** |
   | 200 | 0 | 1.00 | — | — | no — clock stopped |

   The setpoint is a provisioning rule: `share ≤ 0.5` is exactly `churn ≥ work`, so the
   cluster is sized at **2× the work load**.

5. **Yielding is priced in the direction people expect least, and here the price is
   immediate.** Slowing the churn lengthens the epoch, so more queries land inside one
   epoch and the filter must grow: **~1–2 bits per item per halving**, monotone from a
   26-bit nominal.

   | churn share | rate | epoch | queries/epoch | bits/item |
   | --- | --- | --- | --- | --- |
   | 100 % | 0.20/s | 5 s | 2 000 | 26 |
   | 50 % | 0.10/s | 10 s | 4 000 | 27 |
   | 25 % | 0.05/s | 20 s | 8 000 | 29 |
   | 6 % | 0.01/s | 80 s | 32 000 | 32 |

   It also *widens* the acceptance window, which is a discount on clock tolerance and a
   cost in replay lifetime.

   **The yield band is empty in the shipped configuration, and this is the correction that
   matters.** Two constraints pin the epoch from opposite sides — the window must be at
   least the skew budget ([ADR 0003](0003-lagged-tick-window.md)) and at most the replay
   tolerance. With `ClusterRotationPeriod = 5 s` both are 10 s, so the rotation rate is not
   a free parameter: it is exactly `skew/2`. A 10 % yield already produces an 11.1 s window
   and breaches. "Slow the churn" is therefore not a lever — it is a decision to widen the
   replay tolerance, with its own stated cost. **Adding capacity is the only replay-safe
   response**, and the default.

6. **The design is judged by the attacker's margin, not by whether it admits one.** An
   attacker that behaves competently is served — and served *identically*, so competence
   converts into no lever. Destabilization must cost more, in priced units, than the repair
   it forces. Competence is the inverse of the real deal: what compounds for a client is a
   flat cost for an attacker.

## Destabilization is a losing trade

The honest boundary above says the measure cannot know intent. That is not a concession,
because this design does not try to exclude a competent attacker — it tries to make the
trade negative. Two properties do that, and both are measured.

**1. Admission parity means competence buys no lever.** Every claimant gets the same
ceiling, however well it derives:

| client | continuity | admission |
| --- | --- | --- |
| plain alias | 0 s — any rotation it missed is fatal | 40/s |
| isotoped (follows the chain) | 10 s, one lagged window | 40/s |
| hydrated from `F(epoch+1)` | 10 s, and never misses | 40/s |

Derivation buys continuity, which compounds for a client. It buys **zero** extra
admission, so the marginal return on an attacker's competence is zero. The same act has
opposite sign for the two, which is what "the inverse of the real deal" means
mechanically (`TestCompetenceBuysTheSameRateSoItBuysNothing`).

**2. Rotation is a tax that falls on the taxpayer.** A rotation hands the attacker
nothing. Its only output is invalidation, which is symmetric — both sides burn 2 per
rotation, eviction at 60 — so inducing one charges the tax to the inducer:

| holder | burned over 30 rotations | refund |
| --- | --- | --- |
| honest cohort | 60 | 550 ms, re-issued from derived state |
| attacker cohort | 60 | 0.75 s, rebuilt by hand at the shared ceiling |

The tax is identical; only the refund differs, and the refund is the derivation the
attacker cannot get for free because it is *state*, not arithmetic
(`TestRotationIsATaxThatFallsOnTheTaxpayer`).

**3. Every measured attack costs more than its repair, and leaves nothing.** Nothing in
the table below transfers value to the attacker:

| attempt | attacker spends | cluster spends | keeps |
| --- | --- | --- | --- |
| saturate the parental filter (100 k entries, 100 identities) | 1 m 40 s at the ceiling | 87 ms rebuild from derivation | 0 |
| evict an honest cohort | 30 rotations, own cohort burned | 550 ms hydration | 0 |
| flood with stale alarms | 10 s of alarms | 0 — refusals advance nothing | 0 |
| guess a stamp | 7 attempts, then self-eviction | 0 — the measure decides | 0 |
| raise load to widen the window | capacity | a policy decision the attacker does not own | 0 under this config |

The margin ranges from 19× to unbounded, and one row is honest about being close: cohort
eviction is 750 ms against a 550 ms repair, a mere 1.4×. It is not expensive *in time* —
it is worthless, because the evicted cohort is re-issued from state the attacker cannot
produce (`TestDestabilizationIsALosingTrade`).

The one exception worth naming is the load column. Raising load is the only attacker
spend that converts into defender exposure, and under the shipped configuration it
converts into nothing at all: the yield band is empty (corollary 5), so widening the
window requires the defender to widen the replay tolerance — a decision the attacker does
not own, bought once, and non-compounding
(`TestTheOnlyUncapturedGainDoesNotCompound`).

So the design goal is not "no attacker gets in". It is: **the attacker's spend is linear
in their effort while their gain is flat, the defender's repair is bounded and independent
of how much the attacker spends, and every effect is refunded by derivation rather than
excluded by secrecy.** An adversary who finds that unworthwhile is the intended outcome;
one who does it anyway has bought the cluster a bounded, priced repair.

## Security does not rest on obscurity

The source is open. An attacker knows the naming format, the chain rule, the schedule,
the tolerance and the budget formula, and knows that they know it. Nothing above may be
load-bearing because it is hard to read.

| | status |
| --- | --- |
| the code, the pricing, the invariants, `F(epoch)` and its schedule | **public by design** |
| an issued stamp value, a possessed chain position | **load-bearing** |
| the measure (residual, holonomy, distance) and the verdict it produces | **load-bearing, and public** |
| the unguessable-but-fixed format of a stamp | a price, not a security claim |

What actually holds:

- **Possession, not secrecy.** A chain position must have been issued; an unissued one is
  refused whether or not its derivation is public (`TestPossessionIsTheBoundary`). A stamp
  must have been handed out; a perfect forgery of the *format* is still refused, and one
  hex character of difference is enough (`TestKnownFormatGivesNoAdvantage`).
- **The measure, not the algorithm.** A claim that is internally consistent but does not
  satisfy the invariant is caught by the invariant: a drifted inversion has residual
  1e-2 against a proper one at 0.0, and the verdict is derivable by anyone
  (`TestTheMeasureIsWhatCannotBeFaked`). Publishing the measure costs nothing, because
  satisfying it is the work.
- **Priced, not hidden.** A wrong guess costs the fuzz tier and eviction follows within
  seven attempts; the per-instance space being 32 bits is a budget, not a secret
  (`TestObscurityIsNotCounted`).

The honest boundary: the measure makes *deviation measurable*, not *intent knowable*. A
deriver who satisfies every invariant with honest arithmetic and dishonest purpose is
indistinguishable from a good client by any quantity computed here. That is not a defect
in the measure; it is the limit of measuring, and it is the reason the remaining control
is identity cost rather than a cleverer check.

## Consequences

**Positive**

- One principle instead of four, so a new mechanism has a test: does it charge the
  non-deriving path, or discount the deriving one? If neither, it is priced by volume
  and it belongs to a different design.
- Derivation is cheap to give away because it costs the cluster nothing to verify: a
  functor is evaluated locally (0 messages), a chain position is compared (S−1
  comparisons), a filter rebuild is hashing plus transfer (870 ms at 10⁶ entries).
- The flood has no volume lever. Making a flood larger changes nothing: the plateau is
  `tolerance`, the saturation cost is per membership, and the refusal is charged to the
  claimant.
- Capacity planning is expressible: 2× work. The yield band is empty by default, which
  makes the planning rule unambiguous — capacity, not churn-rate, is the knob.
- The attacker's expected value is negative by construction rather than by exclusion, so
  the defender's cost is bounded and independent of how much the attacker spends.

**Negative and costs**

- **Derivation is a capability requirement, not a virtue check.** Clients must follow the
  chain, stay inside the epoch and carry their parent, or they are charged. That is a real
  barrier to entry for small or stateless clients — the plain-alias form exists precisely
  because some callers cannot afford provenance, and it buys no tolerance at all.
- Yielding churn moves the epoch, so the 10 s skew budget of
  [ADR 0003](0003-lagged-tick-window.md) is tied to the replay tolerance. Widening one
  widens the other, and that coupling is a policy decision with a stated cost, not a free
  response to load.
- Memory grows when the cluster is under pressure — the bits-per-item cost lands exactly
  when the operator is least able to absorb it.

**Risks**

- **Competence is served, and that is the design rather than a hole.** An attacker who
  follows the chain, stays current and pipelines within `tolerance` is served — at the same
  rate as every other claimant, so competence buys no lever. What bounds them is the
  measure (a claim that does not satisfy the invariant is caught whether or not it is
  *sincere*), the symmetry of the rotation tax, and identity cost. Anything stronger would
  have to be obscurity, which is not security.
- **Near break-even in time exists; near break-even in value does not.** Cohort eviction
  is 750 ms of attacker time against 550 ms of repair. The margin is small because the
  repair is derivation, which is cheap — not because exclusion is doing the work. If
  hydration ever costs more than the eviction, this row stops being a losing trade.
- If identity is free, the cap is per identity and adds up across identities. Every
  quorum argument in [ADR 0001](0001-alarm-membership-tiers.md) rests on identity cost,
  which this ADR does not supply.
- A mechanism that prices by volume will look correct in calm conditions and invert under
  pressure, because volume is the attacker's cheapest input.

## Alternatives considered

**Flat pricing by volume.** Rejected: volume is the one input a flood has in unlimited
supply, so pricing it makes the attacker's cheapest move the effective one.

**Punish all churn.** Rejected: churn is the clock. Charging it makes the security
machinery the first casualty of load, which is the failure this principle avoids by
planning churn as a base load and provisioning 2× work.

**Yield churn freely above the setpoint.** Rejected after measurement: the nominal window
already equals both the skew budget and the replay tolerance, so the yield band is empty
and any slowdown breaches the tolerance. Slowing churn is a decision to widen replay
lifetime, not a load-shedding knob.

**Reward all work.** Rejected: a flood is work. It is work that did not do the derivable
part, which is why the discount has to be conditional on the derivation and not on the
arrival.

**Derive the capability too.** Rejected in [ADR 0002](0002-golden-is-a-derived-reference.md):
a public function is computable by the attacker, so the *expectation* may be derived while
the *chain position* must be possessed. Deriving both would reward anyone who can do
arithmetic.

## What this does not decide

- Whether derivation checks are enforced in the hot path or only at admission.
- How identity cost is priced — the load-bearing input for every quorum argument.
- Whether the replay tolerance should ever be raised above the skew budget to create a
  yield band, and if so by how much — the only way "slow the churn" becomes available.
- The monitoring threshold for a consistent skew breach.

## Verifying the numbers in this ADR

```sh
go test ./internal/mcp_faults/ -run 'Flood|Tumble|Scale|Groups|Churn|FiftyPercent|Yielding|YieldBand|Capacity|Destabilization|Competence|Tax|Uncaptured' -v
```

| claim | test |
| --- | --- |
| a stale flood plateaus at `tolerance` | `TestStaleFloodCannotForceRotations` |
| pipelining is caught by chain skew | `TestPipelinedFloodIsCaughtBySkew` |
| the flood costs the claimant, not the cluster | `TestFloodCostsTheClaimantNotTheCluster` |
| saturation is priced per membership, per filter | `TestGroupsOfThreeRestoreScaleInvariance` |
| 4× insertions ⇒ 64 % false admission | `TestAlarmFloodSaturatesTheFilter` |
| rebuild from derivation ⇒ 0.82 %, 870 ms | `TestGoldenChangesThePayoffNotTheArithmetic`, `TestGoldenRebuildWinsTheRace`, `TestRepairFitsTwiceInATick` |
| the functor costs 0 messages | `TestGoldenAnchorGivesContentAgreementForFree` |
| hydration lead is 550 ms, cold is 60 ticks late | `TestGoldenSpawnsHydratedNodes` |
| the setpoint implies 2× provisioning | `TestFiftyPercentSetpointIsTheProvisioningRule` |
| yielding costs ~1–2 bits/item per halving | `TestYieldingCostsBitsPerItem` |
| the yield band is empty at the shipped period | `TestTheYieldBandIsEmpty` |
| capacity is the replay-safe response | `TestAddingCapacityIsTheReplaySafeResponse` |
| the controlled variables are exportable | `TestChurnShareIsMonitorable` |
| every attack costs more than its repair | `TestDestabilizationIsALosingTrade` |
| competence buys admission parity, not a lever | `TestCompetenceBuysTheSameRateSoItBuysNothing` |
| the rotation tax falls on the taxpayer | `TestRotationIsATaxThatFallsOnTheTaxpayer` |
| the only uncaptured gain does not compound | `TestTheOnlyUncapturedGainDoesNotCompound` |
| a public format confers no advantage | `TestKnownFormatGivesNoAdvantage` |
| only possessed chain positions are accepted | `TestPossessionIsTheBoundary` |
| the measure discriminates without secrecy | `TestTheMeasureIsWhatCannotBeFaked` |
| unguessability is a price, not a claim | `TestObscurityIsNotCounted` |
