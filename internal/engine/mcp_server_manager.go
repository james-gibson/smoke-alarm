package engine

import (
	"fmt"
	"sync"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/isotope"
	"github.com/james-gibson/smoke-alarm/internal/mcp_faults"
	"github.com/james-gibson/smoke-alarm/internal/targets"
)

// MCPServerManager tracks MCP/ACP server health via tool proxies and 42i distance.
// This integrates honeypot tool prefix validation with the eviction/health system.
type MCPServerManager struct {
	// Map of server ID → agent state (tracks 42i distance from MCP failures)
	serverAgents map[string]*isotope.AgentState

	// Map of server ID → tool proxy cluster (honeypot validation)
	toolProxies map[string]*mcp_faults.ToolProxyCluster

	// Tracks which servers are under attack (based on honeypot metrics)
	attackedServers map[string]bool

	// Eviction threshold: servers with distance > this are candidates for removal
	evictionThreshold int

	// Honeypot mode: log all suspicious calls
	honeypotMode bool

	mu sync.RWMutex
}

// NewMCPServerManager creates a manager for tracking MCP server health.
func NewMCPServerManager(honeypotMode bool) *MCPServerManager {
	return &MCPServerManager{
		serverAgents:      make(map[string]*isotope.AgentState),
		toolProxies:       make(map[string]*mcp_faults.ToolProxyCluster),
		attackedServers:   make(map[string]bool),
		evictionThreshold: 60, // Distance > 60 triggers eviction consideration
		honeypotMode:      honeypotMode,
	}
}

// RegisterMCPServer sets up honeypot tool proxies for a new MCP server.
func (m *MCPServerManager) RegisterMCPServer(serverID string, toolNames []string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Create agent state to track 42i distance
	m.serverAgents[serverID] = isotope.NewAgentState(serverID)

	// Create tool proxies with honeypot validation
	m.toolProxies[serverID] = mcp_faults.NewToolProxyCluster(toolNames, m.honeypotMode)
}

// ValidateToolCall checks if a tool invocation has the correct proxy prefix.
// Returns: (hasCorrectPrefix, reason, shouldReject)
//
// Tool calls WITHOUT the prefix are suspicious but may be legitimate (legacy clients, etc).
// The decision to reject is based on pattern: single suspicious call is logged,
// many calls indicate attack and trigger rejection.
func (m *MCPServerManager) ValidateToolCall(serverID, toolName, callName string) (
	hasCorrectPrefix bool,
	suspicion string,
	shouldReject bool,
	metrics mcp_faults.ToolProxyMetrics,
) {
	m.mu.RLock()
	cluster, ok := m.toolProxies[serverID]
	agent, agentOk := m.serverAgents[serverID]
	m.mu.RUnlock()

	if !ok || !agentOk {
		// Unknown server
		return false, "unknown-server", true, mcp_faults.ToolProxyMetrics{
			ToolName:    toolName,
			Distance42i: 32,
		}
	}

	// Validate tool call
	hasCorrect, suspicion, metrics := cluster.ValidateToolCall(toolName, callName)

	// Record suspicious calls as MCP failures
	if !hasCorrect {
		mode := mcp_faults.GetFailureMode(mapSuspicionToFailureType(suspicion))

		event := isotope.MCPFailureEvent{
			FailureType:    suspicion,
			ServerID:       serverID,
			ToolName:       toolName,
			MethodName:     "tools/call",
			DistanceWeight: mode.DistanceWeight,
			Direction:      mode.Direction,
			Severity:       mode.Severity,
			ErrorMessage:   mode.Description,
			Recoverable:    mode.Recoverable,
		}

		agent.RecordMCPFailure(event)
	}

	// Determine if this call should be rejected
	//  - Multiple suspicious calls (pattern attack): reject
	//  - Single/few suspicious calls: allow but log
	shouldReject = false
	if metrics.TotalInvalid > 3 {
		shouldReject = true

		m.mu.Lock()
		m.attackedServers[serverID] = true
		m.mu.Unlock()
	}

	return hasCorrect, suspicion, shouldReject, metrics
}

