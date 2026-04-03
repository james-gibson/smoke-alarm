package isotope

import (
	"fmt"
	"time"
)

// HostedService represents a service under test that runs inside the simulator.
// The cluster's alarms will test this service and report consensus on its health.
type HostedService struct {
	ServiceID       string
	State           *AgentState // The service's own 42i state
	Tests           map[string]*ServiceTest
	TestHistory     []ServiceTestRun
	StartTime       time.Time
	LastTestTime    time.Time
	ConsecutiveFails int
}

// ServiceTest defines one test that alarms will run against the service.
type ServiceTest struct {
	IsotopeFamily   string
	Description     string
	SLALatencyMs    int // SLA: max latency
	SLAErrorRate    float64 // SLA: max error rate (0.0-1.0)
	DeclaredBehavior string // What the service claims to do
	TestFunc        func() bool // The actual test (true=pass, false=fail)
}

// ServiceTestRun records the result of running a test.
type ServiceTestRun struct {
	Test        *ServiceTest
	Passed      bool
	LatencyMs   int
	Timestamp   time.Time
	AlarmID     AlarmID
	ErrorReason string
}

// NewHostedService creates a service under test.
func NewHostedService(serviceID string) *HostedService {
	return &HostedService{
		ServiceID:   serviceID,
		State:       NewAgentState(serviceID),
		Tests:       make(map[string]*ServiceTest),
		TestHistory: []ServiceTestRun{},
		StartTime:   time.Now(),
	}
}

// RegisterTest registers an isotope test that alarms will run against this service.
func (hs *HostedService) RegisterTest(isotopeFamily string, test *ServiceTest) {
	test.IsotopeFamily = isotopeFamily
	hs.Tests[isotopeFamily] = test
}

// RunTest executes a registered test and records the result.
func (hs *HostedService) RunTest(isotopeFamily string) (bool, int, string) {
	test, ok := hs.Tests[isotopeFamily]
	if !ok {
		return false, 0, fmt.Sprintf("test not registered: %s", isotopeFamily)
	}

	start := time.Now()
	passed := test.TestFunc()
	latencyMs := int(time.Since(start).Milliseconds())

	var errorReason string
	if !passed {
		if latencyMs > test.SLALatencyMs {
			errorReason = fmt.Sprintf("SLA violation: latency %dms > %dms", latencyMs, test.SLALatencyMs)
		} else {
			errorReason = "test assertion failed"
		}
	}

	// Record in service history
	hs.TestHistory = append(hs.TestHistory, ServiceTestRun{
		Test:        test,
		Passed:      passed,
		LatencyMs:   latencyMs,
		Timestamp:   time.Now(),
		ErrorReason: errorReason,
	})

	return passed, latencyMs, errorReason
}

// GetIsotope returns an isotope for this service's current state.
// Used when alarms report on the service's health.
func (hs *HostedService) GetIsotope(isotopeFamily string, version int, signingKey []byte) (Isotope, error) {
	// Service isotope signature represents "service running this test"
	canonical := fmt.Sprintf("SERVICE:%s:TEST:%s:V%d", hs.ServiceID, isotopeFamily, version)

	return Isotope{
		Family:    fmt.Sprintf("svc-%s-%s", hs.ServiceID, isotopeFamily),
		Version:   version,
		Signature: fmt.Sprintf("%x", signingKey), // Placeholder signature
		Raw:       canonical,
	}, nil
}

// SimulateLatencyDegradation adds artificial latency to tests.
// Simulates gradual performance degradation (SLA violations).
func (hs *HostedService) SimulateLatencyDegradation(percent int) {
	// Modify all test functions to add latency
	for _, test := range hs.Tests {
		originalFunc := test.TestFunc
		test.TestFunc = func() bool {
			// Add latency equal to percent of SLA
			extraLatency := (test.SLALatencyMs * percent) / 100
			time.Sleep(time.Duration(extraLatency) * time.Millisecond)
			return originalFunc()
		}
	}
}

// SimulateConstantOutput makes the service return the same output repeatedly.
// Simulates loss of entropy (compromised randomness).
func (hs *HostedService) SimulateConstantOutput(value bool) {
	for _, test := range hs.Tests {
		test.TestFunc = func() bool {
			return value
		}
	}
}

