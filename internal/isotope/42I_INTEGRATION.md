# 42i Integration with Isotope System

The isotope system integrates with the 42i mathematical framework to track agent capability through test failures mapped to imaginary-space distances.

## Core Model

### Agent Position in 42i Space

Each agent/service has a position: `42 + 42i_distance * i`

- **Real axis**: Always 42 (ground truth anchor)
- **Imaginary axis**: Accumulated from isotope test failures (distance from ground truth)
- **Magnitude**: `sqrt(42² + distance²)` - overall trustworthiness metric

```
Agent at 42 + 0i        → Perfect, all tests pass
Agent at 42 + 16i       → One test failed, slight drift
Agent at 42 + 100i      → Multiple failures, significantly drifted
Agent at 42 + 220i      → Extreme drift, near maximum allowed distance
```

### Rung Boundaries

Agents climb from rung 0 (untrusted) to rung 6 (fully certified) as they pass tests and keep 42i_distance low:

| Rung | Name | Max Distance | Description |
|------|------|--------------|-------------|
| 0 | Absolute Zero | 0 | Bootstrapped, no certifications |
| 1 | Read-Only | 20 | Entropy & isotope variation verified |
| 2 | Harness Tools | 60 | Tool scope defined, input responsiveness proven |
| 3 | Mock Secrets | 100 | Safe isotope handling, declared behavior verified |
| 4 | Higher Authority | 140 | Timing uncorrelated, independent decisions |
| 5 | Delegation | 180 | Can delegate to peer services |
| 6 | Certification | 220 | Can certify other agents |

### Test Failure Weights

Each isotope test family contributes to 42i_distance when it fails:

```go
var DefaultWeights = map[string]int{
    "entropy-check":                16, // Output unpredictability
    "input-correlation":            12, // Input responsiveness
    "constant-output-detection":    20, // Output independence
    "isotope-variation":             8, // Signature uniqueness
    "timing-correlation":           24, // Entity independence
    "agreement-pattern":            32, // Decision independence
    "state-bleed-detection":        28, // State isolation
    "harness-blindness":            16, // Tool scope
    "declared-behavior-compliance": 12, // Spec adherence
    "scope-compliance":             20, // Boundary respect
    "secret-flow-violation":        24, // Isotope containment
}
```

## Usage

### Recording Isotope Test Results

```go
agent := isotope.NewAgentState("service-a")

// When an isotope test runs
testIsotope := isotope.Isotope{
    Family:  "entropy-check",
    Version: 1,
    Signature: "...",
}

result := agent.RecordIsotopeTestResult(testIsotope, passed)

// Result includes:
// - BeforePosition: agent's position before test
// - AfterPosition: agent's position after test
// - Weight: how much this test contributed
// - RungChanged: whether agent was promoted/demoted
// - CostAdjustment: lemon cost multiplier
```

### Checking Rung Boundaries

```go
alert := agent.CheckRungBoundary()

// Status can be:
// - "ok": healthy, rung stable
// - "warning": approaching rung threshold (40-20 units away)
// - "critical": very close to demotion (< 20 units)
// - "demoted": exceeded threshold, promoted to higher-numbered rung

if alert.Status == "critical" {
    fmt.Println(alert.Message)
    // "CRITICAL: 42i_distance=95 approaching rung 3 threshold of 100 (only 5 units remaining)"
}
```

### Cost Adjustment

As 42i_distance increases, operational costs increase:

```go
baseCost := 1 << uint(rung+4)  // 2^(rung+4)
adjustedCost := int(float64(baseCost) * (1 + float64(distance)/100))

// Example:
// Rung 3: base_cost = 128 lemon units
// distance = 50: adjusted = 128 × 1.5 = 192 units
// distance = 100: adjusted = 128 × 2 = 256 units
```

### Byzantine Consensus Gaps

When alarms disagree on isotope, that failure contributes to 42i_distance:

