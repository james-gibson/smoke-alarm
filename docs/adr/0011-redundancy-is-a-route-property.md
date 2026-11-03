# ADR 0011 — Redundancy is a route property: replicas, placement, blocks

- **Status:** Proposed
- **Date:** 2026-10-22
- **Related:** [ADR 0001](0001-alarm-membership-tiers.md), [ADR 0002](0002-golden-is-a-derived-reference.md), [ADR 0010](0010-trust-rotates-with-the-layout.md)

## Context

Three different guarantees were being discussed with one vocabulary, and RAID has separate
names for them:

| guarantee | RAID name | what it actually protects |
| --- | --- | --- |
| the route keeps answering | mirror (RAID 1) | *service*, per replica |
| a skill can be found anywhere | (no RAID analogue) | *addressability*, per key |
| a block survives a lost member | parity (RAID 5/6) | *data*, per stripe |

Conflating them produces a system that is highly available and has lost its data, or one that
keeps every block and cannot serve a request. So this ADR fixes the three independently, and
states which one each mechanism buys.

The unit of redundancy here is the **route**, not the node. Replicas of a route are hot paths
of one logical object - `RouteRef.ID` is invariant across replicas by construction
([ADR 0002](0002-golden-is-a-derived-reference.md)) - so "how redundant is this route" is a
count of replicas, and "how durable is this data" is a separate count of parity shares.

## Decision

**Route redundancy, key placement and block durability are three separate mechanisms with
three separate budgets. None of them substitutes for another.**

### 1. Replicas are hot paths, and availability is `1 - p^k`

| replicas | availability at 1 % failure | downtime/year |
| --- | --- | --- |
| 1 | 0.990000 | 5 256 min |
| 2 | **0.999900** | 53 min |
| 3 | 0.999999 | 0.5 min |
| 4 | 1.000000 | 0.01 min |

The knee is at **two**. The second replica buys 99× the availability; the fourth buys almost
nothing. Redundancy beyond three has to be justified by something other than availability -
typically read locality or blast-radius limits.

### 2. Placement is computed, not looked up

A skill is owned by `rendezvous(key, n)`: the member with the highest mixed hash wins. Any
member computes the owner, so there is **no directory** to keep consistent and nothing to
disclose ([ADR 0007](0007-conveniences-are-attack-surfaces.md)):

```
10 000/10 000 keys placed identically on every member
load spread: min 1183, max 1310, ideal 1250
directory entries: 0; hops: 1
```

The reason for rendezvous rather than modulo is the resize cost, which is the RAID
"add a disk" question in route terms:

| function | keys moved when adding a member |
| --- | --- |
| modulo | **80.2 %** |
| rendezvous | **19.7 %** (ideal 20 %) |

Modulo remaps nearly the whole key space, so every skill lookup briefly points at the wrong
member. Rendezvous moves the ideal `1/(n+1)`.

### 3. Block data gets parity per route class, not per cluster

A block striped across `n` members with no parity loses that shard on any single loss. With
single parity it survives one loss; with dual parity, two. The redundancy level is a property
of the **route class**, because it follows what the data is:

| class | protection | overhead |
| --- | --- | --- |
| derivable (rotations, filters) | none - regenerate from `F(epoch)` | **0 %** |
| authored (issued skills, receipts) | mirror or dual parity | 50–100 % |
| block payload, read-heavy | single parity | 25 % at `n = 4` |
| block payload, write-heavy | mirror | 50 % |

Derivation is free and strictly better where it applies
([ADR 0002](0002-golden-is-a-derived-reference.md)); it simply does not apply to anything
unauthored, which is why the other rows exist.

### 4. Parity exclusion: a block's server is not its parity holder

From [ADR 0010](0010-trust-rotates-with-the-layout.md): parity for block `b` sits on
`(b + n - 1) mod n`. That member must not serve `b`, or a single failure takes the data and
its protection path together.

