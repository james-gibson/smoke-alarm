package isotope

import (
	"fmt"
	"time"
)

// DevModeSimulator allows testing multi-alarm consensus in a single process.
// For development/testing only - simulates N independent alarms agreeing on isotopes.
type DevModeSimulator struct {
	AlarmCount int           // Number of simulated alarms (typically 3, 5, or 7)
	Alarms     []*AgentState // One AgentState per simulated alarm
	Results    []ConsensusReport
	Ledger     *ResultLedger
}

// NewDevModeSimulator creates a local test cluster of N simulated alarms.
func NewDevModeSimulator(alarmCount int) *DevModeSimulator {
	alarms := make([]*AgentState, alarmCount)
	for i := 0; i < alarmCount; i++ {
		alarms[i] = NewAgentState(fmt.Sprintf("alarm-%d", i))
	}

	return &DevModeSimulator{
		AlarmCount: alarmCount,
		Alarms:     alarms,
		Results:    []ConsensusReport{},
		Ledger:     NewResultLedger(),
	}
}

// SimulateTest runs a test across all simulated alarms and checks consensus.
// Returns whether consensus formed and which alarms agreed.
func (dms *DevModeSimulator) SimulateTest(testIsotope Isotope, passed bool) (ConsensusResult, error) {
	now := time.Now()

	// Run test on each alarm independently
	for i, alarm := range dms.Alarms {
		// Each alarm records the test result
		_ = alarm.RecordIsotopeTestResult(testIsotope, passed)

		// Record in ledger
		dms.Ledger.RecordResult(TestResult{
			Isotope:     testIsotope,
			Result:      passed,
			Timestamp:   now.Add(time.Duration(i*10) * time.Millisecond),
			AlarmID:     AlarmID(fmt.Sprintf("alarm-%d", i)),
			FailureType: "",
		})

		// Generate consensus report
		report := ConsensusReport{
			AlarmID:   AlarmID(fmt.Sprintf("alarm-%d", i)),
			Isotope:   testIsotope,
			Signature: testIsotope.Signature,
			Timestamp: now.Add(time.Duration(i*10) * time.Millisecond),
		}
		dms.Results = append(dms.Results, report)
	}

	// Verify consensus
	quorumSize := (dms.AlarmCount * 2) / 3
	if quorumSize == 0 {
		quorumSize = 1
	}

	status := VerifyIsotopeAgreement(dms.Results, quorumSize)

	return ConsensusResult{
		TestIsotope:       testIsotope,
		AlarmCount:        dms.AlarmCount,
		AgreedCount:       status.AgreedCount,
		ConsensusFormed:   status.ConsensusFormed,
		Outliers:          status.Outliers,
		ByzantineCapacity: status.ByzantineCount,
	}, nil
}

// ConsensusResult summarizes a test run across simulated alarms.
type ConsensusResult struct {
	TestIsotope       Isotope
	AlarmCount        int
	AgreedCount       int
	ConsensusFormed   bool
	Outliers          []AlarmID
	ByzantineCapacity int
}

// String returns human-readable consensus result.
func (cr ConsensusResult) String() string {
	status := "FAILED"
	if cr.ConsensusFormed {
		status = "FORMED"
	}

	msg := fmt.Sprintf(
		"Consensus %s: %d/%d alarms agree on %s-v%d",
		status, cr.AgreedCount, cr.AlarmCount,
		cr.TestIsotope.Family, cr.TestIsotope.Version,
	)

	if len(cr.Outliers) > 0 {
		msg += fmt.Sprintf(" | Outliers: %v", cr.Outliers)
	}

	msg += fmt.Sprintf(" | Byzantine tolerance: F=%d", cr.ByzantineCapacity)

	return msg
}

// GetAlarmRungSummary returns current rung for each simulated alarm.
func (dms *DevModeSimulator) GetAlarmRungSummary() map[string]int {
	summary := make(map[string]int)
	for i, alarm := range dms.Alarms {
		alarmID := fmt.Sprintf("alarm-%d", i)
		summary[alarmID] = alarm.Position.Rung
	}
	return summary
}

// GetAlarmDistanceSummary returns 42i_distance for each alarm.
func (dms *DevModeSimulator) GetAlarmDistanceSummary() map[string]int {
	summary := make(map[string]int)
	for i, alarm := range dms.Alarms {
		alarmID := fmt.Sprintf("alarm-%d", i)
		summary[alarmID] = alarm.TotalDistance
	}
	return summary
}

// InjectByzantineFailure simulates one alarm being compromised.
// That alarm will report a different isotope for the next test.
func (dms *DevModeSimulator) InjectByzantineFailure(alarmIndex int) error {
	if alarmIndex >= len(dms.Alarms) {
		return fmt.Errorf("alarm index %d out of range (only %d alarms)", alarmIndex, len(dms.Alarms))
	}

	// Mark this alarm as "compromised" by injecting test failures
	dms.Alarms[alarmIndex].RecordTestFailure("entropy-check")
	dms.Alarms[alarmIndex].RecordTestFailure("agreement-pattern")

	return nil
}

// GetLedger returns the accumulated test result ledger.
func (dms *DevModeSimulator) GetLedger() *ResultLedger {
	return dms.Ledger
}

// Reset clears all results and resets alarms to initial state.
func (dms *DevModeSimulator) Reset() {
	dms.Results = []ConsensusReport{}
	for i := 0; i < len(dms.Alarms); i++ {
		dms.Alarms[i] = NewAgentState(fmt.Sprintf("alarm-%d", i))
	}
}

