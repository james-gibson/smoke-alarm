# ADR 0020 — The boundary is the interface

- **Status:** Proposed
- **Date:** 2026-10-23
- **Related:** [ADR 0011](0011-redundancy-is-a-route-property.md), [ADR 0014](0014-polarity-is-a-correlation-channel.md), [ADR 0015](0015-the-dashboard-sees-rates-not-identities.md), [ADR 0017](0017-a-maladaptive-cluster-is-a-lab-strain.md), [ADR 0019](0019-lab-results-import-as-priors.md)

## Context

A controller of `lezz.sea` is more credible for what the app **refuses** than for what it can do.
Any dashboard can display a cluster; very few can show, on the screen, the rule that stops the
dashboard from becoming a surveillance surface, and the price of removing it.

The design already behaves this way in the documents. §9 of the matrices says "the open cells
are honest": the one cell nobody closed is drawn, not omitted. ADR 0017 makes a lab a different
protocol surface. ADR 0019 makes an import a bounded prior. All three are claims of the form
*this cannot happen here, and here is the mechanism that prevents it.*

The app has not said that. It shows data and modes; it does not show the invariants that make
the data safe to show, and it does not show what a bypass would cost. So the operator's strongest
argument — "this is what it will not do" — is made in prose, in a repo, that the user of the app
will never read.

This ADR makes the boundary a visible, first-class part of the interface.

## Decision

**The app must demonstrate its own constraints: show the invariant, show the test that holds it,
and show the price of breaking it. The interface may never be more permissive than the protocol.**

### 1. Three primitives

| primitive | what it renders | why it is not a tooltip |
| --- | --- | --- |
| **absence** | a control that does not exist, with the rule that removes it | a hidden control implies the capability exists and is merely off |
| **price** | what breaking the rule would require: new identity, lost certification, key custody, quorum change | "why can't I" is answered with a cost, not a refusal |
| **attribution** | which test or conformance check holds the rule | a claim without an evidence pointer is marketing |

### 2. The rule: no UI verb without a protocol verb

If the protocol has no observer verb, the app has no "observe" control — not disabled, absent,
with the absence explained. A greyed button that says "not available" is already too permissive:
it suggests the capability exists. A line that says *production has no per-route delivery verb*
is the truth.

This is the interface half of [ADR 0017](0017-a-maladaptive-cluster-is-a-lab-strain.md)'s
attestation argument, and it is checkable: every control maps to a verb, and every verb is in the
capability manifest.

### 3. The invariants panel

The concrete instance, and the reason this ADR exists: a read-only panel listing the rules that
make the rest of the app safe, each with the mechanism that holds it and the failure it prevents.

| invariant | held by | broken means |
| --- | --- | --- |
| data residence is one tick | `TestDataResidenceIsOneTick` | a correlation channel across turns |
| placement is a pure function | `TestPlacementIsByFunctionNotDirectory` | a directory, and an enumeration surface |
| production has no per-route delivery | conformance (no test yet) | a correlator in every deployment |
| the helix advances one per tick | `TestCoprimeHelicesReconstructTheIndex` | period 30 → 15, 10 or 6 |
| lab results import as priors | conformance (the import path does not exist yet) | authority laundering through a coefficient |
| redundancy is three budgets | `TestRouteRedundancyIsNotBlockDurability` | availability with lost data |

Two entries have no test. They are shown as **conformance gaps**, not as passes, because a panel
that renders an untested claim as held is worse than no panel.

### 4. It must not become a tutorial

Naming the cost of a bypass is one step from describing how to perform it. The rule: the panel
names the *price*, never the *procedure*. "A lab cluster forfeits certification and holds the
namespace key" is a price. "Set `debug_observer` and mint a key with…" is a procedure, and it
does not belong in the UI of the production app.

## Consequences

**Positive**

- The operator can show a sceptic the constraint on the screen, with the test that holds it.
- The panel doubles as an honest gap list: the two conformance entries are visible debt.
- It enforces a discipline on the UI: no control without a verb, no claim without an evidence
  pointer.
- It makes the mode unmistakable, which is the failure [ADR 0017](0017-a-maladaptive-cluster-is-a-lab-strain.md)
  worried about.

**Negative and costs**

- **Less impressive.** A list of things the app will not do is not a demo. That is the point, and
  it is a cost.
- **A greyed absence looks like a bug.** Users report missing controls. The copy has to carry the
  reason, and the reason has to be true.
- **Static claims go stale.** The panel names tests that exist today; if a test is renamed or
  removed, the panel lies. It needs the same discipline as the matrices — the test is the
  authority, the panel is an index.
- **Two of six entries are gaps.** Shipping a panel that admits this is unusual, and correct.

**Risks**

- **Boundary theatre.** A panel that lists invariants nobody checks is worse than silence, because
  it manufactures confidence. Mitigation: every row links to a test or is labelled a gap.
- **Price as procedure.** Covered in §4; the line is thin and needs review, not a style guide.
- **Refusal logging becomes a store.** If refusals are recorded to show "what was blocked", that
  is a history ([ADR 0015](0015-the-dashboard-sees-rates-not-identities.md)) and the join returns
  through the log. Refusals are rendered, not retained.

## Alternatives considered

**Hide the constraints; document them in the repo.** Rejected: the person who needs to believe the
app is safe is not reading the ADRs. Documentation does not travel with the binary.

**A "developer mode" switch that reveals everything.** Rejected: that is the flag that drifts
([ADR 0017](0017-a-maladaptive-cluster-is-a-lab-strain.md)), and it puts a bypass one toggle from
production.

**Show a green "all invariants hold" badge.** Rejected: it is a claim the app cannot verify at
runtime — the tests run in CI, not in the app — so it would be boundary theatre by construction.

**Make the panel interactive (let the operator try a bypass).** Rejected: an interactive bypass is
a procedure, and a procedure is the thing §4 forbids.

## What this does not decide

- Exact copy for each price line, which needs review against §4.
- Whether the panel also shows live mode (prod/lab) from the cluster, or only the documented rules.
  Live mode is a status; documented rules are an index, and they should not be mixed silently.
- Whether the conformance gaps get tests, and what a conformance test for an absence looks like.
- Whether the panel is a tab, a sheet, or a section of the isotope view.

## Verifying

| claim | source |
| --- | --- |
| the panel's rows name real tests | `LezzSeaTests/BoundariesTests.swift` — every `.test` row is non-empty, every gap is labelled |
| residence is one tick | `TestDataResidenceIsOneTick` |
| placement is a pure function | `TestPlacementIsByFunctionNotDirectory` |
| the helix period is a property of the modulus set | `TestCoprimeHelicesReconstructTheIndex` |
| availability is not durability | `TestRouteRedundancyIsNotBlockDurability` |

```sh
go test ./internal/mcp_faults/ -run 'DataResidence|PlacementIsByFunction|Coprime|RouteRedundancy' -count=1
cd ui/xcode/Lab && xcodebuild -project LezzSea.xcodeproj -scheme LezzSea -destination 'platform=macOS' test
```

Note what the panel cannot do: it cannot verify the tests at runtime, so it does not claim they
pass. It names them, and the CI is the authority.