// SimulateInputIgnorance makes the service ignore its inputs.
// Tests for input-correlation will fail.
func (hs *HostedService) SimulateInputIgnorance() {
	for _, test := range hs.Tests {
		test.TestFunc = func() bool {
			// Always return fixed result, ignoring what we were asked
			return true
		}
	}
	// Record that service is now broken
	hs.State.RecordTestFailure("input-correlation")
}

// Recover resets the service to healthy state.
func (hs *HostedService) Recover() {
	// Reset test functions to pass
	for _, test := range hs.Tests {
		test.TestFunc = func() bool {
			return true
		}
	}

	// Reset state
	hs.State = NewAgentState(hs.ServiceID)
	hs.ConsecutiveFails = 0
}

// GetHealthSummary returns current health status.
func (hs *HostedService) GetHealthSummary() ServiceHealthSummary {
	totalTests := len(hs.TestHistory)
	passCount := 0

	for _, run := range hs.TestHistory {
		if run.Passed {
			passCount++
		}
	}

	successRate := 0.0
	if totalTests > 0 {
		successRate = float64(passCount) / float64(totalTests)
	}

	avgLatencyMs := 0
	if totalTests > 0 {
		totalLatency := 0
		for _, run := range hs.TestHistory {
			totalLatency += run.LatencyMs
		}
		avgLatencyMs = totalLatency / totalTests
	}

	return ServiceHealthSummary{
		ServiceID:        hs.ServiceID,
		SuccessRate:      successRate,
		TotalTestsRun:    totalTests,
		ConsecutiveFails: hs.ConsecutiveFails,
		AvgLatencyMs:     avgLatencyMs,
		Uptime:           time.Since(hs.StartTime),
		CurrentRung:      hs.State.Position.Rung,
		Distance42i:      hs.State.TotalDistance,
	}
}

// ServiceHealthSummary summarizes a service's health.
type ServiceHealthSummary struct {
	ServiceID        string
	SuccessRate      float64
	TotalTestsRun    int
	ConsecutiveFails int
	AvgLatencyMs     int
	Uptime           time.Duration
	CurrentRung      int
	Distance42i      int
}

// String returns human-readable health summary.
func (shs ServiceHealthSummary) String() string {
	status := "HEALTHY"
	if shs.SuccessRate < 0.95 {
		status = "DEGRADED"
	}
	if shs.SuccessRate < 0.80 {
		status = "CRITICAL"
	}

	return fmt.Sprintf(
		"%s [%s]: %.1f%% success rate | %d tests | %dms avg latency | rung %d | 42i_distance %d",
		shs.ServiceID, status, shs.SuccessRate*100, shs.TotalTestsRun, shs.AvgLatencyMs, shs.CurrentRung, shs.Distance42i,
	)
}

// ServiceCluster extends DevModeSimulator to host and test a service.
type ServiceCluster struct {
	*DevModeSimulator
	HostedServices map[string]*HostedService
	SigningKey     []byte
}

// NewServiceCluster creates a simulator that can host services.
func NewServiceCluster(alarmCount int, signingKey []byte) *ServiceCluster {
	return &ServiceCluster{
		DevModeSimulator: NewDevModeSimulator(alarmCount),
		HostedServices:  make(map[string]*HostedService),
		SigningKey:      signingKey,
	}
}

// HostService registers a service to be tested by the cluster.
func (sc *ServiceCluster) HostService(service *HostedService) {
	sc.HostedServices[service.ServiceID] = service
}

