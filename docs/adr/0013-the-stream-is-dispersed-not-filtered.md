# ADR 0013 — The stream is dispersed, not filtered

- **Status:** Proposed
- **Date:** 2026-10-22
- **Related:** [ADR 0002](0002-golden-is-a-derived-reference.md), [ADR 0006](0006-the-name-space-is-isotropic.md), [ADR 0008](0008-rotation-as-a-helix-of-residues.md), [ADR 0009](0009-the-side-channel-three-filters.md), [ADR 0012](0012-the-isotope-view-is-the-chart-of-the-nuclides.md)

## Context

Three readings of the same data now exist, and each answers a different question:

| reading | question it answers | what it cannot say |
| --- | --- | --- |
| §2 grid | is this threat addressed by this mechanism | what the threat becomes |
| nuclide chart ([ADR 0012](0012-the-isotope-view-is-the-chart-of-the-nuclides.md)) | what does it decay into, how fast | what is being exposed |
| exposure fields | exposed to what, how much, threshold or probability | where the threat goes |

The missing word is **where**. A matrix says a mechanism acts; a decay says what the threat
becomes; an exposure says what absorbs it. None of them says *by how much it is deflected*,
which is the quantity the design actually measures.

There is a reading that supplies it, and it is not a new metaphor — it is the same instrument
family as the isotope chart, in a different domain. In mass spectrometry the sector separates
by mass-to-charge and the detector reads an angle. In optics a prism separates by wavelength
and the detector reads an angle. Both are **dispersion**: nothing is absorbed, everything
exits somewhere, and the measurement is where.

That reading has three parts, and the design already contains all three:

- **The prism.** A coherent stream enters and one element spreads it by a conserved property.
  Here the stream is the alarm/probe/derivation traffic before admission, the conserved
  property is the isotope (which threat variant this is), and the spreading element is the
  admission stack — tiers, golden, tick, hydration, churn, isotropy, convenience, helix, side.
- **The rotation.** A kaleidoscope is a dispersive element inside a rotation group: mirrors at
  `π/n` produce `n`-fold symmetry and the image repeats under rotation. The 42i model twists
  the same way — the helix's residues `mod 2, 3, 5` give a period of 30
  ([ADR 0008](0008-rotation-as-a-helix-of-residues.md)), and the lift has no period at all.
  The "twist" is not a change of state; it is the same object seen through the next rotation.
- **The pure path.** Every dispersive instrument that measures a *ratio* needs a second beam
  that is not dispersed: the reference arm. Without it there is only an absolute reading, and
  an absolute reading drifts with the lamp. In this design the reference arm is the golden
  derivation and the local fast path — re-derived, never stored
  ([ADR 0002](0002-golden-is-a-derived-reference.md)), resolved locally 99% of the time.

The word "pure" is doing real work here. It is not a claim that honest traffic is clean; it is
the statement that **there must exist a path that no mechanism touches**, or the instrument has
no baseline to difference against.

## Decision

**The model is dispersive, not absorptive: mechanisms deflect, they do not block. The
deflection angle is 42i distance. Rotation is a symmetry action on the same stream, and an
undispersed reference path is a requirement, not an optimisation.**

### 1. Angle is the coordinate we already measure

This is why the prism is honest where a flow chart was not. A flow chart needs an edge *width*
— a volume, a concentration — and §2 has none. A fan needs an *angle*, and the angle is in the
ledger: the 42i weights are deflection, the rung ceilings are the screen, and the helix's
attribution delta is which slit the ray came through.

