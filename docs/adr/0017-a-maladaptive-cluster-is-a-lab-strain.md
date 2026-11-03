# ADR 0017 — A maladaptive cluster is a lab strain

- **Status:** Proposed
- **Date:** 2026-10-23
- **Related:** [ADR 0007](0007-conveniences-are-attack-surfaces.md), [ADR 0014](0014-polarity-is-a-correlation-channel.md), [ADR 0015](0015-the-dashboard-sees-rates-not-identities.md), [ADR 0016](0016-observation-is-a-protocol-role.md)

## Context

[ADR 0016](0016-observation-is-a-protocol-role.md) said `lezz.sea` may hold the debug golden
role only on a cluster that declares it. The sharper statement, and the one this ADR adopts:

> **To observe, `lezz.sea` must change the rules. Only clusters that allow the change are
> approachable. That is the invariant, and it is what separates the lab from production.**

The sharper form, adopted below: **the lab is not a gatekeeper that refuses production —
production fails the lab's tests — and a cluster is subject to the lab's authority only where
it declares that authority.**

Two consequences follow, and neither is a configuration detail:

- **The rule change is a capability, not a setting.** A cluster that hands per-route records to
  an observer has a protocol verb that a production cluster does not have. That is a different
  protocol surface, not a different value in the same one.
- **A maladaptive cluster can therefore be launched, and it can never be trusted by
  production.** Not because production is told not to trust it, but because the thing that
  makes it useful for debugging is the same thing that fails production's admission.

The word *maladaptive* is exact. A lab strain is not a weaker production cluster; it is a
different fitness landscape. It optimises for visibility, and production selects for trust.
Nothing that wins in the lab should survive selection in production, and a design that lets it
is not a design.

## Decision

**The boundary is a test suite, not a refusal. Production fails the lab's tests because the
tests assert lab rules, and the lab's authority extends only to clusters that declare it. The
debug capability is attested by behaviour, not declared; the lab identity is a different
feature family, so lab evidence cannot certify production features.**

### 1. Mode is a rule set, not a flag

| | production | lab (launched by `lezz.sea`) |
| --- | --- | --- |
| per-route delivery to an observer | **verb does not exist** | verb exists |
| golden role for a non-member | no | yes, declared in the handshake |
| data | real | synthetic only |
| certification rights | rung 6 | **none** |
| key custody | cluster members | launch-time, memory only |
| identity family | prod features | lab features — distinct |
| trust outcome | certifiable | **never certifiable by prod** |

The first two rows are the whole difference. If a production cluster had the observer verb
"for emergencies", it would have the capability, and the mode distinction would be decorative.

### 2. Attestation, not declaration

A `debug: false` field proves nothing; it is a claim by the party that benefits from the claim.
The capability is therefore read from behaviour:

- the handshake advertises the verb — a production peer that advertises it is lab;
- a peer that delivers per-route records **has already demonstrated the capability**, whatever
  it declared — the first unsolicited record is the evidence;
- a peer that refuses the debug handshake is not necessarily production, but it cannot become a
  lab observer, which is enough for the lab side.

This is the same move as [ADR 0007](0007-conveniences-are-attack-surfaces.md): the surface is
the declaration. You cannot have the capability and not reveal it, because the capability is "I
will send you something", and that something is visible.

### 3. Identity is the gate, not policy

The lab's artefacts must be unusable as production evidence even if they are copied, replayed,
or pasted into a report. The isotope mechanism already does this without a new rule:
certification verifies `(feature_family, feature_id, signature)`, and a lab feature lives in a
different family. `TestVerificationFailsWhenTheFeatureIdDoesNotMatch`-shaped behaviour — the
construction tests already assert it — means a lab isotope cannot verify a production feature,
and a production isotope cannot be minted for a lab feature without the family.

So "never trusted by prod" is mechanical: prod's admission requires a certification chain over
production features, and the lab cannot produce one, because it is not running them. The
construction contract is already written down as a scenario — `features/isotope-construction.feature`,
"verification fails when the feature id does not match" — and this ADR depends on it being
implemented, not on a new mechanism.

### 4. Tests, not refusals

The lab does not need a policy that refuses production. Production fails the lab's tests, and
that is the mechanism:

- the lab's conformance suite asserts lab rules — the observer verb exists, per-route delivery
  is accepted, the data is synthetic. A production cluster fails every one of those assertions,
  because it does not have the verb. The failure *is* the mode indicator;
- a suite that both modes could pass would be a suite that does not test the boundary, and
  would be worthless as an authority;
- so there is no allowlist and no blacklist. Trust is earned by passing tests, exactly as a rung
  is earned, and a production cluster is not "banned" from the lab — it simply comes back
  not-certified.

The one refusal that remains is operational, not authoritative: the launcher starts the lab on a
distinct port, with a distinct identity and a badge, and a debug client should not be pointed at
a production endpoint by accident. That is a guard rail, not a rule of the protocol.

### 5. A dev-prod cluster declares the lab authoritative

The useful topology is not "lab here, prod there". It is a **dev-prod** cluster: production
rules, no observer verb, development data — and it **declares the lab as its authority**. The
declaration is what makes the lab's numbers evidence rather than opinion, and it is made by the
subject, not claimed by the observer.

Capability is attested; authority is declared. They are different kinds of claim, and each is
used where it is verifiable: behaviour cannot be faked, and consent cannot be inferred.

That declaration buys a debug channel that does not need the verb:

1. the cluster derives `F(epoch)` under production rules and publishes a **state digest** — a
   commitment over its derived state, not the records;
