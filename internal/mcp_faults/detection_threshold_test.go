package mcp_faults

import (
	"math"
	"math/rand"
	"testing"
)

// isEven is the independent check for isOdd. The invariant is total and cheap:
// for every n the two must disagree.
func isEven(n int) bool { return n%2 == 0 }

// TestIsOddIsEvenInvariant is the quick check the example rests on. If this ever
// fails, the validator is not independent and no departure rate below means
// anything.
func TestIsOddIsEvenInvariant(t *testing.T) {
	for n := -1000; n <= 1000; n++ {
		if isOdd(n) == isEven(n) {
			t.Fatalf("isOdd and isEven agree at %d; the validator is not independent", n)
		}
	}
}

// biasedResolver answers isOdd, and is wrong with probability p. That covers both
// compromised shapes named: a weighted coin, and a node that randomly says notOdd.
type biasedResolver struct {
	p   float64
	rng *rand.Rand
}

func (b *biasedResolver) IsOdd(n int) bool {
	if b.rng.Float64() < b.p {
		return !isOdd(n)
	}
	return isOdd(n)
}

// binomCDF is P(X <= t) for X ~ Binomial(k, p).
func binomCDF(k int, p float64, t int) float64 {
	if t < 0 {
		return 0
	}
	if t >= k {
		return 1
	}
	if p <= 0 {
		return 1
	}
	if p >= 1 {
		return 0
	}

	sum := math.Pow(1-p, float64(k)) // P(X = 0)
	term := sum
	for i := 1; i <= t; i++ {
		term *= float64(k-i+1) / float64(i) * p / (1 - p)
		sum += term
	}
	return sum
}

// samplesForDetection returns the smallest number of readings k such that seeing
// more than `tolerated` wrong answers has probability >= confidence.
//
// tolerated = 0 means a single false reading is a departure; raising it buys
// tolerance for an honest node with a bug, at the cost of sample count.
func samplesForDetection(p float64, tolerated int, confidence float64) int {
	for k := 1; k <= 100000; k++ {
		if 1-binomCDF(k, p, tolerated) >= confidence {
			return k
		}
	}
	return -1
}

// TestFalseReadingBiasDetectionThreshold answers the question directly: given a
// per-reading false rate, how many readings before departure is detected at 99%.
// The headline is that the percentage hardly matters - the window does.
func TestFalseReadingBiasDetectionThreshold(t *testing.T) {
	const confidence = 0.99

	rates := []float64{0.001, 0.005, 0.01, 0.02, 0.05, 0.10, 0.25, 0.50}

	t.Logf("%-8s %-14s %-14s %-14s", "false %", "t=0 (readings)", "t=1", "t=2")
	for _, p := range rates {
		k0 := samplesForDetection(p, 0, confidence)
		k1 := samplesForDetection(p, 1, confidence)
		k2 := samplesForDetection(p, 2, confidence)
		t.Logf("%-8.1f%% %-14d %-14d %-14d", p*100, k0, k1, k2)

		if k0 <= 0 || k1 <= 0 || k2 <= 0 {
			t.Fatalf("no detection window found for p=%.3f", p)
		}
		if k0 > k1 || k1 > k2 {
			t.Errorf("tolerance must cost readings: p=%.3f gave %d/%d/%d", p, k0, k1, k2)
		}
	}

	// Monotonicity: a heavier bias is caught in fewer readings.
	prev := 0
	for i := len(rates) - 1; i >= 0; i-- {
		k := samplesForDetection(rates[i], 0, confidence)
		if prev != 0 && k < prev {
			t.Errorf("p=%.3f needs %d readings, more biased than the %d before it", rates[i], k, prev)
		}
		prev = k
	}

	// The two shapes named in the question: a coin weighted toward "not odd",
	// and a node that simply says notOdd. At 50% both are caught in a handful.
	if k := samplesForDetection(0.5, 0, confidence); k > 7 {
		t.Errorf("a fair coin should be caught in <=7 readings at 99%%, got %d", k)
	}
	// A 10% bias still costs only ~44 readings, because reads are cheap.
	if k := samplesForDetection(0.10, 0, confidence); k > 50 {
		t.Errorf("a 10%% bias should be caught in <=50 readings, got %d", k)
	}
}