| optics | here | measured? |
| --- | --- | --- |
| coherent source | traffic before admission | rate, §5 |
| wavelength | isotope: which threat variant | §2 |
| refractive index | mechanism | yes — the verdict |
| deviation angle | 42i distance | yes — the weights |
| prism | the admission stack | §2 columns |
| detector arc | the three filters, three polarities | [ADR 0009](0009-the-side-channel-three-filters.md) |
| reference arm | golden derivation, local fast path | 870 ms; 99% local |
| mirror angle | rotation group of the layout | period 30 at {2,3,5} |
| axis of rotation | the isotropic direction | [ADR 0006](0006-the-name-space-is-isotropic.md) |
| fringe phase | rotation residue | [ADR 0008](0008-rotation-as-a-helix-of-residues.md) |
| path for the pure light | the undispersed reference | a requirement |

One consequence is immediate: **a `·` cell is not "blocked."** In a dispersive instrument
nothing is blocked; the isotope simply has no refractive index at that stage and passes
straight through. The grid's `·` and the chart's absent channel both mean *undeflected here*,
which is a different and more useful statement than *stopped*. It is also why no mechanism can
claim immunity: a prism does not destroy light.

The quantiser at the end of the fan — the part that turns a deflection into a bit, with a
false-positive rate that rises with occupancy — is
[ADR 0014](0014-polarity-is-a-correlation-channel.md).

### 2. The twist is a sinogram, not a state change

Rotate a dispersive element and record where the beam lands at each angle: the result is a
sinogram, and a sinogram is the raw data of tomography. The design already says this in the
only place it could:

- the helix's residues are projections at coprime angles, `mod 2, 3, 5`;
- the period is their least common multiple, 30 — the number of projections needed before the
  pattern repeats;
- the attribution delta `15/20/6` is the reconstruction: it names which projection disagrees,
  which is which liar;
- `TestCoprimeHelicesReconstructTheIndex` is named for reconstruction, not for rotation.

So "the 42i model twists like a kaleidoscope" is not decoration. **Rotation plus dispersion is
tomography**, and the coprime moduli are the reason a non-coprime pair (`2, 4`) wastes a path:
two projections 45° apart cannot separate two objects, so the period collapses to 4.

This is also the answer to "where and when": the fan gives **where** (angle), the rotation
gives **when** (phase). A threat that is invisible in one projection is separable in another,
which is exactly what `TestNonCoprimeModuliWasteAPath` measures by failing.

### 3. The reference arm must stay cheap

If the reference path is dispersed, the instrument measures nothing but its own drift. In
system terms: if honest traffic is taxed to detect dishonest traffic, the baseline moves and
every number in the ledger becomes a relative claim about an unknown reference.

So the following are not conveniences, they are the reference arm:

| reference path | property | why it must stay undispersed |
| --- | --- | --- |
| local fast path | 99% resolved locally | a remote check adds a hop and a failure mode to the baseline |
| convenience rule | no claim, uniform refusal | rejecting honest traffic to look strict moves the baseline |
| parity in the lift | no period | a periodic reference aliases with the rotation |
| golden derivation | re-derived, never stored | a stored reference drifts with its snapshot |

`TestFastPathIsAnExclusionChannel` is the test of "the reference arm exists." If it stops
holding, the isotope chart is still correct and the exposure fields are still correct, and
every conclusion drawn from them becomes an absolute claim — which §9 refuses.

### 4. The figure, if it is drawn

One beam, one prism, a fan of rays onto an arc of detectors indexed by 42i angle, and a
straight reference arm drawn beside the fan and labelled with its two numbers (870 ms, 99%
local). Rotated through the 30 phases, the fan traces the sinogram; the reconstruction is the
attribution.

**Not built.** It is a fourth figure and it should be one picture, not a mode in the widget. It
needs no new measurements, which is the test of whether it is honest: every coordinate on it is
already in §7.

## Consequences

**Positive**

- The three existing readings get a shared geometry: the grid says *acted on*, the chart says
  *decays to*, the exposure says *absorbed by*, and the fan says *deflected by how much*.
- `·` becomes "undeflected here" instead of "blocked", which is the truthful reading and
  removes the last place where the matrix could look like immunity.
