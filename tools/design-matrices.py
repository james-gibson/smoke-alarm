#!/usr/bin/env python3
"""Generate docs/design-matrices.md and docs/design-matrices.svg from one data set.

Every number here is copied from the ADR that measured it, and every row names the test
that produces it. Regenerate with:  python3 tools/design-matrices.py
"""
import html
import math
import pathlib

MECHANISMS = [
    ("tiers", "membership tiers + pricing"),
    ("golden", "golden derived reference"),
    ("tick", "lagged tick window"),
    ("hydration", "predictive hydration"),
    ("churn", "churn yield at 50%"),
    ("isotropy", "isotropic name space"),
    ("convene", "convenience rule"),
    ("helix", "helix in the lift"),
    ("side", "side channel, 3 filters"),
]

# (threat, [cell per mechanism in MECHANISMS order])
# status: closed | partial | open | na ; label is short and factual.
MATRIX = [
    # Only cells a mechanism actually addresses are marked; a mechanism that merely bounds a
    # different threat is shown as not addressed rather than stretched.
    ("stale flood (10^6 alarms)", [
        ("closed", "plateau"), ("na", "-"), ("na", "-"), ("na", "-"), ("na", "-"),
        ("na", "-"), ("na", "-"), ("na", "-"), ("na", "-"),
    ]),
    ("pipelined flood", [
        ("closed", "stale by shape"), ("na", "-"), ("closed", "chain skew"), ("na", "-"), ("na", "-"),
        ("na", "-"), ("na", "-"), ("na", "-"), ("na", "-"),
    ]),
    ("filter saturation (4x)", [
        ("partial", "per-group"), ("closed", "870 ms"), ("na", "-"), ("na", "-"), ("na", "-"),
        ("na", "-"), ("na", "-"), ("partial", "period"), ("closed", "canary 16x"),
    ]),
    ("cohort eviction (30 rot)", [
        ("partial", "priced"), ("closed", "hydrate"), ("na", "-"), ("closed", "550 ms"), ("na", "-"),
        ("na", "-"), ("na", "-"), ("na", "-"), ("open", "not closed"),
    ]),
    ("stamp forgery (guess)", [
        ("closed", "7 tries"), ("na", "-"), ("na", "-"), ("na", "-"), ("na", "-"),
        ("closed", "2^32"), ("closed", "no format"), ("closed", "commit"), ("na", "-"),
    ]),
    ("replay across ticks", [
        ("closed", "issuance"), ("closed", "1 epoch"), ("na", "-"), ("na", "-"), ("na", "-"),
        ("na", "-"), ("na", "-"), ("closed", "no period"), ("partial", "flag only"),
    ]),
    ("competent derivation", [
        ("partial", "rate cap"), ("na", "-"), ("closed", "10 s win"), ("closed", "550 ms"), ("closed", "2x cap"),
        ("closed", "parity"), ("closed", "no claim"), ("closed", "attribution"), ("closed", "99% local"),
    ]),
    ("common-mode drift", [
        ("na", "-"), ("na", "-"), ("na", "-"), ("na", "-"), ("na", "-"),
        ("na", "-"), ("na", "-"), ("closed", "difference 2d"), ("na", "-"),
    ]),
    ("load surge (work > 50%)", [
        ("na", "-"), ("na", "-"), ("partial", "10 s"), ("closed", "1:6 min"), ("closed", "add capacity"),
        ("na", "-"), ("na", "-"), ("na", "-"), ("na", "-"),
    ]),
    ("wall-following probe", [
        ("na", "-"), ("na", "-"), ("na", "-"), ("na", "-"), ("na", "-"),
        ("closed", "469 ms"), ("closed", "5.2x"), ("closed", "isotropic"), ("na", "-"),
    ]),
    ("gate enumeration", [
        ("na", "-"), ("na", "-"), ("na", "-"), ("na", "-"), ("na", "-"),
        ("na", "-"), ("closed", "51x"), ("na", "-"), ("na", "-"),
    ]),
]

