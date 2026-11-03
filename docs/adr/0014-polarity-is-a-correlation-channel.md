# ADR 0014 — Polarity is a correlation channel

- **Status:** Proposed
- **Date:** 2026-10-22
- **Related:** [ADR 0007](0007-conveniences-are-attack-surfaces.md), [ADR 0009](0009-the-side-channel-three-filters.md), [ADR 0010](0010-trust-rotates-with-the-layout.md), [ADR 0012](0012-the-isotope-view-is-the-chart-of-the-nuclides.md), [ADR 0013](0013-the-stream-is-dispersed-not-filtered.md)

## Context

Three pieces of prior art were offered for this model, and they say the same thing at
different scales:

- **Circuit diagrams.** Every node has an expected voltage. A digital pin is still an analogue
  surface underneath: the `0` and the `1` are thresholds on a voltage that has a range, a
  tolerance and a failure mode. Reading the bit without knowing the range is how you build a
  circuit that works on the bench and not in the field.
- **PCB design.** Traces are routed for the composition of what travels on them — impedance,
  length, reference plane. Different compositions take different paths, and a stub or a via is
  where they stop doing so.
- **The Bloom filter.** A probabilistic membership structure: `k` hashes set `k` bits, a query
  answers *possibly in* or *definitely not*. Its error rate is a function of occupancy, so it
  is not really a digital object at all — it is a **one-bit ADC whose bias grows with load**.

That last reading is the one that matters here, because `docs/design-matrices.md` already
records the transfer function:

| occupancy | false-positive rate |
| --- | --- |
| design | **0.82%** |
| 4x design | **54%** |

A Bloom filter at four times its design occupancy is an ADC driven past full scale. The bit
does not become "wrong"; it becomes *uninformative*, which is worse, because it still looks
like a bit.

### The side channel is already polarised

[ADR 0009](0009-the-side-channel-three-filters.md) describes **three filters with three
polarities**, and the polarity table in §3 of the matrices says what a false positive means for
each. That is a polariser/analyser arrangement: a route's state is not one bit but a small code
— three filters, each with a sign — and "did this route change between turns" is a cheap
comparison because the signs are known.

Which is the property that makes it a **correlation channel**. Comparing two polarised beams
does not require knowing what is in either one. You only need the signs to line up.

### The question

If beams of different composition travel separate paths, and the paths are polarised, then a
reader who can compare paths learns something about the *route*, not the *payload*. So: can you
see PII?

## Decision

**The side channel is an ADC with a documented transfer function, polarity is a correlation
channel, and the privacy boundary is three properties that must all hold: the payload never
transits, the identifier is nonce-salted, and the polarity state does not outlive its tick.**

### 1. The filter is an ADC, and its full scale is the design point

Treat `0.82% → 54%` as a transfer curve, not two facts. The design point is the occupancy at
which the bit is worth reading; saturation is clipping. This is why the admission stack is
tuned to a rate (`tiers`, `tick`, `churn` at 50%) rather than to a count: an ADC has a range,
and a range is a rate discipline.

Consequence for the diagrams: a `●` is a *reading within range*, and the matrix's `·` is a
channel with no reading at all. Neither is a guarantee. `TestAlarmFloodSaturatesTheFilter` is
the clipping test.

### 2. Three filters, three polarities = a three-bit code per route

Polarity is a sign, and a sign is what makes comparison cheap. So the side channel does not
leak a payload; it leaks a **stable code per route**: which filter spoke, and with which sign.
Two routes whose codes differ are distinguishable without either payload being readable.

That is the same structure as the isotope identifier one level up: a digest, not an item. The
isotope is `base64url(SHA256(feature_id || ":" || SHA256(payload) || ":" || nonce))` — 43
characters, salted per observation, so two sightings of the same feature and payload do not
link. The side channel's code is not salted, but it is *per route*, and that is a different
kind of identifier.

### 3. The boundary: payload out, salt in, polarity rotating

Three properties carry the entire privacy claim. Break any one and the channel becomes a
quasi-identifier:

| property | what it prevents | where it is specified |
| --- | --- | --- |
| payload never transits — only the isotope id | reading the content | `docs/features/catalog/isotope-transit.md` |
| the id is nonce-salted | linking two sightings of one payload | isotope construction |
| polarity state lives one tick and trust rotates | accumulating a code across turns | [ADR 0010](0010-trust-rotates-with-the-layout.md) |
| the polarity nonce is a tuple that includes the tick | linking one route across turns | this ADR, §5 |

The third is the one that is easy to lose. §4 records the side channel at **10 bits per turn**
of memory, and the helix period is 30 turns ([ADR 0008](0008-rotation-as-a-helix-of-residues.md)).
If that state were *retained* rather than overwritten, the ceiling is 300 bits over a period —
comfortably enough to single out one route among 10^6. The design's protection is not that the bits are few; it is that **data residence
is one tick**, so no member accumulates them, and `1/n` duty means no member sees the same
route every rotation.

### 4. So: can you see PII?

**Not from one beam. Yes from the join.**

- One isotope id: no. It is a salted digest, the payload is absent, and the nonce makes two
  observations unlinkable.
- One route's polarity code within a tick: a pseudonym, not a person. It identifies a route,
  and a route is infrastructure.
- A reader that **joins** codes across many routes within a tick, or across turns via the
  residues: that is where a stable bit becomes a behavioural fingerprint, and a fingerprint is
  what makes a pseudonym personally identifiable in practice. The join, not the beam, is the
  disclosure — which is why §5 puts the tick inside the nonce.

