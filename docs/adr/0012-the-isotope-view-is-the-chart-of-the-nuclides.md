# ADR 0012 — The isotope view is the chart of the nuclides

- **Status:** Proposed
- **Date:** 2026-10-22
- **Related:** [ADR 0002](0002-golden-is-a-derived-reference.md), [ADR 0006](0006-the-name-space-is-isotropic.md), [ADR 0007](0007-conveniences-are-attack-surfaces.md), [ADR 0009](0009-the-side-channel-three-filters.md), [ADR 0011](0011-redundancy-is-a-route-property.md)

## Context

`docs/design-matrices.md` §2 is a property table: threats as rows, mechanisms as
columns, a mark in each cell. It answers one question — *is this threat addressed by this
mechanism* — and it answers it well. It cannot answer three others, because its shape
forbids them:

- **Variants are flattened into rows.** `stale flood` and `pipelined flood` are two rows
  with no stated relation, although they are one threat delivered two ways. The same is true
  of `stamp forgery` and `replay across ticks`, and of `wall-following probe` and
  `gate enumeration`.
- **Nothing says what a threat becomes.** A `●` means the threat stopped being a problem for
  that mechanism. It does not say the threat is gone, and in every measured case it is not:
  it is priced, windowed, attributed, localised. The daughter state is in the ADRs but not in
  the grid.
- **Time is a separate document.** Residence lives in §5, so a reader has to hold two tables
  in their head to see that the same mechanism can be fast on one threat and irrelevant on
  another.

The classical expansion for exactly this problem is chemistry's: the **periodic table**
collapses each element into one cell, indexed by identity; the **chart of the nuclides**
(Segrè chart) expands each element into its isotopes on a proton × neutron lattice and
colours each one by its decay mode and half-life. The first is a property table. The second
is a transition chart: it shows what a thing *becomes*, how likely, and how fast.

The question this ADR settles is whether our data has that shape. It does, and the reason is
not decorative:

| chart of the nuclides | here | evidence |
| --- | --- | --- |
| element | threat class | one class, several carriers |
| isotope | one row of §2 | `stale` and `pipelined` are one class |
| neutron number | which carrier variant | mass label `Fl-1`, `Fl-2` |
| decay mode | mechanism | the nine columns |
| a channel | a non-empty cell | `·` is *no channel at this mass*, not a zero |
| branch ratio | `●` vs `◐` | `◐` is both branches, not a weaker `●` |
| half-life | §5 detection latency | 469 ms, 870 ms, 50 s |
| daughter nuclide | the state the threat becomes | priced, windowed, attributed |
| stability ridge | classes with no open channel | narrow, and not a guarantee |
| stable isotope | a cell marked `●` | stable still absorbs neutrons |
| source term | the work an isotope emits | §2, §4 |
| pathway | the surface that work crosses | the mechanism column |
| dose | accumulated 42i distance, or occupancy, or work share | §7, 42i weights |
| deterministic vs stochastic | threshold effect vs probability | §5 |

That last row is the whole reason to draw it. A **stable isotope is not a harmless one** — it
still has an absorption cross-section, it still poisons a reactor, it still scatters. The
matrix already says this in prose ("every cell is a factor or a repair time, never immunity");
the nuclide chart says it structurally, because stability in the chart is a statement about
decay, not about safety.

### Dosimetry is load-bearing, and it is not radiological

The isotope model was not chosen for its imagery. It was chosen because it is an **exposure
model**, the same family as the compartment models used for covid-19: a source term, a medium,
an intake pathway, a receptor, a dose that accumulates, and an effect that is either a
threshold or a probability. Refusing the dosimetry vocabulary would delete the part of the
model that does the work, so the vocabulary is tracked explicitly, with non-radiological
quantities:

| exposure concept | here | where it is measured |
| --- | --- | --- |
| source term | the work an actor emits: alarms, probes, derivations, churn | §2, §4 |
| medium | the name space, the route table, the filter | ADR 0006, ADR 0011 |
| pathway | which surface the work crosses: alarm ingress, probe answer, derivation rate, tick boundary | §2 columns |
| receptor | a member's filter, a route's canary slots, an agent's rung | §7 |
| dose | accumulated 42i distance, or filter occupancy, or work share | `internal/isotope/42I_INTEGRATION.md` |
| dose rate | load: arrivals per second, work as a share of the tick | §5 |
| dose limit | the rung ceiling (20, 60, 100, 140, 180, 220) | ADR 0001, 42i |
| deterministic effect | a threshold: eviction at 30 rotations, saturation at 4x, demotion at a ceiling | §2, §7 |
| stochastic effect | a probability: 0.82% FP at design and 54% at 4x; 99.9% detection in 10 turns | §5 |
| residence | the time a population survives a channel | §5 |
| stable isotope | a `●` cell: no channel in the table acts on it | §2 |

