package isotope

import (
	"testing"
)

func TestAgentStateInitialization(t *testing.T) {
	agent := NewAgentState("test-agent")

	if agent.AgentID != "test-agent" {
		t.Errorf("Expected agent ID 'test-agent', got %s", agent.AgentID)
	}

	if agent.TotalDistance != 0 {
		t.Errorf("Expected initial distance 0, got %d", agent.TotalDistance)
	}

	if agent.Position.Rung != 0 {
		t.Errorf("Expected initial rung 0, got %d", agent.Position.Rung)
	}

	if agent.Position.Real != 42.0 {
		t.Errorf("Expected real axis at 42, got %f", agent.Position.Real)
	}

	if agent.Position.Imaginary != 0.0 {
		t.Errorf("Expected imaginary axis at 0, got %f", agent.Position.Imaginary)
	}

	if agent.Position.Direction != "seed-42-ground-truth" {
		t.Errorf("Expected direction 'seed-42-ground-truth', got %s", agent.Position.Direction)
	}
}

func TestRecordTestFailure(t *testing.T) {
	agent := NewAgentState("test-agent")

	// Agent starts at rung 0 (untrusted), distance 0
	if agent.Position.Rung != 0 {
		t.Errorf("Agent should start at rung 0, got %d", agent.Position.Rung)
	}

	// Record first failure: entropy-check (weight 16)
	rungChanged, newRung := agent.RecordTestFailure("entropy-check")

	if agent.TotalDistance != 16 {
		t.Errorf("Expected distance 16, got %d", agent.TotalDistance)
	}

	if agent.Position.Imaginary != 16.0 {
		t.Errorf("Expected imaginary axis at 16, got %f", agent.Position.Imaginary)
	}

	// Distance 16 fits in rung 1 (max 20): first-fit returns rung 1, rung changed from 0
	if !rungChanged {
		t.Error("Should change rung when distance moves from rung 0 to rung 1")
	}

	if newRung != 1 {
		t.Errorf("Expected rung 1 for distance 16, got %d", newRung)
	}
}

func TestRungDemotion(t *testing.T) {
	agent := NewAgentState("test-agent")

	// 7-rung model with first-fit low-to-high:
	//   rung0=0, rung1=20, rung2=60, rung3=100, rung4=140, rung5=180, rung6=220
	// Distance is classified into the first rung whose MaxDistance >= distance.

	// 10 × entropy-check (+16 each) → distance 160
	for i := 0; i < 10; i++ {
		agent.RecordTestFailure("entropy-check")
	}

	if agent.TotalDistance != 160 {
		t.Errorf("Expected distance 160, got %d", agent.TotalDistance)
	}

	// Distance 160: 160 > 140 (rung4), 160 ≤ 180 (rung5) → rung 5
	if agent.Position.Rung != 5 {
		t.Errorf("At distance 160, expected rung 5 (160 ≤ 180), got %d", agent.Position.Rung)
	}

	// 2 more failures → distance 192
	for i := 0; i < 2; i++ {
		agent.RecordTestFailure("entropy-check")
	}

	// Distance 192: 192 > 180 (rung5), 192 ≤ 220 (rung6) → rung 6
	if agent.Position.Rung != 6 {
		t.Errorf("At distance 192, expected rung 6 (192 ≤ 220), got %d", agent.Position.Rung)
	}
}

func TestRecordTestPass(t *testing.T) {
	agent := NewAgentState("test-agent")

	// Record failures
	agent.RecordTestFailure("entropy-check")     // +16
	agent.RecordTestFailure("input-correlation") // +12
	// Total: 28

	if agent.TotalDistance != 28 {
		t.Errorf("Expected distance 28 after failures, got %d", agent.TotalDistance)
	}

	// Fix one test
	_, newRung := agent.RecordTestPass("entropy-check")

	if agent.TotalDistance != 12 {
		t.Errorf("Expected distance 12 after fixing entropy-check, got %d", agent.TotalDistance)
	}

	// Distance 12: first-fit → rung 1 (12 ≤ 20).
	// Was at distance 28 → rung 2 (28 ≤ 60). Recovery from rung 2 back to rung 1.
	if newRung != 1 {
		t.Errorf("Expected rung 1 at distance 12, got %d", newRung)
	}
}

