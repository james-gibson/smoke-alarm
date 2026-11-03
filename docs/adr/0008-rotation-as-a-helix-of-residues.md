# ADR 0008 — Verify rotation as a helix of coprime residues

- **Status:** Proposed
- **Date:** 2026-10-22
- **Related:** [ADR 0002](0002-golden-is-a-derived-reference.md), [ADR 0006](0006-the-name-space-is-isotropic.md), [ADR 0007](0007-conveniences-are-attack-surfaces.md), [ADR 0009](0009-the-side-channel-three-filters.md)

## Context

[ADR 0007](0007-conveniences-are-attack-surfaces.md) established that a static invariant
cannot measure rotation: `isOdd` has a zero derivative, so a node that never rotates scores
perfect health on it. That conclusion is correct for *one* invariant and wrong for *two*.

Two paths, one recording under an odd invariant and one under an even one, are not two
independent proofs. They are one **entanglement**. Path A at index `2t+1` and path B at
`2t` are the same rotation observed twice, and the pair constrains their *relative*
position: A is B advanced by one. That is precisely what a rotation measure needs, and it is
checkable without either path revealing its index.

The construction generalizes: each path records its index under a modulus, and the set of
residues determines the index wherever the moduli are coprime. The state becomes a helix —
predictable one turn ahead from the previous turn's residue — rather than a statistic.

Three conditions decide whether the helix is an expected state or a tautology:

1. **Possession.** Residues must come from values the path was issued. Anyone can compute
   `n` and `n+1`; only a participant holds the pair.
2. **Coprimality.** One modulus aliases. A parity helix cannot see a drift of two, ever.
3. **Commitment.** A published plaintext residue tells an observer the expected state, which
   hands back the guidance oracle [ADR 0007](0007-conveniences-are-attack-surfaces.md)
   removed from the refusal reasons.

## Decision

**Rotation state is verified by a helix: k ≥ 2 paths, each recording its index under a
pairwise-coprime modulus, over issued values, published as commitments.**

- **Minimum two, recommended three.** `{2, 3}` gives a period of 6; `{2, 3, 5}` gives 30,
  which at a 5 s epoch is 150 s of unambiguous index. Four paths (`{2,3,5,7}` → 210) are for
  when the blind spot must exceed a shift or an incident window.
- **Pairwise coprime, or the path is wasted.** The period is the lcm, not the product: `{2,4}`
  reaches 4, not 8, and the mod-2 path adds nothing.
- **Residues bind to the epoch**, as commitments over `(issued value, epoch)`, so a residue
  cannot be replayed from an earlier turn.
- **One modulus is forbidden.** Not because it is weak, but because its blind spot is its own
  period, which is small by construction.
- **Derived residues are a tautology.** If the residue can be computed from the public epoch,
  the helix constrains nothing and must not gate anything
  ([ADR 0007](0007-conveniences-are-attack-surfaces.md), register entry).

## The numbers

### Relative position, verified per turn

A node that guesses its residue is right with probability `1/2` per turn, so `k` turns give
`1 − 2^−k`:

| turns | detection | latency at 5 s |
| --- | --- | --- |
| 1 | 50.0 % | 5 s |
| 3 | 87.5 % | 15 s |
| 5 | 96.9 % | 25 s |
| 10 | **99.9 %** | **50 s** |
| 20 | 99.9999 % | 100 s |

A *deterministic* drift is caught on the first turn, however small, provided the modulus does
not divide it.

### The aliasing a single modulus hides

| drift | mod 2 sees it | mod 2, 3, 5 sees it |
| --- | --- | --- |
| 1 | yes | yes |
| 2 | **no** | yes |
| 4 | **no** | yes |
| 6 | **no** | yes |
| 8 | **no** | yes |
| 30 | no | **no** — the blind spot |

A node that always skips exactly two rotations passes a parity helix forever. That is the
whole argument for coprimality: the blind spot moves from the period of one modulus to the
product of all of them.

### Reconstruction

