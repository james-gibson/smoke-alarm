package isotope

import (
	"testing"
	"time"
)

func TestIsotopeAgreementThreeAlarms(t *testing.T) {
	// All three alarms report same isotope
	isotope := Isotope{
		Family:    "iso-login",
		Version:   1,
		Signature: "abc123def456ghi789",
		Raw:       "FEATURE:name:user-auth\nSCENARIO:name:valid-login\nGIVEN:hash:xyz",
	}

	reports := []ConsensusReport{
		{
			AlarmID:   "alarm-a",
			Isotope:   isotope,
			Signature: isotope.Signature,
			Timestamp: time.Now(),
		},
		{
			AlarmID:   "alarm-b",
			Isotope:   isotope,
			Signature: isotope.Signature,
			Timestamp: time.Now().Add(100 * time.Millisecond),
		},
		{
			AlarmID:   "alarm-c",
			Isotope:   isotope,
			Signature: isotope.Signature,
			Timestamp: time.Now().Add(200 * time.Millisecond),
		},
	}

	status := VerifyIsotopeAgreement(reports, 2) // Require 2/3 agreement

	if !status.ConsensusFormed {
		t.Error("Expected consensus to form with 3/3 alarms agreeing")
	}

	if status.AgreedCount != 3 {
		t.Errorf("Expected 3 alarms to agree, got %d", status.AgreedCount)
	}

	if len(status.Outliers) != 0 {
		t.Errorf("Expected no outliers, got %d", len(status.Outliers))
	}
}

func TestIsotopeDisagreementDetection(t *testing.T) {
	// Alarm C reports different isotope
	isotope1 := Isotope{
		Family:    "iso-login",
		Version:   1,
		Signature: "abc123def456ghi789",
		Raw:       "FEATURE:name:user-auth\nSCENARIO:name:valid-login",
	}

	isotope2 := Isotope{
		Family:    "iso-login",
		Version:   1,
		Signature: "wrongwrongwrongwrong",
		Raw:       "FEATURE:name:user-auth\nSCENARIO:name:valid-login",
	}

	reports := []ConsensusReport{
		{AlarmID: "alarm-a", Isotope: isotope1, Timestamp: time.Now()},
		{AlarmID: "alarm-b", Isotope: isotope1, Timestamp: time.Now()},
		{AlarmID: "alarm-c", Isotope: isotope2, Timestamp: time.Now()}, // Disagreer
	}

	status := VerifyIsotopeAgreement(reports, 2)

	if !status.ConsensusFormed {
		t.Error("Consensus should form with 2/3 agreement")
	}

	if status.AgreedCount != 2 {
		t.Errorf("Expected 2 alarms to agree, got %d", status.AgreedCount)
	}

	if len(status.Outliers) != 1 {
		t.Errorf("Expected 1 outlier, got %d", len(status.Outliers))
	}

	if status.Outliers[0] != "alarm-c" {
		t.Errorf("Expected alarm-c to be outlier, got %v", status.Outliers[0])
	}
}

func TestByzantineToleranceFiveAlarms(t *testing.T) {
	// 5 alarms can tolerate F=1 failures
	// If 4 agree and 1 disagrees, consensus forms
	isotope1 := Isotope{
		Family:    "iso-login",
		Version:   1,
		Signature: "sig1",
		Raw:       "canonical-form-1",
	}

	isotope2 := Isotope{
		Family:    "iso-login",
		Version:   1,
		Signature: "sig2",
		Raw:       "canonical-form-2",
	}

	reports := []ConsensusReport{
		{AlarmID: "alarm-a", Isotope: isotope1, Timestamp: time.Now()},
		{AlarmID: "alarm-b", Isotope: isotope1, Timestamp: time.Now()},
		{AlarmID: "alarm-c", Isotope: isotope1, Timestamp: time.Now()},
		{AlarmID: "alarm-d", Isotope: isotope1, Timestamp: time.Now()},
		{AlarmID: "alarm-e", Isotope: isotope2, Timestamp: time.Now()}, // Compromised
	}

	status := VerifyIsotopeAgreement(reports, 3) // Require 3/5 quorum

	if !status.ConsensusFormed {
		t.Error("Consensus should form with 4/5 alarms agreeing")
	}

	if status.ByzantineCount != 1 {
		t.Errorf("Expected Byzantine tolerance F=1, got F=%d", status.ByzantineCount)
	}

	t.Logf("5-alarm consensus: %d agree, %d disagree, can tolerate F=%d Byzantine failures",
		status.AgreedCount, status.DisagreedCount, status.ByzantineCount)
}

func TestConsensusRequiresQuorum(t *testing.T) {
	isotope := Isotope{
		Family:    "iso-login",
		Version:   1,
		Signature: "sig1",
		Raw:       "canonical",
	}

	// Only 2 alarms, require quorum of 3
	reports := []ConsensusReport{
		{AlarmID: "alarm-a", Isotope: isotope, Timestamp: time.Now()},
		{AlarmID: "alarm-b", Isotope: isotope, Timestamp: time.Now()},
	}

	status := VerifyIsotopeAgreement(reports, 3) // Require 3 alarms

	if status.ConsensusFormed {
		t.Error("Consensus should not form with only 2 alarms (quorum=3)")
	}

	if status.AgreedCount != 2 {
		t.Errorf("Expected 2 alarms, got %d", status.AgreedCount)
	}
}