What stays absent is the radiological *quantity*: no becquerel, gray or sievert, no source
inventory, no shielding, no dose coefficients, no safety function. "Exposed to what" is
answered with **work**, and the dose is **distance**. That is a discipline about units, not an
evasion of the model: the model is used, the physics is not borrowed.

The view says this in its own footer, so the claim cannot travel without it.

## Decision

**The isotope view is the chart of the nuclides, it is an exposure model rather than a decay
illustration, and it ships as a standalone widget in lezz.sea that requires no cluster, no
network and no file access.**

It is not a flow chart, not a Sankey, and not a second grid with different colours; those are
recorded as rejected alternatives below. The compact §2 grid remains the periodic-table view,
and the widget carries both, so the relationship is visible rather than asserted.

### 1. Five classes, eleven isotopes

The partition is editorial and the rows are not: every isotope is one row of §2, in order.

| Z | element | isotope | channels | open |
| --- | --- | --- | --- | --- |
| 1 | `Fl` flood | stale flood (10^6 alarms) | tiers | — |
| 1 | `Fl` flood | pipelined flood | tiers, tick | — |
| 2 | `Pr` pressure | filter saturation (4x) | tiers, golden, helix, side | — |
| 2 | `Pr` pressure | cohort eviction (30 rot) | tiers, golden, hydration, side | **side** |
| 2 | `Pr` pressure | load surge (work > 50%) | tick, hydration, churn | — |
| 3 | `Fg` forgery | stamp forgery (guess) | tiers, isotropy, convene, helix | — |
| 3 | `Fg` forgery | replay across ticks | tiers, golden, helix, side | — |
| 4 | `Dv` derivation | competent derivation | eight of nine | — |
| 4 | `Dv` derivation | common-mode drift | helix | — |
| 5 | `Pb` probe | wall-following probe | isotropy, convene, helix | — |
| 5 | `Pb` probe | gate enumeration | convene | — |

Mass numbers (`Fl-1`, `Pr-2`) are **labels for variants, not physical masses**. They exist so
two isotopes of one class can be named in a cell; nothing may be computed from them.

### 2. A channel is a decay channel, and it has a daughter

Each non-empty cell is drawn as a channel with four parts: the mechanism, the verdict, the
cell's own token, and the daughter state. The daughter is the point of the row — the threat
is not removed, it is moved:

| mechanism | daughter | residence (§5) |
| --- | --- | --- |
| membership tiers + pricing | priced | immediate |
| golden derived reference | re-derived | 870 ms |
| lagged tick window | windowed | 10 s |
| predictive hydration | pre-warmed | 550 ms |
| churn yield at 50% | over-provisioned | 1 epoch |
| isotropic name space | symmetrised | 469 ms at 40/s |
| convenience rule | opaque | immediate |
| helix in the lift | attributed | 50 s |
| side channel, 3 filters | localised | 4.7 s at 40/s |

Residence is a property of the **mechanism**, taken from §5 where §5 measures it, and is not
claimed per cell. Two cells sharing a mechanism share the figure; that is honest about what
was measured and where.

### 3. The chart is the source term; "exposed to what" is annotation, not geometry

A nuclide chart answers what decays, into what, and how fast. It does not answer what is being
exposed, because a source term is only one term of an exposure. That gap is the real
visualization problem, and it is not solved by drawing more arrows: at this level of
measurement there is no concentration, so a pathway drawn as a flow would carry widths nobody
measured.

So the widget carries the exposure as **fields, not geometry**. Every isotope names its source
term, the pathway its work crosses, the dose that accumulates, and whether the effect is a
threshold or a probability:

| isotope | source term | pathway | dose | effect |
| --- | --- | --- | --- | --- |
| stale flood | one actor, 10^6 alarms | alarm ingress | filter occupancy | deterministic: plateau |
| pipelined flood | one actor, chained submissions | tick boundary | chain skew | deterministic: ages out together |
| filter saturation | 4x design occupancy | route filter | occupancy x 4 | stochastic: 0.82% → 54% FP |
| cohort eviction | 30 rotations of alarms | admission | identity price, 7 vs 24 | deterministic: eviction |
| load surge | honest work above 50% | durable path | seconds per tick | deterministic: 11.3 s in a 5 s tick |
| stamp forgery | a guessing actor | stamp check | distance 7 | deterministic: 7 tries |
| replay across ticks | a replayed packet | tick window | 1 epoch | deterministic: issuance |
| competent derivation | a peer deriving one step ahead | derivation rate | work share | deterministic: 2x cap |
| common-mode drift | a shared clock or layout | residue lift | difference 2d | deterministic: 2d |
| wall-following probe | a biased prober | probe answer | 19 probes | stochastic: 99.9% in 10 turns |
| gate enumeration | the refusal oracle | gate order | 5 reasons = 2.32 bits | stochastic: 51x at 5 gates |

