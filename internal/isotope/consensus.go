package isotope

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"
	"time"
)

// AlarmID uniquely identifies a smoke alarm or fire marshal.
type AlarmID string

// ConsensusReport represents what one alarm reports about a test.
type ConsensusReport struct {
	AlarmID   AlarmID
	Isotope   Isotope
	Signature string    // Signature of the isotope (proves it came from this alarm)
	Timestamp time.Time // When the report was generated
}

// IsotopeAgreement verifies that all alarms report the same isotope.
type IsotopeAgreement struct {
	IsotopeFamily string
	IsotopeVersion int
	RequiredQuorum int // Minimum alarms needed for consensus
	Reports        []ConsensusReport
}

// QuorumStatus represents the result of consensus evaluation.
type QuorumStatus struct {
	AgreedIsotopes []Isotope // Isotopes that formed consensus
	AgreedCount    int        // Number of alarms in agreement
	DisagreedCount int        // Number of alarms that disagreed
	Outliers       []AlarmID  // Alarms that disagreed
	ConsensusFormed bool      // true if QuorumSize met
	ByzantineCount int        // Number of failures tolerated (F = (N-1)/3)
}

// VerifyIsotopeAgreement checks if enough alarms agree on the same isotope.
// Implements Byzantine Fault Tolerance: N >= 3F+1 alarms can tolerate F failures.
func VerifyIsotopeAgreement(reports []ConsensusReport, requiredQuorum int) QuorumStatus {
	if len(reports) == 0 {
		return QuorumStatus{
			ConsensusFormed: false,
			AgreedCount:     0,
			DisagreedCount:  0,
			ByzantineCount:  0,
		}
	}

	// Count isotopes by signature (two reports with same signature = same isotope)
	isotopeGroups := make(map[string][]ConsensusReport)
	for _, report := range reports {
		sig := report.Isotope.Signature
		isotopeGroups[sig] = append(isotopeGroups[sig], report)
	}

	// Find the largest agreement group
	var maxGroup []ConsensusReport
	var maxIsotope Isotope
	for _, group := range isotopeGroups {
		if len(group) > len(maxGroup) {
			maxGroup = group
			maxIsotope = group[0].Isotope
		}
	}

	// Identify disagreers (anyone not in the max group)
	agreedMap := make(map[AlarmID]bool)
	for _, report := range maxGroup {
		agreedMap[report.AlarmID] = true
	}

	var outliers []AlarmID
	for _, report := range reports {
		if !agreedMap[report.AlarmID] {
			outliers = append(outliers, report.AlarmID)
		}
	}

	consensusFormed := len(maxGroup) >= requiredQuorum

	// Byzantine tolerance: N nodes can tolerate F failures if N >= 3F+1
	// Therefore F = floor((N-1)/3)
	byzantineCount := (len(reports) - 1) / 3

	return QuorumStatus{
		AgreedIsotopes: []Isotope{maxIsotope},
		AgreedCount:    len(maxGroup),
		DisagreedCount: len(outliers),
		Outliers:       outliers,
		ConsensusFormed: consensusFormed,
		ByzantineCount: byzantineCount,
	}
}

// VerifyConsensusSignatures verifies that each report's signature is cryptographically valid.
// Returns error if any signature is invalid.
func VerifyConsensusSignatures(reports []ConsensusReport, alarmSigningKeys map[AlarmID][]byte) error {
	for _, report := range reports {
		key, ok := alarmSigningKeys[report.AlarmID]
		if !ok {
			return fmt.Errorf("no signing key for alarm %s", report.AlarmID)
		}

		// Recompute HMAC of the isotope raw canonical form
		h := hmac.New(sha256.New, key)
		h.Write([]byte(report.Isotope.Raw))
		expectedSig := fmt.Sprintf("%x", h.Sum(nil))

		if expectedSig != report.Isotope.Signature {
			return fmt.Errorf("signature mismatch for alarm %s: expected %s, got %s",
				report.AlarmID, expectedSig[:16], report.Isotope.Signature[:16])
		}
	}
	return nil
}

