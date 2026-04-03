# Isotope Quick Start: Building Local Test Clusters

## The Problem You Identified

**In production**: You need 3-7 independent smoke-alarm instances running as separate services. They each run tests, report isotopes, and reach Byzantine consensus.

**In development**: Setting up that infrastructure locally is tedious. You can't easily test if consensus works without deploying multiple services.

**Solution**: Dev mode lets you simulate N alarms in a single process, giving you instant feedback.

## 30-Second Example

```go
// Create a 3-alarm cluster in your test
cluster := isotope.NewDevModeSimulator(3)

// Define a test
test := isotope.Isotope{
    Family:    "entropy-check",
    Version:   1,
    Signature: "sig",
    Raw:       "raw",
}

// Run test across all alarms
result, _ := cluster.SimulateTest(test, true)

// Check consensus
if result.ConsensusFormed && result.AgreedCount == 3 {
    fmt.Println("✓ Consensus formed: 3/3 alarms agree")
}
```

## Common Development Tasks

### Task 1: Verify Test Stability Through Refactoring

```go
func TestRefactoring(t *testing.T) {
    cluster := isotope.NewDevModeSimulator(3)
    ledger := cluster.GetLedger()

    // Original test version
    v1 := isotope.Isotope{Family: "login-test", Version: 1, Signature: "sig1", Raw: "raw1"}

    // Run 3 times
    cluster.SimulateTest(v1, true)
    cluster.SimulateTest(v1, true)  // Refactored (same isotope)
    cluster.SimulateTest(v1, true)  // Refactored again

    // Check stability
    h := ledger.QueryByIsotope("login-test", 1)
    if h.Stability > 0.99 {
        t.Log("✓ Test stable through refactoring")
    }
}
```

### Task 2: Test Byzantine Tolerance

```go
func TestByzantineDetection(t *testing.T) {
    // 5-alarm cluster tolerates F=1 failure
    cluster := isotope.NewDevModeSimulator(5)

    test := isotope.Isotope{Family: "security-check", Version: 1, Signature: "sig", Raw: "raw"}

    // Run healthy
    result1, _ := cluster.SimulateTest(test, true)
    if result1.ConsensusFormed && result1.AgreedCount == 5 {
        t.Log("✓ All 5 alarms healthy")
    }

    // Compromise one alarm
    cluster.InjectByzantineFailure(0)

    // Run again
    result2, _ := cluster.SimulateTest(test, true)
    if result2.ConsensusFormed && result2.AgreedCount >= 3 {
        t.Log("✓ Quorum still formed (4/5 > 3/5)")
    }

    // Check detection
    distances := cluster.GetAlarmDistanceSummary()
    if distances["alarm-0"] > 0 {
        t.Log("✓ alarm-0 detected as compromised")
    }
}
```

### Task 3: Detect Regressions Across Multiple Test Suites

```go
func TestMultipleSuites(t *testing.T) {
    cluster := isotope.NewDevModeSimulator(3)

    // Same test in multiple languages/formats
    testEnglish := isotope.Isotope{Family: "user-auth", Version: 1, Signature: "sig-en", Raw: "raw-en"}
    testSpanish := isotope.Isotope{Family: "user-auth", Version: 1, Signature: "sig-es", Raw: "raw-es"}

    // Both should have same isotope (canonical form)
    cluster.SimulateTest(testEnglish, true)
    cluster.SimulateTest(testSpanish, true)
    cluster.SimulateTest(testEnglish, false)  // Regression!

    // Query ledger
    ledger := cluster.GetLedger()
    h := ledger.QueryByIsotope("user-auth", 1)

    t.Logf("✓ Aggregated: %d pass, %d fail (%.1f%% stable)",
        h.PassCount, h.FailCount, h.Stability*100)
}
```

### Task 4: Run Pre-Built Test Scenarios

