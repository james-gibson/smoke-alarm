# ADR 0016 — Observation is a protocol role: the debug golden

- **Status:** Proposed
- **Date:** 2026-10-22
- **Related:** [ADR 0001](0001-alarm-membership-tiers.md), [ADR 0002](0002-golden-is-a-derived-reference.md), [ADR 0010](0010-trust-rotates-with-the-layout.md), [ADR 0014](0014-polarity-is-a-correlation-channel.md), [ADR 0015](0015-the-dashboard-sees-rates-not-identities.md)

## Context

Debugging the protocol needs a debugger. A dashboard that sees rates
([ADR 0015](0015-the-dashboard-sees-rates-not-identities.md)) is the right thing to ship and
the wrong thing to debug with, for the same reason a voltmeter is not a logic analyser: it
tells you the shape of the work, never which path a record took.

The observation that makes this tractable is the user's: **if you do not control the golden
cluster's ids, you cannot see the paths; and the golden cluster can only get data if the
protocol requires it.**

Both halves are structural:

- **Control of the ids is visibility.** Golden is a *derived* reference
  ([ADR 0002](0002-golden-is-a-derived-reference.md)): the route set is `F(epoch)`, not a
  stored table. Whoever holds the derivation holds the whole namespace, and can recompute any
  route rather than being told about it. So "see everything" is not a permission — it is
  possession of the derivation function.
- **Position without delivery buys nothing.** A member sees what the wire sends it. If the
  protocol does not carry per-route records to a member, being in the best seat in the cluster
  changes nothing. Observation is a protocol property, not a vantage point.

The circuit-board prior art is exact. A debugger needs pads on the board, a header, and a
part that is not in secure-boot lockdown. You cannot attach a probe to a board that did not
route the trace, no matter how close you hold it. **The pads are the protocol.**

There is also a quantity that is easy to miss: membership buys `1/n`, not `1`. A golden member
owns a shard, so a single member observes a fraction of the routes, and data residence is one
tick ([ADR 0010](0010-trust-rotates-with-the-layout.md)), so it observes that fraction for one
tick. "Everything" requires being the whole golden cluster, or all shards local.

## Decision

**`lezz.sea` may act as a golden cluster only under a declared debug profile on a dev cluster
with synthetic data. Observation is a protocol role with a handshake, not a privilege of
proximity; the production protocol must not require per-route delivery to an observer.**

### 1. Four roles, four visibilities

The visibility ladder is the cost ladder of [ADR 0001](0001-alarm-membership-tiers.md), read
from the top:

| role | what it can derive | what it sees | what it costs |
| --- | --- | --- | --- |
| outsider | nothing | the alarm, priced at distance | 7 vs 24 |
| member | its own shard | its routes, for one tick | `1/n` duty, quorum |
| golden member | the shard's derivation | every record in that shard, as derived | rebuilds, storage, quorum |
| the whole golden cluster | the whole derivation | every route in the namespace | 2 members on local stores, or 4 through a shared one |
| certifier | other agents' isotopes | who may certify whom | rung 6 authority |

`lezz.sea` as a gold cluster sits on the fourth row. That is not a bigger window on the same
data; it is a different relationship to it — the difference between watching a calculation and
holding the key to it.

### 2. The debug profile is declared, synthetic, and badged

A debug golden is admissible when all of the following hold, and not otherwise:

- the cluster declares `debug_observer: true` at boot, so the role is in the protocol and
  visible in `/status`, not inferred by the client;
- the data is synthetic: no real routes, no real isotope preimages, no real payloads;
- the observer has **no certification rights** — rung 6 authority stays with the cluster's own
  members, so a debugger cannot promote anything;
- the derivation key is provisioned at launch, held in memory, never persisted, never
  committed, and never written to a log;
- the UI carries the badge for as long as the role is held. A debugger that looks like a
  dashboard is how a debug cluster ends up in front of a customer.

### 3. What the debug golden is actually for

Not "seeing more". The output that justifies the role is a **derivation diff**: the observer
computes `F(epoch)` independently and compares its state against the cluster's. Divergence
means the derivation is wrong, which today is only detectable after it has produced a security
event — a demotion, a consensus gap, a rebuild that repeats.

That is the logic-analyser move: attach to the bus, sample the same edges as the part, and
compare. It is also the only way to populate an *observed* column in
`docs/design-matrices.md` §7 beside the documented one — the ledger is currently a table of
claims from ADRs, and a debug golden can turn it into a table of claims plus measurements.

