# ADR 0007 — Conveniences are attack surfaces; harden them at introduction

- **Status:** Proposed
- **Date:** 2026-10-22
- **Related:** [ADR 0005](0005-punish-floods-reward-derivation.md), [ADR 0006](0006-the-name-space-is-isotropic.md), [ADR 0008](0008-rotation-as-a-helix-of-residues.md)

## Context

A red team attacks what it can predict. Everything we add for our own convenience is
predictable by construction, because a convenience exists to be reused — by us, by a second
skill, by an operator at 3 a.m. Reuse is the point, and reuse is also the exposure.

Four kinds, all already present:

| kind | example | why it is predictable |
| --- | --- | --- |
| a cheap invariant to test against | `isOdd` / `isEven` | total, deterministic, and computable by anyone |
| readable failure detail | `fuzzed-or-wrong-prefix`, `identity-mismatch` | names the failing check, so it names the next move |
| a published constant or derivation | `ClusterRotationPeriod`, `RouteRef.ID` | deliberately pure, so it can be recomputed anywhere |
| a shared fixture promoted to an instrument | the invariant pair used to measure rotation | a test case becomes a measurement, and measurements get trusted |

The failure is always the same shape: **the convenience can be satisfied without doing the
work it is supposed to attest.** A cheap invariant proves arithmetic; a readable reason
proves you know which wall you hit; a derived id proves you can hash public inputs. None of
them proves participation, possession, or good faith.

That would be harmless if the conveniences stayed diagnostic. The danger is promotion: a
fixture written to test a detector gets reused as the rotation probe, and now a security
decision rests on a quantity an attacker can produce for free.

## Decision

**Every convenience is registered when it is introduced, with four fields, and may only be
used within them.**

1. **What we would like it to attest.**
2. **What it actually proves** — always strictly weaker.
3. **The cheapest way to satisfy it without doing the work.**
4. **The gate it may not open** — blank is not an answer; every entry names one.

Three hard rules follow:

- **A convenience may measure behavior; it may never gate an attestation.** If field 3 is
  cheap, field 4 must be explicit. Participation, possession and rotation are read from
  state that the thing itself changes, never from an answer the client produces.
- **Failure detail is coarse on the wire and fine in the operator's log.** One reason string
  to the claimant — fixed, and fixed-length, so the reason itself is not a gradient
  ([ADR 0006](0006-the-name-space-is-isotropic.md)). The discriminating reason goes to the
  mirror's log, where the claimant cannot read it.
- **Promotion is a review trigger.** A convenience reused by a second skill is no longer a
  fixture; it is an instrument. The moment it is reused is the moment it must be re-checked
  against rule one, because reuse is what makes it trusted.

The register is executable, not prose: `TestConvenienceRegister` in
`internal/mcp_faults/convenience_test.go` holds the entries and fails on an unfilled field,
on `attests == proves`, or on `gatesAttestation`.

## Worked example 1 — the invariant pair cannot measure rotation

`isOdd(n) = n%2 != 0` with `isEven` as its independent check is an excellent test fixture:
total, cheap, deterministic. It is a useless rotation measure, because **a pure function has
a zero derivative.** Sampling it across thirty rotations gives one distinct answer:

```
after 30 rotations: invariant distinct answers=1, stamp distinct values=30
lazy node:          invariant health 30/30, rotations 0
```

A node that never rotates scores perfect health on the invariant, because answering
correctly is not participating. The invariant attests arithmetic
(`TestStaticInvariantCannotMeasureRotation`).

The promotion is easy to make and easy to miss, because the fixture *is* a correct test —
of the wrong question. A second skill that needs a rotation measure and reaches for the
invariant pair will report "no rotation" for a node that rotated thirty times, and the
report will look healthy.

**Rule:** participation is read from state the rotation changed — the issued stamp, the
chain position — never from a static answer.

**Narrowing, because one invariant is not the whole story.** A single static invariant
attests nothing, but a *set* of coprime residues over issued values determines relative
position exactly: two paths under complementary invariants are one entanglement, not two
proofs, and their pair pins where each sits. That construction is
[ADR 0008](0008-rotation-as-a-helix-of-residues.md), and it is what makes the rule
satisfiable — the residue is a function of a possessed value, not of the client's input.

## Worked example 2 — the refusal reason is a guidance oracle

The proxy currently emits five distinct reasons, measured rather than assumed:

```
bare       -> "direct-call-to-tool-bypasses-proxy"
malformed  -> "unprefixed-malformed"
forged     -> "fuzzed-or-wrong-prefix"
neighbour  -> "fuzzed-or-wrong-prefix"
mirror     -> "identity-mismatch" | "unauthorized-replica"
```

