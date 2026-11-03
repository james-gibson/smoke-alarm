package mcp_faults

import (
	"math"
	"testing"
	"time"
)

// An edge handler is an observation member, not a storage member. It sits in the read path, so
// it sees every local call and it can report evidence the cluster cannot otherwise collect:
// side choices, residues, canary fills, staleness, refusal patterns.
//
// Three properties decide whether that is worth building:
//
//	amplification - reads are ~1 us, writes are capped at 40/s, so observation bandwidth is
//	                four orders of magnitude larger than the write path
//	aggregation   - an attacker spreading across edges dilutes per-edge evidence, and only a
//	                read aggregator makes detection latency invariant to that spread
//	redundancy    - one observer blinds a route when compromised, so observation needs the same
//	                two-replica knee as service

const (
	// Local validation cost: one HMAC plus a residue check.
	observeCost = 1 * time.Microsecond
	// Local reads per core per second, before the observation itself becomes the load.
	observePerCore = 1_000_000
	// The per-claimant write/alarm ceiling. Reads are not capped by it.
	writeCeiling = 40.0
)

// TestReadAmplificationIsFourOrdersOfMagnitude is why observation is the cheap side of the
// membership: the ceiling that bounds writes does not bound local reads.
func TestReadAmplificationIsFourOrdersOfMagnitude(t *testing.T) {
	readRate := float64(observePerCore)
	ratio := readRate / writeCeiling

	t.Logf("local reads: %.0f/s per core; claimant writes: %.0f/s", readRate, writeCeiling)
	t.Logf("observation bandwidth exceeds the write ceiling by %.0fx", ratio)

	if ratio < 10_000 {
		t.Errorf("observation should dwarf the write path, got %.0fx", ratio)
	}
	t.Log("an edge is sized for observations per second, not for storage")
}

// TestSpreadDilutesEvidenceWithoutAggregation is the read-side attack: the same claimant
// spreading its probes across edges keeps its total rate at the ceiling but gives each edge
// fewer samples.
func TestSpreadDilutesEvidenceWithoutAggregation(t *testing.T) {
	// The bias test needs 19 samples at a 0.9 side bias ([ADR 0006]).
	const samplesNeeded = 19.0

	t.Logf("%-8s %-12s %-16s %-16s", "edges", "per edge", "no aggregator", "with aggregator")

	for _, edges := range []int{1, 4, 10, 100} {
		perEdge := writeCeiling / float64(edges)
		without := time.Duration(samplesNeeded / perEdge * float64(time.Second))
		// With an aggregator every edge ships evidence, so the sample count is pooled and the
		// latency is unchanged by the spread.
		with := time.Duration(samplesNeeded / writeCeiling * float64(time.Second))

		t.Logf("%-8d %-12.2f %-16s %-16s", edges, perEdge, without, with)

		if with != time.Duration(samplesNeeded/writeCeiling*float64(time.Second)) {
			t.Errorf("aggregated latency must not depend on spread, got %s", with)
		}
		if edges > 1 && without <= with {
			t.Errorf("%d edges must dilute per-edge evidence without an aggregator", edges)
		}
	}

	// The rule, quantified: spreading buys the attacker a linear factor in detection latency,
	// and an aggregator removes it entirely.
	t.Logf("spread across 100 edges: %s without aggregation, %s with it",
		time.Duration(samplesNeeded/(writeCeiling/100)*float64(time.Second)),
		time.Duration(samplesNeeded/writeCeiling*float64(time.Second)))
	t.Log("edge handlers exist to feed the aggregator; without it, they are 100 separate blind spots")
}

// TestSamplingMultipliesDetectionLatency is the fidelity argument. An observer in the read path
// can count every event; a periodic scraper samples, and sampling costs proportionally.
func TestSamplingMultipliesDetectionLatency(t *testing.T) {
	const samplesNeeded = 19.0

	t.Logf("%-10s %-14s %-14s", "sample rate", "events seen", "latency")

	for _, interval := range []int{1, 10, 100} {
		// One sample every `interval` events, at the ceiling rate.
		eventsPerSample := float64(interval)
		latency := time.Duration(samplesNeeded * eventsPerSample / writeCeiling * float64(time.Second))

		t.Logf("1 in %-6d %-14.0f %-14s", interval, samplesNeeded*eventsPerSample, latency)

		if interval > 1 && latency < time.Duration(samplesNeeded/writeCeiling*float64(time.Second))*time.Duration(interval) {
			t.Errorf("sampling every %d events must cost %dx the latency", interval, interval)
		}
	}

	// Complete observation is free here because the observer is already in the path: it does
	// not add a probe, it counts the probes that were going to happen.
	t.Logf("in-path observation sees 100%% of events at %.0f ns each", observeCost.Seconds()*1e9)
	t.Log("sampling is not cheaper, it is just less accurate: the event already happened")
}

// TestOneObserverIsABlindSpot is the redundancy argument, using the same knee as service.
func TestOneObserverIsABlindSpot(t *testing.T) {
	const perObserverCompromise = 0.01

	t.Logf("%-10s %-14s %-14s", "observers", "route blind if", "exposure")

	for k := 1; k <= 3; k++ {
		// A route is blind only if every observer of it is compromised.
		blind := math.Pow(perObserverCompromise, float64(k))
		t.Logf("%-10d %-14s %-14.4f%%", k, "all compromised", 100*blind)

		if k == 1 && blind != perObserverCompromise {
			t.Errorf("one observer must be blinded exactly when that observer is, got %.4f", blind)
		}
		if k > 1 && blind >= perObserverCompromise {
			t.Errorf("%d observers should be harder to blind than one", k)
		}
	}

	// Two is the knee, as with replicas: 1% becomes 0.01%, and a third buys two more nines for
	// another full observer.
	t.Log("observation needs two independent observers per route, not one")
}

// TestBlindnessIsItselfAnObservation is the failure mode that has no write-side analog: an
// edge that stops reporting is not silent, it is a *missing observation*, and that is evidence.
func TestBlindnessIsItselfAnObservation(t *testing.T) {
	type observer struct {
		name    string
		reports int // 0 means silent
	}

	observers := []observer{
		{"edge-a", 120},
		{"edge-b", 0}, // partitioned or compromised
		{"edge-c", 118},
	}

	silent := 0
	for _, o := range observers {
		if o.reports == 0 {
			silent++
			t.Logf("%s: silent -> this is an observation gap, not a quiet route", o.name)
		}
	}

	if silent == 0 {
		t.Fatal("fixture: one observer must be silent")
	}

	// The verdict for a blind route is `unexamined`, not `trusted` ([ADR 0005]).
	if Classify(false, false) != VerdictUnexamined {
		t.Error("a route with no observations is unexamined, and unexamined is not innocent")
	}
	t.Log("silence is reported as unexamined; the cluster must never read it as healthy")
}
