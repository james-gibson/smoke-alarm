# ADR 0010 — Trust rotates with the data layout

- **Status:** Proposed
- **Date:** 2026-10-22
- **Related:** [ADR 0001](0001-alarm-membership-tiers.md), [ADR 0002](0002-golden-is-a-derived-reference.md), [ADR 0008](0008-rotation-as-a-helix-of-residues.md), [ADR 0011](0011-redundancy-is-a-route-property.md)

## Context

A RAID parity layout is a **placement function**: at stripe `i`, member `m` occupies slot
`(m - i) mod n`, and the parity slot is one of those positions. The function is already doing
the job a verifier assignment needs - deciding who is where - and the cluster was assigning
the verifier statically instead: `C` commits, always `C`.

That is one member permanently holding the checker role. It has two costs the rotation
already removed elsewhere:

- **A single target.** Compromise one member and you hold the verifier for as long as you
  like.
- **A single point of correlation.** Every claim in the system is checked by the same party,
  so a systematic error in that party is a systematic error everywhere.

The alternative is to take the verifier role from the same function that places the data, so
the two cannot disagree about who checks whom.

## Decision

**The verifier for tick `i` is the parity holder of stripe `i`, using the array's own
placement function.**

```
slot(m, i) = (m - i) mod n        member m's position in stripe i
parity(i)  = the member in slot n-1
```

Measured for `n = 4` over eight ticks:

| tick | m0 | m1 | m2 | m3 |
| --- | --- | --- | --- | --- |
| 0 | d0 | d1 | d2 | **P** |
| 1 | **P** | d0 | d1 | d2 |
| 2 | d2 | **P** | d0 | d1 |
| 3 | d1 | d2 | **P** | d0 |
| 4 | d0 | d1 | d2 | **P** |

Three properties follow, and each is a constraint the static topology did not have.

**1. Duty is uniform.** Every member is the verifier `1/n` of the time - 3 of 12 ticks each at
`n = 4`. No member is permanently the checker, and no member is permanently unchecked.

**2. Nothing verifies itself.** In the layout, parity and data never share a member within a
stripe, so the verifier holds no data shard in the stripe it verifies. That is the RAID
invariant, and it is automatic rather than a rule to remember.

**3. Control has a duration cost.** This is the security property the layout buys:

| ticks of control | static verifier | rotating verifier | share of membership |
| --- | --- | --- | --- |
| 1 | 1 member | 1 member | 0.25 |
| 2 | 1 member | **2 members** | 0.50 |
| 3 | 1 member | **3 members** | 0.75 |
| 4 | 1 member | **4 members** | 1.00 |

A static verifier costs one member to hold indefinitely. A rotating one costs one member per
tick of control, so **sustained control costs the whole membership** - the same quantity every
quorum argument rests on ([ADR 0001](0001-alarm-membership-tiers.md)).

**Dual parity doubles it.** With RAID-6 style `P` and `Q` slots there are two verifiers per
stripe, so `k` ticks of control cost `2k` members: 2 of 6, then 4 of 6, then all 6.

## The distribution half

The same shift distributes the data, and that changes what a compromise is worth:

- **Residence is one tick.** A member never holds the same shard on consecutive ticks; it
  returns to that slot only after a full cycle, having held every other slot in between.
- **Following one shard across `n` ticks needs all `n` members**, because the shard moves one
  slot per tick.
- **Rebuild moves too.** The member that reconstructs a lost shard is not the member that held
  parity the tick before (`m3` then `m0`, `m0` then `m1`, ...), so there is no permanent
  repairer either.

A compromised host therefore sees every shard briefly and none of them completely. The cost of
assembling one shard's history is the whole membership, not one host.

## Constraints this imposes

1. **One function, both jobs.** Placement and verification must be computed from the same
   `(m, i, n)`. Two independent assignments drift, and the drift is the bug: a member
   verifying a shard it holds.