Five classes is **2.32 bits of guidance per probe**. And the content is worse than the
count: the two mirror reasons separate "change your identity" from "get registered", which
is the same directional hint [ADR 0006](0006-the-name-space-is-isotropic.md) refuses in the
name space, applied to the check space.

The cost is multiplicative against blindness:

| gates | blind (t^k, t=4) | guided (k·t) | ratio |
| --- | --- | --- | --- |
| 2 | 16 | 8 | 2× |
| 3 | 64 | 12 | 5× |
| 5 | 1 024 | 20 | **51×** |
| 8 | 65 536 | 32 | **2 048×** |

A blind refusal is uninformative, so the attacker cannot exclude a gate and their attempts
multiply across gates. A named refusal converts the product into a sum
(`TestGuidanceIsAdditiveWhereBlindnessIsMultiplicative`).

**This is why conveniences must be hardened as they are added:** the oracle's value grows
with the number of gates, so a system that accumulates checks accumulates exposure. The
ratio is 51× at five gates and 2 048× at eight. A leak added today is worth more tomorrow.

## Consequences

**Positive**

- A cheap, reviewable test: *can this be satisfied without doing the work?* One question
  per convenience, answerable at review time.
- The register makes the composition risk visible: an instrument reused by two skills is
  trusted twice as hard and audited half as often.
- Coarse wire reasons cost nothing in detection — refusals are still refusals, still priced
  — while removing 2.32 bits per probe.
- The two worked examples are already covered by tests, so the rules are not aspirational.

**Negative and costs**

- **Diagnosability is the price.** A client that is refused cannot tell why. That is
  deliberate, and it means first-line support has to read the mirror's log rather than the
  client's error, which is a real operational change.
- **Incident response gets slower.** The fine reason must be available *somewhere*, and
  that somewhere must be access-controlled and correlated by claim id, not by claimant.
- The register is a checklist, not a proof: it catches unfilled fields, not a convenience
  nobody registered.
- A fixed-length coarse reason is a small waste of bytes per refusal, and that is fine.

**Risks**

- **The register rots.** A new convenience added without an entry simply is not listed, and
  no test can see what does not exist. Mitigation is process: the register is part of the
  review checklist for any new test fixture or diagnostic string.
- **A coarse reason can still leak through shape.** Different lengths, different timings,
  or a stable ordering of fields is a gradient even with one message. Fixed-length and
  constant-time are part of rule two, not an optimization.
- **The promotion failure is silent.** A fixture reused as an instrument produces a
  plausible number, and plausible numbers are never re-derived. The invariant pair turning
  into a rotation measure would look like a working health check.
- **Composition is not verification.** Two skills agreeing on the same convenience agree on
  the same weakness; agreement is evidence only when the inputs are independent, which is
  the `isEven` idea — and only for the property `isEven` was written to check.

## Alternatives considered

**Hide the conveniences.** Rejected. The source is open, and
[ADR 0005](0005-punish-floods-reward-derivation.md) already establishes that nothing here
rests on obscurity. The reason must be absent from the wire, not merely undocumented.

**Remove the convenience entirely.** Rejected: an undiagnosable cluster is an operational
failure, and the fine reason is genuinely needed. The decision is *where* it is visible,
not whether it exists.

**Accept the leak because it is only diagnostic.** Rejected after measurement: 2.32 bits
per probe, multiplicative across gates, and exactly the directional signal
[ADR 0006](0006-the-name-space-is-isotropic.md) exists to prevent.

**Rely on rate-limiting the probes.** Rejected as insufficient: the per-claimant ceiling
already bounds the rate, and the guidance is per probe, not per second. A slow attacker
gets the same bits, spread out.

## What this does not decide

- Whether the wire reason becomes one string for all refusals or a small number of coarse
  classes — the trade is diagnosability against bits, and this ADR only rules out the
  current five.
- **The hardening is not implemented.** The wire today emits the five reasons above, with
  different lengths. Collapsing them to one fixed-length reason is a code change this ADR
  specifies but does not make.
- Whether the mirror's fine reasons should ever be visible to a non-member peer.
- Who owns the register, and whether it lives in the test or in `docs/`.

## Verifying the numbers in this ADR

```sh
go test ./internal/mcp_faults/ -run 'StaticInvariant|RefusalReason|ConvenienceRegister|GuidanceIsAdditive' -v
```

| claim | test |
| --- | --- |
| a pure invariant has a zero derivative across rotations | `TestStaticInvariantCannotMeasureRotation` |
| a never-rotating node scores 30/30 on the invariant | `TestStaticInvariantCannotMeasureRotation` |
| the wire emits five distinct reasons, 2.32 bits each | `TestRefusalReasonIsAGuidanceOracle` |
| every registered convenience names what it really proves | `TestConvenienceRegister` |
| blindness multiplies across gates, guidance adds | `TestGuidanceIsAdditiveWhereBlindnessIsMultiplicative` |