```go
gap := isotope.ConsensusGap{
    IsotopeFamily: "entropy-check",
    AlarmCount:    1,        // 1 alarm disagreed
    TotalAlarms:   3,        // out of 3 total
    GapWeight:     48,       // 16 * 1 alarm + 32 quorum penalty
    Reason:        "Only 2/3 alarms agreed",
}

agent.RecordConsensusFailure(gap)
// agent.TotalDistance increases by 48
```

## Direction Inference

Failed tests determine qualitative direction:

```
entropy-check + input-correlation fail
  → "unpredictable-behavior" (agent is erratic)
  → Vector toward chaos in 42i space

agreement-pattern + timing-correlation fail
  → "coordinated-signaling" (agent conspires with others)
  → Vector toward conspiracy in 42i space

harness-blindness + secret-flow-violation fail
  → "unauthorized-access" (agent sees/uses forbidden tools)
  → Vector toward breach in 42i space

state-bleed-detection + scope-compliance fail
  → "boundary-violation" (agent violates isolation)
  → Vector toward violation in 42i space
```

## Automatic Demotion

When 42i_distance exceeds a rung's threshold, agent is automatically demoted:

```
Rung 4, max distance 140
  agent.TotalDistance = 135 (ok)
  agent.TotalDistance = 145 (EXCEEDS THRESHOLD)

  → Agent drops to rung 3 (max distance 100)
  → If 145 > 100: drops to rung 2
  → Continues until distance fits

Final position: 42 + 145i, rung 1 (max 20)
```

## Real-Time Monitoring

Example timeline:

```
T=0: Agent at rung 6, distance=0, all tests pass

T=100ms: entropy-check fails
  - distance → 16
  - rung → 6 (still fits, 16 <= 220)
  - alert: "ok"

T=500ms: input-correlation fails
  - distance → 28
  - rung → 6 (still fits, 28 <= 220)
  - alert: "warning" (approaching rung 5 boundary at 180? no...)

T=1000ms: constant-output-detection fails
  - distance → 48
  - rung → 6
  - alert: "warning"

T=2000ms: agreement-pattern fails
  - distance → 80
  - rung → 6 (80 <= 220)
  - alert: "ok" (plenty of room at rung 6)

T=5000ms: state-bleed-detection fails
  - distance → 108
  - rung → 6
  - alert: "critical" (108 + 140 = 248 > 220, approaching rung 5 threshold at 180)

T=6000ms: Consensus gap detected (2 alarms disagree)
  - distance → 156
  - rung → 6 (still fits, 156 <= 220)
  - alert: "critical" (156 + 24 more and we're demoted)

T=7000ms: One more failure
  - distance → 172
  - rung → 6 (fits but barely: 172 <= 220)
  - alert: "critical" (172 + 48 = 220, exactly at boundary)

T=8000ms: Another failure
  - distance → 188
  - rung → DEMOTED to 5 (188 > 180 threshold for rung 5)
  - Effective cost multiplier increases
  - JWT scopes downgraded
  - Alert: "DEMOTED: 42i_distance=188 exceeds rung 6 threshold, moved to rung 5"
```

## Integration with Fire-Marshal

Fire-marshal generates test harnesses that tag results with isotopes. Each failure contributes to 42i_distance. When consensus forms across multiple alarms on an isotope test result:

1. **Isotope signature verified** - cryptographic proof test was run
2. **42i distance calculated** - weight from DefaultWeights lookup
3. **Rung boundary checked** - automatic demotion if exceeded
4. **Cost adjusted** - future operations charged at new rate
5. **Escalation triggered** - if critical threshold reached

## See Also

- `USAGE.md` - Isotope system usage examples
- `README.md` - Isotope system architecture overview
- `/seeds/smoke-alarm-42i-mapping` - Original 42i test mapping specification
- `/seeds/capability-lattice` - Unified rungs/JWT/lemon cost model