// TestService runs all registered tests against a service and collects consensus.
// Each alarm in the cluster runs the tests independently.
func (sc *ServiceCluster) TestService(serviceID string) (ServiceTestingResult, error) {
	service, ok := sc.HostedServices[serviceID]
	if !ok {
		return ServiceTestingResult{}, fmt.Errorf("service not hosted: %s", serviceID)
	}

	now := time.Now()
	var alarmResults []AlarmTestResult

	// Each alarm independently tests the service
	for alarmIdx := range sc.Alarms {
		alarmID := fmt.Sprintf("alarm-%d", alarmIdx)
		alarmResult := AlarmTestResult{
			AlarmID:    AlarmID(alarmID),
			ServiceID:  serviceID,
			Timestamp:  now,
			TestResults: []IndividualTestResult{},
		}

		// Run each test
		for testFamily, test := range service.Tests {
			passed, latencyMs, errReason := service.RunTest(testFamily)

			testResult := IndividualTestResult{
				TestFamily: testFamily,
				Passed:     passed,
				LatencyMs:  latencyMs,
				SLALatencyMs: test.SLALatencyMs,
				ErrorReason: errReason,
			}

			alarmResult.TestResults = append(alarmResult.TestResults, testResult)

			// Update service's 42i state based on test result
			if !passed {
				service.State.RecordTestFailure(testFamily)
				service.ConsecutiveFails++
			} else {
				service.State.RecordTestPass(testFamily)
				service.ConsecutiveFails = 0
			}

			// Alarm also records the service's health as consensus
			iso, _ := service.GetIsotope(testFamily, 1, sc.SigningKey)
			report := ConsensusReport{
				AlarmID:   AlarmID(alarmID),
				Isotope:   iso,
				Signature: iso.Signature,
				Timestamp: now,
			}

			// Verify consensus among alarms
			alarmResult.ConsensusReports = append(alarmResult.ConsensusReports, report)
		}

		alarmResults = append(alarmResults, alarmResult)
	}

	// Check if consensus formed on service health
	consensus := sc.analyzeServiceConsensus(alarmResults)

	service.LastTestTime = now

	return ServiceTestingResult{
		ServiceID:      serviceID,
		Timestamp:      now,
		AlarmResults:   alarmResults,
		Consensus:      consensus,
		ServiceHealth:  service.GetHealthSummary(),
	}, nil
}

// analyzeServiceConsensus checks if alarms agree on service health.
func (sc *ServiceCluster) analyzeServiceConsensus(alarmResults []AlarmTestResult) ServiceConsensus {
	// For each test family, check if alarms agree on pass/fail
	testAgreement := make(map[string]int) // family → agreed count

	if len(alarmResults) == 0 {
		return ServiceConsensus{ConsensusFormed: false}
	}

	// Get test families from first alarm
	var families []string
	if len(alarmResults[0].TestResults) > 0 {
		for _, tr := range alarmResults[0].TestResults {
			families = append(families, tr.TestFamily)
		}
	}

	// Count agreement on each test
	for _, family := range families {
		passCount := 0
		for _, ar := range alarmResults {
			for _, tr := range ar.TestResults {
				if tr.TestFamily == family && tr.Passed {
					passCount++
				}
			}
		}
		testAgreement[family] = passCount
	}

	// Consensus formed if quorum agrees on each test
	quorumSize := (len(alarmResults) * 2) / 3
	if quorumSize == 0 {
		quorumSize = 1
	}

	allTestsConsensus := true
	for _, agreed := range testAgreement {
		if agreed < quorumSize {
			allTestsConsensus = false
			break
		}
	}

	return ServiceConsensus{
		ConsensusFormed: allTestsConsensus,
		QuorumSize:      quorumSize,
		TestAgreement:   testAgreement,
	}
}

// ServiceTestingResult summarizes testing a service.
type ServiceTestingResult struct {
	ServiceID    string
	Timestamp    time.Time
	AlarmResults []AlarmTestResult
	Consensus    ServiceConsensus
	ServiceHealth ServiceHealthSummary
}

// AlarmTestResult records what one alarm observed.
type AlarmTestResult struct {
	AlarmID          AlarmID
	ServiceID        string
	Timestamp        time.Time
	TestResults      []IndividualTestResult
	ConsensusReports []ConsensusReport
}

// IndividualTestResult is a single test run by an alarm.
type IndividualTestResult struct {
	TestFamily      string
	Passed          bool
	LatencyMs       int
	SLALatencyMs    int
	ErrorReason     string
}

// ServiceConsensus reports if alarms agree on service health.
type ServiceConsensus struct {
	ConsensusFormed bool
	QuorumSize      int
	TestAgreement   map[string]int // family → count of alarms that passed
}

