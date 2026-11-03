package mcp_faults

import (
	"fmt"
	"math"
	"testing"
	"time"
)

// "Turn left until the end of the maze" is the left-hand rule: a deterministic walk that
// needs no map, only a direction and a wall. Its whole advantage is that it is
// *systematic* rather than random - it converts an exponential search into a polynomial
// traversal.
//
// That advantage requires a gradient. Our names are drawn from a CSPRNG, so the space is
// isotropic: left and right are exchangeable, and there is nothing to follow. The
// consequence is a trade the attacker cannot win from either side:
//
//	with a gradient    polynomial search, but a visible bias  -> caught in ~20 steps
//	without a gradient exponential search, and no bias        -> never caught, never won
//
// So the question "how fast can we spot deviation" has a closed form: detection needs
// evidence, evidence accrues at the KL rate of the follower's bias, and latency is that
// step count divided by the arrival rate - which the per-claimant ceiling caps. Load buys
// latency, never evidence.

const (
	// Sequential test confidence: one false accusation in a thousand claimants.
	detectionAlpha = 1e-3
	// A left-hand rule with an occasional opening: nine turns in ten go the same way.
	wallsFollowed = 0.9
)

// klBern is D(p || 1/2) in nats: the evidence one probe carries about a follower whose
// side choice is biased to p.
func klBern(p float64) float64 {
	if p <= 0 || p >= 1 {
		return math.Inf(1)
	}
	return p*math.Log(2*p) + (1-p)*math.Log(2*(1-p))
}

// stepsToDetect is the SPRT step count for a bias p at confidence alpha.
//
// Evidence accumulates linearly in the number of probes and in the bias's KL divergence,
// so a crude follower is caught almost immediately and a subtle one costs samples.
func stepsToDetect(p, alpha float64) float64 {
	d := klBern(p)
	if d <= 0 {
		return math.Inf(1)
	}
	return math.Log(1/alpha) / d
}

// latencyAt is how long detection takes when probes arrive at the given rate.
func latencyAt(p, alpha, rate float64) time.Duration {
	steps := stepsToDetect(p, alpha)
	if math.IsInf(steps, 1) || rate <= 0 {
		return 0
	}
	return time.Duration(steps / rate * float64(time.Second))
}

// TestWallFollowerIsCaughtByItsOwnBias is the first half: a deterministic walk leaks a
// bias, and a bias is almost free to detect.
func TestWallFollowerIsCaughtByItsOwnBias(t *testing.T) {
	t.Logf("%-8s %-10s %-8s %-12s", "bias", "nats/step", "steps", "at 40/s")

	for _, p := range []float64{0.99, 0.9, 0.75, 0.6} {
		steps := stepsToDetect(p, detectionAlpha)
		t.Logf("%-8.2f %-10.4f %-8.0f %-12s", p, klBern(p), steps, latencyAt(p, detectionAlpha, alarmsPerClaimant))

		if p >= wallsFollowed && steps > 25 {
			t.Errorf("a follower at p=%.2f should be caught in under 25 steps, took %.0f", p, steps)
		}
	}

	// The headline: a crude wall-follower is caught in well under a second at the ceiling.
	crude := latencyAt(wallsFollowed, detectionAlpha, alarmsPerClaimant)
	if crude > time.Second {
		t.Errorf("a 9-in-10 follower should be caught within a second at the ceiling, took %s", crude)
	}
	t.Logf("crude follower caught in %.0f steps, i.e. %s at the %0.f/s ceiling",
		stepsToDetect(wallsFollowed, detectionAlpha), crude, alarmsPerClaimant)
}

