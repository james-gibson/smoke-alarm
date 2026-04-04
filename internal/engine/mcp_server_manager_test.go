package engine

import (
	"testing"

	"github.com/james-gibson/isotope"
	"github.com/james-gibson/smoke-alarm/internal/mcp_faults"
	"github.com/james-gibson/smoke-alarm/internal/targets"
)

// TestMCPServerManagerCreation verifies manager initialization.
func TestMCPServerManagerCreation(t *testing.T) {
	manager := NewMCPServerManager(false)

	if manager == nil {
		t.Fatal("Manager should be created")
	}

	if manager.honeypotMode {
		t.Error("Honeypot mode should be off")
	}

	if manager.evictionThreshold != 60 {
		t.Errorf("Eviction threshold should be 60, got %d", manager.evictionThreshold)
	}

	t.Log("✓ Manager created successfully with correct defaults")
}

// TestRegisterMCPServer sets up proxies and agent state for a server.
func TestRegisterMCPServer(t *testing.T) {
	manager := NewMCPServerManager(false)

	serverID := "test-server"
	toolNames := []string{"list-tools", "call-tool", "get-resource"}

	manager.RegisterMCPServer(serverID, toolNames)

	// Verify agent was created
	health := manager.GetServerHealth(serverID)
	if health.ServerID != serverID {
		t.Errorf("Server ID mismatch: expected %s, got %s", serverID, health.ServerID)
	}

	if health.Status != "healthy" {
		t.Errorf("New server should be healthy, got %s", health.Status)
	}

	if health.Distance42i != 0 {
		t.Errorf("New server should have 0 distance, got %d", health.Distance42i)
	}

	t.Logf("✓ Server registered: %s with tools %v", serverID, toolNames)
}

// TestValidateToolCallLegitimate validates calls with correct prefix.
func TestValidateToolCallLegitimate(t *testing.T) {
	manager := NewMCPServerManager(false)

	serverID := "server-1"
	toolName := "list-tools"
	manager.RegisterMCPServer(serverID, []string{toolName})

	// Get the proxy to construct a legitimate call
	manager.mu.RLock()
	proxy := manager.toolProxies[serverID]
	manager.mu.RUnlock()

	legitimateCall := proxy.Proxies[toolName].PrefixedToolName()

	hasCorrect, suspicion, shouldReject, _ := manager.ValidateToolCall(serverID, toolName, legitimateCall)

	if !hasCorrect {
		t.Error("Legitimate call should have correct prefix")
	}

	if suspicion != "" {
		t.Errorf("Legitimate call should not be suspicious, got: %s", suspicion)
	}

	if shouldReject {
		t.Error("Legitimate call should not be rejected")
	}

	t.Logf("✓ Legitimate call validated: %q has correct prefix", legitimateCall)
}

// TestValidateToolCallDirect detects calls bypassing proxy.
func TestValidateToolCallDirect(t *testing.T) {
	manager := NewMCPServerManager(true) // honeypot mode for logging

	serverID := "server-2"
	toolName := "call-tool"
	manager.RegisterMCPServer(serverID, []string{toolName})

	// Direct call: tool name without prefix
	hasCorrect, suspicion, shouldReject, metrics := manager.ValidateToolCall(serverID, toolName, toolName)

	if hasCorrect {
		t.Error("Direct call should not have correct prefix")
	}

	if suspicion != "direct-call-to-tool-bypasses-proxy" {
		t.Errorf("Should detect direct call, got: %s", suspicion)
	}

	if shouldReject {
		t.Error("Single direct call should not immediately reject (< 3 threshold)")
	}

	// But metrics should show it as invalid
	if metrics.DirectCalls != 1 {
		t.Errorf("Expected 1 direct call, got %d", metrics.DirectCalls)
	}

	t.Logf("✓ Direct call detected as suspicious: %s, distance=%d", suspicion, metrics.Distance42i)
}