// String returns human-readable consensus.
func (sc ServiceConsensus) String() string {
	if !sc.ConsensusFormed {
		return fmt.Sprintf("Consensus NOT FORMED (quorum %d)", sc.QuorumSize)
	}

	return fmt.Sprintf("Consensus FORMED (quorum %d): all tests passed by %d/%d alarms", sc.QuorumSize, sc.QuorumSize, len(sc.TestAgreement))
}

// ChaosPhase defines one degradation stage in a chaos test.
type ChaosPhase struct {
	DurationCycles  int    // number of test cycles to run this phase
	DegradationType string // "latency", "entropy", "input-ignorance", "none"
	Severity        int    // 0-100 for latency %, 0-1 for entropy (bool)
	Description     string
}

// ChaosProfile defines a chaos testing scenario.
type ChaosProfile struct {
	Name        string
	Description string
	Phases      []ChaosPhase
}

// ChaosSnapshot captures service and consensus state at one test cycle.
type ChaosSnapshot struct {
	Cycle              int
	ServiceDistance    int
	ServiceRung        int
	ConsensusFormed    bool
	HealthyAlarmCount  int
	FailedAlarmCount   int
	AverageLatencyMs   int
	SuccessRate        float64
}

// ChaosTestResult summarizes a chaos test run.
type ChaosTestResult struct {
	ServiceID          string
	Profile            ChaosProfile
	Timeline           []ChaosSnapshot
	DetectionCycle     int // when consensus first failed
	EvictionCycle      int // when service would be evicted (rung threshold exceeded)
	FinalDistance      int
	FinalRung          int
	TotalCycles        int
	RecoverySucceeded  bool
	TimeToDetection    int // cycles from start to detection
	TimeToEviction     int // cycles from start to eviction threshold
}

// String returns human-readable chaos test result.
func (ctr ChaosTestResult) String() string {
	return fmt.Sprintf(
		"%s (%s): Detection@%d cycles, Eviction@%d cycles | Final: distance=%d, rung=%d | Recovery: %v",
		ctr.ServiceID, ctr.Profile.Name, ctr.TimeToDetection, ctr.TimeToEviction,
		ctr.FinalDistance, ctr.FinalRung, ctr.RecoverySucceeded,
	)
}

// RunChaosTest executes a chaos scenario against a service and tracks degradation metrics.
func (sc *ServiceCluster) RunChaosTest(service *HostedService, profile ChaosProfile) ChaosTestResult {
	result := ChaosTestResult{
		ServiceID:      service.ServiceID,
		Profile:        profile,
		Timeline:       []ChaosSnapshot{},
		DetectionCycle: -1,
		EvictionCycle:  -1,
		TotalCycles:    0,
	}

	currentPhaseIdx := 0
	cycleInPhase := 0
	totalCycles := 0

	// Run test cycles through all phases
	for currentPhaseIdx < len(profile.Phases) {
		phase := profile.Phases[currentPhaseIdx]

		// Apply degradation for this phase
		switch phase.DegradationType {
		case "latency":
			service.SimulateLatencyDegradation(phase.Severity)
		case "entropy":
			if phase.Severity > 0 {
				service.SimulateConstantOutput(false) // entropy loss
			}
		case "input-ignorance":
			service.SimulateInputIgnorance()
		case "none":
			// No degradation for this phase
		}

		// Run test cycles in this phase
		testResult, _ := sc.TestService(service.ServiceID)
		totalCycles++

		// Capture snapshot
		snapshot := ChaosSnapshot{
			Cycle:             totalCycles,
			ServiceDistance:   service.State.TotalDistance,
			ServiceRung:       service.State.Position.Rung,
			ConsensusFormed:   testResult.Consensus.ConsensusFormed,
			HealthyAlarmCount: testResult.Consensus.QuorumSize,
			FailedAlarmCount:  len(result.Timeline) - testResult.Consensus.QuorumSize,
			SuccessRate:       testResult.ServiceHealth.SuccessRate,
		}

		// Calculate average latency from test results
		totalLatency := 0
		latencyCount := 0
		for _, tr := range testResult.AlarmResults[0].TestResults {
			totalLatency += tr.LatencyMs
			latencyCount++
		}
		if latencyCount > 0 {
			snapshot.AverageLatencyMs = totalLatency / latencyCount
		}

		result.Timeline = append(result.Timeline, snapshot)

		// Track detection: when consensus first fails
		if result.DetectionCycle == -1 && !testResult.Consensus.ConsensusFormed {
			result.DetectionCycle = totalCycles
			result.TimeToDetection = totalCycles
		}

		// Track eviction: when service's rung exceeds healthy threshold (rung > 3 = degraded/critical)
		if result.EvictionCycle == -1 && service.State.Position.Rung > 3 {
			result.EvictionCycle = totalCycles
			result.TimeToEviction = totalCycles
		}

		cycleInPhase++
		if cycleInPhase >= phase.DurationCycles {
			cycleInPhase = 0
			currentPhaseIdx++
		}
	}

	result.TotalCycles = totalCycles
	result.FinalDistance = service.State.TotalDistance
	result.FinalRung = service.State.Position.Rung

	// Test recovery
	service.Recover()
	testResult, _ := sc.TestService(service.ServiceID)
	result.RecoverySucceeded = testResult.Consensus.ConsensusFormed && testResult.ServiceHealth.SuccessRate == 1.0

	return result
}

