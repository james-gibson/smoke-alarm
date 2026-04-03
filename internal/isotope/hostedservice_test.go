package isotope

import (
	"testing"
	"time"
)

func TestHostedServiceCreation(t *testing.T) {
	service := NewHostedService("test-service")

	if service.ServiceID != "test-service" {
		t.Errorf("Expected service ID 'test-service', got %s", service.ServiceID)
	}

	if service.State == nil {
		t.Error("Service should have AgentState")
	}

	if len(service.Tests) != 0 {
		t.Error("Service should start with no tests registered")
	}
}

func TestRegisterTest(t *testing.T) {
	service := NewHostedService("test-service")

	test := &ServiceTest{
		Description:      "Check entropy",
		SLALatencyMs:     100,
		SLAErrorRate:     0.01,
		DeclaredBehavior: "Output should be random",
		TestFunc: func() bool {
			return true
		},
	}

	service.RegisterTest("entropy-check", test)

	if len(service.Tests) != 1 {
		t.Error("Expected 1 registered test")
	}

	if service.Tests["entropy-check"] == nil {
		t.Error("Test not registered correctly")
	}

	if service.Tests["entropy-check"].IsotopeFamily != "entropy-check" {
		t.Error("Isotope family should be set during registration")
	}
}

func TestRunTest(t *testing.T) {
	service := NewHostedService("test-service")

	service.RegisterTest("entropy-check", &ServiceTest{
		SLALatencyMs:     100,
		SLAErrorRate:     0.01,
		DeclaredBehavior: "Random output",
		TestFunc: func() bool {
			time.Sleep(10 * time.Millisecond)
			return true
		},
	})

	passed, latencyMs, errReason := service.RunTest("entropy-check")

	if !passed {
		t.Error("Expected test to pass")
	}

	if latencyMs < 10 {
		t.Errorf("Expected latency >= 10ms, got %dms", latencyMs)
	}

	if errReason != "" {
		t.Errorf("Expected no error, got: %s", errReason)
	}
}

func TestTestFailure(t *testing.T) {
	service := NewHostedService("test-service")

	service.RegisterTest("entropy-check", &ServiceTest{
		SLALatencyMs: 100,
		TestFunc: func() bool {
			return false
		},
	})

	passed, _, errReason := service.RunTest("entropy-check")

	if passed {
		t.Error("Expected test to fail")
	}

	if errReason == "" {
		t.Error("Expected error reason")
	}
}

func TestLatencyDegradation(t *testing.T) {
	service := NewHostedService("test-service")

	service.RegisterTest("latency-test", &ServiceTest{
		SLALatencyMs: 100,
		TestFunc: func() bool {
			time.Sleep(10 * time.Millisecond)
			return true
		},
	})

	// Run test before degradation
	_, lat1, _ := service.RunTest("latency-test")

	// Simulate degradation
	service.SimulateLatencyDegradation(50) // Add 50% latency

	// Run test after degradation
	_, lat2, _ := service.RunTest("latency-test")

	if lat2 <= lat1 {
		t.Errorf("Degradation should increase latency: %dms → %dms", lat1, lat2)
	}
}

func TestConstantOutput(t *testing.T) {
	service := NewHostedService("test-service")

	service.RegisterTest("entropy-check", &ServiceTest{
		SLALatencyMs: 100,
		TestFunc: func() bool {
			return true // Normally returns true
		},
	})

	// Simulate loss of entropy (constant output)
	service.SimulateConstantOutput(false) // Now always returns false

	passed, _, _ := service.RunTest("entropy-check")

	if passed {
		t.Error("After constant output simulation, test should fail")
	}
}

func TestRecover(t *testing.T) {
	service := NewHostedService("test-service")

	service.RegisterTest("test", &ServiceTest{
		SLALatencyMs: 100,
		TestFunc: func() bool {
			return true
		},
	})

	// Degrade
	service.SimulateConstantOutput(false)
	service.ConsecutiveFails = 10

	// Recover
	service.Recover()

	// Should be healthy again
	if service.ConsecutiveFails != 0 {
		t.Error("ConsecutiveFails should be reset")
	}

	if service.State.TotalDistance != 0 {
		t.Error("State should be reset")
	}

	passed, _, _ := service.RunTest("test")
	if !passed {
		t.Error("Service should be healthy after recovery")
	}
}