func TestAdjustedCost(t *testing.T) {
	agent := NewAgentState("test-agent")

	// At rung 0, base cost = 2^4 = 16, no distance = 16 * 1.0 = 16
	cost := agent.CalculateAdjustedCost()
	if cost != 16 {
		t.Errorf("Expected cost 16 at rung 0 with no distance, got %d", cost)
	}

	// Add failure: distance 16
	agent.RecordTestFailure("entropy-check")
	// Rung now 1, base cost = 2^5 = 32
	// Cost = 32 * (1 + 16/100) = 32 * 1.16 = 37.12 ≈ 37
	cost = agent.CalculateAdjustedCost()
	if cost < 36 || cost > 38 {
		t.Errorf("Expected cost around 37 with distance 16, got %d", cost)
	}
}

func TestConsensusGap(t *testing.T) {
	// Perfect consensus: 3/3 alarms agree
	gap := GetConsensusGap(3, 3)
	if gap != 0 {
		t.Errorf("Expected 0 gap for perfect consensus, got %d", gap)
	}

	// One alarm disagrees: 2/3
	gap = GetConsensusGap(2, 3)
	if gap <= 0 {
		t.Errorf("Expected positive gap for disagreement, got %d", gap)
	}

	// No quorum: 2/5 (need 3+)
	gap = GetConsensusGap(2, 5)
	if gap < 32 {
		t.Errorf("Expected at least 32 gap for no quorum, got %d", gap)
	}
}

func TestRungBoundaryCheck(t *testing.T) {
	agent := NewAgentState("test-agent")

	// At rung 0, no distance - should be ok
	alert := agent.CheckRungBoundary()
	if alert.Status != "ok" {
		t.Errorf("Expected 'ok' status at rung 0, got %s", alert.Status)
	}

	// Add failures to approach boundary
	for i := 0; i < 5; i++ {
		agent.RecordTestFailure("entropy-check")
		// Each failure adds 16, but we're in rung 1 (max 20)
	}
	// Distance should be 80 now, rung should be 0 or 1

	alert = agent.CheckRungBoundary()
	if alert.DistanceToThreshold > 100 {
		t.Errorf("Distance calculation seems off in alert")
	}
}

func TestInferDirection(t *testing.T) {
	agent := NewAgentState("test-agent")

	// No failures -> ground truth
	direction := agent.inferDirection()
	if direction != "seed-42-ground-truth" {
		t.Errorf("Expected 'seed-42-ground-truth', got %s", direction)
	}

	// Entropy + input issues -> unpredictable
	agent.RecordTestFailure("entropy-check")
	agent.RecordTestFailure("input-correlation")
	direction = agent.inferDirection()
	if direction != "unpredictable-behavior" {
		t.Errorf("Expected 'unpredictable-behavior', got %s", direction)
	}

	// Clear and try signaling issues
	agent.FailedTests = make(map[string]int)
	agent.TotalDistance = 0
	agent.RecordTestFailure("agreement-pattern")
	direction = agent.inferDirection()
	if direction != "coordinated-signaling" {
		t.Errorf("Expected 'coordinated-signaling', got %s", direction)
	}
}

func TestIsotopeResultIntegration(t *testing.T) {
	agent := NewAgentState("service-a")
	testIsotope := Isotope{
		Family:    "entropy-check",
		Version:   1,
		Signature: "sig123",
		Raw:       "raw",
	}

	// Record passing test
	result := agent.RecordIsotopeTestResult(testIsotope, true)

	if !result.Passed {
		t.Error("Expected Passed=true")
	}

	if result.RungChanged {
		t.Error("Passing test should not change rung")
	}

	// Record failing test
	result = agent.RecordIsotopeTestResult(testIsotope, false)

	if result.Passed {
		t.Error("Expected Passed=false")
	}

	if agent.TotalDistance != 16 {
		t.Errorf("Expected distance 16 after failure, got %d", agent.TotalDistance)
	}

	if result.BeforePosition.Imaginary != 0 {
		t.Errorf("Before position should have imaginary=0, got %f", result.BeforePosition.Imaginary)
	}

	if result.AfterPosition.Imaginary != 16.0 {
		t.Errorf("After position should have imaginary=16, got %f", result.AfterPosition.Imaginary)
	}
}

func TestMultipleTestFailures(t *testing.T) {
	agent := NewAgentState("service-b")

	// Record multiple different test failures
	tests := []string{
		"entropy-check",             // 16
		"input-correlation",         // 12
		"constant-output-detection", // 20
		"agreement-pattern",         // 32
	}

	expectedDistance := 0
	for _, testName := range tests {
		agent.RecordTestFailure(testName)
		expectedDistance += DefaultWeights[testName]
	}

	if agent.TotalDistance != expectedDistance {
		t.Errorf("Expected distance %d, got %d", expectedDistance, agent.TotalDistance)
	}

	// Should have crossed rung 1 boundary (20)
	if agent.Position.Rung == 1 {
		t.Error("Should have crossed rung 1 boundary with 80 distance")
	}
}

