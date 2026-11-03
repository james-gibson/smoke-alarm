package mcp_faults

import (
	"testing"
)

// voteMargin is how many wrong answers a round of R replicas can absorb before
// the majority result changes: a majority needs floor(R/2)+1 correct, so
// R - (floor(R/2)+1) bad answers are harmless.
func voteMargin(replicas int) int {
	return replicas - (replicas/2 + 1)
}

// TestTolerancePolicyIsTheVoteMargin quantifies the policy choice between the two
// detection channels.
//
//   - Failing fast (t=0) bounds damage at a constant: the readings consumed before
//     detection is 2 for the distance channel (eviction at 60 against a 32 weight)
//     and ~4.6 for a 99% statistical window, whatever the bias and read rate. The
//     extra damage from tolerance is therefore a constant, not a multiple.
//   - What fail-fast buys with that constant is spent on false closure, and closing
//     a route costs the entire vote margin: the replica is gone, so the cluster's
//     ability to absorb a *future* wrong answer goes with it.
func TestTolerancePolicyIsTheVoteMargin(t *testing.T) {
	const (
		falsePositiveNoise = 0.005 // per-reading chance an honest node reads departure
		confidence         = 0.99
	)

	// Damage is a constant in the bias: extra bad answers per tolerance step.
	t.Logf("%-8s %-10s %-10s %-10s", "bias", "t=0", "t=1", "t=2")
	for _, p := range []float64{0.01, 0.02, 0.10, 0.50} {
		consumed := func(tol int) float64 {
			return float64(samplesForDetection(p, tol, confidence)) * p
		}
		t.Logf("%-8.0f%% %-10.1f %-10.1f %-10.1f", p*100, consumed(0), consumed(1), consumed(2))

		// The point: each tolerance step costs about the same number of bad answers
		// at every bias. If it scaled with p, tolerance would be a gamble on p.
		step := consumed(1) - consumed(0)
		if step > 4 {
			t.Errorf("p=%.2f: one tolerance step cost %.1f bad answers; expected a small constant", p, step)
		}
	}

	// False closure: rate at which an honest node is wrongly evicted inside its own
	// detection window, for a fixed window of 44 readings.
	const window = 44
	for _, tol := range []int{0, 1, 2} {
		falseAlarm := 1 - binomCDF(window, falsePositiveNoise, tol)
		t.Logf("t=%d: false-eviction risk in %d readings = %.2f%%", tol, window, falseAlarm*100)
	}

	// Replica counts and the margin a false eviction destroys.
	for _, replicas := range []int{2, 3, 5, 7} {
		t.Logf("R=%d: absorbs %d wrong answers/round; after one eviction R=%d absorbs %d",
			replicas, voteMargin(replicas), replicas-1, voteMargin(replicas-1))
	}

	// A false eviction must reduce the margin, or closure would be costless and the
	// policy question would vanish.
	if voteMargin(2) >= voteMargin(3) {
		t.Errorf("evicting from 3 replicas to 2 must cost margin: %d vs %d",
			voteMargin(3), voteMargin(2))
	}
	if voteMargin(1) != 0 {
		t.Errorf("a lone replica absorbs nothing; got %d", voteMargin(1))
	}

	// Therefore: tolerate up to the margin, never beyond it.
	for _, replicas := range []int{2, 3, 5} {
		margin := voteMargin(replicas)
		if replicas > 2 && margin < 1 {
			t.Errorf("R=%d should absorb at least one wrong answer", replicas)
		}
		if replicas == 2 && margin != 0 {
			t.Errorf("R=2 has no margin; got %d", margin)
		}
	}
}

// TestLatencyIsNotTheBindingConstraint checks the part the policy does *not*
// depend on: detection delay in wall-clock terms scales with 1/(p*rate), but the
// damage it admits is a constant. So read rate changes how long it looks, not what
// it costs.
func TestLatencyIsNotTheBindingConstraint(t *testing.T) {
	const (
		p          = 0.05
		confidence = 0.99
	)

	k := samplesForDetection(p, 0, confidence)
	badAnswers := float64(k) * p

	t.Logf("p=5%%: %d readings, %.1f bad answers consumed, regardless of rate", k, badAnswers)

	for _, rate := range []float64{1, 10, 100} {
		t.Logf("  at %3.0f readings/sec: detection in %.1fs", rate, float64(k)/rate)
	}

	// The damage bound is a constant: doubling the read rate halves the seconds but
	// not the bad answers.
	if badAnswers > 5 {
		t.Errorf("damage should stay near ln(1/confidence)=4.6 bad answers, got %.1f", badAnswers)
	}
}
