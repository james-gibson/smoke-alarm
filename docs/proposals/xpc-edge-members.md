# Proposal — XPC edge handlers as observation members

- **Status:** Draft for discussion. Not an ADR: it proposes a component, and the decisions it
  depends on are already recorded.
- **Date:** 2026-10-22
- **Consumer:** `lezz.sea` (hosted edge), `lezz` (local CLI and agents)
- **Depends on:** [0001](../adr/0001-alarm-membership-tiers.md) tiers, [0002](../adr/0002-golden-is-a-derived-reference.md) golden, [0005](../adr/0005-punish-floods-reward-derivation.md) derivation, [0006](../adr/0006-the-name-space-is-isotropic.md) isotropy, [0007](../adr/0007-conveniences-are-attack-surfaces.md) conveniences, [0008](../adr/0008-rotation-as-a-helix-of-residues.md) helix, [0009](../adr/0009-the-side-channel-three-filters.md) side channel, [0010](../adr/0010-trust-rotates-with-the-layout.md) trust rotation, [0011](../adr/0011-redundancy-is-a-route-property.md) route redundancy

## 1. What this proposes

An **XPC handler** is a per-host process that sits in the **read path** of a cluster and reports
what it observes. It is not a store and not a write replica: the reason to put a member at the
edge is that observation is the cheap side of the protocol and the side the cluster is blind on.

"XPC" here means *cross-process communication*, not Apple's transport: a unix socket at
`~/.lezz/run/xpc.sock`, peer credentials checked, so the same handler runs on macOS and on
whatever `lezz.sea` deploys.

The asymmetry that motivates the whole component:

| | rate | cap |
| --- | --- | --- |
| local reads (validate, observe) | ~1 000 000/s per core at 1 µs each | **no cap** |
| claimant writes (alarms, rotations) | 40/s | per-claimant ceiling |

**Observation bandwidth is 25 000× the write ceiling.** The handler is sized for observations
per second, and its memory is a filter slice, not a store.

## 2. What an edge observes

| observation | why the cluster cannot see it | what it feeds |
| --- | --- | --- |
| every validation and its side choice | the caller's side choice never leaves the host otherwise | bias test, 19 probes at 0.9 ([0006](../adr/0006-the-name-space-is-isotropic.md)) |
| helix residues per turn | residues are per-route, per-turn state | position and attribution ([0008](../adr/0008-rotation-as-a-helix-of-residues.md)) |
| canary fill per route | the canary is only meaningful at route granularity | 16.1× earlier saturation warning ([0009](../adr/0009-the-side-channel-three-filters.md)) |
| staleness and grace remaining | a local clock and a local ledger | partition behaviour, `stale-epoch` |
| refusal classes | coarse on the wire, fine at the observer | gate enumeration, 2.32 bits/probe ([0007](../adr/0007-conveniences-are-attack-surfaces.md)) |
| latency per call | the same host, so no network term | skew and saturation symptoms |
| **silence** | nobody else notices an observer that stopped | `unexamined`, below |

The last row has no write-side analog and is the point of §6.

## 3. Why the edge is the right place to observe

Two properties, both measured:

**Complete, not sampled.** The handler is in the read path, so it counts events that were going
to happen anyway. A periodic scraper samples, and sampling multiplies detection latency by the
sample interval:

| sample rate | events needed | latency at 40/s |
| --- | --- | --- |
| every event | 19 | **475 ms** |
| 1 in 10 | 190 | 4.8 s |
| 1 in 100 | 1 900 | 47.5 s |

**Cheap.** At 1 µs per observation, one core sustains a million of them; the write ceiling caps
the claimant at 40/s. There is no budget pressure on the read side, which is why the handler can
afford to observe *everything* rather than a sample.

## 4. The aggregation requirement is the whole design

Per-edge evidence dilutes when an attacker spreads. The same claimant keeps its 40/s ceiling
but hands each edge a fraction:

| edges | per edge | detection without aggregation | with aggregation |
| --- | --- | --- | --- |
| 1 | 40.0/s | 475 ms | 475 ms |
| 4 | 10.0/s | 1.9 s | **475 ms** |
| 10 | 4.0/s | 4.8 s | **475 ms** |
| 100 | 0.4/s | **47.5 s** | **475 ms** |

Spreading buys the attacker a linear factor in detection latency, and **an aggregator removes
it entirely**. So the handlers only make sense as a fleet feeding one evidence sink; deployed
without it they are a hundred separate blind spots.

That is the central decision this proposal asks for, and it belongs to `lezz.sea`: an
**evidence aggregator** that pools per-route observations across edges and evaluates the bias,
helix and canary tests on the pooled stream.

## 5. Observation needs the same redundancy as service

A route is blind when every observer of it is compromised or silent:

| observers | route blind if | exposure at 1 % |
| --- | --- | --- |
| 1 | that one is compromised | 1.0000 % |
| 2 | both are | **0.0100 %** |
| 3 | all three are | 0.0001 % |