That is enough to answer "exposed to what" for the eleven things on screen — a filter to
alarms, a canary to routes, a rung to failures — without pretending to measure a
concentration. The honest summary: the nuclide chart is the **inventory**; the exposure model
is **the inventory plus four annotations**. Turning the annotations into a diagram is a
separate decision, and it waits on a measured quantity to put on an axis.

### 4. A class reports its best channel, and flags its worst

`cohort eviction` is closed by `golden` and `hydration`, partial by `tiers`, and **open** by
`side`. Reporting the class as `open` would discard three working channels; reporting it as
`closed` would hide the one that does not exist. So the cell fills with its best channel and
carries a dashed rim in the open colour when any channel is open. In the widget this is the
only cell with a dashed rim, which is exactly the honest picture: cohort eviction is not
prevented by the side channel ([ADR 0009](0009-the-side-channel-three-filters.md)), and no
amount of green elsewhere should bury that.

### 5. The widget is pure

`LezzSea/Models/NuclideChart.swift` is data plus arithmetic; `LezzSea/Views/NuclideChartView.swift`
is SwiftUI over that data. Neither touches `AppModel`, `URLSession`, the filesystem, or the
network, and neither needs a cluster. Three consequences, all intended:

- The chart renders in the "no cluster" state, and is reachable from it. Lights and events
  keep their gate; the gate moved inside the tabs that need it, and the event stream — which
  never needed a cluster, since it tails a log — is now ungated as a side effect.
- The same view runs in a WidgetKit extension unchanged, because a widget can do none of the
  things the view does not ask for.
- Public SwiftUI only. No private API, no injection, no new entitlements, no code-signing
  change: the view is ordinary first-party UI. A widget extension is a provider, not a
  refactor, and is deliberately not part of this decision.

App surface: a third tab, ⌘3, in `ui/xcode/Lab/LezzSea`.

## Consequences

**Positive**

- Variants become visible: `stale` and `pipelined` are two isotopes of one class, and the
  channels that separate them (tick, chain skew) sit next to each other instead of two rows
  apart.
- Daughters are on the page. "Priced" and "attributed" are the actual outcomes, and the chart
  cannot show a closed cell without naming one.
- Residence travels with the channel, so §5 is readable from the same object.
- The exposure fields answer the question the chart alone cannot: source term, pathway, dose,
  and whether the effect is a threshold or a probability. Without them the isotope view would
  be a decay table with a security skin.
- The `cohort eviction` open channel is structural, not a footnote.
- The chart needs nothing running, which is what makes it a widget and what makes it survive
  the cluster being down.

**Negative and costs**

- **Mostly whitespace.** 5 classes × 3 carrier slots = 15 positions for 11 isotopes. A
  denser layout would have to lie about the lattice.
- **The mass numbers are invented.** They order variants and nothing else. Any code that
  treats `Fl-2` as heavier is wrong; a reader who imports chemistry intuition will be wrong
  in the same direction.
- **Hand-transcribed data.** The table mirrors §2 by hand, so it can drift from the
  generator. Tests pin the shape, the order, and the two cells most easily rounded off
  (`cohort eviction` side = open, `gate enumeration` = one channel); they cannot pin every
  label.
- **Two vocabularies in one app.** `Lights` speaks adhd; `Isotopes` speaks the design
  matrices. The tab boundary is doing real work and should stay sharp.
- **The source term is prose, not a concentration.** "One actor, 10^6 alarms" names who emits
  the work; it does not give an activity, a rate per pathway or an intake fraction. The
  exposure is qualitative where a real exposure model would be quantitative, and the fields
  must not be read as if they had units.
- **Effect is per isotope, not per channel.** A class can have both kinds at once: filter
  saturation has a deterministic repair (870 ms rebuild) and a stochastic response (0.82% to
  54% FP). The isotope carries its dominant effect, so the other half is invisible at that
  level. Moving effect onto the channel is the obvious fix and would add 40 judgements, one
  per non-empty cell.

**Risks**