// TestDetectionLatencyIsStepsOverRate is the "under what load" answer, and it runs the
// way an operator might not expect: more load means faster detection in wall-clock time.
func TestDetectionLatencyIsStepsOverRate(t *testing.T) {
	const p = wallsFollowed
	steps := stepsToDetect(p, detectionAlpha)

	t.Logf("%-10s %-12s", "rate", "latency")

	var previous time.Duration = 1 << 62
	for _, rate := range []float64{1, 5, 10, 40, 400} {
		latency := latencyAt(p, detectionAlpha, rate)
		t.Logf("%-10.0f %-12s", rate, latency)

		if latency > previous {
			t.Errorf("latency must fall as the rate rises: %s at %.0f/s after %s", latency, rate, previous)
		}
		previous = latency
	}

	// The floor is the attacker's own ceiling: they cannot send faster than
	// alarmsPerClaimant, so detection can never be slower than steps/ceiling.
	floor := latencyAt(p, detectionAlpha, alarmsPerClaimant)
	if floor <= 0 {
		t.Fatal("the ceiling must produce a finite detection floor")
	}
	t.Logf("floor at the ceiling: %s for %.0f steps (%.1f nats of evidence)",
		floor, steps, steps*klBern(p))

	// And the corollary that makes the ceiling a security parameter rather than a
	// throughput limit: the only way to slow detection is to send more slowly, which is
	// the same as not running the maze.
	slowed := latencyAt(p, detectionAlpha, 1)
	if slowed <= floor {
		t.Error("sending more slowly must delay detection, or there is no dilemma")
	}
	t.Logf("slowing to 1/s delays detection to %s, at which rate the maze takes 2^32 s", slowed)
}

// TestSubtletyCostsQuadratically is the "early vs later" answer: a follower that hides
// its bias by turning almost evenly pays for it quadratically in samples.
func TestSubtletyCostsQuadratically(t *testing.T) {
	const base = 0.01 // bias above an even coin

	t.Logf("%-8s %-10s %-10s %-14s", "bias", "nats/step", "steps", "at 40/s")

	var stepsAgain []float64
	for _, eps := range []float64{base, base / 2, base / 4, base / 8} {
		p := 0.5 + eps
		steps := stepsToDetect(p, detectionAlpha)
		stepsAgain = append(stepsAgain, steps)
		t.Logf("%-8.3f %-10.6f %-10.0f %-14s",
			p, klBern(p), steps, latencyAt(p, detectionAlpha, alarmsPerClaimant))
	}

	// Halving the bias must roughly quadruple the samples: D ~ 2*eps^2 near an even coin.
	for i := 1; i < len(stepsAgain); i++ {
		ratio := stepsAgain[i] / stepsAgain[i-1]
		if ratio < 3 || ratio > 5 {
			t.Errorf("halving the bias should cost ~4x the samples, got %.2fx", ratio)
		}
	}

	// The early/late split, stated plainly: early detection is cheap and catches the
	// crude follower; later detection is expensive and catches the patient one.
	early := latencyAt(0.9, detectionAlpha, alarmsPerClaimant)
	late := latencyAt(0.5+base/8, detectionAlpha, alarmsPerClaimant)
	t.Logf("early (bias 0.40): %s; late (bias 0.005): %s; ratio %.0fx", early, late, late.Seconds()/early.Seconds())
}

// TestRandomizedFollowingIsJustAGuess is the other branch, and the reason isotropy is the
// property that matters: if the follower randomizes its side choice to defeat the bias
// test, it stops following - and its search reverts to the full space.
func TestRandomizedFollowingIsJustAGuess(t *testing.T) {
	// A randomized follower has zero drift, so no number of samples detects it.
	if d := klBern(0.5); d != 0 {
		t.Errorf("an even coin must carry no evidence, got %f nats", d)
	}
	if steps := stepsToDetect(0.5, detectionAlpha); !math.IsInf(steps, 1) {
		t.Errorf("an even coin must be undetectable by bias, got %.0f steps", steps)
	}

	// What is left is success, and the space is 32 bits wide.
	const spaceBits = 32
	expectedSteps := math.Pow(2, spaceBits)

	// At the ceiling, that is years. So time is not the control; price is.
	years := expectedSteps / alarmsPerClaimant / 60 / 60 / 24 / 365
	t.Logf("randomized follower: expected %.3g steps, %.3g years at %0.f/s",
		expectedSteps, years, alarmsPerClaimant)

	if years < 1 {
		t.Errorf("a random walk over 32 bits must not be reachable in a year, got %.3g", years)
	}

	// The control that still holds: every wrong guess is priced, so the attacker's
	// spend is bounded even though their time is not.
	price := 8.0 // fuzz tier per wrong guess
	evictions := evictionThreshold / price
	t.Logf("every attempt costs %.0f, so an identity survives %.0f guesses", price, evictions)
	if evictions*price < evictionThreshold {
		t.Error("the price must reach the eviction threshold, or guesses are free")
	}
}

