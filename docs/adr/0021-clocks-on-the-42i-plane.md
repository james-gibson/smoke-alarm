# ADR 0021 — Clocks on the 42i plane: lag is provable, lead is believed

- **Status:** Proposed
- **Date:** 2026-10-23
- **Related:** [ADR 0008](0008-rotation-as-a-helix-of-residues.md), [ADR 0017](0017-a-maladaptive-cluster-is-a-lab-strain.md), [ADR 0018](0018-the-helix-rate-is-a-clock-not-a-throttle.md), [ADR 0019](0019-lab-results-import-as-priors.md)

## Context

`internal/isotope/42I_INTEGRATION.md` fixes the real axis at 42 — "ground truth anchor" — and
puts accumulated drift on the imaginary axis: `42 + d·i`, magnitude `sqrt(42² + d²)`.

The clock reading refines that, and explains a constant that looked arbitrary:

- a helix clock **slower** than production sits in `0 > 42` space — on the real axis, short of
  the anchor;
- production **is** 42;
- clocks **faster** than production sit at `42 + d·i` — off the real axis, in the imaginary
  direction. Production cannot prove them. Someone believes them.

That last clause is the whole point. 42 is not a magic number, it is **the production clock**:
the rate everyone agrees on. Everything else is a displacement from it, and the direction of the
displacement decides whether the result is proof or belief.

## Decision

**42 is the proclaimed zeroth of the 42i plane — a coordinate origin, not a measurement. A
slower clock is a lag and is provable by waiting; a faster clock is a lead and is only
believable. Proof is lag-tolerant and lead-intolerant, so a fast experiment can inform
production but can never certify it.**

### 0. The zeroth is proclaimed, not measured

The model's own axiom is `0 < 1`: there is a least element and a strict order. That is the
requirement that makes a zeroth definable at all — without a strict order, "the least element"
is not a property anything can have, and a reference plane has no definition.

So 42 is chosen as the zeroth, and the choice is the whole instrument:

- **It carries no information.** A coordinate origin is not a quantity; only differences from it
  are. This is the actual reason `MATH.md` forbids deriving from 42 — not that the arithmetic is
  hard, but that there is nothing there to derive from. A gauge is not a measurement.
- **It is shared by consent.** Anyone who agrees that 42 is the zeroth may use the same plane;
  anyone who does not is on a different one. Interoperability is agreement on the origin, which
  is a declaration, not a calibration — the same structure as
  [ADR 0017](0017-a-maladaptive-cluster-is-a-lab-strain.md)'s declared authority and
  [ADR 0019](0019-lab-results-import-as-priors.md)'s declared priors.
- **Only departures are portable.** `d` is a difference and survives a change of origin; `42`
  does not, because it *is* the origin. A distance can be quoted to another plane; a value
  cannot.

Two zeros therefore coexist and must not be confused: the **distance zero** (`zero-status:
established`, which `PLANE.md` maps to the nominal tier) and the **proclaimed reference** (42,
the declared unit of the imaginary axis). One is where a value stops departing; the other is
what the departure is measured *against*.

That distinction is also where this ADR meets the lab's documents, and the meeting is a
**unification rather than a conflict**:

- `PLANE.md` puts the **nominal tier on the real axis** and the departure on the imaginary:
  `real tier + i·(42i distance)`.
- This ADR puts a **rate** there, because that is the reading of "slower than prod", "prod at
  42", "faster at `42 + d·i`".

They are the same coordinate, because **a tier is a rate**: a conforming client is stamped at a
better rate of tokens per unit work, and a non-conforming one pays more. The tier is a band on
the rate axis, not a second axis. So the real coordinate is the *stamp rate*, and "nominal tier"
is how that rate is bucketed.

What follows:

- **Rate and tier move together.** A client that conforms gets more stamped tokens for the same
  work; a client that does not, gets fewer. There is no separate "tier" quantity to cache.
- **A fast clock is high-rate and still unprovable.** It has a higher real coordinate *and* a
  positive departure, because its advances cannot be indexed. The real axis alone never decides
  proof status — that is the imaginary coordinate's job.
- **One axis, two names.** `PLANE.md`'s tier and this ADR's rate are one reading; calling them
  two was the error to avoid, and it is corrected here.

What this does **not** do is reconcile the three numeric schemes. It says what the axis *means*,
not which thresholds sit on it; the contradictory ceiling tables in `MATH.md` remain
unreconciled, and no number in this ADR is a calibrated bound.

### 0a. Charge is a label, not a magnitude

An inaccurate clock is **negatively charged**, and that is accurate as long as the word stays a
label. `MATH.md` is precise about it: `plane` charges `±1`, and it is an integer because
`q_a + q_b = 0` is the invariance condition, "not because `±1` is a score".

So:

| reading | charge | what it means |
| --- | --- | --- |
| conforming, pairs to zero with its counterpart | `+1` | the value can cancel |
| inaccurate: drifted, lagging, or stamped wrong | `−1` | the value cannot cancel |

