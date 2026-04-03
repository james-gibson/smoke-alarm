package isotope

import (
	"testing"
	"time"
)

func TestRecordAndQueryResult(t *testing.T) {
	ledger := NewResultLedger()
	isotope := Isotope{
		Family:    "iso-login",
		Version:   1,
		Signature: "sig123",
		Raw:       "raw",
	}

	result := TestResult{
		Isotope:     isotope,
		Result:      true,
		Reason:      "test passed",
		Timestamp:   time.Now(),
		AlarmID:     "alarm-a",
		FailureType: "",
		ExecutionMs: 100,
	}

	ledger.RecordResult(result)

	history := ledger.QueryByIsotope("iso-login", 1)
	if history == nil {
		t.Fatal("Expected to find history for iso-login-v1")
	}

	if len(history.Results) != 1 {
		t.Errorf("Expected 1 result, got %d", len(history.Results))
	}

	if history.PassCount != 1 || history.FailCount != 0 {
		t.Errorf("Expected 1 pass, 0 fail, got %d pass, %d fail", history.PassCount, history.FailCount)
	}
}

func TestRefactoringKeepsSameIsotope(t *testing.T) {
	ledger := NewResultLedger()
	isotope := Isotope{
		Family:    "iso-login",
		Version:   1,
		Signature: "sig123",
		Raw:       "raw",
	}

	now := time.Now()

	// Record results from refactored versions of same test (same isotope)
	results := []TestResult{
		{Isotope: isotope, Result: false, Timestamp: now, AlarmID: "alarm-a", Reason: "timeout"},
		{Isotope: isotope, Result: false, Timestamp: now.Add(100 * time.Millisecond), AlarmID: "alarm-a", Reason: "timeout"},
		{Isotope: isotope, Result: true, Timestamp: now.Add(200 * time.Millisecond), AlarmID: "alarm-a"},
	}

	for _, r := range results {
		ledger.RecordResult(r)
	}

	history := ledger.QueryByIsotope("iso-login", 1)
	if history.PassCount != 1 || history.FailCount != 2 {
		t.Errorf("Expected 1 pass, 2 fail, got %d pass, %d fail", history.PassCount, history.FailCount)
	}

	// All results should be under same isotope despite refactoring
	if len(history.Results) != 3 {
		t.Errorf("Expected 3 results aggregated under same isotope, got %d", len(history.Results))
	}
}

func TestDetectFlakiness(t *testing.T) {
	ledger := NewResultLedger()
	isotope := Isotope{
		Family:    "iso-flaky",
		Version:   1,
		Signature: "sig",
		Raw:       "raw",
	}

	now := time.Now()

	// Create alternating pass/fail pattern (flaky)
	for i := 0; i < 10; i++ {
		result := TestResult{
			Isotope:   isotope,
			Result:    i%2 == 0, // Alternating
			Timestamp: now.Add(time.Duration(i*100) * time.Millisecond),
			AlarmID:   "alarm-a",
		}
		ledger.RecordResult(result)
	}

	isFlaky, failRate := ledger.DetectFlakiness("iso-flaky", 1)
	if !isFlaky {
		t.Error("Expected to detect flakiness in alternating pattern")
	}

	if failRate < 0.4 || failRate > 0.6 {
		t.Errorf("Expected fail rate around 0.5, got %.2f", failRate)
	}
}

func TestDetectRegression(t *testing.T) {
	ledger := NewResultLedger()
	isotope := Isotope{
		Family:    "iso-regression",
		Version:   1,
		Signature: "sig",
		Raw:       "raw",
	}

	now := time.Now()

	// First 5 results: all pass
	for i := 0; i < 5; i++ {
		ledger.RecordResult(TestResult{
			Isotope:   isotope,
			Result:    true,
			Timestamp: now.Add(time.Duration(i*100) * time.Millisecond),
			AlarmID:   "alarm-a",
		})
	}

	// Then 3 failures (regression)
	regressionTime := now.Add(500 * time.Millisecond)
	for i := 0; i < 3; i++ {
		ledger.RecordResult(TestResult{
			Isotope:   isotope,
			Result:    false,
			Timestamp: regressionTime.Add(time.Duration(i*100) * time.Millisecond),
			AlarmID:   "alarm-a",
			Reason:    "regression",
		})
	}

	isRegression, timestamp := ledger.DetectRegression("iso-regression", 1)
	if !isRegression {
		t.Error("Expected to detect regression")
	}

	if timestamp.Before(regressionTime) || timestamp.After(regressionTime.Add(200*time.Millisecond)) {
		t.Errorf("Regression timestamp is not in expected range")
	}
}

func TestVerifyFix(t *testing.T) {
	ledger := NewResultLedger()
	isotope := Isotope{
		Family:    "iso-fixed",
		Version:   1,
		Signature: "sig",
		Raw:       "raw",
	}

	now := time.Now()
	t1 := now.Add(100 * time.Millisecond)
	t2 := now.Add(200 * time.Millisecond)
	t3 := now.Add(300 * time.Millisecond)

	// Initially failing
	ledger.RecordResult(TestResult{Isotope: isotope, Result: false, Timestamp: now, AlarmID: "alarm-a"})
	ledger.RecordResult(TestResult{Isotope: isotope, Result: false, Timestamp: t1, AlarmID: "alarm-a"})

	// Fix deployed
	fixTime := t2

	// After fix: 4 consecutive passes
	ledger.RecordResult(TestResult{Isotope: isotope, Result: true, Timestamp: t2, AlarmID: "alarm-a"})
	ledger.RecordResult(TestResult{Isotope: isotope, Result: true, Timestamp: t3, AlarmID: "alarm-a"})
	ledger.RecordResult(TestResult{Isotope: isotope, Result: true, Timestamp: t3.Add(100 * time.Millisecond), AlarmID: "alarm-a"})
	ledger.RecordResult(TestResult{Isotope: isotope, Result: true, Timestamp: t3.Add(200 * time.Millisecond), AlarmID: "alarm-a"})

	fixed := ledger.VerifyFix("iso-fixed", 1, fixTime, 4)
	if !fixed {
		t.Error("Expected to verify fix (4 consecutive passes after fix time)")
	}

	// Require 5 passes but only 4 occurred
	fixed = ledger.VerifyFix("iso-fixed", 1, fixTime, 5)
	if fixed {
		t.Error("Should not verify fix with only 4 passes when 5 required")
	}
}