POLARITY = [
    ("parental filter (admission)", "(tick, rotation)", "admit a stranger",
     "only with golden repair", "cannot see replay across ticks"),
    ("replay filter", "rotation", "accuse an honest node", "never - flag only",
     "sees repeats, not intent"),
    ("canary filter", "route", "none - saturation is the signal", "yes, volume alarm", "no signal until 4n"),
    ("residue helix", "lift mod m", "none - detected by expectation", "yes, position", "period = lcm"),
    ("inversion gate", "issued value", "N/A - equality", "yes, receipt", "period 2 abelian"),
    ("bias test", "side choice", "none - statistical", "no - not an attestation", "needs 19 probes"),
]

COST = [
    ("tiers + pricing", "0 flags", "7 or 2 alarms/identity", "1 per alarm", "immediate", "no"),
    ("golden rebuild", "870 ms", "n x 32 B transfer", "0 messages", "870 ms", "no"),
    ("lagged tick", "0", "0", "0", "10 s window", "no"),
    ("hydration", "550 ms", "n x 32 B", "0 messages", "550 ms lead", "no"),
    ("churn yield", "0", "26-32 bits/item", "0", "epoch x turns", "yes"),
    ("isotropy test", "0", "0", "0", "469 ms at 40/s", "yes - faster"),
    ("convenience rule", "0", "0", "0", "uniform refusal", "no"),
    ("helix residues", "0.78 ns", "3 words", "0 extra", "10 turns = 50 s", "no"),
    ("HMAC envelope", "478 ns", "0", "same message", "0", "no"),
    ("group commitment", "47 ns", "1 element", "same message", "0", "no"),
    ("canary filter", "0", "bits x 1/16 n", "0", "186 insertions", "yes"),
    ("side channel", "0", "10 bits/turn", "0 on 99%", "local", "no"),
]

LATENCY = [
    ("stale flood refusal", "0", "0", "no", "exact"),
    ("bias test (0.9 side bias)", "18.8 s", "469 ms", "yes - faster", "99.9% in 10 turns"),
    ("helix (3 residues)", "50 s", "50 s", "no", "exact per turn"),
    ("canary saturation", "minutes", "4.7 s", "yes - faster", "exact count"),
    ("golden rebuild", "870 ms", "870 ms", "no", "0.82% FP"),
    ("hydration lead", "550 ms", "550 ms", "no", "60 ticks of cold"),
    ("gate enumeration", "5 probes", "5 probes", "no", "exact"),
]