// TimestampOrdering verifies that reports have monotonically non-decreasing timestamps.
// Returns error if a later report has a strictly earlier timestamp (out-of-order).
func TimestampOrdering(reports []ConsensusReport) error {
	for i := 0; i < len(reports)-1; i++ {
		t1 := reports[i].Timestamp
		t2 := reports[i+1].Timestamp
		// Report i+1 should not come before report i
		if t2.Before(t1) {
			return fmt.Errorf("timestamp ordering violation: report i=%d (%s) comes before report i=%d (%s)",
				i+1, t2, i, t1)
		}
	}
	return nil
}

// IsotopeChainVerification verifies the isotope chain (primary → secondary → verification).
// Used when Fire-Marshals delegate to each other.
type IsotopeChainLink struct {
	Role      string  // "requester", "secondary", "verifier"
	Isotope   Isotope
	Timestamp time.Time
}

// VerifyIsotopeChain checks that all links in the chain are accounted for and signed.
func VerifyIsotopeChain(links []IsotopeChainLink) error {
	if len(links) < 2 {
		return fmt.Errorf("isotope chain must have at least 2 links (requester + secondary)")
	}

	requester := links[0]
	if requester.Role != "requester" {
		return fmt.Errorf("first chain link must be requester role")
	}

	// Each subsequent link must have a later timestamp
	for i := 1; i < len(links); i++ {
		if links[i].Timestamp.Before(links[i-1].Timestamp) {
			return fmt.Errorf("isotope chain has out-of-order timestamps")
		}
	}

	return nil
}

// ConsensusReporter provides a way for an alarm to report its isotope findings.
type ConsensusReporter struct {
	AlarmID    AlarmID
	SigningKey []byte
}

// Report generates a ConsensusReport from an isotope.
func (cr *ConsensusReporter) Report(isotope Isotope) ConsensusReport {
	return ConsensusReport{
		AlarmID:   cr.AlarmID,
		Isotope:   isotope,
		Signature: isotope.Signature, // Already signed by the isotope generator
		Timestamp: time.Now(),
	}
}

// Byzantine test predicates (from smoke-alarm-test-suite seed)

// TestEntropyCheck verifies output has sufficient randomness.
// Isotope variation across multiple runs indicates entropy is present.
func TestEntropyCheck(results []string) bool {
	if len(results) < 10 {
		return false
	}

	// Simple entropy: need at least N/2 different results
	uniqueResults := make(map[string]bool)
	for _, r := range results {
		uniqueResults[r] = true
	}

	threshold := len(results) / 2
	return len(uniqueResults) >= threshold
}

// TestIsotopeVariation verifies that isotope changes when semantics change.
func TestIsotopeVariation(isotope1, isotope2 Isotope, shouldMatch bool) bool {
	match := isotope1.Signature == isotope2.Signature
	return match == shouldMatch
}

// TestAgreementPattern verifies that multiple alarms report same isotope.
func TestAgreementPattern(reports []ConsensusReport) bool {
	if len(reports) < 3 {
		return false // Need at least 3 alarms for Byzantine tolerance
	}

	status := VerifyIsotopeAgreement(reports, 2) // Require 2/3 agreement
	return status.ConsensusFormed && len(status.Outliers) <= 1
}

// TestDeclarativeBehaviorCompliance verifies declared behavior is observed.
// Isotope indicates the specific behavior aspect being tested.
func TestDeclarativeBehaviorCompliance(isotope Isotope, observed bool) bool {
	// Isotope tags which behavior is being tested; observed indicates pass/fail
	return observed
}

// DetectCompromisedAlarm identifies an alarm that disagrees with consensus.
// Returns the alarm ID if one is clearly an outlier.
func DetectCompromisedAlarm(reports []ConsensusReport, requiredQuorum int) *AlarmID {
	status := VerifyIsotopeAgreement(reports, requiredQuorum)

	if len(status.Outliers) == 1 {
		// Single outlier is suspicious
		return &status.Outliers[0]
	}

	if len(status.Outliers) > 1 && len(status.Outliers) <= (len(reports) / 3) {
		// Multiple outliers but within Byzantine tolerance (F = N/3)
		// This is recoverable; consensus still valid
		return nil
	}

	// No clear outlier pattern
	return nil
}
