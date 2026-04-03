# Chaos Testing: Validating Cluster Eviction Behavior

**Purpose**: Simulate service degradation in controlled phases, measure when consensus detects failure, and verify cluster eviction policies work correctly.

This extends the dev mode simulator with systematic chaos testing: you can intentionally degrade a service, watch how the cluster responds, and measure the cost accumulation.

## Quick Start

```go
cluster := isotope.NewServiceCluster(5, []byte("key"))

service := isotope.NewHostedService("api-server")
service.RegisterTest("health-check", &isotope.ServiceTest{
    SLALatencyMs: 100,
    TestFunc: func() bool {
        return true
    },
})

cluster.HostService(service)

// Run chaos: gradual latency degradation
profile := isotope.LatencyEscalationProfile()
result := cluster.RunChaosTest(service, profile)

fmt.Printf("Detection cycle: %d\n", result.DetectionCycle)
fmt.Printf("Eviction cycle: %d\n", result.EvictionCycle)
fmt.Printf("Final distance: %d\n", result.FinalDistance)
fmt.Printf("Recovery: %v\n", result.RecoverySucceeded)
```

## Available Chaos Profiles

### 1. Latency Escalation

```go
profile := isotope.LatencyEscalationProfile()
```

Phases:
- Baseline: 2 cycles (healthy)
- +25% latency: 3 cycles
- +50% latency: 3 cycles
- +75% latency: 3 cycles (severe SLA violations)

**Measures**: How quickly consensus detects performance degradation and when the service would be evicted.

### 2. Entropy Collapse

```go
profile := isotope.EntropyCollapseProfile()
```

Phases:
- Baseline: 2 cycles (healthy)
- Entropy loss: 5 cycles (constant output)

**Measures**: Impact of lost entropy (compromised randomness) on cluster consensus.

### 3. Input Ignorance

```go
profile := isotope.InputIgnoranceProfile()
```

Phases:
- Baseline: 2 cycles (healthy)
- Input ignorance: 5 cycles (service stops responding to inputs)

**Measures**: Detection speed for input-correlation failures.

### 4. Cascading Failure

```go
profile := isotope.CascadingFailureProfile()
```

Phases:
- Baseline: 2 cycles
- Stage 1: Latency (+50%): 3 cycles
- Stage 2: Entropy loss: 3 cycles
- Stage 3: Input ignorance: 3 cycles

**Measures**: How the cluster handles multi-stage degradation and whether consensus remains robust through each phase.

## Understanding the Results

```go
type ChaosTestResult struct {
    ServiceID          string           // Service being tested
    Profile            ChaosProfile     // Which chaos scenario was run
    Timeline           []ChaosSnapshot  // State at each cycle
    DetectionCycle     int              // When consensus first failed (-1 if never)
    EvictionCycle      int              // When service hit eviction threshold (-1 if never)
    FinalDistance      int              // Final 42i_distance
    FinalRung          int              // Final rung position
    TotalCycles        int              // Total test cycles run
    RecoverySucceeded  bool             // Could service recover after chaos?
    TimeToDetection    int              // Cycles until consensus failure
    TimeToEviction     int              // Cycles until eviction threshold
}
```

### Metrics to Watch

- **TimeToDetection**: How many cycles before consensus detected the service was unhealthy
  - Fast detection (2-4 cycles): Cluster responds quickly to issues
  - Slow detection (>5 cycles): May allow bad service to linger

- **TimeToEviction**: When service's 42i_distance crossed the eviction threshold (rung > 3)
  - Should correlate with DetectionCycle (detection → accumulation → eviction)

- **FinalDistance**: Total cost accumulated during chaos
  - Higher distance = more severe degradation
  - Used to calculate rung position

- **RecoverySucceeded**: Service recovers fully after degradation stops
  - Should always be true if service can reset cleanly
  - False suggests lingering state or detection bias

## Timeline Snapshots

Each cycle captures:

```go
type ChaosSnapshot struct {
    Cycle              int          // Test cycle number
    ServiceDistance    int          // Service's 42i_distance at this cycle
    ServiceRung        int          // Service's rung (0-6)
    ConsensusFormed    bool         // Did alarms agree on health?
    HealthyAlarmCount  int          // How many alarms reported health
    FailedAlarmCount   int          // How many alarms reported failure
    AverageLatencyMs   int          // Mean latency this cycle
    SuccessRate        float64      // % of tests that passed (0.0-1.0)
}
```

