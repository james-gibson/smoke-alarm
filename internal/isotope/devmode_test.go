package isotope

import (
	"fmt"
	"testing"
)

func TestDevModeSimulatorInitialization(t *testing.T) {
	dms := NewDevModeSimulator(3)

	if dms.AlarmCount != 3 {
		t.Errorf("Expected 3 alarms, got %d", dms.AlarmCount)
	}

	if len(dms.Alarms) != 3 {
		t.Errorf("Expected 3 alarm instances, got %d", len(dms.Alarms))
	}

	summary := dms.GetAlarmRungSummary()
	if len(summary) != 3 {
		t.Errorf("Expected 3 alarm summaries, got %d", len(summary))
	}

	// All should start at rung 0
	for i := 0; i < 3; i++ {
		if dms.Alarms[i].Position.Rung != 0 {
			t.Errorf("Alarm %d should start at rung 0, got %d", i, dms.Alarms[i].Position.Rung)
		}
	}
}

func TestDevModeSimulateTestPass(t *testing.T) {
	dms := NewDevModeSimulator(3)

	isotope := Isotope{
		Family:    "entropy-check",
		Version:   1,
		Signature: "sig123",
		Raw:       "raw",
	}

	result, err := dms.SimulateTest(isotope, true)

	if err != nil {
		t.Fatalf("SimulateTest failed: %v", err)
	}

	if !result.ConsensusFormed {
		t.Error("Expected consensus to form when all 3 alarms pass")
	}

	if result.AgreedCount != 3 {
		t.Errorf("Expected 3 alarms to agree, got %d", result.AgreedCount)
	}

	if len(result.Outliers) != 0 {
		t.Errorf("Expected no outliers, got %v", result.Outliers)
	}
}

func TestDevModeSimulateTestFail(t *testing.T) {
	dms := NewDevModeSimulator(3)

	isotope := Isotope{
		Family:    "entropy-check",
		Version:   1,
		Signature: "sig123",
		Raw:       "raw",
	}

	result, err := dms.SimulateTest(isotope, false)

	if err != nil {
		t.Fatalf("SimulateTest failed: %v", err)
	}

	// All alarms should report same isotope (the failure)
	if !result.ConsensusFormed {
		t.Error("Expected consensus even on failure")
	}

	if result.AgreedCount != 3 {
		t.Errorf("Expected all 3 alarms to report failure, got %d", result.AgreedCount)
	}

	// Check that alarms recorded the failure in their 42i distance
	for i, alarm := range dms.Alarms {
		if alarm.TotalDistance != DefaultWeights["entropy-check"] {
			t.Errorf("Alarm %d should have distance 16, got %d", i, alarm.TotalDistance)
		}
	}
}

func TestDevModeInjectByzantine(t *testing.T) {
	dms := NewDevModeSimulator(3)

	err := dms.InjectByzantineFailure(0)
	if err != nil {
		t.Fatalf("InjectByzantineFailure failed: %v", err)
	}

	// Alarm 0 should have test failures now
	if dms.Alarms[0].TotalDistance == 0 {
		t.Error("Alarm 0 should have accumulated distance after Byzantine injection")
	}

	// Other alarms should be unaffected
	if dms.Alarms[1].TotalDistance != 0 {
		t.Error("Alarm 1 should not be affected")
	}
	if dms.Alarms[2].TotalDistance != 0 {
		t.Error("Alarm 2 should not be affected")
	}
}

func TestDevModeReset(t *testing.T) {
	dms := NewDevModeSimulator(3)

	// Run a test
	isotope := Isotope{
		Family:    "test",
		Version:   1,
		Signature: "sig",
		Raw:       "raw",
	}
	dms.SimulateTest(isotope, false)

	// Alarms should have distance
	if dms.Alarms[0].TotalDistance == 0 {
		t.Error("Alarm should have distance after test")
	}

	// Reset
	dms.Reset()

	// Should be back to initial state
	for i, alarm := range dms.Alarms {
		if alarm.TotalDistance != 0 {
			t.Errorf("Alarm %d should have distance 0 after reset, got %d", i, alarm.TotalDistance)
		}
	}
}

func TestDevModeLedger(t *testing.T) {
	dms := NewDevModeSimulator(3)

	isotope := Isotope{
		Family:    "test",
		Version:   1,
		Signature: "sig",
		Raw:       "raw",
	}

	dms.SimulateTest(isotope, true)

	// Query ledger
	history := dms.Ledger.QueryByIsotope("test", 1)

	if history == nil {
		t.Error("Expected to find results in ledger")
	}

	if len(history.Results) != 3 {
		t.Errorf("Expected 3 results in ledger, got %d", len(history.Results))
	}
}

