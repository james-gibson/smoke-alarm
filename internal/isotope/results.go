package isotope

import (
	"fmt"
	"sync"
	"time"
)

// TestResult represents the outcome of running a test.
type TestResult struct {
	Isotope     Isotope
	Result      bool   // true = PASS, false = FAIL
	Reason      string // failure reason or note
	Timestamp   time.Time
	AlarmID     AlarmID
	FailureType string // "timeout", "assertion", "error", etc.
	ExecutionMs int    // How long the test took
}

// IsotopeHistory aggregates all results for a specific isotope.
type IsotopeHistory struct {
	Isotope   Isotope
	Family    string // Convenience copy
	Version   int    // Convenience copy
	Results   []TestResult
	FirstSeen time.Time
	LastSeen  time.Time
	PassCount int
	FailCount int
	Flakiness float64 // 0.0 to 1.0
	Stability float64 // Pass rate
	Trend     string  // "improving", "stable", "degrading", "unknown"
}

// ResultLedger tracks test results keyed by isotope.
type ResultLedger struct {
	mu        sync.RWMutex
	histories map[string]*IsotopeHistory // Key: isotope.Family + "-v" + version
}

// NewResultLedger creates a new result tracking ledger.
func NewResultLedger() *ResultLedger {
	return &ResultLedger{
		histories: make(map[string]*IsotopeHistory),
	}
}

// RecordResult stores a test result keyed by its isotope.
func (rl *ResultLedger) RecordResult(result TestResult) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	key := fmt.Sprintf("%s-v%d", result.Isotope.Family, result.Isotope.Version)

	history, ok := rl.histories[key]
	if !ok {
		history = &IsotopeHistory{
			Isotope:   result.Isotope,
			Family:    result.Isotope.Family,
			Version:   result.Isotope.Version,
			Results:   []TestResult{},
			FirstSeen: result.Timestamp,
		}
		rl.histories[key] = history
	}

	history.Results = append(history.Results, result)
	history.LastSeen = result.Timestamp

	if result.Result {
		history.PassCount++
	} else {
		history.FailCount++
	}

	// Update derived metrics
	history.recalculateMetrics()
}

// QueryByIsotope retrieves all results for a specific isotope.
func (rl *ResultLedger) QueryByIsotope(family string, version int) *IsotopeHistory {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	key := fmt.Sprintf("%s-v%d", family, version)
	return rl.histories[key]
}

// QueryByFamily retrieves all versions of a test family.
func (rl *ResultLedger) QueryByFamily(family string) []*IsotopeHistory {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	var results []*IsotopeHistory
	for _, history := range rl.histories {
		if history.Family == family {
			results = append(results, history)
		}
	}
	return results
}

// DetectFlakiness identifies if an isotope's tests are flaky (50% or more fail rate with random pattern).
func (rl *ResultLedger) DetectFlakiness(family string, version int) (bool, float64) {
	history := rl.QueryByIsotope(family, version)
	if history == nil || len(history.Results) < 10 {
		return false, 0.0
	}

	// Check for pattern: alternating or mixed passes and failures
	alternateCount := 0
	for i := 1; i < len(history.Results); i++ {
		prev := history.Results[i-1].Result
		curr := history.Results[i].Result
		if prev != curr {
			alternateCount++
		}
	}

	alternationRate := float64(alternateCount) / float64(len(history.Results)-1)
	failRate := float64(history.FailCount) / float64(len(history.Results))

	// Flaky if fails are frequent AND results alternate frequently
	isFlaky := failRate >= 0.4 && alternationRate >= 0.3

	return isFlaky, failRate
}

// DetectRegression identifies when a test that was passing starts failing.
func (rl *ResultLedger) DetectRegression(family string, version int) (bool, time.Time) {
	history := rl.QueryByIsotope(family, version)
	if history == nil || len(history.Results) < 2 {
		return false, time.Time{}
	}

	// Look for first transition from pass to fail
	lastResult := true // Assume first is pass
	for _, result := range history.Results {
		if lastResult && !result.Result {
			// Transition from pass to fail
			return true, result.Timestamp
		}
		lastResult = result.Result
	}

	return false, time.Time{}
}