| moduli | period | reconstructs uniquely | blind spot |
| --- | --- | --- | --- |
| {2} | 2 | yes | 2 |
| {2, 3} | 6 | yes | 6 |
| {2, 3, 5} | 30 | yes | 30 |
| {2, 3, 5, 7} | 210 | yes | 210 |

The inversion is exact: index `17` records as residues `[1, 2, 2]` and reconstructs to `17`,
so the next turn (`18`) is an **expected state**, not merely a plausible one. From `23` the
helix implies `24`; a holder that presents `[0, 0, 4]` instead of `[1, 2, 3]` is stale and
fails on the first comparison.

### Against the bias test

| check | unit | latency | load-sensitive | answers |
| --- | --- | --- | --- | --- |
| bias test ([ADR 0006](0006-the-name-space-is-isotropic.md)) | samples | 469 ms | yes | is the client *following*? |
| three-helix CRT | turns | 50 s | **no** | is the node *where it claims*? |

They are complements, not substitutes. The helix is slower because it is a schedule: its
latency is turns × epoch, so it carries an SLO. The bias test is faster because it is a
statistic over a stream, so more load helps it. Neither is a replacement for the other.

## Node C: the tuple, and why it commits

A and B each hold a residue for the same turn, computed in **opposing** directions — A
ascending, B descending — and hand C the tuple. Three findings, the middle one of which
corrects the natural implementation.

**1. Opposition sees a common-mode drift; sameness cancels it.** If both paths drift
together (a replayed coordinator, a shared lagging clock), a same-direction helix cancels it
in the sum and is blind. Opposing helices cancel the sum and **double** the difference:

| common drift | sum | difference | visible |
| --- | --- | --- | --- |
| 0 | 0 | 1 | — |
| 1 | 0 | 3 | yes |
| 2 | 0 | 0 | yes |
| 3 | 0 | 2 | yes |

The difference needs **odd moduli**: with modulus 2, `2d ≡ 0` and a common drift of one
vanishes. This is a constraint on the modulus set, not a property of the arithmetic.

**2. CRT maps a lie; it does not reject it.** An honest tuple `[1,1,2]` reconstructs to 17;
corrupt one residue to `[1,1,3]` and it reconstructs to **23** — a perfectly valid index.
Every residue is consistent with *some* index, so a consistency check detects nothing. What
detects is the comparison against the expectation (`previous + 1`). An implementation that
trusts reconstruction as validation has no validation at all.

**3. The delta names the liar.** The move caused by corrupting path `i` is a multiple of the
lcm of the *other* moduli and not of `i`'s own, so one modulus fails to divide it:

| corrupted path | delta | lcm of the others | divides | named |
| --- | --- | --- | --- | --- |
| `{2}` | 15 | 15 | yes | 0 |
| `{3}` | 20 | 10 | yes | 1 |
| `{5}` | 6 | 6 | yes | 2 |

Three moduli are the minimum for exact attribution; with two, the delta names a class rather
than a member. So C needs a third source even when it only compares A and B — and the
cheapest third source is the *expectation*, which costs no message at all because it comes
from the previous turn.

**Consequences for the topology.** C receives two residues per turn, does one reconstruction
(30 candidates at `{2,3,5}`) and one divisibility test. Detection therefore takes two turns —
one to establish the expectation, one to compare — which is 10 s at the shipped epoch, and
attribution takes the same one comparison. A one-turn lag reconstructs to a legitimate
previous index, so **grace is a phase, not a mismatch**: the comparison must be against the
expected index, and a two-turn-old tuple is the first thing that is actually deviant.

C is still the committer: the tuple is evidence for C's check against the derived identity,
never a ballot. A and B disagreeing is a question for C, and A/B redundancy exists to catch
C rather than to outvote it.

## Cryptographic wrapping: what a key can and cannot carry

The obvious next question is whether a hash or a key can do this work instead of a
small-modulus residue. Partly, and the boundary is precise.