// TestInverseQuestion is the reverse framing, which is usually the one that
// matters operationally: with a fixed window of readings, what bias is the
// smallest that can be detected at 99%?
func TestInverseQuestion(t *testing.T) {
	const confidence = 0.99

	for _, window := range []int{10, 100, 1000} {
		// Rise p from 0 until the window is enough.
		detectable := 0.0
		for p := 0.0001; p < 1; p += 0.0001 {
			if 1-binomCDF(window, p, 0) >= confidence {
				detectable = p
				break
			}
		}
		t.Logf("window of %4d readings detects bias >= %.2f%% at 99%%", window, detectable*100)
		if detectable <= 0 {
			t.Errorf("no detectable bias in a %d-reading window", window)
		}
	}

	// A window of 100 readings cannot see a bias smaller than ~4.5%.
	if p := 0.01; 1-binomCDF(100, p, 0) >= confidence {
		t.Error("a 1% bias should be invisible in 100 readings, so 4.5% is the floor")
	}
}

// TestBiasedResolverIsCaughtWithinBound checks the analytic bound against a
// simulated resolver, so the table above is not just arithmetic.
func TestBiasedResolverIsCaughtWithinBound(t *testing.T) {
	const (
		trials     = 2000
		confidence = 0.99
		tolerated  = 0
	)

	for _, p := range []float64{0.02, 0.10, 0.25, 0.50} {
		bound := samplesForDetection(p, tolerated, confidence)
		detections := make([]int, 0, trials)

		for trial := 0; trial < trials; trial++ {
			rng := rand.New(rand.NewSource(int64(1000 + trial)))
			compromised := &biasedResolver{p: p, rng: rng}

			detected := -1
			for n := 1; n <= bound; n++ {
				answer := compromised.IsOdd(n)
				if answer != isOdd(n) { // the independent check
					detected = n
					break
				}
			}
			if detected >= 0 {
				detections = append(detections, detected)
			}
		}

		caught := float64(len(detections)) / trials
		// 2000 trials put one standard error at ~0.2 points, so allow a small
		// band rather than re-deriving the exact tail on every run.
		if caught < confidence-0.015 {
			t.Errorf("p=%.2f: caught by reading %d in %.1f%% of trials, want >=%.0f%%",
				p, bound, caught*100, confidence*100)
		}
		if len(detections) == 0 {
			t.Fatalf("p=%.2f: no trial detected a bias at all", p)
		}
	}
}

// TestMutualVerificationSquaresFalsePositives is the alarm-a/alarm-b structure:
// two rotational peers verifying each other individually. Requiring both to read
// departure squares the false-positive rate, where either-of-them would roughly
// double it - the sample count grows linearly while the noise falls away.
func TestMutualVerificationSquaresFalsePositives(t *testing.T) {
	const (
		trials = 20000
		noise  = 0.05 // per-checker chance of a false departure reading
	)

	either, both := 0, 0
	for trial := 0; trial < trials; trial++ {
		rng := rand.New(rand.NewSource(int64(trial)))
		a := rng.Float64() < noise
		b := rng.Float64() < noise
		if a || b {
			either++
		}
		if a && b {
			both++
		}
	}

	eitherRate := float64(either) / trials
	bothRate := float64(both) / trials

	t.Logf("either checker flags: %.2f%%; both checkers flag: %.2f%%", eitherRate*100, bothRate*100)

	if bothRate >= eitherRate {
		t.Errorf("requiring agreement must be stricter: both %.4f vs either %.4f", bothRate, eitherRate)
	}
	// Both is ~noise^2 (0.25%), either is ~2*noise (10%): two orders apart.
	if bothRate > 3*noise*noise {
		t.Errorf("agreement rate %.4f is above the %0.4f a squared noise rate predicts", bothRate, noise*noise)
	}
	if eitherRate < noise {
		t.Errorf("either-of checkers should be at least one checker's noise: %.4f", eitherRate)
	}
}