Plot the timeline to see degradation patterns:

```go
for _, snap := range result.Timeline {
    fmt.Printf("Cycle %d: distance=%d, rung=%d, consensus=%v, success=%.1f%%\n",
        snap.Cycle, snap.ServiceDistance, snap.ServiceRung,
        snap.ConsensusFormed, snap.SuccessRate*100)
}
```

## Common Scenarios

### Scenario 1: Verify Fast Detection

You want consensus to detect unhealthy services within 2-3 cycles.

```go
cluster := isotope.NewServiceCluster(5, []byte("key"))
service := isotope.NewHostedService("api")
service.RegisterTest("health", ...)
cluster.HostService(service)

result := cluster.RunChaosTest(service, isotope.EntropyCollapseProfile())

if result.TimeToDetection <= 3 {
    fmt.Println("✓ Cluster detects failures quickly")
} else {
    fmt.Printf("✗ Slow detection: %d cycles\n", result.TimeToDetection)
}
```

### Scenario 2: Measure Eviction Threshold

At what distance do you want to evict a service? Test to find it.

```go
result := cluster.RunChaosTest(service, isotope.CascadingFailureProfile())

fmt.Printf("Eviction at distance=%d, rung=%d\n", result.FinalDistance, result.FinalRung)
// Use this to tune your eviction policy in production
```

### Scenario 3: Compare Profiles

Which degradation pattern is most detectable?

```go
profiles := []isotope.ChaosProfile{
    isotope.LatencyEscalationProfile(),
    isotope.EntropyCollapseProfile(),
    isotope.InputIgnoranceProfile(),
}

for _, profile := range profiles {
    result := cluster.RunChaosTest(service, profile)
    fmt.Printf("%s: detection at cycle %d\n", profile.Name, result.TimeToDetection)
}
```

### Scenario 4: Test Recovery

Verify services can fully recover from degradation.

```go
result := cluster.RunChaosTest(service, isotope.LatencyEscalationProfile())

if !result.RecoverySucceeded {
    t.Error("Service failed to recover—lingering state or detection issue")
}
```

## Integration with Fire-Marshal

Chaos mode in the dev cluster validates the behavior you'd expect in production:

1. **In dev**: `RunChaosTest()` shows cost accumulation and eviction thresholds
2. **In seed set**: Calculate expected 42i_distance for real failure patterns
3. **In fire-marshal**: Use those thresholds to evict unhealthy services

The dev cluster gives you fast feedback on eviction policies before deploying.

## Limitations

Current chaos mode does NOT simulate:
- Network latency or partition
- Goroutine races or timing issues
- Cascading failures across multiple services
- Delayed consensus (all alarms test immediately in parallel)

For those scenarios, use the Docker Compose or Kubernetes deployment.

## Example Test Suite

```go
func TestClusterEvictionPolicy(t *testing.T) {
    cluster := isotope.NewServiceCluster(5, []byte("key"))

    service := isotope.NewHostedService("critical-api")
    service.RegisterTest("liveness", &isotope.ServiceTest{
        SLALatencyMs: 100,
        TestFunc: func() bool { return true },
    })

    cluster.HostService(service)

    // Test 1: Fast detection of entropy loss
    result1 := cluster.RunChaosTest(service, isotope.EntropyCollapseProfile())
    if result1.TimeToDetection > 4 {
        t.Errorf("Too slow to detect entropy loss: %d cycles", result1.TimeToDetection)
    }

    // Test 2: Cascading failures still trigger eviction
    result2 := cluster.RunChaosTest(service, isotope.CascadingFailureProfile())
    if result2.EvictionCycle == -1 {
        t.Error("Service should reach eviction threshold")
    }

    // Test 3: Recovery works
    if !result2.RecoverySucceeded {
        t.Error("Service should recover after chaos ends")
    }
}
```

## See Also

- **DEVMODE.md** - Dev mode simulator basics
- **QUICKSTART.md** - Getting started with isotopes
- **42I_INTEGRATION.md** - How 42i distance and rungs work