LEDGER = [
    ("alarms to evict a cohort", "30", "rotations", "0001", "TestAlarmRotationsEvictTheCohort"),
    ("cost of a forged stamp", "8", "distance", "0001", "TestHoneypotInvocationIsTheAlarmsReceipt"),
    ("cost of a direct call", "24", "distance", "0001", "TestQuorumMultipliesTheAttackCost"),
    ("cost of past-grace staleness", "2", "distance", "0001", "TestAlarmFloodSaturatesTheFilter"),
    ("budget cost of groups of 3", "1.33", "x membership", "0001", "TestGroupsOfThreeRestoreScaleInvariance"),
    ("global filter at 100 ingresses", "4", "% correct", "0001", "TestIngressParallelismMultipliesTheAttacker"),
    ("golden rebuild, warm", "870", "ms", "0002", "TestGoldenRebuildWinsTheRace"),
    ("rebuild, hashing only", "550", "ms", "0002", "TestGoldenRebuildWinsTheRace"),
    ("rebuilds absorbed by one tick", "5", "rebuilds", "0002", "TestRepairFitsTwiceInATick"),
    ("twice-over margin at 1M", "2.87", "x", "0002", "TestRepairFitsTwiceInATick"),
    ("twice-over holds to", "2.9", "M entries", "0002", "TestRepairFitsTwiceInATick"),
    ("rebuilt filter FP", "0.82", "%", "0002", "TestGoldenChangesThePayoffNotTheArithmetic"),
    ("saturated filter FP at 4x", "54", "%", "0002", "TestAlarmFloodSaturatesTheFilter"),
    ("content agreement cost", "0", "messages", "0002", "TestGoldenAnchorGivesContentAgreementForFree"),
    ("cold restore", "300", "s", "0002/0004", "TestColdGoldenIsTheOnlyWayToLose"),
    ("acceptance window", "10", "s", "0003", "TestTwoLiveEpochsDoubleTheAttackersCost"),
    ("hydration lead", "550", "ms", "0004", "TestGoldenSpawnsHydratedNodes"),
    ("hydration idle reduction", "545", "x", "0004", "TestGoldenSpawnsHydratedNodes"),
    ("cold spawn lateness", "60", "ticks", "0004", "TestGoldenSpawnsHydratedNodes"),
    ("churn setpoint", "50", "% work share", "0005", "TestFiftyPercentSetpointIsTheProvisioningRule"),
    ("provisioning rule", "2", "x work load", "0005", "TestFiftyPercentSetpointIsTheProvisioningRule"),
    ("bits per item at nominal churn", "26", "bits", "0005", "TestYieldingCostsBitsPerItem"),
    ("bits added per churn halving", "1-2", "bits", "0005", "TestYieldingCostsBitsPerItem"),
    ("replay-safe yield band", "0", "turns", "0005", "TestTheYieldBandIsEmpty"),
    ("bias test at 0.9", "19", "probes", "0006", "TestWallFollowerIsCaughtByItsOwnBias"),
    ("bias test latency at 40/s", "469", "ms", "0006", "TestDetectionLatencyIsStepsOverRate"),
    ("quadratic subtlety cost", "4", "x per halving", "0006", "TestSubtletyCostsQuadratically"),
    ("random walk over 32 bits", "3.4", "years at 40/s", "0006", "TestRandomizedFollowingIsJustAGuess"),
    ("one-sided vs two-sided cost", "5.2", "x", "0006", "TestTheCheapOneSidedTestDoesNotApply"),
    ("invariant signal across 30 rotations", "1", "distinct answers", "0007", "TestStaticInvariantCannotMeasureRotation"),
    ("lazy node invariant health", "30/30", "ticks", "0007", "TestStaticInvariantCannotMeasureRotation"),
    ("refusal reasons on the wire", "5", "classes", "0007", "TestRefusalReasonIsAGuidanceOracle"),
    ("guidance per refusal", "2.32", "bits", "0007", "TestRefusalReasonIsAGuidanceOracle"),
    ("blindness multiplier at 5 gates", "51", "x", "0007", "TestGuidanceIsAdditiveWhereBlindnessIsMultiplicative"),
    ("blindness multiplier at 8 gates", "2048", "x", "0007", "TestGuidanceIsAdditiveWhereBlindnessIsMultiplicative"),
    ("helix period at 2,3,5", "30", "turns", "0008", "TestCoprimeHelicesReconstructTheIndex"),
    ("helix period at 2,3,5,7", "210", "turns", "0008", "TestCoprimeHelicesReconstructTheIndex"),
    ("attribution delta (path 2,3,5)", "15/20/6", "index steps", "0008", "TestTheDeltaNamesTheLiar"),
    ("non-coprime pair 2,4 period", "4", "turns", "0008", "TestNonCoprimeModuliWasteAPath"),
    ("inversion replay pass rate", "100", "%", "0008", "TestInversionHasPeriodTwo"),
    ("non-abelian periods", "4 and 3", "turns", "0008", "TestNonAbelianConjugationSetsThePeriod"),
    ("residue cost", "0.78", "ns", "0008", "BenchmarkResidue"),
    ("HMAC-SHA256 cost", "478", "ns", "0008", "BenchmarkHMACSHA256"),
    ("61-bit modexp cost", "47", "ns", "0008", "BenchmarkGroupCommit"),
    ("helix CPU duty at 5 s", "0.0001", "% of core", "0008", "TestCommitCostsAreIrrelevantAtThisEpoch"),
    ("phase 1/30 order", "30", "steps", "0008", "TestPhasesAreAQuotientAndHaveTorsion"),
    ("phase-keyed filter after a wrap", "30/30", "false returns", "0008", "TestFilterOverTheQuotientSaturates"),
    ("fast path resolved locally", "99", "%", "0008", "TestFastPathIsAnExclusionChannel"),
    ("canary warning lead at 1/16", "16.1", "x earlier", "0009", "TestCanaryFilterWarnsSixteenTimesEarlier"),
    ("design FP, full and canary", "0.82", "%", "0009", "TestCanaryFilterWarnsSixteenTimesEarlier"),
    ("slow-path escalation rate", "1.05", "%", "0009", "TestSideChannelDrainsTheUnexaminedBacklog"),
    ("flooded canary FP", "87.7", "%", "0009", "TestPerRouteCanariesLocalizeAFlood"),
    ("derived durable write, 1M routes", "65", "ms/tick", "0002", "TestDerivedPathFitsTheTickTwiceOver"),
    ("materialized durable write", "11.3", "s = 2.26 ticks", "0002", "TestMaterializedPathBreaksTheTick"),
    ("store rate for twice-over", "400", "k rows/s", "0002", "TestMaterializedPathBreaksTheTick"),
    ("filter artifact at 26 bits/item", "3.25", "MB", "0002", "TestDerivedPathFitsTheTickTwiceOver"),
    ("cold restore from object store", "10.3", "s = 2.06 ticks", "0002", "TestColdRestoreFromObjectStore"),
    ("cold restore vs live repair", "12", "x", "0002", "TestColdRestoreFromObjectStore"),
    ("locality factor, local vs shared store", "3", "x rows/s", "0001", "TestMembershipPartitionsTheDurablePath"),
    ("members for twice-over, local stores", "2", "shards", "0001", "TestMembershipPartitionsTheDurablePath"),
    ("members for twice-over, shared store", "4", "shards", "0001", "TestMembershipPartitionsTheDurablePath"),
    ("member-local pipeline, 4 shards", "0.99", "s/tick", "0002", "TestMembershipTurnsElevenSecondsIntoOne"),
    ("membership improvement over shared", "11.4", "x", "0001", "TestMembershipTurnsElevenSecondsIntoOne"),
    ("verifier duty, single parity", "1/n", "of ticks", "0010", "TestLeftSymmetricRotatesTrustAndData"),
    ("control cost, k consecutive ticks", "k", "members", "0010", "TestTrustRotationCostsDuration"),
    ("control cost, dual parity", "2k", "members", "0010", "TestDualParityDoublesTheVerifierSet"),
    ("data residence per member", "1", "tick", "0010", "TestDataResidenceIsOneTick"),
    ("members to follow one shard", "n", "of n", "0010", "TestDataResidenceIsOneTick"),
    ("write amplification per-record durability", "625", "x", "0002", "TestWriteAmplificationIsTheRealConstraint"),
    ("RAID-5 rebuild, n=4", "320", "ms", "0002", "TestParityRebuildVersusDerivation"),
    ("derivation rebuild (no transfer)", "550", "ms", "0002", "TestParityRebuildVersusDerivation"),
    ("availability, 2 route replicas", "99.99", "%", "0011", "TestRouteReplicasFollowAvailabilityMath"),
    ("resize churn, modulo placement", "80.2", "% keys moved", "0011", "TestResizeCostDependsOnTheFunction"),
    ("resize churn, rendezvous placement", "19.7", "% keys moved", "0011", "TestResizeCostDependsOnTheFunction"),
    ("blocks self-served at 1 replica", "25", "%", "0011", "TestSelfServeConflictAtBlockGranularity"),
    ("directory entries for placement", "0", "entries", "0011", "TestPlacementIsByFunctionNotDirectory"),
    ("shared filter after same traffic", "0.000", "FP", "0009", "TestPerRouteCanariesLocalizeAFlood"),
]

