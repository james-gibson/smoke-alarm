# ADR 0019 — Lab results import as priors, never as authority

- **Status:** Proposed
- **Date:** 2026-10-23
- **Related:** [ADR 0002](0002-golden-is-a-derived-reference.md), [ADR 0011](0011-redundancy-is-a-route-property.md), [ADR 0014](0014-polarity-is-a-correlation-channel.md), [ADR 0017](0017-a-maladaptive-cluster-is-a-lab-strain.md), [ADR 0018](0018-the-helix-rate-is-a-clock-not-a-throttle.md)

## Context

[ADR 0017](0017-a-maladaptive-cluster-is-a-lab-strain.md) says a lab cluster is never trusted by
production. That sentence is about **authority**, and it was read too broadly: it sounded like
isolation, when the actual requirement is a one-way channel.

Labs exist to run experiments, and the useful output of an experiment is not a certificate. It
is a measurement: this mechanism deflected this much, on this hardware, at this occupancy, in
this epoch. Production wants that as **data for path routing** — a prior it can start from — and
it must be able to take the data while still refusing the authority.

So the distinction this ADR fixes:

- **Authority** is what may decide: a rung, a certification, a trust relationship. A lab never
  has it, and importing its data does not create it.
- **Information** is what may inform: a weight, a threshold, an ordering, a calibration curve.
  A lab can produce it, and production can consume it without being told what to trust.

A lab result crossing into production is horizontal gene transfer with an immune system: the
plasmid is real, it can change the phenotype, and it cannot become the organism.

## Decision

**Lab results import into production as bounded, signed, expiring priors for path routing. They
seed a weight and never set a rule; the first local measurement overrides them; and no import can
carry evidence, a route, a key, or a payload.**

### 1. Two channels, and only one crosses

| channel | carries | may change | may not change |
| --- | --- | --- | --- |
| evidence | certificates, rungs, promotions | nothing from a lab | — |
| data | priors, calibration curves, measured rates | weights, ordering, design points | placement, trust, residence |

The evidence channel is closed. The data channel is the only reason a lab is worth running, and
it is deliberately narrow.

### 2. What a prior may and may not touch

This is the load-bearing boundary, because "just a weight" can still be a routing decision:

| may be seeded by a prior | why it is safe |
| --- | --- |
| the filter's design occupancy and its FP curve | it is a calibration, and the local measurement re-fits it |
| fast-path vs slow-path thresholds | a preference, reversibly wrong |
| replica preference order, retry order | it changes latency, not correctness |
| provisioning and churn setpoint | bounded by the 2x work rule |

| must not be touched | why |
| --- | --- |
| **placement** (rendezvous) | it is a pure function of the key and the member set; a weight there is a directory by another name ([ADR 0011](0011-redundancy-is-a-route-property.md)) |
| **trust**: rungs, certifications | that is authority, and it does not cross |
| **residence**: the one-tick rule | it is a security property, not a performance knob |
| **the helix rate** | it is a clock ([ADR 0018](0018-the-helix-rate-is-a-clock-not-a-throttle.md)) |

A prior is admissible only where being wrong is recoverable within a tick.

### 3. The prior is smaller than one tick of evidence

The bound is principled rather than tuned: **an imported prior must weigh less than a single
tick of local measurement**, so it can never outvote the instrument. After the first tick, local
data dominates by construction, and the prior decays to nothing.

That also bounds the bandwidth of the channel. A prior that could move a weight arbitrarily far
would be a way for a lab to steer production traffic — a low-rate covert channel with a routing
side effect. Bounded amplitude, `O(log n)` bits per import, and an expiry turn that into a nudge
rather than a steering wheel.

### 4. Signed, epoch-stamped, and attributable

Every import carries:

- the lab's identity, in its own feature family, so it can never verify as production evidence;
- the epoch and the rule set it was measured under, because a prior from a different clock is a
  different instrument;
- the experiment's declared conditions (synthetic data, occupancy, member count), because a
  prior without its conditions is a superstition.

And every routing decision influenced by a prior records that fact. If an experiment turns out
to be wrong, the rollback is "drop the prior", not "re-derive why the cluster behaved oddly for
a week".

### 5. Local override is the reference arm

