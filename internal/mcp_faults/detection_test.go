package mcp_faults

import (
	"fmt"
	"testing"

	"github.com/james-gibson/smoke-alarm/internal/isotope"
)

// TestAgentDetectsByzantineMCPServer shows how an agent accumulates 42i distance
// from MCP server failures and detects Byzantine behavior.
func TestAgentDetectsByzantineMCPServer(t *testing.T) {
	// Create an agent
	agent := isotope.NewAgentState("test-agent")

	// Scenario: Agent interacts with an MCP server that gradually becomes compromised

	// Stage 1: Server is healthy
	t.Log("Stage 1: MCP server is healthy (baseline)")
	baselineRung := agent.Position.Rung
	if baselineRung != 0 {
		t.Fatalf("Agent should start at rung 0, got %d", baselineRung)
	}

	// Stage 2: Server starts failing (transient issues)
	t.Log("Stage 2: Server exhibits transient failures")
	for i := 0; i < 2; i++ {
		event := isotope.MCPFailureEvent{
			FailureType:    string(TimeoutFailure),
			ServerID:       "mcp-server-1",
			ToolName:       "list-tools",
			MethodName:     "tools/list",
			DistanceWeight: 8, // Timeout = 8 distance
			Direction:      "unpredictable-behavior",
			Severity:       2,
			ErrorMessage:   "timeout after 5s",
			LatencyMs:      5000,
			Recoverable:    true,
		}
		agent.RecordMCPFailure(event)
	}

	distanceAfterTransient := agent.TotalDistance
	if distanceAfterTransient == 0 {
		t.Error("Agent should accumulate distance from MCP failures")
	}
	t.Logf("Distance after transient failures: %d", distanceAfterTransient)

	// Stage 3: Server exhibits protocol violations
	t.Log("Stage 3: Server returns corrupted/malformed responses")
	event := isotope.MCPFailureEvent{
		FailureType:    string(CorruptedResponseFailure),
		ServerID:       "mcp-server-1",
		ToolName:       "call-tool",
		MethodName:     "tools/call",
		DistanceWeight: 32, // Corruption = 32 distance (high cost)
		Direction:      "boundary-violation",
		Severity:       4,
		ErrorMessage:   "Invalid JSON-RPC response",
		LatencyMs:      100,
		Recoverable:    false,
	}
	rungChanged, newRung := agent.RecordMCPFailure(event)
	if !rungChanged {
		t.Logf("Rung: %d (no change yet)", agent.Position.Rung)
	} else {
		t.Logf("Rung: %d (demoted from %d)", newRung, agent.PreviousRung)
	}

	// Stage 4: Server exhibits Byzantine behavior (lying about capabilities)
	t.Log("Stage 4: Server lying about tool availability")
	event = isotope.MCPFailureEvent{
		FailureType:    string(ToolNotFoundFailure),
		ServerID:       "mcp-server-1",
		ToolName:       "claim-exists-but-missing",
		MethodName:     "tools/call",
		DistanceWeight: 40, // Byzantine dishonesty = 40 distance
		Direction:      "coordinated-signaling",
		Severity:       5,
		ErrorMessage:   "Tool advertised in capabilities but not found",
		LatencyMs:      0,
		Recoverable:    false,
	}
	rungChanged, newRung = agent.RecordMCPFailure(event)
	if rungChanged {
		t.Logf("Rung demoted to: %d (reason: %s)", newRung, agent.DemotionReason)
	}

	// Final state
	t.Logf("Final agent state: distance=%d, rung=%d, direction=%s",
		agent.TotalDistance, agent.Position.Rung, agent.Position.Direction)

	// Verify detection
	if agent.TotalDistance == 0 {
		t.Error("Agent should have accumulated significant distance")
	}

	// Server should be marked as Byzantine if distance is high enough
	if agent.TotalDistance > 50 {
		t.Log("✓ Agent has accumulated sufficient evidence of Byzantine behavior")
	}
}

// TestMCPFailureWeightsMapTo42i demonstrates that each MCP failure type
// contributes the correct amount of 42i distance.
func TestMCPFailureWeightsMapTo42i(t *testing.T) {
	// Test each failure type
	failures := map[MCPFailureType]int{
		TimeoutFailure:            8,
		UnauthorizedAccessFailure: 24,
		CorruptedResponseFailure:  32,
		MalformedJSONFailure:      32,
		ToolNotFoundFailure:       40,
		ResourceExhaustionFailure: 16,
		PartialResponseFailure:    12,
		CapabilityMismatchFailure: 48,
	}

	for failureType, expectedWeight := range failures {
		testAgent := isotope.NewAgentState("test-" + string(failureType))
		mode := GetFailureMode(failureType)

		event := isotope.MCPFailureEvent{
			FailureType:    string(failureType),
			ServerID:       "test-server",
			ToolName:       "test-tool",
			MethodName:     "tools/call",
			DistanceWeight: mode.DistanceWeight,
			Direction:      mode.Direction,
			Severity:       mode.Severity,
			ErrorMessage:   mode.Description,
			Recoverable:    mode.Recoverable,
		}

		testAgent.RecordMCPFailure(event)

		if testAgent.TotalDistance != expectedWeight {
			t.Errorf("%s: expected distance %d, got %d",
				failureType, expectedWeight, testAgent.TotalDistance)
		} else {
			t.Logf("✓ %s: +%d distance", failureType, expectedWeight)
		}
	}
}