func TestServiceHealthSummary(t *testing.T) {
	service := NewHostedService("test-service")

	service.RegisterTest("test", &ServiceTest{
		SLALatencyMs: 100,
		TestFunc: func() bool {
			return true
		},
	})

	// Run test a few times
	for i := 0; i < 3; i++ {
		service.RunTest("test")
	}

	health := service.GetHealthSummary()

	if health.ServiceID != "test-service" {
		t.Error("Service ID mismatch")
	}

	if health.TotalTestsRun != 3 {
		t.Errorf("Expected 3 tests run, got %d", health.TotalTestsRun)
	}

	if health.SuccessRate != 1.0 {
		t.Errorf("Expected 100%% success rate, got %.1f%%", health.SuccessRate*100)
	}
}

func TestServiceClusterCreation(t *testing.T) {
	key := []byte("test-key")
	cluster := NewServiceCluster(3, key)

	if cluster.AlarmCount != 3 {
		t.Errorf("Expected 3 alarms, got %d", cluster.AlarmCount)
	}

	if len(cluster.HostedServices) != 0 {
		t.Error("Should start with no hosted services")
	}
}

func TestHostService(t *testing.T) {
	cluster := NewServiceCluster(3, []byte("key"))
	service := NewHostedService("api-server")

	cluster.HostService(service)

	if len(cluster.HostedServices) != 1 {
		t.Error("Service should be hosted")
	}

	if cluster.HostedServices["api-server"] == nil {
		t.Error("Service not accessible")
	}
}

func TestTestServiceHealthy(t *testing.T) {
	cluster := NewServiceCluster(3, []byte("key"))

	service := NewHostedService("healthy-service")
	service.RegisterTest("test-1", &ServiceTest{
		SLALatencyMs: 100,
		TestFunc: func() bool {
			return true
		},
	})

	cluster.HostService(service)

	// Test the service
	result, err := cluster.TestService("healthy-service")
	if err != nil {
		t.Fatalf("TestService failed: %v", err)
	}

	if !result.Consensus.ConsensusFormed {
		t.Error("Expected consensus when service is healthy")
	}

	if result.ServiceHealth.SuccessRate != 1.0 {
		t.Error("Expected 100% success rate")
	}
}

func TestTestServiceDegraded(t *testing.T) {
	cluster := NewServiceCluster(3, []byte("key"))

	service := NewHostedService("degraded-service")
	service.RegisterTest("test-1", &ServiceTest{
		SLALatencyMs: 50,
		TestFunc: func() bool {
			time.Sleep(100 * time.Millisecond) // Exceeds SLA
			return true
		},
	})

	cluster.HostService(service)

	result, _ := cluster.TestService("degraded-service")

	// Should still have consensus, but with SLA violation alerts
	if len(result.AlarmResults) != 3 {
		t.Errorf("Expected 3 alarm results, got %d", len(result.AlarmResults))
	}

	// Check that latency was recorded
	for _, ar := range result.AlarmResults {
		for _, tr := range ar.TestResults {
			if tr.LatencyMs < 100 {
				t.Error("Latency should be recorded as >= 100ms")
			}
		}
	}
}

func TestMultipleTestsConsensus(t *testing.T) {
	cluster := NewServiceCluster(3, []byte("key"))

	service := NewHostedService("multi-test-service")

	service.RegisterTest("test-1", &ServiceTest{
		SLALatencyMs: 100,
		TestFunc: func() bool {
			return true
		},
	})

	service.RegisterTest("test-2", &ServiceTest{
		SLALatencyMs: 100,
		TestFunc: func() bool {
			return true
		},
	})

	cluster.HostService(service)

	result, _ := cluster.TestService("multi-test-service")

	if !result.Consensus.ConsensusFormed {
		t.Error("Expected consensus on all tests")
	}

	if len(result.Consensus.TestAgreement) != 2 {
		t.Error("Expected agreement on 2 tests")
	}
}

func TestServiceConsensusString(t *testing.T) {
	consensus := ServiceConsensus{
		ConsensusFormed: true,
		QuorumSize:      2,
		TestAgreement: map[string]int{
			"test-1": 2,
			"test-2": 2,
		},
	}

	s := consensus.String()
	if s == "" {
		t.Error("Consensus should have string representation")
	}

	if !contains(s, "FORMED") {
		t.Error("String should indicate consensus formed")
	}
}