**A hash has no inverse, so forward and reverse stop being directions.** A residue is
invertible: `-r mod m` recovers the position, and opposition is that inversion. SHA-256 is
not invertible, so "forward" and "reverse" become two *labels* - `H(k, "asc"‖i)` and
`H(k, "desc"‖i)` - which are unrelated images. The opposition has to be constructed by domain
separation rather than read off the arithmetic.

**Hashing destroys the structure that attribution depends on.** This is the real cost. A
corrupted residue moves the reconstruction by a multiple of the other moduli's lcm, and that
divisibility names the culprit: deltas 15, 20 and 6 name paths `{2}`, `{3}` and `{5}`. A tag
carries no such structure - it is all-or-nothing. A hash detects with certainty and
**attributes never**, so attribution falls back to a majority over at least three claimants.

**What a key does buy** is exactly the three properties [ADR 0007](0007-conveniences-are-attack-surfaces.md)
required of the wire form:

| property | from the residue | from the key |
| --- | --- | --- |
| binding | none - any value is a valid residue | the tag cannot change with the value fixed |
| hiding | none - `r` leaks the position mod 30 | an observer learns nothing without the key |
| epoch binding | none - a residue is stable forever | replaying yesterday's tag fails |
| attribution | **exact, from the lcm** | none - equality only |
| position leak | yes | no |

So the construction is **wrap, don't replace**: residues stay the algebraic payload, a keyed
hash is the envelope, and the opening travels on the private channel to C while the tag
travels on the public one. C sees `r` and can attribute; observers see tags and learn
nothing.

**The middle ground is a group, not a hash.** If C must also be blind, commit in a group where
the exponent is the residue: `C = g^r`. Additive structure survives - opposing commitments
multiply to the identity, and a common drift appears as a `g^(2d)` factor without either
residue being revealed. Two constraints come with it:

- **The modulus must divide the group's order.** With `p = 31` the group has order 30, so a
  period-5 helix works (`g^3 · g^2 = g^30 = 1`). A naive `g^3 · g^2 = g^5 ≠ 1` is what
  happens when this is ignored, and every sum check silently fails. A 210-period helix needs a
  group whose order is divisible by 210.
- **Divisibility attribution is still lost.** A group element has no factor structure a
  verifier can test, so a blind verifier can detect but not name.

### Forced inversion: the rotation *is* the inverse

A cheaper and stronger variant of the wrapper: instead of recording a residue, we supply a
number downstream and require it to be **inverted as part of the rotation**. The check is
composition to the identity - one multiplication, exact, public, revealing nothing but the
relation.

What this buys over a tag:

- **Attribution is free.** Each downstream gets its own number, so the pair that fails to
  compose names the node. This is exactly what the lcm was doing, and it needs no arithmetic
  structure at all - which is the answer to the previous section's problem, where hashing
  destroyed attribution.
- **The gate is on our number.** The node cannot turn without applying the group law to the
  value we supplied, so a claimed rotation has to be a performed one.

What it does **not** buy, and both limits are structural:

| claim | reality |
| --- | --- |
| "it forces work" | `x^-1` is computable from `x` by anyone, with no secret: the gate proves *receipt*, not *expenditure* |
| "then even receipt is evidence" | only if `x` was **issued** - a public supply makes the gate a tautology, since computing the inverse needs nothing from us |
| "it gives a rotation, so it must have a period" | `f(f(x)) = x` - any inversion-defined rotation is an involution, so the period is **2** |
| "add a per-epoch step and the period grows" | in an abelian group `g · x^-1` is also an involution, because `g` cancels |

The period-2 result is the parity blind spot built into the rotation: a node replaying
`x, x^-1, x, x^-1` passes every check on every turn - 10/10 measured. So forced inversion
alone cannot carry the helix's aliasing budget.

**The fix is to leave the abelian world.** In a non-abelian group, `f(x) = τ · x^-1` gives
`f²(x) = τ x τ^-1` - conjugation - so the period is governed by the conjugacy class rather
than forced to 2:

| `τ` | order of `τ` | period of `f` |
| --- | --- | --- |
| transposition | 2 | 4 |
| three-cycle | 3 | 3 |