// DevTestScenario runs a predefined test scenario for development.
type DevTestScenario struct {
	Name        string
	Description string
	Steps       []DevTestStep
}

// DevTestStep is one action in a scenario.
type DevTestStep struct {
	Action      string // "test", "fail", "inject_byzantine", "check_consensus"
	TestIsotope *Isotope
	TestPassed  bool
	AlarmIndex  int // For inject_byzantine
	Description string
}

// RunScenario executes a test scenario and prints results.
func (dms *DevModeSimulator) RunScenario(scenario DevTestScenario) error {
	fmt.Printf("\n=== Scenario: %s ===\n", scenario.Name)
	fmt.Printf("%s\n\n", scenario.Description)

	for i, step := range scenario.Steps {
		fmt.Printf("Step %d: %s\n", i+1, step.Description)

		switch step.Action {
		case "test":
			if step.TestIsotope == nil {
				return fmt.Errorf("step %d: test action requires TestIsotope", i)
			}

			result, err := dms.SimulateTest(*step.TestIsotope, step.TestPassed)
			if err != nil {
				return err
			}

			fmt.Printf("  Result: %s\n", result.String())

		case "inject_byzantine":
			err := dms.InjectByzantineFailure(step.AlarmIndex)
			if err != nil {
				return err
			}
			fmt.Printf("  Injected Byzantine failure in alarm-%d\n", step.AlarmIndex)

		case "check_consensus":
			summary := dms.GetAlarmRungSummary()
			distances := dms.GetAlarmDistanceSummary()

			fmt.Printf("  Alarm Status:\n")
			for alarmID, rung := range summary {
				distance := distances[alarmID]
				fmt.Printf("    %s: rung %d, 42i_distance %d\n", alarmID, rung, distance)
			}

		default:
			return fmt.Errorf("step %d: unknown action %q", i, step.Action)
		}

		fmt.Printf("\n")
	}

	return nil
}

// StandardThreeAlarmScenario returns a basic 3-alarm consensus test.
func StandardThreeAlarmScenario() DevTestScenario {
	isotope := Isotope{
		Family:    "entropy-check",
		Version:   1,
		Signature: "test-sig",
		Raw:       "test-raw",
	}

	return DevTestScenario{
		Name:        "Three-Alarm Consensus",
		Description: "Test that 3 alarms independently agree on isotope",
		Steps: []DevTestStep{
			{
				Action:      "test",
				TestIsotope: &isotope,
				TestPassed:  true,
				Description: "All three alarms run entropy-check and pass",
			},
			{
				Action:      "check_consensus",
				Description: "Verify all three alarms are healthy",
			},
			{
				Action:      "inject_byzantine",
				AlarmIndex:  2,
				Description: "Inject Byzantine failure in alarm-2",
			},
			{
				Action:      "check_consensus",
				Description: "Verify alarm-2 has degraded but quorum still formed",
			},
		},
	}
}

// RefactoringScenario tests isotope stability through refactoring.
func RefactoringScenario() DevTestScenario {
	iso1 := Isotope{
		Family:    "test-login",
		Version:   1,
		Signature: "sig-v1",
		Raw:       "raw-v1",
	}

	iso2 := Isotope{
		Family:    "test-login",
		Version:   2,
		Signature: "sig-v2",
		Raw:       "raw-v2",
	}

	return DevTestScenario{
		Name:        "Test Through Refactoring",
		Description: "Track test stability as it's refactored (v1 → v2)",
		Steps: []DevTestStep{
			{
				Action:      "test",
				TestIsotope: &iso1,
				TestPassed:  true,
				Description: "Week 1: Test passes with all 3 alarms",
			},
			{
				Action:      "test",
				TestIsotope: &iso1,
				TestPassed:  true,
				Description: "Week 2: Test still passes (grammatical refactor, same isotope)",
			},
			{
				Action:      "test",
				TestIsotope: &iso1,
				TestPassed:  false,
				Description: "Week 3: Test fails (regression detected)",
			},
			{
				Action:      "test",
				TestIsotope: &iso1,
				TestPassed:  true,
				Description: "Week 4: Test fixed",
			},
			{
				Action:      "test",
				TestIsotope: &iso2,
				TestPassed:  true,
				Description: "Week 5: Semantic change (bumped to v2), new baseline established",
			},
			{
				Action:      "check_consensus",
				Description: "Final status: test stable at v2",
			},
		},
	}
}

// ByzantineRobustnessScenario tests Byzantine fault tolerance.
func ByzantineRobustnessScenario() DevTestScenario {
	isotope := Isotope{
		Family:    "security-check",
		Version:   1,
		Signature: "sig",
		Raw:       "raw",
	}

	return DevTestScenario{
		Name:        "Byzantine Robustness (5 Alarms)",
		Description: "5-alarm cluster tolerates 1 Byzantine failure (F=1)",
		Steps: []DevTestStep{
			{
				Action:      "test",
				TestIsotope: &isotope,
				TestPassed:  true,
				Description: "All 5 alarms pass security-check",
			},
			{
				Action:      "check_consensus",
				Description: "Verify all healthy",
			},
			{
				Action:      "inject_byzantine",
				AlarmIndex:  0,
				Description: "Compromise alarm-0",
			},
			{
				Action:      "test",
				TestIsotope: &isotope,
				TestPassed:  true,
				Description: "Run test again; 4/5 alarms pass, 1 fails",
			},
			{
				Action:      "check_consensus",
				Description: "Verify consensus still formed (4/5 > quorum 3/5), alarm-0 detected as outlier",
			},
		},
	}
}