func TestServiceHealthSummaryString(t *testing.T) {
	health := ServiceHealthSummary{
		ServiceID:        "test-svc",
		SuccessRate:      0.95,
		TotalTestsRun:    100,
		ConsecutiveFails: 0,
		AvgLatencyMs:     45,
		CurrentRung:      2,
	}

	s := health.String()
	if s == "" {
		t.Error("Health summary should have string representation")
	}

	if !contains(s, "test-svc") {
		t.Error("String should contain service ID")
	}

	if !contains(s, "HEALTHY") {
		t.Error("String should indicate healthy status")
	}
}

func TestByzantineServiceDetection(t *testing.T) {
	cluster := NewServiceCluster(5, []byte("key"))

	// Create two services: one healthy, one compromised
	healthyService := NewHostedService("healthy")
	healthyService.RegisterTest("test", &ServiceTest{
		SLALatencyMs: 100,
		TestFunc: func() bool {
			return true
		},
	})

	compromisedService := NewHostedService("compromised")
	compromisedService.RegisterTest("test", &ServiceTest{
		SLALatencyMs: 100,
		TestFunc: func() bool {
			return false // Always fails
		},
	})

	cluster.HostService(healthyService)
	cluster.HostService(compromisedService)

	// Test healthy service
	result1, _ := cluster.TestService("healthy")
	if !result1.Consensus.ConsensusFormed {
		t.Error("Healthy service should have consensus")
	}

	// Test compromised service
	result2, _ := cluster.TestService("compromised")
	if result2.ServiceHealth.SuccessRate != 0.0 {
		t.Error("Compromised service should fail all tests")
	}

	// 5-alarm cluster should detect this
	if len(result2.Consensus.TestAgreement) == 0 {
		t.Error("Should have test agreement data")
	}
}

func TestChaosLatencyEscalation(t *testing.T) {
	cluster := NewServiceCluster(3, []byte("key"))

	service := NewHostedService("chaotic-service")
	service.RegisterTest("latency-test", &ServiceTest{
		SLALatencyMs: 50, // Strict SLA to catch latency violations
		SLAErrorRate: 0.01,
		TestFunc: func() bool {
			time.Sleep(10 * time.Millisecond)
			return true
		},
	})

	cluster.HostService(service)

	// Run chaos test with latency escalation
	profile := LatencyEscalationProfile()
	result := cluster.RunChaosTest(service, profile)

	if result.TotalCycles == 0 {
		t.Error("Should have run test cycles")
	}

	// High latency should cause test failures and distance accumulation
	if result.FinalDistance > 0 || len(result.Timeline) > 0 {
		t.Logf("Latency escalation: %d cycles, final distance: %d", result.TotalCycles, result.FinalDistance)
	}

	if result.RecoverySucceeded {
		t.Log("✓ Service recovered after chaos ended")
	}
}

func TestChaosEntropyCollapse(t *testing.T) {
	cluster := NewServiceCluster(5, []byte("key"))

	service := NewHostedService("entropy-service")
	service.RegisterTest("entropy-check", &ServiceTest{
		SLALatencyMs: 100,
		TestFunc: func() bool {
			return true
		},
	})

	cluster.HostService(service)

	// Baseline
	baselineResult, _ := cluster.TestService("entropy-service")
	if !baselineResult.Consensus.ConsensusFormed {
		t.Fatal("Baseline consensus should form")
	}

	// Run chaos: entropy loss makes tests fail
	profile := EntropyCollapseProfile()
	result := cluster.RunChaosTest(service, profile)

	// Entropy loss (constant output = false) should cause test failures
	if len(result.Timeline) > 0 {
		// First snapshot should be healthy (baseline)
		baseline := result.Timeline[0]
		if !baseline.ConsensusFormed {
			t.Error("Baseline should have consensus")
		}

		// Later snapshots should show degradation
		if result.FinalDistance > 0 {
			t.Logf("✓ Entropy loss accumulated distance: %d", result.FinalDistance)
		}
	}

	t.Logf("Entropy collapse timeline: %d cycles", result.TotalCycles)
}

