# Isotope Dev Mode: Local Testing Without Distributed Alarms

**Problem**: In production, you'd run 3-7 independent smoke-alarms as separate processes/VMs. In development, setting up that infrastructure is tedious. You need a quick way to verify consensus logic works without running multiple alarm instances.

**Solution**: `DevModeSimulator` simulates N alarms (typically 3, 5, or 7) in a single process, allowing you to test Byzantine consensus locally.

## Quick Start

```go
package main

import (
	"fmt"
	"ocd-smoke-alarm/internal/isotope"
)

func main() {
	// Create a simulated 3-alarm cluster
	cluster := isotope.NewDevModeSimulator(3)

	// Define a test
	test := isotope.Isotope{
		Family:    "entropy-check",
		Version:   1,
		Signature: "sig123",
		Raw:       "raw",
	}

	// Run test across all alarms
	result, _ := cluster.SimulateTest(test, true)

	fmt.Println(result.String())
	// Output: Consensus FORMED: 3/3 alarms agree on entropy-check-v1 | Byzantine tolerance: F=0
}
```

## Key Methods

### Initialize Cluster

```go
// 3-alarm cluster (no Byzantine tolerance, all must agree)
cluster := isotope.NewDevModeSimulator(3)

// 5-alarm cluster (tolerates F=1 Byzantine failure)
cluster := isotope.NewDevModeSimulator(5)

// 7-alarm cluster (tolerates F=2 Byzantine failures)
cluster := isotope.NewDevModeSimulator(7)
```

### Run Test

```go
result, err := cluster.SimulateTest(isotope, passed)

if result.ConsensusFormed {
    fmt.Printf("%d/%d alarms agree\n", result.AgreedCount, result.AlarmCount)
}

if len(result.Outliers) > 0 {
    fmt.Printf("Outliers: %v\n", result.Outliers)
}

fmt.Printf("Byzantine tolerance: F=%d\n", result.ByzantineCapacity)
```

### Inject Byzantine Failure

Simulate one alarm being compromised:

```go
cluster.InjectByzantineFailure(0) // Compromise alarm-0

// After injection, alarm-0 will report different results
cluster.SimulateTest(isotope, true)
// Alarm-0 will show test failures (42i_distance increases)
```

### Check Status

```go
// Get rung for each alarm
rungs := cluster.GetAlarmRungSummary()
for alarmID, rung := range rungs {
    fmt.Printf("%s at rung %d\n", alarmID, rung)
}

// Get 42i_distance for each alarm
distances := cluster.GetAlarmDistanceSummary()
for alarmID, dist := range distances {
    fmt.Printf("%s 42i_distance=%d\n", alarmID, dist)
}
```

### Query Results

Access the accumulated ledger:

```go
ledger := cluster.GetLedger()

// Query results by isotope
history := ledger.QueryByIsotope("entropy-check", 1)
fmt.Printf("Passed: %d, Failed: %d, Stability: %.1f%%\n",
    history.PassCount, history.FailCount, history.Stability*100)

// Detect flakiness
isFlaky, failRate := ledger.DetectFlakiness("entropy-check", 1)
if isFlaky {
    fmt.Printf("Test is FLAKY (%.1f%% fail rate)\n", failRate*100)
}
```

## Pre-Built Scenarios

Dev mode includes ready-to-run test scenarios:

### Three-Alarm Consensus

```go
cluster := isotope.NewDevModeSimulator(3)
scenario := isotope.StandardThreeAlarmScenario()
cluster.RunScenario(scenario)
```

Tests:
1. All 3 alarms pass test
2. Inject Byzantine failure in alarm-2
3. Verify quorum still formed

### Test Through Refactoring

```go
cluster := isotope.NewDevModeSimulator(3)
scenario := isotope.RefactoringScenario()
cluster.RunScenario(scenario)
```

Simulates:
1. Test passes (v1)
2. Test passes (grammatical refactor, still v1)
3. Test fails (regression)
4. Test fixed (v1 stable again)
5. Semantic change (bumped to v2)

### Byzantine Robustness (5 Alarms)

```go
cluster := isotope.NewDevModeSimulator(5)
scenario := isotope.ByzantineRobustnessScenario()
cluster.RunScenario(scenario)
```

Tests:
1. All 5 alarms healthy
2. Compromise alarm-0
3. Verify 4/5 quorum still forms
4. Verify alarm-0 detected as outlier

## Usage Pattern for Development

### 1. Unit Test a Single Alarm

```go
func TestSingleAlarmBehavior(t *testing.T) {
	agent := isotope.NewAgentState("test-service")

	// Test 42i distance tracking
	agent.RecordTestFailure("entropy-check")
	if agent.TotalDistance != 16 {
		t.Errorf("Expected distance 16, got %d", agent.TotalDistance)
	}
}
```