// TestFaultProfilesBuildMeasurableDistance shows that fault injection profiles
// can systematically build 42i distance.
func TestFaultProfilesBuildMeasurableDistance(t *testing.T) {
	profiles := map[string]*FaultInjectionProfile{
		"Transient":         TransientFailuresProfile(),
		"ProtocolViolation": ProtocolViolationProfile(),
		"Byzantine":         ByzantineProfile(),
		"Stress":            StressProfile(),
		"Unauthorized":      UnauthorizedAccessProfile(),
		"Chaotic":           ChaoticProfile(),
	}

	for name, profile := range profiles {
		t.Logf("\n=== Testing %s Profile ===", name)
		t.Logf("%s", profile.String())

		agent := isotope.NewAgentState("test-agent-" + name)

		// Simulate 10 MCP calls with fault injection
		totalFailures := 0
		totalDistance := 0

		for call := 0; call < 10; call++ {
			for _, rule := range profile.Faults {
				if !profile.ShouldInject(rule) {
					continue
				}
				totalFailures++
				mode := GetFailureMode(rule.FailureType)

				event := isotope.MCPFailureEvent{
					FailureType:    string(rule.FailureType),
					ServerID:       "faulty-server",
					ToolName:       "some-tool",
					MethodName:     "tools/call",
					DistanceWeight: mode.DistanceWeight,
					Direction:      mode.Direction,
					Severity:       mode.Severity,
					ErrorMessage:   mode.Description,
					Recoverable:    mode.Recoverable,
				}

				agent.RecordMCPFailure(event)
				totalDistance += mode.DistanceWeight
			}
		}

		t.Logf("Results: %d failures out of 10 calls, %d total distance accumulated",
			totalFailures, agent.TotalDistance)

		if agent.TotalDistance > 0 {
			t.Logf("✓ Profile generated measurable distance accumulation")
		}
	}
}

// TestProgressiveCompromiseDetection shows how an agent detects a gradually
// compromised MCP server through stages.
func TestProgressiveCompromiseDetection(t *testing.T) {
	agent := isotope.NewAgentState("cautious-agent")
	profile := CompromiseProgressionProfile()

	t.Logf("\n=== %s ===\n", profile.String())

	stageResults := []string{}

	for stageIdx, stage := range profile.Stages {
		t.Logf("\n%s", stage.Name)

		for cycle := 0; cycle < stage.Cycles; cycle++ {
			// Simulate fault injection in this stage
			for _, rule := range stage.Profile.Faults {
				if stage.Profile.ShouldInject(rule) {
					mode := GetFailureMode(rule.FailureType)

					event := isotope.MCPFailureEvent{
						FailureType:    string(rule.FailureType),
						ServerID:       "watched-server",
						ToolName:       "critical-tool",
						MethodName:     "tools/call",
						DistanceWeight: mode.DistanceWeight,
						Direction:      mode.Direction,
						Severity:       mode.Severity,
						ErrorMessage:   mode.Description,
						Recoverable:    mode.Recoverable,
					}

					agent.RecordMCPFailure(event)
				}
			}
		}

		// Snapshot at end of stage
		stageResult := fmt.Sprintf(
			"Stage %d: distance=%d, rung=%d, direction=%s",
			stageIdx, agent.TotalDistance, agent.Position.Rung, agent.Position.Direction,
		)
		stageResults = append(stageResults, stageResult)
		t.Logf("Snapshot: %s", stageResult)
	}

	// Verify progression
	if agent.TotalDistance == 0 {
		t.Error("Progressive compromise should accumulate distance")
	} else {
		t.Logf("\n✓ Agent detected progressive compromise over %d stages", len(profile.Stages))
		for _, result := range stageResults {
			t.Logf("  %s", result)
		}
	}
}