2. the lab derives the same epoch and compares digests;
3. the first divergence localises to a subtree, and bisection over the commitment narrows it to
   a shard, then a tick, then a record *index* — never a record;
4. each round discloses one hash, so the debug conversation costs `O(log n)` hashes rather than
   `n` records.

So a cluster with no observer verb can still be debugged, provided it volunteers to be compared
against an authority it named. That is the difference between a wiretap and a differential test,
and it is the only debug channel production rules can carry.

### 6. What the lab is for

Two channels, one purpose. On the **lab strain** the derivation diff has full visibility: every
route, every mechanism counted, every rebuild timed. On a **dev-prod** cluster the same diff
runs over a state digest and `O(log n)` bisection, under production rules. Both populate §7 with
an *observed* column beside the documented one: the lab is where "the matrix says 870 ms"
becomes "the matrix said 870 ms and we measured 913 ms on this machine".

That is the point of a strain: it is allowed to be unfit, in order to measure fitness.

## Consequences

**Positive**

- Production needs no observer-disable policy, because the capability is absent from its rule
  set. There is nothing to turn off.
- Debugging never weakens the thing being debugged, which is the usual failure of "temporary"
  instrumentation.
- Lab evidence is structurally unusable as production evidence, so a leaked artefact is an
  embarrassment rather than a certification. Measurements are a different channel and may still
  import as bounded priors ([ADR 0019](0019-lab-results-import-as-priors.md)), which is how a lab
  stays useful without becoming authoritative.
- Authority is a declaration by the subject, so the lab cannot self-promote into a production
  reviewer. A cluster that does not name the lab is not affected by it.
- The dev-prod topology is debuggable without the observer verb: a declared authority plus a
  state digest gives a differential test under production rules.

**Negative and costs**

- **Two protocol surfaces.** The lab diverges, and divergence drifts. The lab needs a
  conformance test that asserts production lacks the verb, or the drift is silent.
- **The lab is second-class by construction**, which means some bugs only appear in the class
  production uses. That is inherent: a strain that behaves like production is production.
- **Launcher responsibility.** `lezz.sea` now mints an identity, provisions a key and chooses a
  port. Each of those is a way to be wrong, and none of them is a UI concern.
- **The honest cost of the word.** A maladaptive cluster has no upgrade path to production. If
  it ever needs one, it needs a decision, not a flag.

**Risks**

- **Lab creed.** "It's only the lab" is how lab rules get copied into production code paths.
  Mitigation: the verb lives in a separate build/profile, and the conformance test is a gate.
- **Identity collision.** If lab and production share a feature family, the certification gate
  disappears and everything above is policy again. The family separation is the load-bearing
  detail.
- **Observer by accident.** A lab cluster that accepts any debug client on a fixed port is a
  service. It should require the handshake, not merely allow it.

## Alternatives considered

**A build flag in one binary.** Rejected as the primary mechanism: a flag is not attestable, and
the same binary can present both surfaces. A separate profile with a separate identity is the
minimum that survives a copy-paste.

**A separate fork.** Rejected: two codebases, one of which is never tested against the other's
bugs. The lab must be the same implementation under different rules, or the diff is meaningless.

**A trusted debug proxy in front of production.** Rejected: the proxy is a correlator in the
production path ([ADR 0015](0015-the-dashboard-sees-rates-not-identities.md)), and it needs the
namespace key, which is the disclosure [ADR 0016](0016-observation-is-a-protocol-role.md) was
trying to bound.

**Declaring `debug: true` and trusting peers to respect it.** Rejected by §2: a declaration is
a claim, and the capability is the evidence.

**An allowlist of clusters the lab may approach.** Rejected: it converts an earned trust
relation into a maintained list, and it puts the decision in the observer instead of the
subject. A cluster opts in by declaring the lab authoritative; a list is a light version of
the same thing with more places to be wrong.

## What this does not decide

- The exact handshake field and whether it is part of the MCP capability manifest or a separate
  pre-protocol check.
- How the lab's feature family is named, and whether one lab can certify another (a strain of a
  strain is a research programme, not a default).
- Whether a production cluster in the field may declare a lab authority. A dev-prod cluster
  may; a field cluster should not, and that is the cluster's policy decision, not a capability
  the lab can assert.
- Whether the state digest is a Merkle root over the derived routes, and what the bisection
  round count (`log n`, and how many hashes per round) is in practice.
- Whether the observed column in `docs/design-matrices.md` is generated from a lab run or
  committed by hand from one.

## Verifying

| claim | source |
| --- | --- |
| a lab isotope cannot verify a production feature | `features/isotope-construction.feature` — "verification fails when the feature id does not match" |
| certification is a chain over features, not a claim | `features/skill-certification.feature` |
| the golden role is what buys per-route visibility | `TestGoldenRebuildWinsTheRace` |
| visibility is tiered, so the mode boundary is the cost ladder | `TestAlarmRotationsEvictTheCohort` |
| residence is one tick, so the lab observes one tick at a time | `TestDataResidenceIsOneTick` |

```sh
go test ./internal/mcp_faults/ -run 'GoldenRebuild|AlarmRotations|DataResidence' -count=1
```

**Not yet tested:** that production's protocol surface lacks the observer verb, that the lab's
suite fails a production cluster, and that both sides refuse the operational half — a debug
client on a production port. Those are conformance properties. The first is an absence, and an
absence needs a test that fails the day the verb appears.

The digest channel in §5 is a proposal: a commitment over derived state, compared round by
round. Nothing in the repository implements it, and the bisection cost (`log n` hashes) is an
estimate from the tree shape, not a measurement.