Two is the same knee as service replicas ([0011](../adr/0011-redundancy-is-a-route-property.md)):
the second observer buys two nines, the third buys almost nothing. A single observer is not a
cheaper design, it is a route whose observation is one compromise deep.

Corroboration follows: the same claim validated by two independent observers is worth more than
either alone, and the aggregator should require agreement for any conclusion it acts on.

## 6. Silence is an observation

An observer that stops reporting is not a quiet route. The verdict for a route with no
observations is `unexamined`, and **unexamined is not innocent**
([0005](../adr/0005-punish-floods-reward-derivation.md)).

Concretely, the aggregator must distinguish:

- **no traffic** — the route is idle; nothing to observe, and idle is not suspicious.
- **no observer** — the route had traffic and nobody reported it; that is a gap, and it is
  reported as such.

The second is an alarm about the observation plane itself, which is the class of failure a
write-oriented design cannot see.

## 7. What the handler must not do

- **Conclude.** It reports evidence; the aggregator decides. No local eviction, no local
  quarantine of another member.
- **Sample silently.** If it must drop observations under load, it reports the drop rate; an
  unmeasured sample is a false confidence.
- **Answer proximity questions.** Equality is the only acceptance rule, and the read surface
  must not grow a "how close was that" answer ([0006](../adr/0006-the-name-space-is-isotropic.md)).
- **Expose fine refusal detail to clients.** Coarse on the wire, fine to the aggregator
  ([0007](../adr/0007-conveniences-are-attack-surfaces.md)).
- **Become a store.** The moment it writes records it inherits the 11.3 s pipeline and the
  stripe discipline, for no observation benefit.

## 8. Surface

Read side, streaming:

```
Observe{route, tick, kind, value}   → per-event or batched, 1 µs each
Validate{stamp}                     → ≤ 1 ms, verdict plus a coarse reason
Snapshot{epoch}                     → filter digest, canary state, grace remaining
Hello                               → protocol version, epoch, cluster id
```

Write side, three operations only:

```
Join{route, tier}      → member id, slice, filter digest
Adopt{seq}             → replay forward after a partition
Leave                  → slot released
```

The asymmetry is deliberate: three writes, an unbounded read stream. If the handler needs a
fourth write operation, that is a signal it has drifted into being a store.

## 9. Budget

| work | cost | frequency |
| --- | --- | --- |
| observe one event | 1 µs | unbounded |
| validate a stamp | ≤ 1 ms | per call |
| filter digest apply | ≤ 250 ms | per tick |
| evidence ship | batched, ≤ 250 ms | per tick |
| derivation rebuild | not performed | — |
| local store | none | — |

On a partition the handler buffers evidence for at most one epoch and reports the gap; it does
not grow the buffer to hide the gap.

## 10. What `lezz.sea` must provide

1. **An evidence aggregator** — pooled per-route observations, evaluated by the existing tests.
   Without it, edge observation is worse than useless (§4).
2. **Corroboration** — two independent observers before any conclusion the cluster acts on (§5).
3. **A blindness alarm** — silence reported as `unexamined`, never as healthy (§6).
4. **A privacy boundary** — pooled evidence is telemetry about callers, so it must be coarse,
   aggregate and retention-limited, or the observation plane becomes the disclosure surface
   [ADR 0007](../adr/0007-conveniences-are-attack-surfaces.md) exists to prevent.

## 11. Rollout

| phase | capability | risk |
| --- | --- | --- |
| 1 | observe and ship, no local verdicts | none: reporting only |
| 2 | aggregate across edges, corroborate | none: analysis only |
| 3 | local `Validate` using the derived state | a wrong verdict costs one refused call |
| 4 | `Join`/`Adopt`, local `Issue` | bounded by grace ([0002](../adr/0002-golden-is-a-derived-reference.md)) |

Phases 1 and 2 are the ones that answer the question this proposal started from: they need no
write path at all, and they close the observation gap that a cluster of any size currently has.

## 12. Open questions

- **Evidence volume.** Per-event shipping at 1 µs of observation is cheap on the observer and
  not necessarily cheap on the aggregator; batching windows need a stated loss bound.
- **Silence threshold.** Time-based ("no reports in N ticks") or count-based ("no reports for M
  events")? The second needs the aggregator to know M, which is itself an observation.
- **Sampling policy.** High-volume routes may need sampling; if so, the sample rate must be
  reported with the evidence, or every detection latency silently multiplies (§3).
- **Corroboration cost.** Two observers per route doubles the observer count; for a route served
  by one host that is impossible, so single-host routes need a stated exemption and a stated
  discount in their trust.
- **Whether `Observe` is a separate socket.** A read stream on the same socket as `Validate`
  couples the observation plane's backpressure to the caller's latency.

## 13. What would make this an ADR

The component does not need one; its decisions do. The candidates, in the order they will be
forced: the aggregation requirement (§4), the two-observer rule (§5), and silence-as-unexamined
(§6). Each is a short record with a test once the implementation settles the numbers, in the
same form as the ten existing ADRs.