### 4. Production must not require the role

The reversible mistake is making per-route delivery mandatory "so we can observe". That would
put a correlator in every deployment and void [ADR 0015](0015-the-dashboard-sees-rates-not-identities.md)
in the same commit. The production protocol carries aggregates; per-route records go to
members because members must derive them, not because an observer wants them.

## Consequences

**Positive**

- The protocol gets a real debugger: derivation diffing, path visibility, consensus gaps, and
  the ability to answer "which record took which path" without a production correlator.
- The role is explicit in `/status`, so "is this cluster being observed" is a query, not a
  guess.
- The design matrices gain an observed column, which is the difference between a specification
  and a measurement.
- The JTAG analogy gives the boundary a shape: pads, header, lockdown — protocol, declaration,
  key custody.

**Negative and costs**

- **The debug golden holds the namespace key.** That is total visibility by construction, and
  no UI can make it partial. Custody is a launch-time secret, and a debug build that is
  screenshotted, shipped, or logged is a namespace disclosure.
- **It is a quorum member.** Adding an observer to a cluster changes availability arithmetic
  (`1 - p^k`) and rebuild timing; a debug cluster is not the production cluster with more logs.
- **It sees one tick at a time**, even as the whole cluster, because residence is one tick. A
  debugger that wants history must add it deliberately and own that decision.
- **Two profiles, one binary.** The role must be compiled in but inert by default, which is
  exactly the kind of flag that gets flipped in the field.

**Risks**

- **Debug drift.** A `debug_observer: true` that survives into a demo, then a pilot, then a
  customer. Mitigations: the badge, the synthetic-data precondition, and no certification
  rights — but the real mitigation is that the key is provisioned per launch and never stored.
- **Observer as oracle.** A debug golden that answers questions about routes is a membership
  oracle with better branding. It should emit diffs and counts, not lookups.
- **The 1/n trap.** Running one golden member and concluding "the paths look fine" from a
  quarter of the namespace. The profile must state its coverage in the same badge.

## Alternatives considered

**A passive tap on the wire** (mirror the messages to the debugger). Rejected as the primary
mechanism: it requires the same protocol change as a declared role, gives no derivation to
diff against, and produces a packet stream that is harder to keep synthetic than a state
comparison.

**Log replay.** Rejected: the log is aggregate by design ([ADR 0015](0015-the-dashboard-sees-rates-not-identities.md)),
so replay cannot reconstruct a path. It is a record of what was shown, not of what happened.

**A remote debug port on every member.** Rejected: it is the correlator in production, with
worse authentication than the cluster already has.

**Let the dashboard join the cluster as an ordinary member.** Rejected: membership is `1/n` and
one tick, so it observes a fraction and cannot answer whole-namespace questions. Half a
debugger is worse than a declared one, because it looks like the real thing.

## What this does not decide

- Whether the debug profile is a separate build target, a launch flag, or a distinct binary.
  A separate binary is the safer default and the more annoying one.
- How the derivation key is provisioned (file, environment, keychain) and how it is destroyed.
- Whether the debug golden may certify, which this ADR forbids, or whether a long-lived
  "staging certifier" is a separate role with its own record.
- Whether observed counts are written back into `docs/design-matrices.md` by the generator, or
  kept as a separate artifact that the generator links.
- Whether the observer's coverage (which shards, which tick) is part of the badge or a separate
  panel.

## Verifying

| claim | test |
| --- | --- |
| non-members get the alarm, members get the station: visibility is tiered | `TestAlarmRotationsEvictTheCohort` |
| membership is what multiplies cost, so it is what buys visibility | `TestQuorumMultipliesTheAttackCost` |
| a golden member rebuilds from derivation rather than receiving records | `TestGoldenRebuildWinsTheRace` |
| adding a member changes availability arithmetic | `TestRouteReplicasFollowAvailabilityMath` |
| residence is one tick, so even a full observer sees one tick | `TestDataResidenceIsOneTick` |
| the fast path is local, so a debug golden must not become the path | `TestFastPathIsAnExclusionChannel` |

```sh
go test ./internal/mcp_faults/ -run 'AlarmRotations|Quorum|GoldenRebuild|RouteReplicas|DataResidence|FastPath' -count=1
```

**Not yet tested, and the gap that matters:** that a debug observer cannot certify, and that
the production protocol exposes no per-route delivery. Both are protocol properties with no
test today; the first is a rung check, the second is an absence that only a conformance test
can hold.