DECISIONS = [
    ("0001", "membership tiers: non-members get the alarm, members the station", "30 rotations to evict; 8 vs 24 pricing", "TestAlarmRotationsEvictTheCohort"),
    ("0002", "golden is a derived reference, not a snapshot", "870 ms rebuild; twice over in a 5 s tick", "TestRepairFitsTwiceInATick"),
    ("0003", "one lagged tick: a 10 s acceptance window", "window = 2 epochs = 10 s", "TestTwoLiveEpochsDoubleTheAttackersCost"),
    ("0004", "predictive hydration from the next tick", "550 ms lead vs 60 ticks cold", "TestGoldenSpawnsHydratedNodes"),
    ("0005", "punish floods, reward derivation", "churn >= work: 2x provisioning", "TestFiftyPercentSetpointIsTheProvisioningRule"),
    ("0006", "the name space is isotropic", "19 probes, 469 ms at 40/s", "TestWallFollowerIsCaughtByItsOwnBias"),
    ("0007", "conveniences are attack surfaces; harden on introduction", "5 reasons = 2.32 bits/probe; 51x at 5 gates", "TestRefusalReasonIsAGuidanceOracle"),
    ("0008", "rotation state as a helix in the torsion-free lift", "period 30 at {2,3,5}; no period in the lift", "TestCoprimeHelicesReconstructTheIndex"),
    ("0009", "the side channel is three filters with three polarities", "16.1x canary lead; 1.05% escalation", "TestCanaryFilterWarnsSixteenTimesEarlier"),
    ("0010", "trust rotates with the data layout (parity schedule)", "duty 1/n; k ticks cost k members", "TestTrustRotationCostsDuration"),
    ("0011", "redundancy is a route property: replicas, placement, blocks", "2 replicas = 99.99%; rendezvous moves 19.7%", "TestResizeCostDependsOnTheFunction"),
]