func TestTimestampOrdering(t *testing.T) {
	now := time.Now()

	// Valid: monotonically increasing
	validReports := []ConsensusReport{
		{AlarmID: "a", Timestamp: now},
		{AlarmID: "b", Timestamp: now.Add(100 * time.Millisecond)},
		{AlarmID: "c", Timestamp: now.Add(200 * time.Millisecond)},
	}

	err := TimestampOrdering(validReports)
	if err != nil {
		t.Errorf("TimestampOrdering should accept monotonic timestamps: %v", err)
	}

	// Invalid: out of order
	invalidReports := []ConsensusReport{
		{AlarmID: "a", Timestamp: now},
		{AlarmID: "b", Timestamp: now.Add(200 * time.Millisecond)},
		{AlarmID: "c", Timestamp: now.Add(100 * time.Millisecond)}, // Earlier than previous
	}

	err = TimestampOrdering(invalidReports)
	if err == nil {
		t.Error("TimestampOrdering should reject out-of-order timestamps")
	}
}

func TestDetectCompromisedAlarm(t *testing.T) {
	isotope1 := Isotope{
		Family:    "iso-test",
		Version:   1,
		Signature: "sig1",
		Raw:       "raw1",
	}

	isotope2 := Isotope{
		Family:    "iso-test",
		Version:   1,
		Signature: "sig2",
		Raw:       "raw2",
	}

	// Three alarms, one disagreeing
	reports := []ConsensusReport{
		{AlarmID: "alarm-a", Isotope: isotope1, Timestamp: time.Now()},
		{AlarmID: "alarm-b", Isotope: isotope1, Timestamp: time.Now()},
		{AlarmID: "alarm-c", Isotope: isotope2, Timestamp: time.Now()}, // Compromised
	}

	compromised := DetectCompromisedAlarm(reports, 2)
	if compromised == nil {
		t.Error("Should detect compromised alarm")
	} else if *compromised != "alarm-c" {
		t.Errorf("Expected alarm-c to be compromised, got %s", *compromised)
	}
}

func TestEntropyCheckPredicate(t *testing.T) {
	tests := []struct {
		name     string
		results  []string
		expected bool
	}{
		{
			name:     "High entropy (many unique)",
			results:  []string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"},
			expected: true,
		},
		{
			name:     "Low entropy (few unique)",
			results:  []string{"a", "a", "a", "a", "a", "b", "b", "b", "b", "b"},
			expected: false, // 10 runs, 2 unique = 20% (threshold 50% = 5 unique needed)
		},
		{
			name:     "Too low entropy (constant output)",
			results:  []string{"a", "a", "a", "a", "a", "a", "a", "a", "a", "a"},
			expected: false,
		},
		{
			name:     "Insufficient data",
			results:  []string{"a", "b"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TestEntropyCheck(tt.results)
			if result != tt.expected {
				t.Errorf("TestEntropyCheck(%v) = %v, expected %v", tt.name, result, tt.expected)
			}
		})
	}
}

func TestAgreementPatternPredicate(t *testing.T) {
	isotope1 := Isotope{
		Family:    "iso-test",
		Version:   1,
		Signature: "sig1",
		Raw:       "raw1",
	}

	isotope2 := Isotope{
		Family:    "iso-test",
		Version:   1,
		Signature: "sig2",
		Raw:       "raw2",
	}

	// Three alarms, two agree
	reports := []ConsensusReport{
		{AlarmID: "a", Isotope: isotope1},
		{AlarmID: "b", Isotope: isotope1},
		{AlarmID: "c", Isotope: isotope2},
	}

	if !TestAgreementPattern(reports) {
		t.Error("AgreementPattern should detect 2/3 consensus")
	}

	// Three alarms, all disagree
	reports2 := []ConsensusReport{
		{AlarmID: "a", Isotope: isotope1},
		{AlarmID: "b", Isotope: isotope2},
		{AlarmID: "c", Isotope: isotope2},
	}

	if !TestAgreementPattern(reports2) {
		t.Error("AgreementPattern should accept 2/3 consensus (even if different isotopes)")
	}

	// Only two alarms (insufficient for Byzantine)
	reports3 := []ConsensusReport{
		{AlarmID: "a", Isotope: isotope1},
		{AlarmID: "b", Isotope: isotope1},
	}

	if TestAgreementPattern(reports3) {
		t.Error("AgreementPattern should reject <3 alarms")
	}
}

func TestConsensusReporter(t *testing.T) {
	isotope := Isotope{
		Family:    "iso-test",
		Version:   1,
		Signature: "sig123",
		Raw:       "raw",
	}

	reporter := &ConsensusReporter{
		AlarmID:    "alarm-a",
		SigningKey: []byte("key"),
	}

	report := reporter.Report(isotope)

	if report.AlarmID != "alarm-a" {
		t.Errorf("Report should have correct alarm ID")
	}

	if report.Isotope.Signature != isotope.Signature {
		t.Errorf("Report should preserve isotope signature")
	}

	if report.Timestamp.IsZero() {
		t.Error("Report should have valid timestamp")
	}
}