- The fast path gains a structural reason to exist. It is not a cache; it is the reference arm,
  and a design that taxes it loses the ability to make ratio claims.
- The rotation is explained rather than asserted: coprime projections reconstruct, and the
  period is the lcm — the same statement ADR 0008 already makes.

**Negative and costs**

- **No intensity.** The fan says where a wavelength lands, not how much of it there is. There
  is no abundance, no quantum yield, no concentration; a bright ray and a dim one are drawn
  identically. That is the same gap ADR 0012 records, unchanged.
- **Two beams double the drawing.** A dual-beam figure is roughly twice the ink of a single
  fan, and the reference arm is boring by construction — which is the point, and a drawback
  for a diagram whose job is to be looked at.
- **"Pure light" is a design claim, not a measurement.** The 99% is measured; "undispersed" is
  the interpretation. If the fast path were ever routed through a check, the word would
  become false without any test failing — the property is a shape, not a number.

**Risks**

- **Tomography over-read.** The sinogram reading is an analogy for *reconstruction from
  coprime projections*, which the helix does. It is not a claim that the system performs
  tomography on an attacker, and no inversion beyond the attribution delta is implemented.
- **Kaleidoscope read as chaos.** A kaleidoscope is highly symmetric, not random. The name
  invites the opposite reading; the axis and the mirrors are the content, the colours are not.
- **Prism read as permission.** "Nothing is blocked" can be heard as "nothing is refused."
  Refusal is a different axis (the gate polarity table in §3 of the matrices), and the fan does
  not model it.

## Alternatives considered

**A filter bank** (threats blocked by successive filters). Rejected: absorption language, and
the matrix has no "blocked" cell — every row that is not `·` is a deflection with a daughter.
Filter language also implies a threat can be removed, which §9 refuses.

**A single-beam prism.** Rejected: without the reference arm there is no ratio, only an
absolute angle, and an absolute reading drifts with whatever changed last. The dual beam is the
whole reason the ledger can state factors instead of verdicts.

**The kaleidoscope as the primary figure.** Rejected: the symmetry is a property of the name
space and the layout, not of any one threat. Drawn alone it shows mirrors and no source term.

**The instrument flow chart from ADR 0012's alternatives.** Superseded by the fan: a flow chart
needs widths, a fan needs angles, and the angles are measured. The flow chart is still the
right figure for *transport* (sample path, lag, hydration lead), which is a different question
from dispersion.

## What this does not decide

- Whether the fan ships, and whether it is a static figure or a mode over live traffic.
- Whether the deflection is drawn as the distance itself or as `atan(distance / 42)`; the
  arctangent is the optical convention and the distance is the measured quantity, and the two
  differ at large distance, which is exactly where the rung ceilings sit.
- Whether the reference arm is drawn as the golden rebuild (870 ms) or the fast path (99%).
  They are two different reference arms with two different failure modes.
- Whether the 30 phases are drawn as 30 rays or as one ray with a phase axis. The sinogram
  suggests the second, the kaleidoscope suggests the first.

## Verifying

This decision adds no numbers; it re-reads numbers that already have tests.

| claim | test |
| --- | --- |
| the reference arm resolves locally, so it is genuinely undispersed | `TestFastPathIsAnExclusionChannel` |
| coprime projections reconstruct the index | `TestCoprimeHelicesReconstructTheIndex` |
| a non-coprime pair wastes a projection and collapses the period | `TestNonCoprimeModuliWasteAPath` |
| no probe reveals a direction, so the axis is real | `TestWallFollowerIsCaughtByItsOwnBias` |
| the reference is re-derived, not stored | `TestGoldenRebuildWinsTheRace` |
| a saturating filter is still a deflection, not a block | `TestAlarmFloodSaturatesTheFilter` |

```sh
go test ./internal/mcp_faults/ -run 'FastPath|Coprime|NonCoprime|WallFollower|GoldenRebuild|Saturates' -v
```