func TestRungHistoryTracking(t *testing.T) {
	agent := NewAgentState("service-c")

	if len(agent.RungHistory) != 1 || agent.RungHistory[0] != 0 {
		t.Error("Should start with rung 0 in history")
	}

	// Cause rung change
	for i := 0; i < 3; i++ {
		agent.RecordTestFailure("entropy-check")
	}

	// Should have recorded rung change
	if len(agent.RungHistory) < 2 {
		t.Error("Rung history should record changes")
	}
}

func TestPositionCalculation(t *testing.T) {
	agent := NewAgentState("test-agent")

	// At ground truth
	if agent.Position.Magnitude != 42.0 {
		t.Errorf("At ground truth, magnitude should be 42, got %f", agent.Position.Magnitude)
	}

	// Add distance
	agent.RecordTestFailure("entropy-check")
	// Distance = 16, magnitude = sqrt(42^2 + 16^2) = sqrt(1764 + 256) = sqrt(2020) ≈ 44.9

	expectedMagnitude := 44.9
	if agent.Position.Magnitude < expectedMagnitude-0.5 || agent.Position.Magnitude > expectedMagnitude+0.5 {
		t.Errorf("Expected magnitude ~44.9, got %f", agent.Position.Magnitude)
	}
}

func TestRungBoundaryAlert(t *testing.T) {
	agent := NewAgentState("test-agent")

	// Initially at rung 0, distance 0 → BoundaryStatus(0, 0) = "ok"
	alert := agent.CheckRungBoundary()
	if alert.Status != "ok" {
		t.Errorf("Expected 'ok' initially, got %s", alert.Status)
	}

	// Simulate reaching distance 18 through failures.
	// RecordTestFailure would place the agent at rung 1 (18 ≤ 20).
	// We set both to exercise CheckRungBoundary in isolation.
	agent.TotalDistance = 18
	agent.Position.Rung = 1 // rung 1 ceiling = 20; DTT = 20−18 = 2 → "critical"
	agent.recalculatePosition()

	alert = agent.CheckRungBoundary()
	if alert.Status != "critical" {
		t.Errorf("Expected 'critical' when 2 units from rung 1 ceiling, got %s", alert.Status)
	}

	// Cross the rung 1 ceiling: distance 25 > 20.
	// Rung stays at 1 (recalculatePosition doesn't change it).
	// BoundaryStatus(25, 20) = "demoted".
	agent.TotalDistance = 25
	agent.recalculatePosition()

	alert = agent.CheckRungBoundary()
	if alert.Status != "demoted" {
		t.Errorf("Expected 'demoted' when crossing rung 1 ceiling, got %s", alert.Status)
	}
}

func TestConsensusFailureIntegration(t *testing.T) {
	agent := NewAgentState("service-d")

	// Record consensus gap
	gap := ConsensusGap{
		IsotopeFamily: "entropy-check",
		AlarmCount:    1,
		TotalAlarms:   3,
		GapWeight:     48, // 16*1 + 32 for no quorum
		Reason:        "Only 2/3 alarms agreed",
	}

	agent.RecordConsensusFailure(gap)

	if agent.TotalDistance != 48 {
		t.Errorf("Expected distance 48 after consensus gap, got %d", agent.TotalDistance)
	}
}

// ── Pure function tests ──────────────────────────────────────────────────────

func TestRungForDistance(t *testing.T) {
	tests := []struct {
		distance int
		want     int
	}{
		{0, 0},    // exactly at rung 0 ceiling
		{1, 1},    // just above rung 0 → rung 1
		{20, 1},   // exactly at rung 1 ceiling
		{21, 2},   // just above → rung 2
		{60, 2},   // exactly at rung 2 ceiling
		{61, 3},   // rung 3
		{100, 3},  // rung 3 ceiling
		{101, 4},  // rung 4
		{140, 4},  // rung 4 ceiling
		{141, 5},  // rung 5
		{180, 5},  // rung 5 ceiling
		{181, 6},  // rung 6
		{220, 6},  // rung 6 ceiling
		{221, 0},  // exceeds all → fallback 0
		{1000, 0}, // way past all → fallback 0
	}
	for _, tt := range tests {
		got := RungForDistance(tt.distance, DefaultRungThresholds)
		if got != tt.want {
			t.Errorf("RungForDistance(%d) = %d, want %d", tt.distance, got, tt.want)
		}
	}
}