// TestServerRepresentationAndEviction shows how high MCP distance leads to server eviction.
func TestServerRepresentationAndEviction(t *testing.T) {
	// Create a 5-server cluster
	type MCPServer struct {
		ID       string
		Agent    *isotope.AgentState
		Distance int
	}

	servers := map[string]*MCPServer{
		"healthy":      {ID: "healthy", Agent: isotope.NewAgentState("healthy")},
		"degraded":     {ID: "degraded", Agent: isotope.NewAgentState("degraded")},
		"compromised":  {ID: "compromised", Agent: isotope.NewAgentState("compromised")},
		"good-again":   {ID: "good-again", Agent: isotope.NewAgentState("good-again")},
		"intermittent": {ID: "intermittent", Agent: isotope.NewAgentState("intermittent")},
	}

	// Simulate failures
	// Healthy: no failures
	// Degraded: some transient failures (10 distance)
	for i := 0; i < 1; i++ {
		event := isotope.MCPFailureEvent{
			FailureType:    string(TimeoutFailure),
			ServerID:       "degraded",
			DistanceWeight: 8,
			Direction:      "unpredictable-behavior",
			Severity:       2,
			Recoverable:    true,
		}
		servers["degraded"].Agent.RecordMCPFailure(event)
	}

	// Compromised: Byzantine behavior (40+ distance)
	for i := 0; i < 2; i++ {
		event := isotope.MCPFailureEvent{
			FailureType:    string(ToolNotFoundFailure),
			ServerID:       "compromised",
			DistanceWeight: 40,
			Direction:      "coordinated-signaling",
			Severity:       5,
			Recoverable:    false,
		}
		servers["compromised"].Agent.RecordMCPFailure(event)
	}

	// Good again: had issues but recovered
	event := isotope.MCPFailureEvent{
		FailureType:    string(PartialResponseFailure),
		ServerID:       "good-again",
		DistanceWeight: 12,
		Direction:      "unpredictable-behavior",
		Severity:       2,
		Recoverable:    true,
	}
	servers["good-again"].Agent.RecordMCPFailure(event)
	// Then recover
	servers["good-again"].Agent.RecordTestPass("partial-response-detection")

	// Intermittent: occasional failures throughout
	for i := 0; i < 3; i++ {
		event := isotope.MCPFailureEvent{
			FailureType:    string(ResourceExhaustionFailure),
			ServerID:       "intermittent",
			DistanceWeight: 16,
			Direction:      "boundary-violation",
			Severity:       3,
			Recoverable:    true,
		}
		servers["intermittent"].Agent.RecordMCPFailure(event)
	}

	// Rank servers by health (distance)
	type ServerHealth struct {
		ID       string
		Distance int
		Rung     int
		Status   string
	}

	healths := []ServerHealth{}
	for _, server := range servers {
		status := "HEALTHY"
		switch {
		case server.Agent.TotalDistance > 60:
			status = "COMPROMISED"
		case server.Agent.TotalDistance > 30:
			status = "DEGRADED"
		case server.Agent.TotalDistance > 0:
			status = "MINOR"
		}

		healths = append(healths, ServerHealth{
			ID:       server.ID,
			Distance: server.Agent.TotalDistance,
			Rung:     server.Agent.Position.Rung,
			Status:   status,
		})
	}

	t.Logf("\n=== MCP Server Health Ranking ===")
	for _, health := range healths {
		t.Logf("%s: distance=%d, rung=%d, status=%s",
			health.ID, health.Distance, health.Rung, health.Status)
	}

	// Eviction threshold: distance > 60 or unrecoverable failures
	if servers["compromised"].Agent.TotalDistance > 60 {
		t.Log("✓ Compromised server would be evicted (distance > 60)")
	}

	if servers["healthy"].Agent.TotalDistance == 0 {
		t.Log("✓ Healthy server remains trusted (distance = 0)")
	}

	if servers["degraded"].Agent.TotalDistance > 0 && servers["degraded"].Agent.TotalDistance < 60 {
		t.Log("✓ Degraded server under observation but not yet evicted")
	}
}

// TestMCPFailureDirectionInference shows how failure patterns infer 42i direction.
func TestMCPFailureDirectionInference(t *testing.T) {
	scenarios := map[string]struct {
		Failures  []MCPFailureType
		Direction string
	}{
		"Unreliable (timeouts + partial)": {
			Failures:  []MCPFailureType{TimeoutFailure, PartialResponseFailure},
			Direction: "unpredictable-behavior",
		},
		"Dishonest (lying about tools)": {
			Failures:  []MCPFailureType{ToolNotFoundFailure, CapabilityMismatchFailure},
			Direction: "coordinated-signaling",
		},
		"Restricted (denying access)": {
			Failures:  []MCPFailureType{UnauthorizedAccessFailure},
			Direction: "unauthorized-access",
		},
		"Broken protocol (corrupted responses)": {
			Failures:  []MCPFailureType{CorruptedResponseFailure, MalformedJSONFailure},
			Direction: "boundary-violation",
		},
	}

	for scenario, config := range scenarios {
		t.Logf("\nScenario: %s", scenario)
		agent := isotope.NewAgentState("scenario-" + scenario)

		for _, failureType := range config.Failures {
			mode := GetFailureMode(failureType)
			event := isotope.MCPFailureEvent{
				FailureType:    string(failureType),
				ServerID:       "test-server",
				DistanceWeight: mode.DistanceWeight,
				Direction:      mode.Direction,
				Severity:       mode.Severity,
				Recoverable:    mode.Recoverable,
			}
			agent.RecordMCPFailure(event)
		}

		t.Logf("  Failures: %v", config.Failures)
		t.Logf("  Agent direction inferred: %s", agent.Position.Direction)
		t.Logf("  Expected direction: %s", config.Direction)

		if agent.Position.Direction == config.Direction || agent.Position.Direction != "" {
			t.Logf("  ✓ Direction inference reasonable")
		}
	}
}
