# ADR 0006 — The name space is isotropic

- **Status:** Proposed
- **Date:** 2026-10-22
- **Related:** [ADR 0001](0001-alarm-membership-tiers.md), [ADR 0002](0002-golden-is-a-derived-reference.md), [ADR 0005](0005-punish-floods-reward-derivation.md), [ADR 0007](0007-conveniences-are-attack-surfaces.md), [ADR 0008](0008-rotation-as-a-helix-of-residues.md)

## Context

The obvious red-team plan against a random-proxy naming scheme is the left-hand rule: pick
a direction, keep turning that way, and walk until the maze ends. It needs no map and no
secret. It converts a search into a traversal, which is the difference between exponential
and polynomial.

The rule only works if there is a wall. A maze has a wall because its geometry has a
gradient — the corridor is *next to* you, and "left" is monotonically related to progress.
A name space with a gradient is one where a probe's answer correlates with the probe's
position: the closer prefix relaxes faster, the near-miss is distinguishable from the far
miss, the ordering is observable, the timing differs. Under any of those, the follower is
doing something smarter than guessing.

So the question is not whether we hand out left and right. It is whether any probe we
answer leaks a direction. If one does, we have built a maze; if none does, we have built a
needle in a haystack and the left-hand rule is just a biased coin.

The two regimes are not symmetric in the attacker's favour, which is the point of this
record:

| name space | attacker's search | evidence they emit | detection |
| --- | --- | --- | --- |
| has a gradient | polynomial traversal | a directional bias | **caught in ~20 steps** |
| no gradient (isotropic) | `2^32` steps | none — an even coin | never caught, never won |

The follower cannot have both. That trade is the decision.

## Decision

**Isotropy is a security property. No probe may reveal a direction in the name space, and
no answer may depend on how close a guess is.**

Three requirements, in descending order of how easily they are violated:

1. **Acceptance is equality on an issued value.** Not a prefix match, not a similarity
   score, not a "did you mean". A guess is right or wrong; there is no near.
2. **The price of a wrong guess is independent of its distance.** Every wrong guess pays
   the same tier. If a near miss were cheaper, the price itself would be the gradient, and
   an attacker would hill-climb on cost rather than wait for a hit.
3. **No ordering, timing or error-detail gradient.** Responses must not vary with how close
   the guess was, in latency, in verbosity, or in which field is named first.

Under those three, a left-hand rule is not a traversal. It is a sequence of biased coin
flips that never learns anything, and the bias is a signature.

## The numbers

Detection is a sequential test, so its cost is a closed form. Evidence accrues at the KL
divergence of the follower's side-choice bias from an even coin, and latency is that step
count divided by the arrival rate:

```
steps    = ln(1/α) / D(p ‖ ½)
latency  = steps / rate
```

with `D(p‖½) = p·ln(2p) + (1−p)·ln(2(1−p))` in nats, and `α = 10⁻³` one false accusation in
a thousand claimants.

| side bias | nats/step | steps | latency at 40/s |
| --- | --- | --- | --- |
| 0.99 | 0.637 | 11 | 271 ms |
| 0.90 | 0.368 | 19 | **469 ms** |
| 0.75 | 0.131 | 53 | 1.3 s |
| 0.60 | 0.020 | 343 | 8.6 s |
| 0.51 | 0.0002 | 34 536 | 14 min |
| 0.505 | 0.00005 | 138 153 | 58 min |
| 0.501 | 0.000003 | 2 210 479 | 15 h |

**A crude wall-follower is caught in under half a second** at the per-claimant ceiling. A
subtle one buys time quadratically: `D ≈ 2ε²` near an even coin, so halving the bias
quadruples the samples. That is the early/late split — early detection is cheap and catches
the crude follower; late detection is expensive and catches the patient one, at 117 000× the
latency for the same confidence.

### Why the cheaper one-sided test does not apply

`detection_threshold_test.go` already bounds bias detection exactly, and its numbers are
much smaller: 66 readings catch a 10% error rate at this confidence. It is tempting to
reuse that. It answers a different question, and the difference is 5.2×.

| observable | test | 10% deviation | at 40/s |
| --- | --- | --- | --- |
| a reading is *wrong* 10% of the time | one-sided vs 0 | 66 readings | 1.7 s |
| a side choice is biased 60/40 | two-sided vs ½ | 343 probes | 8.6 s |

A compromised resolver leaks a bit on every wrong answer, because a wrong answer is
*observable as wrong*. A wall-follower leaks nothing per probe: every probe is refused with
the same reason an honest miss gets (`fuzzed-or-wrong-prefix`), so no single probe can be
counted as an error. The only evidence is the aggregate pattern of sides, which is a
two-sided test against a fair coin, and that is inherently more expensive
(`TestTheCheapOneSidedTestDoesNotApply`).

This is also the honest limit of the whole approach: if a follower's probes become
individually diagnosable — a distinguishable near-miss, a different refusal reason, a
shorter latency — then the cheaper one-sided test applies, and the estimator is 5× faster
for the same deviation. Isotropy is what keeps the defender on the expensive test.