Nothing forces 2, and the period is at most twice the conjugacy order, so a 30-wide blind
spot needs an element whose conjugacy class has order ≥ 15.

**The division of labor this implies** is the useful conclusion:

- **Forced inversion answers *which node*** - cheap, exact, per-pair, no structure needed.
- **Coprime residues answer *how far*** - they bound the drift with a period.

Neither replaces the other, and one message can carry both: the inverse pair proves the node
turned, the residue triple bounds by how much.

### A braid is the anti-Bloom, and that is why it was a candidate

The Bloom filter in this package is a **set**: bits OR'd together. Union is commutative and
idempotent, so order and repetition vanish, and the structure is deliberately lossy.

A braid is a **stack**: crossings composed. The operation is non-commutative and
non-idempotent, so order and repetition *are* the information, and the structure is exact.

| | Bloom filter (parental filter) | braid group |
| --- | --- | --- |
| operation | OR (union) | composition (stacking) |
| commutative | yes | no (`σ1σ2 ≠ σ2σ1`) |
| idempotent | yes (`a ∨ a = a`) | no (`σ1σ1 ≠ σ1`) |
| order | lost | the whole content |
| repetition | lost | information |
| exactness | one-sided error, measured 25/100 absent admitted | exact: 8 words → 7 matrices, the single merge being the braid relation |
| torsion | not a group | **none** (`σ1^k = [[1,k],[0,1]]` never returns) |

That last row is the reason it appeared here. A helix with moduli has a **period**, and a
drift of exactly the period is invisible - drift 30 against `{2,3,5}`. A torsion-free carrier
has no period at all: 100 crossings and no return, so no drift is free. What it trades that
for is a **guessing space**: the blind spot stops being a period and becomes `2^bits`.

It is also the same decision `PadStack` already made - a stack, not a set, because order and
repetition are information. The braid group is what that decision looks like when the algebra
has to close.

**Two caveats, both load-bearing.**

- **You do not need to say "braid" to get the property.** `B_3 ≅ PSL(2,Z)`, so the faithful
  carrier is a 2×2 integer matrix: `σ1 = [[1,1],[0,1]]` has infinite order, and multiplying
  it is two additions. The simplest torsion-free helix is an integer that only ever increases
  - which is the pad stack itself.
- **Do not build secrecy on braid hardness.** Braid-group cryptosystems
  (Anshel–Anshel–Goldfeld and successors) have been broken repeatedly by length-based and
  linear-representation attacks. Use the group for *structure* - ordering and the absence of
  a period - never for hardness.

### Cycle prevention versus cycle detection

The Bloom/braid contrast sharpens into an exact statement about the question that matters
here: *has anything returned to the start?*

| carrier | answers it by | error | ensemble |
| --- | --- | --- | --- |
| torsion-free group | **prevention** - return is impossible | none | one group element |
| Bloom filter | **detection** - is this state known? | one-sided: no false negatives, false positives | bits per visited state |

They are inverses, and they are **substitutes**: you spend algebra or you spend memory.

**The polarity is the trap.** In the parental filter, "present" means *a rotation I know*, so
a false positive admits a stranger. In a return detector, "present" means *you have already
been here*, so a false positive is a **false accusation of replay**. Same structure, opposite
security polarity:

| use of `Present` | false positive means |
| --- | --- |
| parental filter (known rotation) | admit an unknown rotation |
| return detector (already seen) | accuse an honest node |

Quantified, because the second is the one that would be tempting:

```
1000 nodes, 720 turns/hour, 10 bits/item (the parental design size)
  -> 0.82% false returns
  -> 5898 false accusations per hour

to stay under one accusation per hour: epsilon <= 1.4e-6, i.e. 28 bits per item
```

So a return detector must be paid for at the audience's scale, not the designer's: `epsilon`
has to be smaller than `1/(nodes x turns)`. That is 28 bits per visited state, against zero
for a carrier that cannot return.

**The consequence for the carrier choice.** Per node:

