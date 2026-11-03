# ADR 0003 — A one-lag tick window: 10 s of skew, monitored

- **Status:** Proposed
- **Date:** 2026-10-22
- **Related:** [ADR 0001](0001-alarm-membership-tiers.md), [ADR 0002](0002-golden-is-a-derived-reference.md), [smoke-alarm#52][52]

## Context

The cluster rotates every 5 s. Accepting the current tick **and the one before it**, with
the two offset phase-wise, gives any auth or route a two-tick window rather than a
one-tick window:

```
epoch 5s, lagged epoch 5s, acceptance window 10s
skew 4s/9s/10s  accepted, both ticks live
skew 11s        outside the design window
```

Two consequences follow, and they point in opposite directions. The tolerance doubles,
which is good for clocks and bad for discrimination — except that the attacker now has
to fill *two* filters instead of one:

```
one epoch of design state: 80 000 insertions to saturate
two live epochs:          160 000 insertions (2.0x the attacker's work)
per-group share stays 1.33x of membership per epoch
```

## Decision

**Accept one lagged tick. Treat the resulting 10 s as the skew budget, and alarm on a
consistent breach rather than a sample.**

1. Both the current and the previous tick are valid; the window is `2 × 5 s`.
2. A clock is out of tolerance only when it is beyond the whole window *consistently*
   across the phase offset — one late sample is not a finding.
3. Monitoring is part of the decision, not an operational afterthought: without it the
   window is an assumption rather than a budget.
4. The per-filter grouping property is unchanged, because it is per filter and per epoch.

## Consequences

**Positive**

- Clock tolerance is 10 s, not 5 s, with no loss of discrimination per filter.
- The attacker's work doubles: both live epochs must be saturated to be sure of a
  uniform admission error.
- A transient clock excursion degrades nothing that a single sample would otherwise
  trigger.

**Negative and costs**

- Memory and insert budget are 2× the single-tick design, because two epochs must be
  represented at once.
- Two live epochs mean two legitimate truths; a disagreement inside the window is
  expected rather than a fault, which makes "which tick was this?" an explicit field
  rather than something inferred from arrival time.
- The bound is only real if monitored. Unmonitored, it silently becomes 5 s or 0 s
  depending on what breaks first.

**Risks**

- Phase offset without monitoring is worse than no offset: it hides a consistent drift
  behind occasional good samples.
- Shrinking the epoch to reduce latency reduces this budget proportionally, and the
  budget is a correctness bound, not a performance target.

## Alternatives considered

**One tick only.** Rejected: 5 s of tolerance, and every clock understanding of that
bound is a fire refusal — the worst error available.

**Three ticks.** Rejected: 3× the memory and a 15 s ambiguity window, for a doubling of
attacker cost that [ADR 0001](0001-alarm-membership-tiers.md)'s per-claimant cap already
bounds differently.

**A variable window sized by measured skew.** Rejected: the window is a correctness
bound that clients must be able to rely on, so it cannot move underneath them.

## What this does not decide

- The monitoring threshold and evaluation window (how many consecutive samples count as
  "consistent").
- The phase-offset schedule across sub-clusters.
- Whether the two live ticks share a filter or hold one each.

## Verifying the numbers in this ADR

```sh
go test ./internal/mcp_faults/ -run 'TickWindow|TwoLiveEpochs|SkewBudget' -v
```

| claim | test |
| --- | --- |
| the window is 10 s and 11 s is outside it | `TestTwoTickWindowDoublesTheSkewBudget` |
| two live epochs double the attacker's work | `TestTwoLiveEpochsDoubleTheAttackersCost` |
| the per-group share stays 1.33× per epoch | `TestGroupsOfThreeRestoreScaleInvariance` |

[52]: https://github.com/james-gibson/smoke-alarm/issues/52