### 2. Test Consensus Logic

```go
func TestConsensusFormation(t *testing.T) {
	cluster := isotope.NewDevModeSimulator(3)

	test := isotope.Isotope{...}
	result, _ := cluster.SimulateTest(test, true)

	if !result.ConsensusFormed {
		t.Error("Expected consensus")
	}

	if result.AgreedCount != 3 {
		t.Errorf("Expected 3 alarms, got %d", result.AgreedCount)
	}
}
```

### 3. Test Result Tracking

```go
func TestResultLedger(t *testing.T) {
	cluster := isotope.NewDevModeSimulator(3)

	// Run tests
	cluster.SimulateTest(isotope1, true)
	cluster.SimulateTest(isotope1, false)  // Regression
	cluster.SimulateTest(isotope1, true)   // Fixed

	// Query ledger
	ledger := cluster.GetLedger()
	history := ledger.QueryByIsotope("test", 1)

	if history.PassCount != 2 || history.FailCount != 1 {
		t.Error("Results not aggregated correctly")
	}
}
```

### 4. Test Byzantine Tolerance

```go
func TestByzantineDetection(t *testing.T) {
	cluster := isotope.NewDevModeSimulator(5)

	// Inject failure
	cluster.InjectByzantineFailure(0)

	// Run test; should still form quorum
	result, _ := cluster.SimulateTest(isotope, true)

	if !result.ConsensusFormed {
		t.Error("Should tolerate 1/5 Byzantine failure")
	}

	if len(result.Outliers) != 1 || result.Outliers[0] != "alarm-0" {
		t.Error("Should identify alarm-0 as outlier")
	}
}
```

## Scaling from Dev to Prod

| Aspect | Dev Mode | Production |
|--------|----------|-----------|
| **Setup** | `NewDevModeSimulator(3)` | 3 Docker containers running smoke-alarm |
| **Test Execution** | `cluster.SimulateTest(iso, result)` | HTTP requests to /probe endpoints |
| **Byzantine Simulation** | `InjectByzantineFailure(0)` | Kill/restart actual container |
| **Result Aggregation** | Automatic via ledger | HTTP queries to aggregation service |
| **Cost** | Immediate feedback | Full distributed overhead |

Dev mode allows you to:
- ✓ Verify consensus logic before deployment
- ✓ Test Byzantine tolerance without infrastructure
- ✓ Validate isotope tracking across refactoring
- ✓ Debug 42i_distance calculation
- ✓ Run fast test cycles (ms, not minutes)

## Limitations

Dev mode does NOT simulate:
- Network latency/partition
- Clock skew across alarms
- Concurrency/goroutine races
- Message loss/reordering
- Disk/CPU contention

For those, you need actual distributed testing with multiple processes/VMs. But dev mode gets you 90% of the way there for algorithm validation.

## Example: Full Development Workflow

```go
func TestCompleteWorkflow(t *testing.T) {
	// 1. Start with 3-alarm cluster
	cluster := isotope.NewDevModeSimulator(3)

	// 2. Define tests
	tests := []isotope.Isotope{
		{Family: "entropy-check", Version: 1, Signature: "s1", Raw: "r1"},
		{Family: "input-correlation", Version: 1, Signature: "s2", Raw: "r2"},
	}

	// 3. Run tests in sequence
	for _, test := range tests {
		result, err := cluster.SimulateTest(test, true)
		if err != nil || !result.ConsensusFormed {
			t.Fatalf("Test %s failed", test.Family)
		}
	}

	// 4. Inject Byzantine failure
	cluster.InjectByzantineFailure(1)

	// 5. Re-run tests; verify quorum still forms
	for _, test := range tests {
		result, _ := cluster.SimulateTest(test, true)
		if !result.ConsensusFormed {
			t.Fatal("Quorum lost after Byzantine injection")
		}
	}

	// 6. Check ledger
	ledger := cluster.GetLedger()
	for _, test := range tests {
		h := ledger.QueryByIsotope(test.Family, test.Version)
		if h == nil || h.PassCount == 0 {
			t.Errorf("Test %s not in ledger", test.Family)
		}
	}

	// 7. Verify alarm health
	summary := cluster.GetAlarmDistanceSummary()
	if summary["alarm-1"] == 0 {
		t.Error("alarm-1 should be compromised")
	}
}
```

Run this test with `go test -v` and get instant feedback on:
- Consensus formation
- Byzantine detection
- Result aggregation
- 42i distance tracking

All without running distributed infrastructure!