| carrier | words | blind spot | false accusations |
| --- | --- | --- | --- |
| coprime residues `{2,3,5}` | 3 | period 30 | none |
| Bloom over visited states | 1 per state | none | yes, at `epsilon` |
| torsion-free matrix | 4 | none | none |

If the rotation state is already being carried - and it is, that is the helix - then a
torsion-free carrier removes the periodic blind spot **and** the need for a return filter, for
four words. The residues and the filter were both compensating for a period the carrier was
imposing on itself.

The filter does not become useless: it is the right structure where its error is *repaired by
derivation* rather than trusted, which is exactly the parental filter plus the golden rebuild
([ADR 0002](0002-golden-is-a-derived-reference.md)). And the one-sided error means a filter
may **flag for review** - it never misses a real replay - but it may not **evict**, because a
false positive there is an accusation ([ADR 0007](0007-conveniences-are-attack-surfaces.md),
rule one: measure behavior, never gate an attestation).

### Two paths: the exclusion side channel and the authoritative lift

The torsion-free property is not automatic even when the base math is. `U(1)` is the quotient
`R/2πZ`, and **every quotient has torsion**: a `1/q` phase has order exactly `q`. Measured at
`q = 30`, the smallest step is `|e^(i·2π/30) − 1| = 0.209` - clearly distinguishable - and the
thirtieth step returns to exactly `1`, with no residue left to show the winding.

So "already torsion-free by design" holds exactly while the carrier is the **lift**: the
accumulated angle or action in `R`, which remembers how far it has wound. Projecting to the
phase for the wire reintroduces the period, and with it the blind spot. **Keep the state in
the universal cover; send the projection.**

That gives the two paths, with a precise division of labour:

| path | carries | updated | answers |
| --- | --- | --- | --- |
| side channel (Bloom) | bits per turn | every turn | *certainly absent*, or *maybe present* |
| authoritative (lift) | the group element | on escalation | *where it actually is* |

The side channel is an **exclusion** filter, not an admission filter, and that is what makes
its one-sided error safe: absence is certain, presence is a question. Measured over 100 000
turns at 10 bits/item:

```
escalations 998 (1.00%)      resolved locally 99.00%
bandwidth   side 10 bits/turn, authoritative 256 bits/turn (25.6x less)
no false negatives: every real replay escalated
```

So the common path never touches the slow link, and the escalation rate is the false-positive
rate rather than a function of load.

**The side channel must be keyed on the lift, for the same reason the state is.** Keyed on the
phase, the filter has only `q` distinct keys, so it saturates exactly one winding after the
period: 30/30 new lifts read as already seen, permanently. Keyed on the lift, 0/30 misread.
A side channel over the quotient does not merely lose precision - it stops working at the
first wrap.

**Two costs to name.**

- **Freshness is load-bearing.** "Certainly absent" is certain only for the filter's current
  snapshot. A replay inside the update lag is invisible to the fast path, so the update must
  be per turn rather than eventual, or the slow path's ordering has to cover the gap.
- **Coarseness costs escalations, not correctness.** A nearly-full filter raises the
  escalation rate - 4× insertions reaches 54% ([ADR 0002](0002-golden-is-a-derived-reference.md))
  - so the fast path degrades into the slow path rather than into error. That is the right
  failure direction, and it is another reason a filter may flag but never evict.

### Cost, measured

| operation | time | relative to a residue |
| --- | --- | --- |
| `residue(i, m)` | 0.78 ns | 1× |
| HMAC-SHA256 + hex | 478 ns | ~610× |
| 61-bit modexp (`big.Int`) | 47 ns | ~60× |

Six tag evaluations per turn (three paths, two checks) is 2.9 µs; at 5 s epochs that is
**0.0001 % of a core**. The cost of cryptography is not the constraint here - the *structure*
is. A hash is affordable and blind; a residue is cheap and articulate; a group commitment
buys both, at the price of a group order that must be a multiple of the helix period.

## Consequences

**Positive**