The importing cluster measures the same quantity on its first tick and keeps its own number.
That is [ADR 0013](0013-the-stream-is-dispersed-not-filtered.md)'s reference arm applied to
calibration: a stored prior is a snapshot, and a snapshot drifts. The lab's curve is the factory
calibration; the cluster's own measurement is the working standard, and the working standard
wins.

## Consequences

**Positive**

- Labs stop being a debugging cul-de-sac: an experiment can improve production's starting point
  without weakening its trust model.
- The channel is typed, so "we imported a lab result" cannot be confused with "we certified a
  lab result".
- The bound (less than one tick) makes the influence self-limiting, which is a property tests can
  assert rather than a policy someone remembers.
- Priors are attributable, so a bad experiment is a rollback, not an investigation.

**Negative and costs**

- **A new attack surface: prior poisoning.** A malicious lab can bias routing within the budget.
  Bounds, local override, expiry and attribution reduce it; none eliminates it, because a nudge
  is still a nudge.
- **Cover channel.** Bounded weight changes can encode bits. The amplitude limit is the only
  thing keeping it low-rate, and it is a limit, not an elimination.
- **Import is a supply chain.** A lab's measurement is only as good as its synthetic data; a
  prior fitted to an unrealistic experiment is worse than no prior, because it is confidently
  wrong for one tick.
- **Two clocks.** A prior measured at a different epoch is comparable only if the ratio is
  preserved ([ADR 0018](0018-the-helix-rate-is-a-clock-not-a-throttle.md)). Importing across
  epochs without scaling is the same aliasing failure in a different costume.

**Risks**

- **"It's just a prior" as a way to import a rule.** The table in §2 is the defence; the moment a
  prior can change placement or trust, this ADR is void.
- **Expiry that never fires.** If the cluster is idle, the prior does not decay, and an old
  measurement lingers. Expiry should be by tick count, not by wall time.
- **Provenance laundering.** Re-importing a prior that came from another lab's prior loses the
  original conditions. The chain has to be carried, not regenerated.

## Alternatives considered

**No import at all.** Rejected: it makes the lab a debugging toy and throws away the only durable
output of an experiment. The requirement is real and this is the narrowest channel that serves it.

**Import as configuration or code.** Rejected: a configuration file is authority by another name,
and it would let a lab change rules rather than weights.

**Manual transcription into the repo.** Rejected: it loses provenance and conditions, and it
converts a measurement into a claim — the exact failure the ledger already has to guard against.

**Import everything, review before use.** Rejected: review is a human authority gate, which is
slower and less reliable than a bound plus a local override. Review is still right for the first
import of a new experiment class.

**Weight the prior by lab reputation (rung).** Rejected as circular: a lab cannot have a
production rung, and importing one to weight its data would smuggle authority back in through the
coefficient.

## What this does not decide

- The exact budget fraction below one tick, and whether it is per-path or per-cluster.
- Whether expiry is a tick count, a decay curve, or both.
- Whether the first import of a new experiment class requires human review, and who signs it.
- Whether priors are shared between clusters (a federation of priors is a federation of
  influence, and needs its own record).
- Whether the observed column in `docs/design-matrices.md` §7 is populated by these imports
  directly, or by a separate aggregation step.

## Verifying

| claim | source |
| --- | --- |
| placement is a pure function, so a prior cannot become a directory | `TestPlacementIsByFunctionNotDirectory` |
| the filter's design point is a curve, not a constant | `TestAlarmFloodSaturatesTheFilter` |
| the provisioning rule bounds any imported setpoint | `TestFiftyPercentSetpointIsTheProvisioningRule` |
| residence is a security property, not a knob | `TestDataResidenceIsOneTick` |
| a prior from a different clock aliases | `TestNonCoprimeModuliWasteAPath` |
| the reference arm is local re-measurement | `TestFastPathIsAnExclusionChannel` |

```sh
go test ./internal/mcp_faults/ -run 'PlacementIsByFunction|Saturates|Setpoint|DataResidence|NonCoprime|FastPath' -count=1
```

**Not yet tested, and the gaps that matter:** that no evidence can travel on the data channel,
and that a prior cannot move a weight past the one-tick bound. The first is an absence; the second
is a property of the import path, which does not exist yet. Both need tests before the first
import ships.