The charge says **which side of the pairing a value is on**, not how far wrong it is. There is no
"more negative" clock and no distance to add: an inaccurate clock does not accumulate charge, it
accumulates departure on the imaginary axis. Confusing the two is the derivation error `MATH.md`
warns about, and it is the same error as summing a coordinate.

### 1. Three regimes, three epistemic statuses

| regime | real: stamp rate | imaginary: departure | charge | admissible as |
| --- | --- | --- | --- | --- |
| slow lab (longer epoch, same ratios) | below the reference | ≈ 0 while the ratios hold | `+1` — it pairs after waiting | **evidence, after catch-up** |
| production | at the reference (42) | 0 | `+1` | evidence |
| fast lab (shorter epoch, or uncapped helix) | above the reference | `> 0` — advances cannot be indexed | not paired | **belief only** — a prior |
| inaccurate clock (drift, lag, bad stamps) | below the reference, or fine on rate | `> 0` | `−1` | belief, or repair |

The asymmetry is not a policy choice, it is what comparison permits:

- **A lag is finite.** Prod waits `n` ticks longer, the slow clock's phase arrives, and the two
  sequences line up. Waiting is a proof procedure, and it terminates.
- **A lead is not.** A faster clock produces more advances per prod tick, so prod cannot tell
  which of its advances corresponds to which tick. There is no waiting that fixes this: the
  extra advances are already in the past, unlabelled. The residue sequence still looks valid —
  and that is the danger, because validity is exactly what it retains while correspondence is
  what it loses.

This is [ADR 0018](0018-the-helix-rate-is-a-clock-not-a-throttle.md) seen from the plane: an
uncapped helix is not rejected for being fast, it is rejected for landing where proof cannot
reach.

### 2. A rung is a belief; a certificate is a proof

The design already has two ledgers, and this ADR says which is which:

| ledger | admits | does not admit |
| --- | --- | --- |
| certification, conformance, tests | real-axis claims, `r ≤ 42` | a lead, however plausible |
| rungs, reputation, priors | imaginary claims, `42 + d·i` | authority over security decisions |

"Someone believes them" is exactly a rung: evidence-shaped, socially held, not decided by
comparison. So the rule of [ADR 0019](0019-lab-results-import-as-priors.md) is the plane's rule:
belief may **inform** (a prior, bounded below one tick), and belief may never **decide** (a
certificate, a promotion, a placement).

### 3. Catch-up is the conversion path

A fast experiment does not have to stay unbelievable. Slow it to `r = 42` and replay the same
work: the run becomes a lag, the lag is comparable, and the result can move from the imaginary
ledger to the real one. That is the only legitimate conversion, and it is a *rerun*, not an
argument.

Which implies the practical rule for labs: **optimise a fast experiment for discovery, then
re-run the winning configuration at production rate for proof.** The first run proposes, the
second disposes.

## Consequences

**Positive**

- 42 stops being a magic constant: it is the proclaimed zeroth, and the origin has a reason —
  the axiom `0 < 1` is what makes any zeroth definable.
- The plane is a consent object, so interoperability is explicit: two parties compare because
  they agreed on the origin, not because their hardware matched.
- **Charge makes pairing a label.** "Can this value cancel" is decidable (`q_a + q_b = 0`)
  without inventing a penalty magnitude, so an inaccurate clock can be marked without being
  quantified.
- The provable/believed split has a geometric home, so "we measured it fast" has a definite
  status rather than a debate.
- It explains why priors are the only channel a lab gets: a fast lab is structurally a belief
  source.
- Catch-up gives a lab a route to authority that does not require weakening the proof rules.

**Negative and costs**

- **The magnitude formula breaks.** `sqrt(42² + d²)` assumes the real part is fixed at 42. With a
  variable rate the magnitude is `sqrt(r² + d²)`, and a slow clock at `d = 0` has magnitude
  `< 42` — it would read as *more* trustworthy than production while being merely slower. The
  fix is not a better magnitude: within one scheme, thresholds apply to the additive distance
  `d`, never to a magnitude computed from `r` and `d`, because a magnitude is a derived output
  and `MATH.md` forbids deriving from outputs.
- **There is no single threshold table to apply.** `MATH.md` records three overlapping schemes:
  capability rungs with ceilings `0 ∞, 1 ∞, 2 80, 3 40, 4 16, 5 110, 6 220` (not monotonic),
  engine states mapping `0→0, 8→1, 12→1, 48→2, 80→3` (which contradicts those ceilings), and
  `plane`'s `±1` charge, which is a symmetry label and not a magnitude. A rung means what the
  file that produced it means; carrying one into another file's formula is the error.
- **A slow cluster is not a safe cluster.** `r < 42, d = 0` means "not yet at the reference", not
  "clean". The real axis below 42 is a queue, not a virtue.
- **One axis, two readings that can be confused.** Rate and tier are the same coordinate, so a
  pricing change and a conformance change produce the same displacement; the ledger has to say
  which one moved. And the rate ratio and the epoch ratio are different numbers when the moduli
  change, so the normalisation still needs stating.
- **A plane is not portable.** Quoting a *value* to another cluster is meaningless; only `d` and
  the agreement on the origin travel. A shared plane has to be re-established, not exported.