- Rotation becomes an *attestation of position* rather than a behaviour score, which is the
  distinction [ADR 0007](0007-conveniences-are-attack-surfaces.md) requires: the helix can
  gate a state decision precisely because it is not a function of the client's input.
- The check is small and constant: `k` integers per turn, no transfer of the underlying
  values, no comparison over the whole route.
- Detection is exact rather than statistical. A drift of one is caught on the next turn; no
  accumulation is needed for the deterministic case.
- Latency is expressible and load-invariant, so it can be an SLO rather than a hope.

**Negative and costs**

- **`k` paths must be recorded and verified every turn**, and they must stay phase-aligned.
  An epoch boundary that splits the pair produces a false mismatch.
- **The blind spot is the period.** At `{2,3,5}` a systematic skimmer who always takes
  exactly 30 turns can hide indefinitely; the period must exceed the longest interval over
  which you are willing to be wrong.
- **Possession is load-bearing.** The whole construction collapses to arithmetic if residues
  are derived, which means the helix inherits every requirement of the issued-value path.
- Commitments cost a hash per residue per turn, which is cheap but not free, and they make
  the wire value unreadable to an operator as well.

**Risks**

- **A plaintext residue is a free oracle.** If the published value is the residue itself, an
  observer reads the expected state and proxies it, which is the [ADR 0006](0006-the-name-space-is-isotropic.md)
  gradient reintroduced. Commitment is not optional.
- **Non-coprime moduli look stronger than they are.** `{2,4}` reads as "two paths" and
  delivers a period of 4. The failure is silent and in the safe direction only by accident.
- **Residue replay across epochs.** A commitment over the value alone is stable forever, so
  it must bind the epoch; otherwise the helix certifies a state that was true an hour ago.
- **A drift of exactly the period is invisible**, and the period is public — which is fine,
  because the attacker still needs the issued values to sit at the right residue. But it does
  mean the helix bounds *drift*, not *absence*.

## Alternatives considered

**One modulus with a large period.** Rejected: a modulus large enough not to alias is a
32-bit value, i.e. the stamp itself. The helix exists to check the stamp cheaply with small
invariants; using the value directly is a different mechanism, not a substitute.

**Residues over derived values (compute the expected residue from the epoch).** Rejected: a
derived helix satisfies the relation on every turn with zero work — 100/100 epochs in the
test — so it constrains nothing.

**Publish plaintext residues for operator visibility.** Rejected: it hands the observer the
expected state. Commit, and let the operator recompute from the log.

**Non-coprime moduli for simplicity.** Rejected: the period is the lcm, so shared factors
waste paths while appearing to add coverage.

## What this does not decide

- Which moduli production uses, and therefore what the blind spot is. This ADR fixes the
  rule (pairwise coprime) and the cost model (period = lcm), not the constant.
- The commitment construction. This ADR establishes what each option can carry (the table
  above); choosing between HMAC-with-private-opening and a group commitment is a decision
  about whether C is allowed to see the value.
- Whether the node or the mirror performs the reconstruction, and what a mismatch does per
  route (quarantine versus eviction, still per [ADR 0001](0001-alarm-membership-tiers.md)).
- The side channel's update cadence. Per turn keeps "certainly absent" true; anything slower
  needs the slow path to cover the lag.
- Which carrier the rotation uses in production, and therefore what its period is. A prime
  field gives 2; an elliptic curve is abelian, so it also gives 2; a matrix or braid gives
  more, and a torsion-free carrier (an integer that only increases) removes the period
  entirely and replaces it with a guessing space. That trade - periodic blind spot versus
  `2^bits` - is the decision this ADR leaves open, and it is the one that determines whether
  the coprime residue set is still needed at all.
- How the helix interacts with the grace window. The answer above bounds it: the comparison
  is against the expected index, so a one-turn lag is a phase and a two-turn lag is deviant.
  Whether grace permits the second turn per route is still per-route policy.

## Verifying the numbers in this ADR