func TestMeasureRefactoringEffectiveness(t *testing.T) {
	ledger := NewResultLedger()
	isotope := Isotope{
		Family:    "iso-improving",
		Version:   1,
		Signature: "sig",
		Raw:       "raw",
	}

	now := time.Now()
	refactoringTime := now.Add(1 * time.Second)

	// Before refactoring: 50% pass rate (alternating)
	for i := 0; i < 10; i++ {
		ledger.RecordResult(TestResult{
			Isotope:   isotope,
			Result:    i%2 == 0,
			Timestamp: now.Add(time.Duration(i*100) * time.Millisecond),
			AlarmID:   "alarm-a",
		})
	}

	// After refactoring: 100% pass rate
	for i := 0; i < 5; i++ {
		ledger.RecordResult(TestResult{
			Isotope:   isotope,
			Result:    true,
			Timestamp: refactoringTime.Add(time.Duration(i*100) * time.Millisecond),
			AlarmID:   "alarm-a",
		})
	}

	before, after := ledger.MeasureRefactoringEffectiveness("iso-improving", 1, refactoringTime)

	if before < 0.4 || before > 0.6 {
		t.Errorf("Expected before refactoring ~50%% stability, got %.2f%%", before*100)
	}

	if after != 1.0 {
		t.Errorf("Expected after refactoring 100%% stability, got %.2f%%", after*100)
	}

	improvement := after - before
	if improvement < 0.4 {
		t.Errorf("Expected significant improvement, got %.2f%%", improvement*100)
	}
}

func TestQueryByFamily(t *testing.T) {
	ledger := NewResultLedger()

	// Add results for multiple versions of same family
	for version := 1; version <= 3; version++ {
		isotope := Isotope{
			Family:    "iso-login",
			Version:   version,
			Signature: "sig",
			Raw:       "raw",
		}
		ledger.RecordResult(TestResult{
			Isotope:   isotope,
			Result:    true,
			Timestamp: time.Now(),
			AlarmID:   "alarm-a",
		})
	}

	histories := ledger.QueryByFamily("iso-login")
	if len(histories) != 3 {
		t.Errorf("Expected 3 versions of iso-login, got %d", len(histories))
	}

	for i, h := range histories {
		if h.Family != "iso-login" {
			t.Errorf("History %d has wrong family: %s", i, h.Family)
		}
	}
}

func TestAggregateAcrossVariants(t *testing.T) {
	ledger := NewResultLedger()
	isotope := Isotope{
		Family:    "iso-login",
		Version:   1,
		Signature: "sig",
		Raw:       "raw",
	}

	now := time.Now()

	// Simulate same test running in multiple implementations/languages
	results := []struct {
		source string
		pass   bool
	}{
		{"suite-a-en", true},
		{"suite-a-en", true},
		{"suite-b-es", true},
		{"suite-b-es", false},
		{"suite-c-internal", true},
		{"suite-c-internal", true},
		{"suite-c-internal", true},
	}

	for i, r := range results {
		ledger.RecordResult(TestResult{
			Isotope:   isotope,
			Result:    r.pass,
			Timestamp: now.Add(time.Duration(i*100) * time.Millisecond),
			AlarmID:   AlarmID(r.source),
		})
	}

	pass, fail, stability := ledger.AggregateAcrossVariants("iso-login", 1)
	if pass+fail != 7 {
		t.Errorf("Expected 7 results, got %d", pass+fail)
	}

	if pass != 6 || fail != 1 {
		t.Errorf("Expected 6 pass, 1 fail, got %d pass, %d fail", pass, fail)
	}

	expectedStability := 6.0 / 7.0
	if stability < expectedStability-0.01 || stability > expectedStability+0.01 {
		t.Errorf("Expected %.2f stability, got %.2f", expectedStability, stability)
	}
}

func TestIsotopeHistorySummary(t *testing.T) {
	history := &IsotopeHistory{
		Isotope:   Isotope{Family: "iso-test", Version: 1},
		Family:    "iso-test",
		Version:   1,
		PassCount: 8,
		FailCount: 2,
		Stability: 0.8,
		Trend:     "improving",
		Flakiness: 0.15,
	}

	summary := history.GetSummary()
	if summary == "" {
		t.Error("Expected non-empty summary")
	}

	// Verify summary contains key info
	if !contains(summary, "iso-test-v1") {
		t.Error("Summary should contain isotope identifier")
	}
	if !contains(summary, "8") || !contains(summary, "2") {
		t.Error("Summary should contain pass/fail counts")
	}
	if !contains(summary, "improving") {
		t.Error("Summary should contain trend")
	}
}

func contains(s, substr string) bool {
	for i := 0; i < len(s); i++ {
		match := true
		for j := 0; j < len(substr) && i+j < len(s); j++ {
			if s[i+j] != substr[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}