// LatencyEscalationProfile creates a chaos profile for gradually increasing latency.
func LatencyEscalationProfile() ChaosProfile {
	return ChaosProfile{
		Name:        "Latency Escalation",
		Description: "Gradually increase latency SLA violations",
		Phases: []ChaosPhase{
			{
				DurationCycles:  2,
				DegradationType: "none",
				Description:     "Baseline: service healthy",
			},
			{
				DurationCycles:  3,
				DegradationType: "latency",
				Severity:        25,
				Description:     "Phase 1: +25% latency",
			},
			{
				DurationCycles:  3,
				DegradationType: "latency",
				Severity:        50,
				Description:     "Phase 2: +50% latency",
			},
			{
				DurationCycles:  3,
				DegradationType: "latency",
				Severity:        75,
				Description:     "Phase 3: +75% latency (severe SLA violations)",
			},
		},
	}
}

// EntropyCollapseProfile creates a chaos profile for immediate entropy loss.
func EntropyCollapseProfile() ChaosProfile {
	return ChaosProfile{
		Name:        "Entropy Collapse",
		Description: "Immediate loss of entropy (constant output)",
		Phases: []ChaosPhase{
			{
				DurationCycles:  2,
				DegradationType: "none",
				Description:     "Baseline: service healthy",
			},
			{
				DurationCycles:  5,
				DegradationType: "entropy",
				Severity:        1,
				Description:     "Entropy loss: service returns constant output",
			},
		},
	}
}

// InputIgnoranceProfile creates a chaos profile for input correlation failures.
func InputIgnoranceProfile() ChaosProfile {
	return ChaosProfile{
		Name:        "Input Ignorance",
		Description: "Service stops responding to inputs",
		Phases: []ChaosPhase{
			{
				DurationCycles:  2,
				DegradationType: "none",
				Description:     "Baseline: service healthy",
			},
			{
				DurationCycles:  5,
				DegradationType: "input-ignorance",
				Severity:        1,
				Description:     "Input ignorance: service ignores all inputs",
			},
		},
	}
}

// CascadingFailureProfile creates a chaos profile for cascading degradation.
func CascadingFailureProfile() ChaosProfile {
	return ChaosProfile{
		Name:        "Cascading Failure",
		Description: "Latency → entropy → input ignorance progression",
		Phases: []ChaosPhase{
			{
				DurationCycles:  2,
				DegradationType: "none",
				Description:     "Baseline: service healthy",
			},
			{
				DurationCycles:  3,
				DegradationType: "latency",
				Severity:        50,
				Description:     "Stage 1: Latency degradation (+50%)",
			},
			{
				DurationCycles:  3,
				DegradationType: "entropy",
				Severity:        1,
				Description:     "Stage 2: Entropy loss (constant output)",
			},
			{
				DurationCycles:  3,
				DegradationType: "input-ignorance",
				Severity:        1,
				Description:     "Stage 3: Input ignorance (service unresponsive)",
			},
		},
	}
}