2. **The schedule is public.** `n`, `i` and the layout are published by design
   ([ADR 0006](0006-the-name-space-is-isotropic.md)), so an attacker can plan to be verifier
   at tick `i*`. The defence is not unpredictability - it is that holding `i*` buys one tick,
   and the next one is somebody else's.
3. **Rotation costs a full-stripe write.** Moving every shard every tick is the 320 ms
   sequential write the durable budget already carries
   ([ADR 0002](0002-golden-is-a-derived-reference.md)). Per-record writes are the wrong
   granularity precisely because the records move anyway.
4. **A stable `n`.** Churn changes the stripe width and re-shuffles every role. Membership
   changes have to be applied at a stripe boundary, or the assignment is momentarily ambiguous.

## Consequences

**Positive**

- No permanent checker, so no single member to compromise for indefinite verification
  authority.
- Control duration becomes a cost axis, linear in ticks - an axis the static topology did not
  have at all.
- Self-verification is structurally impossible rather than forbidden by policy.
- Data residence of one tick makes a single compromised host worth a slice of history instead
  of all of it.
- The verifier role needs no new coordination: it is the parity slot, which the array already
  computes.

**Negative and costs**

- **Every member must be able to verify.** The static design only needed `C` to hold the
  checking logic; now every member does, which is a larger trusted-code footprint.
- **Joining just before your turn is cheap.** One identity buys one tick of verifier authority
  at a time of the attacker's choosing. What rotation protects is *duration*, not the single
  tick.
- **Rotation adds a scheduling dependency**: a member that cannot verify on its tick stalls
  the stripe, so verification must be as available as the data path.
- `n` becomes load-bearing twice over - it sets both the parity geometry and the duty cycle.

**Risks**

- **Collusion is linear too.** `k` colluding members buy `k` ticks of control, exactly as an
  attacker buying `k` identities would. If identities are cheap, so is control; this ADR does
  not price identity.
- **A member that is verifier 1/n of the time is still trusted the other (n-1)/n.** The layout
  spreads the checking duty; it does not reduce the number of members that must be honest.
- **Stripe-boundary races.** A membership change mid-stripe can produce two members claiming
  the parity slot for one tick, which is a self-verification loophole if the tie is broken
  wrongly.

## Alternatives considered

**Keep a static verifier.** Rejected: one member holds checking authority indefinitely, which
is the concentration this whole design set exists to avoid.

**Pick a verifier at random per tick.** Rejected: unpredictable, but unauditable - a member
cannot show that it was *supposed* to verify, and a skipped tick looks like a random draw
rather than a failure.

**Verify by quorum.** Rejected as a substitute: a quorum answers "how many agree", which
[ADR 0008](0008-rotation-as-a-helix-of-residues.md) already replaced with "what is the
position". Placement is the stronger primitive because it is checkable by anyone holding the
layout.

## What this does not decide

- The stripe width `n` in production, and therefore the duty cycle and the duration cost.
- Whether dual parity is on by default, which doubles availability cost and doubles the
  attacker's duration cost.
- How a member proves it was *supposed* to verify: the layout says who, but the log has to
  show that they did.
- Whether role rotation is aligned with the rotation epoch of
  [ADR 0008](0008-rotation-as-a-helix-of-residues.md) or offset from it (offsetting may
  prevent a member from being verifier and stale in the same tick).

## Verifying the numbers in this ADR

```sh
go test ./internal/mcp_faults/ -run 'LeftSymmetric|NoMemberVerifies|TrustRotationCosts|DualParity|DataResidence|RebuildMoves' -v
```

| claim | test |
| --- | --- |
| duty is 1/n per member and role = f((m-i) mod n) | `TestLeftSymmetricRotatesTrustAndData` |
| a verifier holds no data shard in its stripe | `TestNoMemberVerifiesItsOwnShard` |
| control costs k members for k ticks | `TestTrustRotationCostsDuration` |
| dual parity doubles the verifier set | `TestDualParityDoublesTheVerifierSet` |
| data residence is one tick | `TestDataResidenceIsOneTick` |
| the rebuilding verifier moves each tick | `TestRebuildMovesTheParityRole` |