- **Belief is sticky.** A believed clock that has been cited for long enough starts to behave
  like evidence in human hands, whatever the ledger says.

**Risks**

- **Fast-lab laundering.** Repeating a prior until it feels measured. The only defence is the
  catch-up rerun, and it must be the thing that gates authority.
- **Stalled versus slow.** A stopped clock has no advances to compare at all, so it is
  indistinguishable from a very slow one until it moves. Stalled peers need a liveness check,
  not a rate reading.
- **Threshold slippage.** If the rung table is ever implemented against the magnitude, a slow
  cluster is promoted. That is a one-line bug with a security consequence.

## Alternatives considered

**Keep the real axis fixed at 42 and carry rate as a label.** Rejected: it loses the asymmetry
that makes the whole point — a label does not say whether comparison is possible.

**A third axis for rate.** Rejected as redundant: the plane already has two coordinates and one
of them was doing nothing but holding the anchor. Making it the rate gives the imaginary axis
its meaning (distance from the rate everyone agreed on).

**Treat a fast clock as slow by decimating it.** Rejected: decimation is the aliasing failure
again — you would be comparing every `k`-th advance of a sequence you cannot index.

**Forbid fast labs entirely.** Rejected: a fast experiment is how a discovery is made, and the
prior channel exists precisely to carry one.

## What this does not decide

- Whether `r` is the epoch ratio, the tick ratio, or the advances-per-second ratio; they agree
  only when the moduli do.
- Whether the rung thresholds are re-expressed in `d` within a single scheme. They must be, if a
  scheme is used at all; reconciling the three schemes is open work and not this ADR's to close.
- Whether catch-up reruns are automated (a lab job) or manual (a person re-runs a configuration).
- Whether a believed clock can be *partially* proved by proving a sub-interval of its phase,
  which would give a graded belief rather than a binary one.

## Verifying

| claim | source |
| --- | --- |
| the period is a property of the moduli, so a rate change is a phase change | `TestCoprimeHelicesReconstructTheIndex` |
| a non-coprime sampling collapses the period | `TestNonCoprimeModuliWasteAPath` |
| the phase is a quotient with torsion, so "advance" is structural | `TestPhasesAreAQuotientAndHaveTorsion` |
| commit cost is irrelevant, so the rate is a clock not a budget | `TestCommitCostsAreIrrelevantAtThisEpoch` |
| Byzantine tolerance is the one grounded relation | `TestByzantineToleranceFiveAlarms`, `F = ⌊(N−1)/3⌋` |

```sh
go test ./internal/mcp_faults/ -run 'Coprime|NonCoprime|Phases|CommitCosts|TrustRotation' -count=1
```

## Grounding: what `MATH.md` allows, and what it forbids

Verified against `lab-safety/lab/MATH.md`. Its advisory is blunt and it applies to this ADR:

- **42 is a fixed base, not a fitted one.** `isotope/plane` computes by closed-form arithmetic
  on indices; the package says "nothing is searched or learned". The weight table in
  `fortytwointegration.go` is hand-chosen. Deriving relationships among these does not converge —
  and §0 gives the reason: 42 is the proclaimed zeroth, so there is no quantity there to fit.
- **Only two things close.** `F = ⌊(N−1)/3⌋` is standard and asserted at `N = 5`; the direction
  "toward `−42i`" is the model's *sign convention*, not a computed limit. Everything else is a
  measured magnitude of departure against linear time.
- **Charge is a label.** `plane` charges `±1` because `q_a + q_b = 0` is the invariance
  condition; it is an integer *label*, not a score. An inaccurate clock is negatively charged in
  that sense and in no other.
- **Figures are outputs, not inputs.** They are the discretised form of the measure in
  `PLANE.md`; deriving among them is not deriving the model.
- **A known non-converging figure:** `devmode.go` accumulates results across calls and computes
  `F` over a duplicated multiset (isotope#9). Internally consistent, externally false.

So this ADR's status is precise: **the plane is a picture with two grounded features — the
direction convention and the standard quorum bound — and no derived numbers.** The lag/lead
asymmetry is an argument about *comparison*, not a computation: waiting terminates, indexing
does not. Nothing here computes a distance, and no threshold in it is calibrated.

The one identity worth stating explicitly, because it is tempting and false: the sum of
reciprocals over the real interval does not land on the imaginary axis. Inversion maps positive
rates to positive periods; it is an involution that preserves the real line. Reaching `−42i`
requires multiplying by `i`, and that multiplication *is* the sign convention — a choice, not a
limit. A slow clock is a long period, and a long period stays on the real axis. It is the
convention, not the arithmetic, that calls that a departure.

The geometric reading is a reinterpretation of `42I_INTEGRATION.md`, which states the real axis
is always 42. Nothing in code reads a rate coordinate today, so nothing is contradicted — but
the document and this ADR now differ, and the document should defer to this one or say why not.
`MATH.md` goes further: the capability rung table it records is non-monotonic and disagrees with
the engine's mapping, so `42I_INTEGRATION.md`'s ceilings are one of three unreconciled schemes
and must not be quoted as a calibrated bound.