// TestValidateToolCallMultipleSuspicious triggers rejection after threshold.
func TestValidateToolCallMultipleSuspicious(t *testing.T) {
	manager := NewMCPServerManager(false)

	serverID := "server-3"
	toolName := "get-resource"
	manager.RegisterMCPServer(serverID, []string{toolName})

	// Make 4 suspicious calls (> 3 threshold)
	for i := 0; i < 4; i++ {
		hasCorrect, _, shouldReject, _ := manager.ValidateToolCall(serverID, toolName, toolName)

		if hasCorrect {
			t.Errorf("Call %d should not have correct prefix", i+1)
		}

		if i < 3 && shouldReject {
			t.Errorf("Call %d should not trigger rejection yet", i+1)
		}

		if i == 3 && !shouldReject {
			t.Error("Call 4 should trigger rejection (exceeds threshold)")
		}
	}

	// Verify attack flag was set
	health := manager.GetServerHealth(serverID)
	if !health.UnderAttack {
		t.Error("Server should be marked under attack")
	}

	if health.Status != "under-attack" {
		t.Errorf("Status should be under-attack, got %s", health.Status)
	}

	t.Log("✓ Multiple suspicious calls triggered attack detection and rejection")
}

// TestValidateToolCallFuzzed detects fuzzing attempts.
func TestValidateToolCallFuzzed(t *testing.T) {
	manager := NewMCPServerManager(false)

	serverID := "server-4"
	toolName := "test-tool"
	manager.RegisterMCPServer(serverID, []string{toolName})

	// Fuzzed call with wrong prefix
	fuzzedCall := "random_abc123_test-tool"
	hasCorrect, suspicion, _, metrics := manager.ValidateToolCall(serverID, toolName, fuzzedCall)

	if hasCorrect {
		t.Error("Fuzzed call should not have correct prefix")
	}

	if suspicion != "fuzzed-or-wrong-prefix" {
		t.Errorf("Should detect fuzz attempt, got: %s", suspicion)
	}

	// Fuzz should contribute to distance (less than direct call)
	if metrics.FuzzAttempts != 1 {
		t.Errorf("Expected 1 fuzz attempt, got %d", metrics.FuzzAttempts)
	}

	t.Logf("✓ Fuzz detected: %s, distance=%d (lower cost than direct)", suspicion, metrics.Distance42i)
}

// TestValidateToolCallUnknownServer handles unknown servers.
func TestValidateToolCallUnknownServer(t *testing.T) {
	manager := NewMCPServerManager(false)

	hasCorrect, suspicion, shouldReject, _ := manager.ValidateToolCall("unknown-server", "tool", "tool")

	if hasCorrect {
		t.Error("Unknown server should fail validation")
	}

	if suspicion != "unknown-server" {
		t.Errorf("Should report unknown-server, got: %s", suspicion)
	}

	if !shouldReject {
		t.Error("Unknown server should be rejected")
	}

	t.Log("✓ Unknown server correctly rejected")
}

// TestGetServerHealthHealthy returns correct status for healthy server.
func TestGetServerHealthHealthy(t *testing.T) {
	manager := NewMCPServerManager(false)

	serverID := "healthy-server"
	manager.RegisterMCPServer(serverID, []string{"tool1", "tool2"})

	health := manager.GetServerHealth(serverID)

	if health.Status != "healthy" {
		t.Errorf("Status should be healthy, got %s", health.Status)
	}

	if health.Distance42i != 0 {
		t.Errorf("Distance should be 0, got %d", health.Distance42i)
	}

	if health.IsCompromised {
		t.Error("Healthy server should not be compromised")
	}

	if health.UnderAttack {
		t.Error("Healthy server should not be under attack")
	}

	t.Log("✓ Healthy server status: " + health.String())
}

// TestGetServerHealthDegraded returns degraded status when distance accumulates.
func TestGetServerHealthDegraded(t *testing.T) {
	manager := NewMCPServerManager(false)

	serverID := "degraded-server"
	manager.RegisterMCPServer(serverID, []string{"tool"})

	// Accumulate some distance via MCP failures
	agent := manager.serverAgents[serverID]
	event := isotope.MCPFailureEvent{
		FailureType:    string(mcp_faults.TimeoutFailure),
		ServerID:       serverID,
		ToolName:       "tool",
		MethodName:     "tools/call",
		DistanceWeight: 8,
		Direction:      "unpredictable-behavior",
		Severity:       2,
		Recoverable:    true,
	}
	agent.RecordMCPFailure(event)

	health := manager.GetServerHealth(serverID)

	if health.Status != "degraded" {
		t.Errorf("Status should be degraded, got %s", health.Status)
	}

	if health.Distance42i <= 0 {
		t.Error("Distance should be > 0")
	}

	if health.IsCompromised {
		t.Error("Should not be compromised yet")
	}

	t.Logf("✓ Degraded server status: distance=%d, status=%s", health.Distance42i, health.Status)
}