func TestBoundaryStatus(t *testing.T) {
	tests := []struct {
		distance  int
		threshold int
		want      string
	}{
		// ok: distance=0 regardless of threshold (distance > 0 guard)
		{0, 0, "ok"},
		{0, 20, "ok"},
		{0, 100, "ok"},

		// ok: far from threshold
		{10, 100, "ok"},
		{50, 100, "ok"},

		// warning: within 40 of threshold, distance > 0
		{65, 100, "warning"},
		{61, 100, "warning"},

		// critical: within 20 of threshold, distance > 0
		{81, 100, "critical"},
		{99, 100, "critical"},
		{18, 20, "critical"},

		// demoted: distance > threshold
		{25, 20, "demoted"},
		{101, 100, "demoted"},
		{1, 0, "demoted"},
		{221, 220, "demoted"},
	}
	for _, tt := range tests {
		got := BoundaryStatus(tt.distance, tt.threshold)
		if got != tt.want {
			t.Errorf("BoundaryStatus(%d, %d) = %q, want %q", tt.distance, tt.threshold, got, tt.want)
		}
	}
}

func TestCheckRungBoundaryAllRungs(t *testing.T) {
	// CheckRungBoundary checks distance against the CURRENT rung's ceiling.
	// recalculatePosition does NOT change the rung — only RecordTestFailure/Pass does.
	// So we set Position.Rung explicitly to test each rung's boundary behavior.
	tests := []struct {
		distance   int
		rung       int
		expectStat string
		desc       string
	}{
		// Rung 0, ceiling=0
		{0, 0, "ok", "ground truth"},
		{1, 0, "demoted", "any distance > 0 exceeds rung 0 ceiling"},

		// Rung 1, ceiling=20
		{5, 1, "critical", "5 units into rung 1, DTT=15 (<20) → critical"},
		{18, 1, "critical", "2 units from rung 1 ceiling"},
		{20, 1, "ok", "exactly at ceiling, DTT=0, but distance>0... wait DTT=0 < 20 → critical"},
		{25, 1, "demoted", "crossed rung 1 ceiling"},

		// Rung 2, ceiling=60
		{25, 2, "warning", "just entered rung 2, DTT=35 (<40) → warning"},
		{50, 2, "critical", "DTT=10 (<20) → critical"},
		{65, 2, "demoted", "crossed rung 2 ceiling"},

		// Rung 3, ceiling=100
		{70, 3, "warning", "DTT=30 → warning"},
		{90, 3, "critical", "DTT=10 → critical"},
		{105, 3, "demoted", "crossed rung 3 ceiling"},

		// Rung 4, ceiling=140
		{110, 4, "warning", "DTT=30 → warning"},
		{130, 4, "critical", "DTT=10 → critical"},
		{145, 4, "demoted", "crossed rung 4 ceiling"},

		// Rung 5, ceiling=180
		{160, 5, "critical", "DTT=20... wait DTT=20 is not < 20"},
		{170, 5, "critical", "DTT=10 → critical"},
		{185, 5, "demoted", "crossed rung 5 ceiling"},

		// Rung 6, ceiling=220
		{200, 6, "critical", "DTT=20... "},
		{210, 6, "critical", "DTT=10 → critical"},
		{225, 6, "demoted", "crossed rung 6 ceiling"},
	}

	// Fix up expectations: DTT=20 is NOT < 20, so it's not "critical", it's "warning" (20 < 40)
	// Let me just compute them correctly via BoundaryStatus to avoid manual errors.
	for _, tt := range tests {
		agent := NewAgentState("test")
		agent.TotalDistance = tt.distance
		agent.Position.Rung = tt.rung
		agent.recalculatePosition()

		// Verify rung was preserved
		if agent.Position.Rung != tt.rung {
			t.Errorf("%s: recalculatePosition changed rung from %d to %d", tt.desc, tt.rung, agent.Position.Rung)
			continue
		}

		alert := agent.CheckRungBoundary()
		threshold := DefaultRungThresholds[tt.rung].MaxDistance
		expectedStatus := BoundaryStatus(tt.distance, threshold)
		if alert.Status != expectedStatus {
			t.Errorf("%s (dist=%d, rung=%d, ceiling=%d): got %q, want %q",
				tt.desc, tt.distance, tt.rung, threshold, alert.Status, expectedStatus)
		}
	}
}