func TestDevModeGetAlarmSummaries(t *testing.T) {
	dms := NewDevModeSimulator(5)

	// Inject failures in a couple alarms
	dms.InjectByzantineFailure(1)
	dms.InjectByzantineFailure(3)

	rungSummary := dms.GetAlarmRungSummary()
	distanceSummary := dms.GetAlarmDistanceSummary()

	if len(rungSummary) != 5 {
		t.Errorf("Expected 5 rung values, got %d", len(rungSummary))
	}

	if len(distanceSummary) != 5 {
		t.Errorf("Expected 5 distance values, got %d", len(distanceSummary))
	}

	// Alarms 1 and 3 should have distance
	if distanceSummary["alarm-1"] == 0 {
		t.Error("alarm-1 should have distance")
	}
	if distanceSummary["alarm-3"] == 0 {
		t.Error("alarm-3 should have distance")
	}

	// Others should have 0
	if distanceSummary["alarm-0"] != 0 {
		t.Error("alarm-0 should have distance 0")
	}
}

func TestDevModeMultipleTests(t *testing.T) {
	dms := NewDevModeSimulator(3)

	iso1 := Isotope{
		Family:    "test-a",
		Version:   1,
		Signature: "sig-a",
		Raw:       "raw-a",
	}

	iso2 := Isotope{
		Family:    "test-b",
		Version:   1,
		Signature: "sig-b",
		Raw:       "raw-b",
	}

	// Run two different tests
	result1, _ := dms.SimulateTest(iso1, true)
	result2, _ := dms.SimulateTest(iso2, false)

	if !result1.ConsensusFormed || !result2.ConsensusFormed {
		t.Error("Both tests should form consensus")
	}

	// Ledger should have both isotopes
	historyA := dms.Ledger.QueryByIsotope("test-a", 1)
	historyB := dms.Ledger.QueryByIsotope("test-b", 1)

	if historyA == nil || len(historyA.Results) == 0 {
		t.Error("Expected results for test-a")
	}

	if historyB == nil || len(historyB.Results) == 0 {
		t.Error("Expected results for test-b")
	}
}

func TestStandardThreeAlarmScenario(t *testing.T) {
	dms := NewDevModeSimulator(3)
	scenario := StandardThreeAlarmScenario()

	err := dms.RunScenario(scenario)

	if err != nil {
		t.Fatalf("Scenario failed: %v", err)
	}

	// After scenario, alarm-2 should be compromised
	if dms.Alarms[2].TotalDistance == 0 {
		t.Error("alarm-2 should have distance after Byzantine injection")
	}
}

func TestRefactoringScenario(t *testing.T) {
	dms := NewDevModeSimulator(3)
	scenario := RefactoringScenario()

	err := dms.RunScenario(scenario)

	if err != nil {
		t.Fatalf("Scenario failed: %v", err)
	}

	// Should have results for both v1 and v2
	historyV1 := dms.Ledger.QueryByIsotope("test-login", 1)
	historyV2 := dms.Ledger.QueryByIsotope("test-login", 2)

	if historyV1 == nil {
		t.Error("Expected results for v1")
	}

	if historyV2 == nil {
		t.Error("Expected results for v2")
	}
}

func TestByzantineRobustnessScenario(t *testing.T) {
	// 5-alarm cluster can tolerate 1 Byzantine failure
	dms := NewDevModeSimulator(5)
	scenario := ByzantineRobustnessScenario()

	err := dms.RunScenario(scenario)

	if err != nil {
		t.Fatalf("Scenario failed: %v", err)
	}

	// After scenario, should have results with mixed health
	summary := dms.GetAlarmDistanceSummary()

	// alarm-0 should be compromised (has distance)
	if summary["alarm-0"] == 0 {
		t.Error("alarm-0 should be compromised")
	}

	// Others should be healthy
	for i := 1; i < 5; i++ {
		alarmID := fmt.Sprintf("alarm-%d", i)
		// They might have some distance from the tests, but less than alarm-0
		if summary[alarmID] > summary["alarm-0"] {
			t.Errorf("alarm-%d should have less distance than compromised alarm-0", i)
		}
	}
}