### 5. The nonce is a tuple, and the tick is in it

The open question in the previous draft was whether polarity is keyed per route or per member.
Neither: it is keyed by a **tuple nonce that includes the tick**.

```
polarity(route, tick) = H(tuple_nonce ‖ route ‖ tick) mod 3
```

Three consequences, and they are why this is the right granularity:

- **Stable within a tick**, so the three filters and the consensus on them can compare signs.
  A code that changed per observation could not show that a route *changed*, which is the
  signal the side channel exists to carry.
- **Unlinkable across ticks**, so no reader — including a well-behaved one — can follow a route
  turn to turn by its code. The salt is temporal, not observational.
- **Still reconstructs**, because the helix does not need an identity to work with. It uses the
  *residue* (`mod 2, 3, 5`) and the *phase* (which tick), and the tick is already in the
  nonce. Attribution names a path, not a code: `TestTheDeltaNamesTheLiar` measures a position
  in the lift, and the lift is index arithmetic, not routing state.

So the salt does not fight [ADR 0008](0008-rotation-as-a-helix-of-residues.md); it *is* the
phase. What the salt removes is the cheap join — the one that needs no reconstruction, only a
matching string.

Which is why the 99% local fast path is a privacy mechanism and not only a latency one
([ADR 0013](0013-the-stream-is-dispersed-not-filtered.md), reference arm): if the comparison
stays local, there is no correlator to join at. The residual risk is any component that
already sees many routes — a shared store, a registry, a dashboard. `lezz.sea` is exactly such
a component.

## Consequences

**Positive**

- The ADC framing gives the filter a range and a full-scale, so "the filter is saturated" is a
  measurable out-of-range condition rather than a vague worry.
- Polarity as sign explains why the three filters are cheap to compare and why the code is
  worth protecting.
- The privacy claim is now three testable properties instead of a sentence.
- The PII question has a structural answer: the disclosure is in the join, so the design
  question is "who is allowed to join", not "what is in the beam".

**Negative and costs**

- **A dashboard is a correlator.** `lezz.sea` reads many routes and renders them together; it
  is the component best placed to build the join. Its isotope view shows the design matrices,
  not live route codes, which is why it is safe today — and why adding live per-route polarity
  to it would change its classification. What it may render instead is decided in
  [ADR 0015](0015-the-dashboard-sees-rates-not-identities.md).
- **Three bits is not much, and that is the trap.** Small codes feel anonymous. The risk is
  temporal stability, not width; 3 bits held for 30 turns is a longer identifier than 64 bits
  held for one.
- **Salt does not cover timing.** A nonce unlinks content, not arrival. Two observations at a
  fixed cadence are linked by the cadence even when the ids differ.

**Risks**

- **Joining across members.** Rotation prevents accumulation *per member*; it does not prevent
  a reader from collecting one sample per member and joining them. Mitigation is a policy about
  joins, not a mechanism in the filter.
- **Reading the ADC as a sensor.** The false-positive rate is a bias, not a measurement of the
  threat. A 54% FP filter does not mean 54% of traffic is hostile.
- **Polarity mistaken for authorisation.** A sign says which way a filter leans; it says
  nothing about whether a route may proceed.

## Alternatives considered

**Treating the filter as digital** (member / not member). Rejected: it hides the occupancy
dependence that produces the 0.82% → 54% curve, and it makes saturation look like a bug rather
than the documented end of a range.

**One filter with one polarity.** Rejected: it removes the comparison, and with it the side
channel's 16.1x lead — but it also removes the correlation channel, so the privacy cost of
three filters is real and is the price of the lead.

**Salting the polarity code per observation.** Rejected as incompatible with the mechanism: a
code that changes on every observation cannot show that a route *changed*, which is the signal.
Per-**tick** salt is the decision (§5): it keeps the within-tick comparison and removes the
cross-tick join.

**Refusing to render live polarity in any UI.** Not decided here, but it is the conservative
reading of the join risk, and it is compatible with everything above.

## What this does not decide

- Whether the tuple nonce also includes the member. It must not on the value, or members cannot
  compare signs for consensus; it may on a signature that does not change the code.
- Whether `lezz.sea` may ever display live per-route polarity; today it shows the design
  matrices only.
- Whether the ADC's transfer function is exposed as a first-class number in the ledger (it is
  two points, `0.82%` and `54%`, not a fitted curve).
- Whether timing cadence needs its own unlinkability rule, which salt does not provide.

## Verifying

| claim | test |
| --- | --- |
| the filter clips at 4x and the bit goes uninformative | `TestAlarmFloodSaturatesTheFilter` |
| the side channel earns its 16.1x lead | `TestCanaryFilterWarnsSixteenTimesEarlier` |
| per-route canaries localise a flood | `TestPerRouteCanariesLocalizeAFlood` |
| trust and data rotate, so state does not accumulate | `TestTrustRotationCostsDuration` |
| data residence is one tick | `TestDataResidenceIsOneTick` |
| the fast path is an exclusion channel, so the join stays local | `TestFastPathIsAnExclusionChannel` |
| payload does not transit | `features/isotope-transit.feature` — "Payload is not stored" |

```sh
go test ./internal/mcp_faults/ -run 'Saturates|CanaryFilter|PerRouteCanaries|TrustRotation|DataResidence|FastPath' -count=1
```