STATUS_COLOR = {
    "closed": "#2f7d4f",
    "partial": "#b8860b",
    "open": "#a33",
    "na": "#e8e8e8",
}


def table(headers, rows):
    out = ["| " + " | ".join(headers) + " |", "|" + "|".join(["---"] * len(headers)) + "|"]
    for row in rows:
        out.append("| " + " | ".join(str(c) for c in row) + " |")
    return "\n".join(out)


def markdown():
    parts = []
    parts.append("""# Design matrices — every number in one place

A consolidation, not a new decision. Each cell names the ADR that measured it and the test
that produces it; the tests are the source of truth and this page is the index.

Generated by `tools/design-matrices.py`. Two diagrams are emitted from the same data:
`design-matrices.svg` (the threat × mechanism grid, coloured by status) and
`design-matrices-latency.svg` (latency against load, log scale).

""")

    parts.append("## 1. Decisions at a glance\n")
    parts.append(table(["ADR", "decision", "load-bearing number", "test"],
                       [[f"[{a}](adr/{a}-{ADR_SLUG[a]}.md)", d, n, f"`{t}`"]
                        for a, d, n, t in DECISIONS]))
    parts.append("")

    parts.append("## 2. Threat × mechanism\n")
    parts.append("`●` closed, `◐` partial, `·` not addressed. The short token is the mechanism's\ncontribution, not a claim of immunity.\n")
    head = ["threat"] + [name for _, name in MECHANISMS]
    rows = []
    for threat, cells in MATRIX:
        row = [threat]
        for status, label in cells:
            mark = {"closed": "●", "partial": "◐", "open": "○", "na": "·"}[status]
            row.append(mark if label == "-" else f"{mark} {label}")
        rows.append(row)
    parts.append(table(head, rows))
    parts.append("")

    parts.append("## 3. Error polarity\n")
    parts.append(table(["mechanism", "key", "false positive means", "may decide?", "known limit"],
                       [list(r) for r in POLARITY]))
    parts.append("")

    parts.append("## 4. Cost and timing\n")
    parts.append(table(["mechanism", "time", "memory", "messages", "latency", "load-sensitive"],
                       [list(r) for r in COST]))
    parts.append("")

    parts.append("## 5. Detection latency vs load\n")
    parts.append(table(["check", "at 1/s", "at 40/s", "improves with load?", "exactness"],
                       [list(r) for r in LATENCY]))
    parts.append("")

    parts.append("## 6. Tick budget at 10^6 routes\n")
    parts.append("One cycle of a golden cluster tracking a million routes, with durability. The\nbranch is whether the durable path carries the records or the derivation.\n")
    parts.append(table(["stage", "derived path", "one shared store", "4 member-local stores"], [
        ["derive records", "-", "100 ms", "100 ms (parallel)"],
        ["hash into the filter", "-", "550 ms", "550 ms (parallel)"],
        ["SQLite commit", "0.1 ms (manifest row)", "**10.0 s** (1 M rows at 100 k rows/s)", "0.83 s (250 k rows each)"],
        ["object store, 2 copies", "65 ms (3.25 MB filter)", "640 ms (32 MB)", "160 ms (8 MB slice each)"],
        ["**total**", "**65 ms**", "**11.3 s = 2.26 ticks**", "**0.99 s = 0.20 ticks**"],
        ["twice-over margin", "38x", "**fails** (needs 22.6 s)", "**2.5x**"],
    ]))
    parts.append("")
    parts.append("Twice-over holds at **2 members** on local stores or **4** through a shared one; "
                 "total bytes shipped are unchanged, so the gain is locality and parallelism, not less work.")
    parts.append("")
    parts.append(table(["path", "cost", "relation to the 5 s tick"], [
        ["live repair from derivation", "870 ms", "fits, twice over"],
        ["durable write per tick", "65 ms", "77x inside a tick"],
        ["cold restore from the store", "10.3 s", "2.06 ticks, 12x the live repair"],
    ]))
    parts.append("")

    parts.append("## 7. The ledger\n")
    parts.append(table(["quantity", "value", "unit", "ADR", "test"],
                       [[q, f"**{v}**", u, a, f"`{t}`"] for q, v, u, a, t in LEDGER]))
    parts.append("""

## 8. Diagrams

![Threat by mechanism grid, coloured by whether each mechanism closes, partially closes, or does not address the threat](design-matrices.svg)

![Detection latency against arrival rate, log scale, showing that sample-based checks accelerate with load and turn-based checks do not](design-matrices-latency.svg)

The **isotope view** is a third reading of the same data: each threat class resolved into its
variants, with every mechanism drawn as a decay channel and a daughter state. It is decided in
[ADR 0012](adr/0012-the-isotope-view-is-the-chart-of-the-nuclides.md) and rendered as a
standalone view in `ui/xcode/Lab/LezzSea`.

The **dispersion** reading — prism, rotation, reference arm — is the fourth: mechanisms
deflect rather than block, and the deflection angle is 42i distance. It is decided in
[ADR 0013](adr/0013-the-stream-is-dispersed-not-filtered.md).

## 9. What the matrices say together

- **Every mechanism is a rate discipline.** Nothing in the grid closes a threat absolutely;
  each cell is a factor or a repair time.
- **Three guarantees, three budgets.** Route replicas buy availability (2 = 99.99%, the knee),
  rendezvous placement buys addressability with no directory and 19.7% churn on resize, and
  parity buys block durability. Conflating them yields high availability with lost data: two
  replicas and no parity still lose a quarter of a block on one member loss.
- **Trust is placed by the same function as the data.** Left-symmetric parity gives every
  member the verifier role 1/n of the time, makes self-verification structurally impossible,
  and turns control duration into a cost: k consecutive ticks needs k members where a static
  verifier needs one. Data residence is a single tick, so following one shard needs the whole
  membership.
- **Membership is the missing dimension.** The records do fit - not by storing fewer, but by
  partitioning across tracked members: 11.3 s shared becomes 0.99 s at four shards, because
  the critical path is the slowest writer, not the sum. Two members suffice on local stores
  (3x faster than a coordinated shared one); four if everyone shares. That is what joining
  buys, and why being tracked is the price of the fast path.
- **The durable path cannot carry the records.** At a million routes per cycle, writing rows
  through SQLite is 10 s of an 11.3 s pipeline, which needs a store sustaining 400 k rows/s
  to fit twice over. The durable artifact has to be the derivation and the filter: 65 ms, 38x
  margin. Cold restore then costs 2.06 ticks, which is acceptable only because repair never
  reads the store.
- **The tick is a budget, not a convention.** A full rebuild at 10^6 entries is 870 ms, so
  `2 x 870 ms = 1.74 s` fits a 5 s epoch with a 2.87x margin, and one tick absorbs five
  rebuilds. That is what makes "a collapse is fixable before the next tick, twice over" a
  number rather than an intention; it holds to 2.9M entries and fails beyond them. Hydration
  is the cheaper 550 ms figure because it re-derives locally, with no transfer.
- **The open cells are honest.** Cohort eviction is not prevented by the side channel; a
  competent deriver is capped, not excluded; the refusal oracle is specified and not yet
  implemented.
- **Polarity, not strength, is the recurring hazard.** Five of the six rows in matrix 3 have
  a false positive that means something *different*, and two of them invert the security
  meaning of the same bit.

## Verifying

```sh
go test ./internal/mcp_faults/ -count=1
python3 tools/design-matrices.py   # regenerates this page and both diagrams
```
""")
    return "\n".join(parts)