| replicas | blocks served by their own parity holder |
| --- | --- |
| 1 | **25 %** - those blocks have no redundant path |
| 2 | **0** - and 25 % are routed to the secondary |

So two replicas are the minimum for a route that holds parity-protected blocks. One replica is
not "less redundant"; it is unable to serve a quarter of its own blocks redundantly, which is a
different and worse failure mode.

## Consequences

**Positive**

- Each guarantee has a number and a separate budget: replicas cost availability and read
  locality, placement costs a hash per lookup, parity costs capacity.
- No routing directory, so no consistency protocol and no enumeration surface.
- Resize is cheap under rendezvous: adding a member moves a fifth of the skills, not four
  fifths.
- The route class decides the protection, so derivable data is never paid for at parity rates.

**Negative and costs**

- **Rendezvous costs `n` hashes per lookup** instead of one. At `n = 8` that is a microsecond;
  it is not free, and it becomes visible if the member set is large.
- **Two replicas minimum** for parity-protected routes, which doubles the hot path count for
  the same data.
- **Route classes must be assigned deliberately.** Misclassifying authored data as derivable
  is silent data loss on the next rebuild.
- Resize still perturbs a fifth of the keys, so a rebalance has to finish before the next
  rotation or two replicas can disagree about an owner mid-tick.

**Risks**

- **Hash choice breaks rendezvous silently.** A naive implementation hashing `key/member` with
  FNV measured **49.8 % moved** instead of 20 %, because hashes differing only in the last byte
  correlate and the "highest random weight" stops being random. The fix is to hash the member
  first and mix the result; the test asserts the ideal fraction, so this cannot regress quietly.
- **Replicas are not durability.** Two replicas at 99.99 % availability with a 4-way stripe and
  no parity still lose 25 % of a block on one loss. The two numbers must be reported separately
  or the availability figure will be read as a durability guarantee.
- **Load spread depends on the key distribution.** The measured spread (1183–1310 against an
  ideal of 1250) assumes roughly uniform keys; a skewed skill namespace skews placement, and
  rendezvous does not correct it.

## Alternatives considered

**A routing directory.** Rejected: it is a consistency problem and an enumeration surface, and
it answers a question the key can answer locally.

**Modulo placement.** Rejected for resize cost: 80 % of keys move to add one member, so every
resize is a brief period of everyone looking in the wrong place.

**Mirror everything, including derivable state.** Rejected: 100 % overhead to protect data that
costs 0 % to regenerate, and it makes the storage bill grow with a quantity the design
deliberately does not store.

**One replica plus parity.** Rejected by the exclusion rule: a quarter of the blocks would be
served by their own parity holder, which is redundancy on paper only.

## What this does not decide

- The stripe width `n`, which sets parity overhead, resize churn and duty cycle together
  ([ADR 0010](0010-trust-rotates-with-the-layout.md)).
- Where a route class is declared - configuration, or inferred from whether the route has a
  functor.
- Whether skill placement and block placement share one function or two; sharing simplifies
  consistency, separating lets them be tuned against different skew.
- The rebalance protocol when the member set changes, and how it interacts with the rotation
  epoch.

## Verifying the numbers in this ADR

```sh
go test ./internal/mcp_faults/ -run 'RouteReplicas|PlacementIsByFunction|ResizeCost|RouteRedundancy|SelfServeConflict' -v
```

| claim | test |
| --- | --- |
| availability is `1 - p^k`, knee at two replicas | `TestRouteReplicasFollowAvailabilityMath` |
| placement is computed, deterministic and even | `TestPlacementIsByFunctionNotDirectory` |
| rendezvous moves `1/(n+1)`, modulo moves nearly everything | `TestResizeCostDependsOnTheFunction` |
| route availability is not block durability | `TestRouteRedundancyIsNotBlockDurability` |
| one replica cannot serve a quarter of its blocks redundantly | `TestSelfServeConflictAtBlockGranularity` |