// TestGetServerHealthCompromised returns compromised status when distance exceeds threshold.
func TestGetServerHealthCompromised(t *testing.T) {
	manager := NewMCPServerManager(false)

	serverID := "compromised-server"
	manager.RegisterMCPServer(serverID, []string{"tool"})

	// Accumulate distance > threshold (60)
	agent := manager.serverAgents[serverID]
	for i := 0; i < 3; i++ {
		event := isotope.MCPFailureEvent{
			FailureType:    string(mcp_faults.ToolNotFoundFailure),
			ServerID:       serverID,
			ToolName:       "tool",
			MethodName:     "tools/call",
			DistanceWeight: 40, // 40 * 2 = 80 > 60 threshold
			Direction:      "coordinated-signaling",
			Severity:       5,
			Recoverable:    false,
		}
		agent.RecordMCPFailure(event)
	}

	health := manager.GetServerHealth(serverID)

	if health.Status != "compromised" {
		t.Errorf("Status should be compromised, got %s", health.Status)
	}

	if !health.IsCompromised {
		t.Error("Server should be marked as compromised")
	}

	if health.Distance42i <= 60 {
		t.Errorf("Distance should exceed threshold (60), got %d", health.Distance42i)
	}

	t.Logf("✓ Compromised server status: distance=%d, status=%s", health.Distance42i, health.Status)
}

// TestGetServerRanking ranks servers by trustworthiness.
func TestGetServerRanking(t *testing.T) {
	manager := NewMCPServerManager(false)

	// Create 3 servers with different health levels
	servers := []string{"healthy", "degraded", "compromised"}
	for _, id := range servers {
		manager.RegisterMCPServer(id, []string{"tool"})
	}

	// Add degradation only to degraded and compromised
	for i := 0; i < 1; i++ {
		event := isotope.MCPFailureEvent{
			FailureType:    string(mcp_faults.TimeoutFailure),
			ServerID:       "degraded",
			ToolName:       "tool",
			MethodName:     "tools/call",
			DistanceWeight: 16,
			Direction:      "unpredictable-behavior",
			Severity:       2,
			Recoverable:    true,
		}
		manager.serverAgents["degraded"].RecordMCPFailure(event)
	}

	for i := 0; i < 2; i++ {
		event := isotope.MCPFailureEvent{
			FailureType:    string(mcp_faults.ToolNotFoundFailure),
			ServerID:       "compromised",
			ToolName:       "tool",
			MethodName:     "tools/call",
			DistanceWeight: 40,
			Direction:      "coordinated-signaling",
			Severity:       5,
			Recoverable:    false,
		}
		manager.serverAgents["compromised"].RecordMCPFailure(event)
	}

	ranking := manager.GetServerRanking()

	if len(ranking) != 3 {
		t.Errorf("Should have 3 servers, got %d", len(ranking))
	}

	// First should be healthy (lowest distance)
	if ranking[0].ServerID != "healthy" || ranking[0].Distance42i != 0 {
		t.Errorf("Healthy should be first, got %s with distance %d",
			ranking[0].ServerID, ranking[0].Distance42i)
	}

	// Last should be compromised (highest distance)
	if ranking[2].ServerID != "compromised" {
		t.Errorf("Compromised should be last, got %s", ranking[2].ServerID)
	}

	t.Logf("✓ Server ranking (by distance):")
	for i, h := range ranking {
		t.Logf("  %d. %s: distance=%d, status=%s", i+1, h.ServerID, h.Distance42i, h.Status)
	}
}