ADR_SLUG = {
    "0001": "alarm-membership-tiers",
    "0002": "golden-is-a-derived-reference",
    "0003": "lagged-tick-window",
    "0004": "predictive-hydration",
    "0005": "punish-floods-reward-derivation",
    "0006": "the-name-space-is-isotropic",
    "0007": "conveniences-are-attack-surfaces",
    "0008": "rotation-as-a-helix-of-residues",
    "0009": "the-side-channel-three-filters",
    "0010": "trust-rotates-with-the-layout",
    "0011": "redundancy-is-a-route-property",
}


SHORT = ["tiers", "golden", "tick", "hydration", "churn", "isotropy", "conv", "helix", "side"]


def svg():
    cw, ch = 122, 40
    left, top = 260, 86
    cols = len(MECHANISMS)
    width = left + cw * cols + 24
    grid_h = ch * len(MATRIX)
    height = top + grid_h + 210
    out = [
        f'<svg xmlns="http://www.w3.org/2000/svg" width="{width}" height="{height}" '
        f'viewBox="0 0 {width} {height}" font-family="ui-monospace, SFMono-Regular, monospace">',
        f'<rect width="{width}" height="{height}" fill="#ffffff"/>',
        '<text x="18" y="32" font-size="17" font-weight="bold">Threat × mechanism</text>',
        '<text x="18" y="52" font-size="10" fill="#555">Every cell is a factor or a repair time, never immunity. Source: docs/design-matrices.md</text>',
    ]

    # Column headers, horizontal and short.
    for j, code in enumerate(SHORT):
        x = left + j * cw + cw // 2
        out.append(f'<text x="{x}" y="{top - 12}" font-size="10" font-weight="bold" '
                   f'text-anchor="middle" fill="#222">{code}</text>')
    out.append(f'<line x1="18" y1="{top - 4}" x2="{width - 18}" y2="{top - 4}" stroke="#999"/>')

    for i, (threat, cells) in enumerate(MATRIX):
        y = top + i * ch
        out.append(f'<text x="{left - 12}" y="{y + ch // 2 + 4}" font-size="10" '
                   f'text-anchor="end" fill="#111">{html.escape(threat)}</text>')
        for j, (status, label) in enumerate(cells):
            x = left + j * cw
            out.append(f'<rect x="{x}" y="{y}" width="{cw - 3}" height="{ch - 3}" '
                       f'fill="{STATUS_COLOR[status]}" fill-opacity="0.9" stroke="#fff" stroke-width="2"/>')
            if label == "-":
                continue
            ink = "#ffffff" if status in ("closed", "open") else "#1a1a1a"
            mark = {"closed": "●", "partial": "◐", "open": "○", "na": "·"}[status]
            out.append(f'<text x="{x + 7}" y="{y + 16}" font-size="11" fill="{ink}">{mark}</text>')
            out.append(f'<text x="{x + (cw - 3) // 2}" y="{y + 27}" font-size="8" '
                       f'text-anchor="middle" fill="{ink}">{html.escape(label)}</text>')

    # Legend below the grid: the codes, and the status scale.
    ly = top + grid_h + 28
    out.append(f'<line x1="18" y1="{ly - 16}" x2="{width - 18}" y2="{ly - 16}" stroke="#999"/>')
    out.append(f'<text x="18" y="{ly}" font-size="10" font-weight="bold">columns</text>')
    for j, ((_, full), code) in enumerate(zip(MECHANISMS, SHORT)):
        row, col = j % 3, j // 3
        x = 18 + col * 460
        y = ly + 18 + row * 20
        out.append(f'<text x="{x}" y="{y}" font-size="10" fill="#333">{code} = {html.escape(full)}</text>')
    sy = ly + 18 + 3 * 20 + 10
    out.append(f'<text x="18" y="{sy}" font-size="10" font-weight="bold">status</text>')
    for k, (status, word) in enumerate([("closed", "closed by this mechanism"), ("partial", "partially closed"), ("open", "not closed / open risk"), ("na", "not addressed")]):
        x = 18 + k * 300
        mark = {"closed": "●", "partial": "◐", "open": "○", "na": "·"}[status]
        out.append(f'<rect x="{x}" y="{sy + 6}" width="14" height="12" fill="{STATUS_COLOR[status]}" '
                   f'fill-opacity="0.9" stroke="#fff"/>')
        out.append(f'<text x="{x + 20}" y="{sy + 16}" font-size="10" fill="#333">{mark} {word}</text>')
    out.append("</svg>")
    return "\n".join(out)