// GetServerHealth returns comprehensive health metrics for a server.
// This replaces the old "check if server is healthy" with quantified 42i metrics.
func (m *MCPServerManager) GetServerHealth(serverID string) ServerMCPHealth {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agent, agentOk := m.serverAgents[serverID]
	cluster, clusterOk := m.toolProxies[serverID]

	if !agentOk || !clusterOk {
		return ServerMCPHealth{
			ServerID:          serverID,
			Status:            "unknown",
			Distance42i:       0,
			IsCompromised:     false,
			UnderAttack:       false,
			SuspiciousCallCnt: 0,
		}
	}

	clusterMetrics := cluster.GetClusterMetrics()
	isCompromised := agent.TotalDistance > m.evictionThreshold
	underAttack := m.attackedServers[serverID] || clusterMetrics.AttackDetected

	status := "healthy"
	if underAttack {
		status = "under-attack"
	} else if isCompromised {
		status = "compromised"
	} else if agent.TotalDistance > 0 {
		status = "degraded"
	}

	return ServerMCPHealth{
		ServerID:          serverID,
		Status:            status,
		Distance42i:       agent.TotalDistance,
		Rung:              agent.Position.Rung,
		Direction:         agent.Position.Direction,
		IsCompromised:     isCompromised,
		UnderAttack:       underAttack,
		SuspiciousCallCnt: clusterMetrics.TotalInvalidCalls,
		ToolsUnderAttack:  clusterMetrics.SuspiciousTools,
		LastUpdated:       time.Now(),
	}
}

// GetServerRanking returns all servers ranked by trustworthiness.
// Used by fire-marshal to decide:
// - Which servers to use for critical operations (prefer healthy)
// - Which to degrade to secondary pool (degraded)
// - Which to evict (compromised)
func (m *MCPServerManager) GetServerRanking() []ServerMCPHealth {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ranking := []ServerMCPHealth{}
	for serverID := range m.serverAgents {
		health := m.GetServerHealth(serverID)
		ranking = append(ranking, health)
	}

	// Sort by distance (lowest = most trusted)
	for i := 0; i < len(ranking)-1; i++ {
		for j := i + 1; j < len(ranking); j++ {
			if ranking[j].Distance42i < ranking[i].Distance42i {
				ranking[i], ranking[j] = ranking[j], ranking[i]
			}
		}
	}

	return ranking
}

// ShouldEvictServer determines if a server should be removed from the pool.
// Eviction criteria:
// - Distance > threshold (accumulated failures)
// - Under attack (coordinated tool access patterns)
// - Byzantine failures (tool-not-found, capability mismatch)
func (m *MCPServerManager) ShouldEvictServer(serverID string) (bool, string) {
	health := m.GetServerHealth(serverID)

	if health.UnderAttack {
		return true, fmt.Sprintf("under-attack: %d suspicious calls detected", health.SuspiciousCallCnt)
	}

	if health.IsCompromised {
		return true, fmt.Sprintf("compromised: 42i_distance=%d (threshold=%d)", health.Distance42i, m.evictionThreshold)
	}

	// Check for Byzantine failures on specific tools
	if len(health.ToolsUnderAttack) > 0 {
		return true, fmt.Sprintf("byzantine-behavior: tools under attack: %v", health.ToolsUnderAttack)
	}

	return false, ""
}

// MapSuspicionToFailureType converts tool proxy suspicion reason to MCP failure type.
func mapSuspicionToFailureType(suspicion string) mcp_faults.MCPFailureType {
	switch suspicion {
	case "direct-call-to-tool-bypasses-proxy":
		return mcp_faults.ToolNotFoundFailure // Byzantine: intentional bypass
	case "fuzzed-or-wrong-prefix":
		return mcp_faults.TimeoutFailure // Transient: fuzz test
	case "unprefixed-malformed":
		return mcp_faults.CorruptedResponseFailure // Protocol violation
	case "unknown-tool":
		return mcp_faults.CapabilityMismatchFailure // Tool doesn't exist
	case "unknown-server":
		return mcp_faults.CapabilityMismatchFailure // Server unknown
	default:
		return mcp_faults.TimeoutFailure
	}
}

// ServerMCPHealth summarizes health of one MCP server.
type ServerMCPHealth struct {
	ServerID          string
	Status            string // "healthy", "degraded", "compromised", "under-attack"
	Distance42i       int    // 42i_distance from MCP failures
	Rung              int    // 42i rung position
	Direction         string // 42i direction (unpredictable, dishonest, etc.)
	IsCompromised     bool
	UnderAttack       bool
	SuspiciousCallCnt int
	ToolsUnderAttack  []string
	LastUpdated       time.Time
}

// String returns human-readable health summary.
func (h ServerMCPHealth) String() string {
	return fmt.Sprintf(
		"%s [%s]: distance=%d, rung=%d, suspicious=%d, direction=%s, tools_attacked=%d",
		h.ServerID, h.Status, h.Distance42i, h.Rung, h.SuspiciousCallCnt,
		h.Direction, len(h.ToolsUnderAttack),
	)
}

// HealthToTargetState adapts MCP server health to target health state.
// Allows engine to use 42i metrics in eviction decisions.
func (m *MCPServerManager) HealthToTargetState(serverID string) targets.HealthState {
	health := m.GetServerHealth(serverID)

	switch health.Status {
	case "under-attack", "compromised":
		return targets.StateOutage
	case "degraded":
		return targets.StateDegraded
	case "healthy":
		return targets.StateHealthy
	default:
		return targets.StateUnknown
	}
}