```sh
go test ./internal/mcp_faults/ -run 'ComplementaryInvariants|ParityHelix|CoprimeHelices|Tautology|LoadInvariant|NonCoprime|Opposing|CRTMaps|DeltaNames|SequenceGives|NodeCCommits|HashHasNo|HashingDestroys|KeyedCommitment|GroupCommitment|CommitCosts|InversionGates|InversionHasPeriodTwo|AbelianInversion|NonAbelianConjugation|BloomJoins|BloomIsLossy|BraidsHaveNoTorsion|BloomAnswersReturn|FalseReturnIsAnAccusation|TorsionFreeSubstitutes|AdditiveLift|PhasesAreAQuotient|FilterOverTheQuotient|FastPathIsAnExclusion' -v
go test ./internal/mcp_faults/ -run XXX -bench 'BenchmarkResidue|BenchmarkHMAC|BenchmarkGroupCommit'
```

| claim | test |
| --- | --- |
| two complementary paths give relative position | `TestComplementaryInvariantsGiveRelativePosition` |
| per-turn detection is `1 − 2^−k` | `TestComplementaryInvariantsGiveRelativePosition` |
| a parity helix cannot see a drift of two | `TestParityHelixIsBlindToEvenDrift` |
| coprime moduli reconstruct the index exactly | `TestCoprimeHelicesReconstructTheIndex` |
| a derived helix is a tautology; a possessed one is not | `TestHelixOverDerivedValuesIsATautology` |
| the helix is load-invariant where the bias test is not | `TestHelixIsALoadInvariantCheck` |
| non-coprime moduli waste a path | `TestNonCoprimeModuliWasteAPath` |
| opposition doubles common-mode drift; sums cancel it | `TestOpposingHelicesSeeCommonModeDrift` |
| CRT maps a lie instead of rejecting it | `TestCRTMapsLiesItDoesNotRejectThem` |
| the delta names the corrupted path | `TestTheDeltaNamesTheLiar` |
| grace is a phase, and a freeze is deviance | `TestSequenceGivesTheExpectation` |
| C commits a verdict rather than counting votes | `TestNodeCCommitsRatherThanCounts` |
| a hash has no forward/reverse, only labels | `TestHashHasNoForwardOrReverse` |
| hashing destroys divisibility attribution | `TestHashingDestroysAttribution` |
| a key binds the opening and the epoch | `TestKeyedCommitmentBindsOpeningAndEpoch` |
| a group commitment keeps the sum and difference checks | `TestGroupCommitmentKeepsTheSumCheck` |
| tag cost is irrelevant at this epoch | `TestCommitCostsAreIrrelevantAtThisEpoch` |
| forced inversion gates the rotation exactly | `TestInversionGatesRotation` |
| inversion alone has period 2, and replay passes forever | `TestInversionHasPeriodTwo` |
| a per-epoch step does not lengthen an abelian period | `TestAbelianInversionWithAStepIsStillInvolution` |
| a non-abelian conjugation sets a period above 2 | `TestNonAbelianConjugationSetsThePeriod` |
| a Bloom filter is a commutative idempotent set; a braid is a stack | `TestBloomJoinsAndBraidsStack` |
| a Bloom filter is lossy; a braid is exact | `TestBloomIsLossyAndBraidIsExact` |
| torsion-freedom removes the periodic blind spot | `TestBraidsHaveNoTorsionSoNoBlindSpot` |
| a Bloom filter detects return; a torsion-free carrier precludes it | `TestBloomAnswersReturnBraidPrecludesIt` |
| a false return is a false accusation, priced per audience | `TestFalseReturnIsAnAccusation` |
| torsion-freedom substitutes for both residues and the filter | `TestTorsionFreeSubstitutesForBoth` |
| the additive lift is torsion-free | `TestAdditiveLiftIsTorsionFree` |
| a `1/q` phase has order exactly `q` | `TestPhasesAreAQuotientAndHaveTorsion` |
| a phase-keyed filter saturates at the first wrap | `TestFilterOverTheQuotientSaturates` |
| the fast path is an exclusion channel, 99% resolved locally | `TestFastPathIsAnExclusionChannel` |