LATENCY_CHART = [
    ("bias test, side bias 0.9", 18.8, 0.469),
    ("helix, 10 turns", 50.0, 50.0),
    ("canary saturation", 300.0, 4.7),
    ("golden rebuild", 0.55, 0.55),
    ("hydration spawn", 0.55, 0.55),
]


def svg_latency():
    left, top, plot_w, row_h = 240, 96, 620, 46
    width = left + plot_w + 150
    height = top + row_h * len(LATENCY_CHART) + 150
    lo, hi = 0.1, 400.0

    def x(t):
        ratio = (math.log10(max(t, lo)) - math.log10(lo)) / (math.log10(hi) - math.log10(lo))
        return left + ratio * plot_w

    out = [
        f'<svg xmlns="http://www.w3.org/2000/svg" width="{width}" height="{height}" '
        f'viewBox="0 0 {width} {height}" font-family="ui-monospace, SFMono-Regular, monospace">',
        f'<rect width="{width}" height="{height}" fill="#ffffff"/>',
        '<text x="18" y="32" font-size="17" font-weight="bold">Detection latency vs load</text>',
        '<text x="18" y="52" font-size="10" fill="#555">'
        'Log scale, seconds. Sample-based checks accelerate with load; turn-based checks do not.</text>',
    ]

    for tick in [0.1, 0.5, 1, 5, 10, 50, 100, 400]:
        gx = x(tick)
        out.append(f'<line x1="{gx:.1f}" y1="{top - 8}" x2="{gx:.1f}" y2="{top + row_h * len(LATENCY_CHART)}" stroke="#e2e2e2"/>')
        label = f"{tick:g}"
        out.append(f'<text x="{gx:.1f}" y="{top + row_h * len(LATENCY_CHART) + 18}" font-size="9" '
                   f'text-anchor="middle" fill="#666">{label}s</text>')

    for i, (name, slow, fast) in enumerate(LATENCY_CHART):
        y = top + i * row_h
        out.append(f'<text x="{left - 14}" y="{y + 22}" font-size="10" text-anchor="end" fill="#111">{html.escape(name)}</text>')

        for value, colour, dy, tag in ((slow, "#b8860b", 6, "1/s"), (fast, "#2f7d4f", 24, "40/s")):
            out.append(f'<rect x="{left}" y="{y + dy}" width="{x(value) - left:.1f}" height="14" '
                       f'fill="{colour}" fill-opacity="0.9"/>')
            out.append(f'<text x="{x(value) + 6:.1f}" y="{y + dy + 11}" font-size="9" fill="#333">'
                       f'{value:g}s ({tag})</text>')

    ly = top + row_h * len(LATENCY_CHART) + 48
    out.append(f'<line x1="18" y1="{ly - 16}" x2="{width - 18}" y2="{ly - 16}" stroke="#999"/>')
    out.append(f'<rect x="18" y="{ly - 4}" width="14" height="12" fill="#b8860b" fill-opacity="0.9"/>')
    out.append(f'<text x="38" y="{ly + 6}" font-size="10" fill="#333">at 1 probe/s</text>')
    out.append(f'<rect x="150" y="{ly - 4}" width="14" height="12" fill="#2f7d4f" fill-opacity="0.9"/>')
    out.append(f'<text x="170" y="{ly + 6}" font-size="10" fill="#333">at 40 probes/s (the per-claimant ceiling)</text>')
    out.append(f'<text x="18" y="{ly + 30}" font-size="10" fill="#555">'
               'Only the bias test and the canary improve with load: they convert samples into evidence. '
               'The helix is a schedule, so its latency is turns x epoch.</text>')
    out.append("</svg>")
    return "\n".join(out)


root = pathlib.Path(__file__).resolve().parent.parent
(root / "docs" / "design-matrices.md").write_text(markdown())
(root / "docs" / "design-matrices.svg").write_text(svg())
(root / "docs" / "design-matrices-latency.svg").write_text(svg_latency())
print("wrote docs/design-matrices.md, docs/design-matrices.svg, docs/design-matrices-latency.svg")