// VerifyFix checks if a previously failing test is now stable (consecutive passes).
func (rl *ResultLedger) VerifyFix(family string, version int, sinceTimestamp time.Time, requiredConsecutivePasses int) bool {
	history := rl.QueryByIsotope(family, version)
	if history == nil {
		return false
	}

	// Count consecutive passes from (and including) the given time
	consecutivePasses := 0
	for _, result := range history.Results {
		// Use After OR Equal to include the fix time itself
		if result.Timestamp.After(sinceTimestamp) || result.Timestamp.Equal(sinceTimestamp) {
			if result.Result {
				consecutivePasses++
				if consecutivePasses >= requiredConsecutivePasses {
					return true
				}
			} else {
				consecutivePasses = 0
			}
		}
	}

	return false
}

// MeasureRefactoringEffectiveness compares stability before and after a refactoring.
func (rl *ResultLedger) MeasureRefactoringEffectiveness(family string, version int, refactoringTime time.Time) (beforeStability, afterStability float64) {
	history := rl.QueryByIsotope(family, version)
	if history == nil || len(history.Results) < 2 {
		return 0.0, 0.0
	}

	var beforePass, beforeTotal int
	var afterPass, afterTotal int

	for _, result := range history.Results {
		if result.Timestamp.Before(refactoringTime) {
			beforeTotal++
			if result.Result {
				beforePass++
			}
		} else {
			afterTotal++
			if result.Result {
				afterPass++
			}
		}
	}

	if beforeTotal > 0 {
		beforeStability = float64(beforePass) / float64(beforeTotal)
	}

	if afterTotal > 0 {
		afterStability = float64(afterPass) / float64(afterTotal)
	}

	return beforeStability, afterStability
}

// AggregateAcrossVariants combines results from tests with the same isotope across multiple sources (languages, implementations).
func (rl *ResultLedger) AggregateAcrossVariants(family string, version int) (totalPass, totalFail int, aggregateStability float64) {
	history := rl.QueryByIsotope(family, version)
	if history == nil {
		return 0, 0, 0.0
	}

	totalPass = history.PassCount
	totalFail = history.FailCount
	total := totalPass + totalFail

	if total > 0 {
		aggregateStability = float64(totalPass) / float64(total)
	}

	return totalPass, totalFail, aggregateStability
}

// recalculateMetrics updates stability, flakiness, and trend metrics for a history.
func (h *IsotopeHistory) recalculateMetrics() {
	if len(h.Results) == 0 {
		h.Stability = 0.0
		h.Flakiness = 0.0
		h.Trend = "unknown"
		return
	}

	// Stability = pass rate
	h.Stability = float64(h.PassCount) / float64(len(h.Results))

	// Flakiness = rate of result changes (alternations)
	alternations := 0
	for i := 1; i < len(h.Results); i++ {
		if h.Results[i-1].Result != h.Results[i].Result {
			alternations++
		}
	}
	h.Flakiness = float64(alternations) / float64(len(h.Results))

	// Trend = slope of stability over time (simplified)
	if len(h.Results) >= 5 {
		// Compare early vs late passes
		earlyPass := 0
		latePass := 0
		mid := len(h.Results) / 2

		for i := 0; i < mid; i++ {
			if h.Results[i].Result {
				earlyPass++
			}
		}

		for i := mid; i < len(h.Results); i++ {
			if h.Results[i].Result {
				latePass++
			}
		}

		earlyStability := float64(earlyPass) / float64(mid)
		lateStability := float64(latePass) / float64(len(h.Results)-mid)

		switch {
		case lateStability > earlyStability+0.1:
			h.Trend = "improving"
		case earlyStability > lateStability+0.1:
			h.Trend = "degrading"
		default:
			h.Trend = "stable"
		}
	} else {
		h.Trend = "unknown"
	}
}

// GetSummary returns a human-readable summary of isotope history.
func (h *IsotopeHistory) GetSummary() string {
	return fmt.Sprintf(
		"Isotope %s-v%d: %d pass, %d fail (%.1f%% stable) | Trend: %s | Flakiness: %.1f%%",
		h.Family, h.Version,
		h.PassCount, h.FailCount,
		h.Stability*100,
		h.Trend,
		h.Flakiness*100,
	)
}