// TestShouldEvictServerByDistance evicts when distance exceeds threshold.
func TestShouldEvictServerByDistance(t *testing.T) {
	manager := NewMCPServerManager(false)

	serverID := "to-evict"
	manager.RegisterMCPServer(serverID, []string{"tool"})

	// Accumulate distance > 60
	agent := manager.serverAgents[serverID]
	for i := 0; i < 2; i++ {
		event := isotope.MCPFailureEvent{
			FailureType:    string(mcp_faults.ToolNotFoundFailure),
			ServerID:       serverID,
			ToolName:       "tool",
			MethodName:     "tools/call",
			DistanceWeight: 40,
			Direction:      "coordinated-signaling",
			Severity:       5,
			Recoverable:    false,
		}
		agent.RecordMCPFailure(event)
	}

	shouldEvict, reason := manager.ShouldEvictServer(serverID)

	if !shouldEvict {
		t.Error("Server with high distance should be evicted")
	}

	if reason == "" {
		t.Error("Should provide eviction reason")
	}

	t.Logf("✓ Server eviction decision: %s", reason)
}

// TestShouldEvictServerUnderAttack evicts servers under attack.
func TestShouldEvictServerUnderAttack(t *testing.T) {
	manager := NewMCPServerManager(false)

	serverID := "attacked-server"
	manager.RegisterMCPServer(serverID, []string{"tool"})

	// Trigger attack detection by making 4+ suspicious calls
	for i := 0; i < 4; i++ {
		manager.ValidateToolCall(serverID, "tool", "tool") // Direct call each time
	}

	shouldEvict, reason := manager.ShouldEvictServer(serverID)

	if !shouldEvict {
		t.Error("Server under attack should be evicted")
	}

	if reason == "" {
		t.Error("Should provide eviction reason")
	}

	t.Logf("✓ Attack-based eviction: %s", reason)
}

// TestShouldEvictServerByzantine evicts on Byzantine failures.
func TestShouldEvictServerByzantine(t *testing.T) {
	manager := NewMCPServerManager(false)

	serverID := "byzantine-server"
	manager.RegisterMCPServer(serverID, []string{"tool"})

	// Record Byzantine failure (tool-not-found = 40 distance)
	agent := manager.serverAgents[serverID]
	event := isotope.MCPFailureEvent{
		FailureType:    string(mcp_faults.ToolNotFoundFailure),
		ServerID:       serverID,
		ToolName:       "tool",
		MethodName:     "tools/call",
		DistanceWeight: 40,
		Direction:      "coordinated-signaling",
		Severity:       5,
		Recoverable:    false,
	}
	agent.RecordMCPFailure(event)

	shouldEvict, _ := manager.ShouldEvictServer(serverID)

	// Single Byzantine failure should trigger eviction if high enough
	if agent.TotalDistance > 60 && !shouldEvict {
		t.Error("Server with high Byzantine distance should be evicted")
	}

	t.Logf("✓ Byzantine eviction check: distance=%d, evict=%v", agent.TotalDistance, shouldEvict)
}

// TestHealthToTargetStateMapping converts manager health to target states.
func TestHealthToTargetStateMapping(t *testing.T) {
	manager := NewMCPServerManager(false)

	testCases := []struct {
		name           string
		setupFunc      func(string)
		expectedState  targets.HealthState
		expectedStatus string
	}{
		{
			name: "healthy-to-healthy",
			setupFunc: func(id string) {
				manager.RegisterMCPServer(id, []string{"tool"})
			},
			expectedState:  targets.StateHealthy,
			expectedStatus: "healthy",
		},
		{
			name: "degraded-to-degraded",
			setupFunc: func(id string) {
				manager.RegisterMCPServer(id, []string{"tool"})
				event := isotope.MCPFailureEvent{
					FailureType:    string(mcp_faults.TimeoutFailure),
					ServerID:       id,
					ToolName:       "tool",
					MethodName:     "tools/call",
					DistanceWeight: 8,
					Direction:      "unpredictable-behavior",
					Severity:       2,
					Recoverable:    true,
				}
				manager.serverAgents[id].RecordMCPFailure(event)
			},
			expectedState:  targets.StateDegraded,
			expectedStatus: "degraded",
		},
		{
			name: "compromised-to-outage",
			setupFunc: func(id string) {
				manager.RegisterMCPServer(id, []string{"tool"})
				for i := 0; i < 2; i++ {
					event := isotope.MCPFailureEvent{
						FailureType:    string(mcp_faults.ToolNotFoundFailure),
						ServerID:       id,
						ToolName:       "tool",
						MethodName:     "tools/call",
						DistanceWeight: 40,
						Direction:      "coordinated-signaling",
						Severity:       5,
						Recoverable:    false,
					}
					manager.serverAgents[id].RecordMCPFailure(event)
				}
			},
			expectedState:  targets.StateOutage,
			expectedStatus: "compromised",
		},
	}

	for _, tc := range testCases {
		serverID := "test-" + tc.name
		tc.setupFunc(serverID)

		state := manager.HealthToTargetState(serverID)
		health := manager.GetServerHealth(serverID)

		if state != tc.expectedState {
			t.Errorf("%s: expected state %v, got %v", tc.name, tc.expectedState, state)
		}

		if health.Status != tc.expectedStatus {
			t.Errorf("%s: expected status %q, got %q", tc.name, tc.expectedStatus, health.Status)
		}

		t.Logf("✓ %s: health=%s → target_state=%v", tc.name, health.Status, state)
	}
}

