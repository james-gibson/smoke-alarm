# ADR 0018 — The helix rate is a clock, not a throttle

- **Status:** Proposed
- **Date:** 2026-10-23
- **Related:** [ADR 0008](0008-rotation-as-a-helix-of-residues.md), [ADR 0010](0010-trust-rotates-with-the-layout.md), [ADR 0016](0016-observation-is-a-protocol-role.md), [ADR 0017](0017-a-maladaptive-cluster-is-a-lab-strain.md)

## Context

The helix advances one residue per tick: residues `mod 2, 3, 5` give a period of 30, and the
lift has no period at all ([ADR 0008](0008-rotation-as-a-helix-of-residues.md)). The measured
cost is 0.78 ns per residue and 0.0001% of a core at a 5 s epoch — so the cap cannot be a
performance limit. A fast machine could run the helix ten thousand times faster and never notice.

So the question is fair: if the lab's helix rate were uncapped, would production trust it?

The answer is no, twice over, and the second reason is the one worth writing down.

## Decision

**The helix rate is a clock, not a throttle. One advance per tick is a semantic constraint, not
a budget; an uncapped helix aliases the projections, destroys the cost of control, and makes the
instrument unable to measure what it exists to measure — so it fails production's tests and the
lab's own.**

### 1. Uncapping aliases the projections

A residue sequence sampled with stride `s` has period `m / gcd(s, m)` for each modulus, and the
joint period is the lcm of those. Uncapping means the stride is arbitrary, and almost every
arbitrary stride loses the period:

| stride between advances | joint period at {2,3,5} | reconstruction |
| --- | --- | --- |
| 1 (one per tick) | **30** | full |
| 2 | 15 | odd residues never sampled |
| 3 | 10 | two moduli collapse |
| 4 | 15 | same as 2 |
| 5 | 6 | the `5` residue is pinned |
| 6 | 5 | only `mod 5` survives |
| 7 | 30 | preserved — `gcd(7,30) = 1` |

Only strides coprime to 30 keep the period, and there are `φ(30) = 8` of those among 30 — about
**27%**. So an uncapped rate is a coin-flip-weighted way to decimate the very signal the helix
exists to produce, and it fails silently: the residues still look like residues, the period is
just shorter, and attribution starts naming paths that were never projected.

This is `TestNonCoprimeModuliWasteAPath` in the time domain rather than the modulus domain. The
design already has a test for the failure; uncapping is that failure applied to the clock.

### 2. The cap is what makes control expensive

[ADR 0010](0010-trust-rotates-with-the-layout.md): `k` consecutive ticks of control cost `k`
members. That sentence is only true if a tick is a unit of *time* the cluster agrees on. Uncapp
the helix and one member can advance thirty rotations inside a tick: the verifier set rotates
thirty times, the residue sequence looks complete, and **a single actor can produce a
self-consistent projection**. Attribution still returns a delta. The delta is just a story one
machine told about itself.

So the cap is a quorum mechanism wearing a scheduler's clothes. It is the same class of thing as
data residence being one tick: not an efficiency, a cost.

### 3. A lab may be maladaptive; it may not be miscalibrated

[ADR 0017](0017-a-maladaptive-cluster-is-a-lab-strain.md) lets the lab be unfit for production.
It does not let the lab be unfit for *measurement*. The lab's whole output is a derivation diff,
and a diff requires both sides to have derived the same thing from the same phase. An uncapped
helix produces a different object, so there is nothing to diff:

- production fails the lab's tests, because the suite asserts one advance per tick — the same
  way it asserts the absence of the observer verb;
- the lab fails its own tests, because its measurements no longer correspond to the documented
  period, so the observed column in §7 would be a measurement of a different instrument;
- and the failure is visible in behaviour, not just in configuration: an uncapped peer's phase
  drifts against every other peer's, which is an attestable mismatch.

Maladaptation is a different *fitness landscape*. Miscalibration is a broken ruler. Only the
first is allowed.

## Consequences

**Positive**

- The rate cap has a stated reason that is not performance, so nobody optimises it away while
  the CPU graph is flat.
- The aliasing table gives a reviewer a one-line check: if the stride is not coprime to the
  period, the projection is not the one the tests assume.
- The cost-of-control argument is now explicit, which makes `k` ticks cost `k` members a
  security property rather than a scheduling accident.

**Negative and costs**

- **The lab cannot be sped up by raising the helix rate.** Testing a 30-turn period at real
  speed takes real time; the honest way to go faster is to shorten the epoch and scale every
  rate together, which changes the clock, not the ratio.
- **A fast peer looks healthy.** An uncapped helix burns almost no CPU, so the usual "it is
  busy, something is wrong" signal never fires. Detection is phase drift, which needs a
  comparison, which needs an observer.
- **The arithmetic is only clean for the current moduli.** Add a modulus and `φ` changes; the
  "27%" is a property of `{2,3,5}`, not a constant.

**Risks**

- **Rate as a config knob.** A `helix_rate` setting invites exactly this failure. The rate
  should not be a tunable; the epoch is.
- **Phase drift mistaken for clock skew.** A real clock difference and an uncapped helix look
  similar from one peer. Distinguishing them needs the declared phase, which is why the handshake
  carries it.
- **Test-time scaling drift.** Speeding up the lab by scaling all rates keeps the ratio, but it
  changes the load-dependent checks (§5), which are exactly the ones that improve with rate.

## Alternatives considered

**Uncap the helix for lab speed.** Rejected: it changes the instrument, not the duration. The
lab would measure a 15- or 10-turn period and report it as 30.

**Cap by CPU budget instead of by tick.** Rejected: the cost is 0.0001% of a core, so a budget
would never bind, and a cap that never binds is not a cap.

**Advance the helix on demand.** Rejected: the residues would then index events rather than
time, and the period would be a property of the caller's impatience. Attribution needs a clock
both sides share.

**Scale the whole clock (shorter epoch, same ratios).** Accepted as the *only* legitimate way to
run the lab faster — and only with the load-dependent checks re-measured, because they are the
ones that care.

## What this does not decide

- Whether the epoch itself is tunable per cluster, and whether two clusters with different epochs
  can attribute across a federation. Today the phases would not be comparable.
- Whether the declared phase belongs in the capability handshake or in the residue exchange.
- Whether a lab may run a deliberately *wrong* period (15 instead of 30) as a negative test.
  That is a test fixture, not a strain, and needs its own record.

## Verifying

| claim | source |
| --- | --- |
| coprime moduli reconstruct the index | `TestCoprimeHelicesReconstructTheIndex` |
| non-coprime moduli waste a path and collapse the period | `TestNonCoprimeModuliWasteAPath` |
| the phase is a quotient with torsion, so the period is structural | `TestPhasesAreAQuotientAndHaveTorsion` |
| the commit cost is irrelevant at this epoch, so the cap is not performance | `TestCommitCostsAreIrrelevantAtThisEpoch` |
| control cost scales with duration | `TestTrustRotationCostsDuration` |
| a phase-keyed filter saturates after a wrap | `TestFilterOverTheQuotientSaturates` |

```sh
go test ./internal/mcp_faults/ -run 'Coprime|NonCoprime|Phases|CommitCosts|TrustRotation|Quotient' -count=1
```

The aliasing arithmetic above is exact for `{2,3,5}` and is repeated in no test; if a modulus is
added, the table and the `φ(30)` figure both need recomputing.