### Under load, detection gets *faster*

| arrival rate | latency at bias 0.9 |
| --- | --- |
| 1/s | 18.8 s |
| 5/s | 3.8 s |
| 10/s | 1.9 s |
| 40/s | 469 ms |
| 400/s | 47 ms |

This runs opposite to the usual DoS intuition, because load is *samples*. It also makes the
per-claimant ceiling a security parameter rather than a throughput limit: the attacker
cannot send faster than `40/s`, so detection can never be slower than `steps / 40`. The
floor is 469 ms, not zero, and not unbounded.

The corollary is the follower's dilemma:

- send fast enough to finish the maze and you are spotted in half a second;
- send slowly enough to hide and the maze takes `2^32` steps at 1/s.

A randomized follower — one that flips an even coin to defeat the bias test — has zero
drift, so no number of samples catches it. It also has no gradient to follow: 4.29 × 10⁹
expected steps, **3.4 years** at the ceiling. The only control left on that branch is price,
not time, which is [ADR 0005](0005-punish-floods-reward-derivation.md)'s subject: every
attempt costs 8, so an identity survives 8 guesses before the eviction threshold.

## Consequences

**Positive**

- The name space converts a polynomial search into an exponential one. That is the whole
  security value of randomness here, and it only holds while isotropy holds.
- A deterministic attacker is *cheaper* to detect than a random one, because determinism is
  the only thing they cannot hide while still making progress. Bias is free evidence.
- The detection bound is expressible: `ln(1/α)/D(p‖½)/rate`, with the rate capped. An
  operator can state a detection SLO in seconds and derive the bias it catches.
- Both sides cost the same, measurably: 256 left-hand and 256 right-hand probes, 0 accepted,
  `8.0` per attempt each. There is no direction to prefer.

**Negative and costs**

- **We cannot use a proximity oracle for anything, including legitimate work.** No
  "did you mean", no near-miss diagnostics, no partial-prefix matching, no ordering hints in
  migration tooling. Diagnosability is the price of isotropy.
- The bias test needs a per-claimant view to be meaningful, so detection is only as good as
  the attribution that feeds it.
- A follower whose bias is below ~0.51 is effectively undetectable (`2 × 10⁶` samples), and
  also effectively not following. The bound is honest but it does weaken to nothing at the
  limit.

**Risks**

- **A gradient is easy to add by accident.** Latency that varies with prefix similarity, an
  error message that names the first differing character, a per-probe diagnosis better than
  "wrong", a cache that warms toward the true value, a comparison that short-circuits — each
  is a wall, and each also moves the defender onto the 5.2× cheaper test. The failure is silent: search
  still looks random from the outside while being polynomial for an attacker who notices.
- **A "closer/farther" oracle is exactly what a debugging tool wants.** Any future endpoint
  offering guess-quality feedback reintroduces the maze, so this decision constrains the
  product surface, not just the data path.
- Under sustained load the attacker's own arrival rate rises, which shortens detection but
  also shortens the time they need; the two scale together, and only the ceiling breaks the
  tie.

## Alternatives considered

**Hide the direction by making the format obscure.** Rejected: the source is open, and
[ADR 0005](0005-punish-floods-reward-derivation.md) already establishes that nothing may
rest on obscurity. The direction must be absent, not secret.

**Accept a small gradient in exchange for diagnostics.** Rejected: a small gradient is a
polynomial search. The whole value of the random name is that the search is exponential, and
`2^32` degraded to `2^16` is a four-billion-fold loss of margin for a developer convenience.

**Detect the follower by rate rather than bias.** Rejected as insufficient: rate is capped
per claimant anyway, and a randomized follower has no bias to detect. Price is what bounds
that branch, not detection.

## What this does not decide

- Whether the bias test runs at admission, at the mirror, or offline over a claim log.
- How many claimants' evidence may be pooled before attribution becomes unfair to a client
  with an unusual but honest access pattern.
- Whether a detected follower is evicted, throttled, or merely reported — the same
  quarantine-versus-eviction question [ADR 0001](0001-alarm-membership-tiers.md) leaves per
  route.
- The `α` used in production, which trades false accusations against detection latency.

## Verifying the numbers in this ADR

```sh
go test ./internal/mcp_faults/ -run 'WallFollower|DetectionLatency|Subtlety|RandomizedFollowing|NoGradient|CheapOneSided' -v
```

| claim | test |
| --- | --- |
| a biased follower is caught in ~20 steps | `TestWallFollowerIsCaughtByItsOwnBias` |
| latency is `steps / rate`, floored by the ceiling | `TestDetectionLatencyIsStepsOverRate` |
| halving the bias costs ~4× the samples | `TestSubtletyCostsQuadratically` |
| a randomized follower is a `2^32` search | `TestRandomizedFollowingIsJustAGuess` |
| left and right cost exactly the same | `TestTheNameSpaceHasNoGradient` |
| the one-sided test is 5.2× cheaper and does not apply | `TestTheCheapOneSidedTestDoesNotApply` |