```go
func TestScenarios(t *testing.T) {
    // Option 1: Simple 3-alarm consensus
    cluster1 := isotope.NewDevModeSimulator(3)
    cluster1.RunScenario(isotope.StandardThreeAlarmScenario())

    // Option 2: Test through refactoring cycle
    cluster2 := isotope.NewDevModeSimulator(3)
    cluster2.RunScenario(isotope.RefactoringScenario())

    // Option 3: 5-alarm Byzantine robustness
    cluster3 := isotope.NewDevModeSimulator(5)
    cluster3.RunScenario(isotope.ByzantineRobustnessScenario())
}
```

## Key Methods Cheat Sheet

```go
// Create cluster
cluster := isotope.NewDevModeSimulator(3)  // 3, 5, or 7 alarms

// Run test across all alarms
result, err := cluster.SimulateTest(isotope, passed)

// Check consensus result
if result.ConsensusFormed {
    fmt.Printf("%d/%d alarms agree\n", result.AgreedCount, result.AlarmCount)
}

// Simulate alarm failure
cluster.InjectByzantineFailure(0)  // Compromise alarm-0

// Check status
rungs := cluster.GetAlarmRungSummary()           // rung for each alarm
distances := cluster.GetAlarmDistanceSummary()   // 42i_distance for each

// Access ledger
ledger := cluster.GetLedger()
history := ledger.QueryByIsotope("family", 1)
isFlaky, failRate := ledger.DetectFlakiness("family", 1)
isRegression, when := ledger.DetectRegression("family", 1)

// Reset for next test
cluster.Reset()
```

## Workflow: From Test to Production

### Phase 1: Dev Mode (This Week)

```
Test consensus logic locally
├─ 3-alarm consensus formation ✓
├─ Byzantine failure detection ✓
├─ Result ledger aggregation ✓
├─ 42i distance calculation ✓
└─ Isotope stability tracking ✓
```

### Phase 2: Docker Compose (Next Week)

```
docker-compose up -d
├─ 3x smoke-alarm containers
├─ Result aggregator
├─ Test runner
└─ Verification scripts
```

### Phase 3: Kubernetes Deployment (Following Week)

```
kubectl apply -f isotope-cluster/
├─ 3-7 smoke-alarm pods
├─ Load balancer
├─ Persistent storage for ledger
└─ Monitoring/alerts
```

## Why Dev Mode Matters

| Scenario | Dev Mode | Full Deployment |
|----------|----------|-----------------|
| Run test | 1ms | 100ms+ |
| Debug failure | Instant | 10 minute redeploy |
| Test consensus | In-process | Cross-network |
| Inject Byzantine | `InjectByzantineFailure(0)` | Kill container, restart |
| Check agreement | `result.ConsensusFormed` | SSH to 3 nodes, check logs |
| Measure stability | `ledger.QueryByIsotope()` | Query aggregation service |

## Test Development Checklist

- [ ] Run test locally with dev mode
- [ ] Verify consensus forms (3+ alarms agree)
- [ ] Test Byzantine detection (inject failure, verify quorum)
- [ ] Check result aggregation (ledger tracks correctly)
- [ ] Measure 42i distance (rung thresholds work)
- [ ] Run pre-built scenarios (3-alarm, refactoring, 5-alarm)
- [ ] Deploy to Docker Compose
- [ ] Verify in Kubernetes (if using)

## See Also

- **DEVMODE.md** - Complete dev mode reference
- **42I_INTEGRATION.md** - How 42i works with isotopes
- **USAGE.md** - Isotope system usage examples
- **README.md** - Architecture overview

## Next Steps

1. **Now**: Use dev mode for algorithm validation
   ```go
   cluster := isotope.NewDevModeSimulator(3)
   cluster.SimulateTest(isotope, true)
   ```

2. **This week**: Add test coverage for your specific isotope families
   ```go
   test := isotope.Isotope{Family: "my-test", Version: 1, ...}
   cluster.SimulateTest(test, true)
   ```

3. **Next week**: Deploy to Docker Compose for integration testing
   ```bash
   docker-compose up -d smoke-alarm
   ```

4. **Following week**: Run in Kubernetes with real services
   ```bash
   kubectl apply -f isotope-cluster.yaml
   ```

The beauty of this approach: you develop with instant feedback (ms), then scale to production (seconds/minutes) without changing your core test logic.