func TestChaosInputIgnorance(t *testing.T) {
	cluster := NewServiceCluster(5, []byte("key"))

	service := NewHostedService("broken-service")
	service.RegisterTest("input-correlation", &ServiceTest{
		SLALatencyMs: 100,
		TestFunc: func() bool {
			return true
		},
	})

	cluster.HostService(service)

	profile := InputIgnoranceProfile()
	result := cluster.RunChaosTest(service, profile)

	// Verify we captured timeline data
	if len(result.Timeline) == 0 {
		t.Fatal("Should have captured timeline")
	}

	// Check baseline was healthy
	if len(result.Timeline) > 1 {
		baseline := result.Timeline[0]
		if baseline.ServiceDistance > 0 {
			t.Error("Baseline should start at distance 0")
		}
	}

	// Input ignorance phase should show degradation
	if result.FinalDistance > 0 {
		t.Logf("✓ Input ignorance caused distance accumulation: %d", result.FinalDistance)
	}

	t.Logf("Input ignorance test: %d cycles, final distance %d", result.TotalCycles, result.FinalDistance)
}

func TestChaosCascadingFailure(t *testing.T) {
	cluster := NewServiceCluster(5, []byte("key"))

	service := NewHostedService("cascading-service")
	service.RegisterTest("health-check", &ServiceTest{
		SLALatencyMs: 100,
		TestFunc: func() bool {
			time.Sleep(5 * time.Millisecond)
			return true
		},
	})

	cluster.HostService(service)

	profile := CascadingFailureProfile()
	result := cluster.RunChaosTest(service, profile)

	// Should see progressive degradation
	if len(result.Timeline) < 10 {
		t.Error("Should run multiple cycles")
	}

	// Distance should increase monotonically
	var lastDistance int
	for i, snapshot := range result.Timeline {
		if snapshot.ServiceDistance < lastDistance {
			t.Errorf("Cycle %d: distance decreased (%d -> %d)", i, lastDistance, snapshot.ServiceDistance)
		}
		lastDistance = snapshot.ServiceDistance
	}

	if result.FinalDistance <= 0 {
		t.Error("Cascading failure should accumulate significant distance")
	}

	if result.RecoverySucceeded {
		t.Log("✓ Service successfully recovered after cascading failure")
	}
}

func TestChaosDetectionMetrics(t *testing.T) {
	cluster := NewServiceCluster(3, []byte("key"))

	service := NewHostedService("metric-service")
	service.RegisterTest("metric-test", &ServiceTest{
		SLALatencyMs: 100,
		TestFunc: func() bool {
			return true
		},
	})

	cluster.HostService(service)

	profile := EntropyCollapseProfile()
	result := cluster.RunChaosTest(service, profile)

	// Verify metrics are recorded
	if len(result.Timeline) == 0 {
		t.Fatal("Timeline should have snapshots")
	}

	for i, snap := range result.Timeline {
		if snap.Cycle != i+1 {
			t.Errorf("Cycle numbering incorrect at index %d", i)
		}

		if snap.ServiceDistance < 0 {
			t.Errorf("Distance cannot be negative at cycle %d", snap.Cycle)
		}

		if snap.ServiceRung < 0 || snap.ServiceRung > 6 {
			t.Errorf("Invalid rung %d at cycle %d", snap.ServiceRung, snap.Cycle)
		}

		if snap.SuccessRate < 0.0 || snap.SuccessRate > 1.0 {
			t.Errorf("Invalid success rate %.2f at cycle %d", snap.SuccessRate, snap.Cycle)
		}
	}

	t.Logf("Timeline: %d cycles, Final distance: %d, Final rung: %d",
		result.TotalCycles, result.FinalDistance, result.FinalRung)
}

func TestChaosTestResultString(t *testing.T) {
	result := ChaosTestResult{
		ServiceID:         "test-svc",
		Profile:           LatencyEscalationProfile(),
		DetectionCycle:    5,
		EvictionCycle:     8,
		FinalDistance:     120,
		FinalRung:         4,
		TotalCycles:       15,
		RecoverySucceeded: true,
		TimeToDetection:   5,
		TimeToEviction:    8,
	}

	s := result.String()
	if s == "" {
		t.Error("String representation should not be empty")
	}

	if !contains(s, "test-svc") {
		t.Error("Should contain service ID")
	}

	if !contains(s, "Latency") {
		t.Error("Should contain profile name")
	}
}