- **"Stable" read as "safe."** Mitigated three ways: the legend says a dashed rim means an
  open channel, the footer says schematic and no safety function, and §9 of the source
  document already refuses immunity. A stable isotope still absorbs; a closed cell still
  costs capacity.
- **Dose read as radiological dose.** The vocabulary is deliberate, so the risk is not that
  dosimetry appears but that a reader supplies a unit the model does not have. Mitigated by
  naming the dose in every detail pane — accumulated 42i distance, occupancy, work share — and
  by refusing Bq, Gy and Sv outright. The footer states both halves: the model is used, the
  physics is not.
- **A deterministic verdict read as a probability.** `●` is a threshold outcome, not a
  detection rate; only three of eleven isotopes have a stochastic effect. The detail pane labels
  which is which and quotes the measured number (0.82% FP, 99.9% in 10 turns, 54% at 4x), so a
  threshold cannot be averaged with a probability.
- **Drift against §2.** The durable fix is a JSON artifact emitted by
  `tools/design-matrices.py` and loaded by the app, which turns transcription into a build
  step. Not done here: it needs a bundle resource and a decode path, and this ADR is about
  the shape of the view.

## Alternatives considered

**A flow chart of the instrument** (sample → ionise → sector → cups → ratio). Rejected as the
primary representation: the matrix records verdicts, not flows, so every edge and every
branch width would be invented. Its one real contribution — interference between threats that
share a path — is a property the data does not measure, so drawing it would assert more than
§2 knows. It becomes the right figure the moment a concentration is measured on one of those
paths; until then the pathway is annotation.

**A second grid coloured by dose.** Rejected for the same reason as latency colouring: the
dose is cumulative 42i distance, which is a property of an agent over time, not of a
threat × mechanism cell. Drawing it on this lattice would put two different quantities on one
axis.

**A Sankey from threat to daughter.** Rejected for the same reason, more sharply: Sankey
widths are volumes, and the ledger has verdicts and latencies, not volumes.

**A second grid coloured by latency.** Rejected as duplication: §5 already exists as
`design-matrices-latency.svg`, and colouring the §2 grid by time would hide the verdict, which
is the primary content.

**A dispersive fan** (one beam, one prism, an arc of detectors). Not rejected — decided
separately in [ADR 0013](0013-the-stream-is-dispersed-not-filtered.md). It is the one figure
that needs no concentration, because its coordinate is an angle and the angle is measured.

**A WidgetKit extension now.** Deferred, not refused. It needs an app group and entitlements,
and it would be the third representation of static data in one app. The model is already pure,
so the extension is a `TimelineProvider` over `NuclideChart.elements` when there is a reason
to put it on the desktop.

## What this does not decide

- Whether the app reads a generated JSON artifact instead of a transcribed Swift table.
- Whether the widget extension ships, and whether its timeline ever carries live cluster
  state — a static chart gains nothing from a timeline today.
- Whether five classes is the right partition. It is editorial; the eleven rows are not.
- Whether the exposure pathway becomes geometry — source → medium → receptor — rather than
  four annotations per isotope. That waits on a measured concentration to put on an axis.
- Whether cumulative 42i distance is charted over time as a dose curve with the rung ceilings
  drawn as limits. That is a different figure and a different data source.
- Whether effect moves from the isotope to the channel, which would let a mixed response be
  drawn honestly instead of summarised.
- Whether the instrument diagram is added as a third figure (see alternatives).

## Verifying

```sh
cd ui/xcode/Lab
xcodebuild -project LezzSea.xcodeproj -scheme LezzSea -destination 'platform=macOS' test
```

| claim | test |
| --- | --- |
| every §2 row appears once, in document order | `elevenRowsInDocumentOrder` |
| every channel names a real mechanism column | `everyChannelIsAColumn` |
| no threat is left with no channel at all | `noThreatIsUnaddressed` |
| `cohort eviction` keeps its open side channel and still reports closed | `cohortEvictionKeepsItsOpenChannel` |
| `gate enumeration` has exactly one channel | `gateEnumerationHasOneChannel` |
| a class reports its best channel, not its first | `bestChannelWins` |
| the lattice is as wide as its widest element | `widestElementSetsTheColumns` |
| every isotope names a source term, pathway, dose and effect | `everyIsotopeNamesItsExposure` |
| both effect kinds survive the model | `bothEffectKindsArePresent` |

The source of every channel and label is `docs/design-matrices.md` §2; the residence figures
are §5; the dose figures are §7 and the 42i weights. If those tables change, this table
changes, and the tests fail loudly on the parts they pin.