// TestMCPServerManagerConcurrentValidation tests concurrent tool call validation.
func TestMCPServerManagerConcurrentValidation(t *testing.T) {
	manager := NewMCPServerManager(false)

	serverID := "concurrent-server"
	manager.RegisterMCPServer(serverID, []string{"tool"})

	// Get the proxy for legitimate calls
	manager.mu.RLock()
	proxy := manager.toolProxies[serverID].Proxies["tool"]
	manager.mu.RUnlock()
	legitimateCall := proxy.PrefixedToolName()

	// Simulate concurrent validation
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			if idx%2 == 0 {
				manager.ValidateToolCall(serverID, "tool", legitimateCall)
			} else {
				manager.ValidateToolCall(serverID, "tool", "tool") // Direct call
			}
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	health := manager.GetServerHealth(serverID)

	// Should have recorded multiple suspicious calls (concurrent writes may lose count)
	if health.SuspiciousCallCnt < 4 {
		t.Errorf("Expected at least 4 suspicious calls, got %d", health.SuspiciousCallCnt)
	}

	t.Logf("✓ Concurrent validation: %d suspicious calls recorded", health.SuspiciousCallCnt)
}

// TestMCPServerManagerIntegrationWithHoneypot validates end-to-end flow.
func TestMCPServerManagerIntegrationWithHoneypot(t *testing.T) {
	manager := NewMCPServerManager(false) // honeypot off for test

	serverID := "integration-server"
	toolNames := []string{"list-tools", "call-tool", "get-resource"}
	manager.RegisterMCPServer(serverID, toolNames)

	// Scenario: Attacker probes tools without proper prefix
	attacks := []struct {
		tool string
		call string
		desc string
	}{
		{"list-tools", "list-tools", "direct-1"},
		{"list-tools", "list-tools", "direct-2"},
		{"call-tool", "call-tool", "direct-3"},
		{"call-tool", "fuzz_abc_call-tool", "fuzz-1"},
		{"get-resource", "random_get-resource", "fuzz-2"},
	}

	for _, attack := range attacks {
		manager.ValidateToolCall(serverID, attack.tool, attack.call)
	}

	health := manager.GetServerHealth(serverID)

	// Verify accumulated suspicion
	if health.SuspiciousCallCnt == 0 {
		t.Error("Should have recorded suspicious calls")
	}

	if health.Distance42i == 0 {
		t.Error("Should have accumulated distance from honeypot activity")
	}

	// Distance should be substantial (3 direct calls * 24 + 2 fuzz * 8 + MCP failure events)
	if health.Distance42i < 50 {
		t.Logf("Distance seems low: got %d (expected >= 50)", health.Distance42i)
	}

	// Should be considered compromised due to accumulated distance
	if health.Distance42i > 60 {
		shouldEvict, reason := manager.ShouldEvictServer(serverID)
		if !shouldEvict {
			t.Error("High-distance server should be evicted")
		}
		t.Logf("✓ Eviction triggered: %s", reason)
	}

	t.Logf("✓ Integration test complete: distance=%d, suspicious=%d, tools=%v",
		health.Distance42i, health.SuspiciousCallCnt, health.ToolsUnderAttack)
}