// TestTheNameSpaceHasNoGradient is the isotropy check on the real proxy: turning left and
// turning right must cost exactly the same, or the maze has a wall to follow.
func TestTheNameSpaceHasNoGradient(t *testing.T) {
	proxy := hashedProxy(t, "list-tools")

	type side struct {
		attempts int
		accepted int
		distance float64
	}

	// A wall follower commits to one side and probes there repeatedly. Each side gets the
	// same number of attempts, and the price is measured as a delta so the comparison is
	// not contaminated by the other side's traffic.
	probe := func(prefixSeed int) side {
		var s side
		before := proxy.GetInvalidCallMetrics().Distance42i
		for i := 0; i < 256; i++ {
			forged := fmt.Sprintf("%08x_%s", i*prefixSeed+1, proxy.ToolName)
			ok, _ := proxy.ValidateCall(forged)
			s.attempts++
			if ok {
				s.accepted++
			}
		}
		s.distance = float64(proxy.GetInvalidCallMetrics().Distance42i - before)
		return s
	}

	left := probe(7919)
	right := probe(104729)

	for name, s := range map[string]side{"left": left, "right": right} {
		t.Logf("%-6s attempts=%d accepted=%d distance/attempt=%.1f",
			name, s.attempts, s.accepted, s.distance/float64(s.attempts))
	}

	// Neither side is cheaper: the price is a property of being wrong, not of which way
	// the attacker turned, so there is no direction to prefer.
	if left.accepted != right.accepted {
		t.Errorf("the two sides must accept equally: %d vs %d", left.accepted, right.accepted)
	}
	if left.distance != right.distance {
		t.Errorf("the two sides must cost equally: %.0f vs %.0f", left.distance, right.distance)
	}
	if left.distance <= 0 {
		t.Error("a wrong guess must be priced, or turning is free")
	}

	// And acceptance depends only on the value, never on the direction: the issued stamp
	// resolves regardless of which side it was derived from.
	if ok, _ := proxy.ValidateCall(proxy.PrefixedToolName()); !ok {
		t.Fatal("the issued stamp must resolve, or the fixture is broken")
	}
	t.Log("no gradient: equality is the only acceptance rule, and both sides cost the same")
}

// TestTheCheapOneSidedTestDoesNotApply reconciles this with the exact one-sided test in
// detection_threshold_test.go, because the obvious objection is that bias detection is
// already solved there in a handful of readings.
//
// It is not the same question. samplesForDetection counts *errors per reading*: a node
// whose answer is wrong 10% of the time leaks one bit on every wrong reading. A
// wall-follower leaks nothing per probe - every probe is refused, which is the expected
// answer - so the only observable is the aggregate of its side choices, and that is a
// two-sided test against a fair coin.
func TestTheCheapOneSidedTestDoesNotApply(t *testing.T) {
	const confidence = 1 - detectionAlpha

	// The one-sided case: 10% of readings are wrong, and a single wrong reading is a
	// signal, so the count is driven by the error rate alone.
	oneSided := samplesForDetection(0.10, 0, confidence)

	// The wall-follower case: a 60/40 side bias, with no wrong reading anywhere.
	twoSided := stepsToDetect(0.60, detectionAlpha)

	t.Logf("one-sided (10%% wrong readings): %d readings", oneSided)
	t.Logf("two-sided (60/40 side bias):    %.0f probes", twoSided)
	t.Logf("inflation: %.1fx, because a refused probe is not an error", twoSided/float64(oneSided))

	if oneSided <= 0 {
		t.Fatal("the one-sided bound must exist, or the comparison is meaningless")
	}
	if twoSided <= float64(oneSided) {
		t.Errorf("the two-sided test must cost more: %.0f vs %d", twoSided, oneSided)
	}

	// The reason, stated as the invariant: a follower's individual probe is
	// indistinguishable from an honest miss, so no single probe can be counted as an
	// error. Only the aggregate is evidence.
	proxy := hashedProxy(t, "list-tools")
	ok, reason := proxy.ValidateCall("00000000_" + proxy.ToolName)
	if ok || reason == "" {
		t.Fatalf("a follower's probe must be refused with a reason, got (%v, %q)", ok, reason)
	}
	t.Logf("a single probe is refused like any other miss (%q); only the pattern of sides is evidence", reason)
}
