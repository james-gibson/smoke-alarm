package mcp_faults

import (
	"fmt"
)

// MCPFailureType categorizes different MCP/ACP protocol failures.
type MCPFailureType string

const (
	// Timeout: Server doesn't respond within SLA
	// 42i direction: unpredictable-behavior (unreliable latency)
	// Severity: moderate (temporary issue, can recover)
	TimeoutFailure MCPFailureType = "timeout"

	// UnauthorizedAccess: Server denied access to a tool or resource
	// 42i direction: unauthorized-access (server enforcing limits or compromised)
	// Severity: high (indicates permission/capability issue)
	UnauthorizedAccessFailure MCPFailureType = "unauthorized-access"

	// CorruptedResponse: Server returned malformed JSON-RPC or invalid protocol message
	// 42i direction: boundary-violation (server violating protocol contract)
	// Severity: high (protocol corruption suggests compromise)
	CorruptedResponseFailure MCPFailureType = "corrupted-response"

	// MalformedJSON: JSON parsing failed on server response
	// 42i direction: boundary-violation (invalid protocol message)
	// Severity: high
	MalformedJSONFailure MCPFailureType = "malformed-json"

	// ToolNotFound: Server claimed to have tool but then reported not found
	// 42i direction: coordinated-signaling (server lying about capabilities)
	// Severity: critical (indicates dishonesty/Byzantine behavior)
	ToolNotFoundFailure MCPFailureType = "tool-not-found"

	// ResourceExhaustion: Server hit limits (rate limit, memory, connections)
	// 42i direction: boundary-violation (exceeding resource contract)
	// Severity: moderate (indicates server stress, can be temporary)
	ResourceExhaustionFailure MCPFailureType = "resource-exhaustion"

	// PartialResponse: Server sent incomplete message (truncated, mid-stream disconnect)
	// 42i direction: unpredictable-behavior (unreliable delivery)
	// Severity: moderate
	PartialResponseFailure MCPFailureType = "partial-response"

	// CapabilityMismatch: Server's actual capabilities don't match advertisement
	// 42i direction: coordinated-signaling (Byzantine: server misrepresenting itself)
	// Severity: critical
	CapabilityMismatchFailure MCPFailureType = "capability-mismatch"
)

// MCPFailureMode defines how a specific MCP failure type maps to 42i cost.
type MCPFailureMode struct {
	// Type: which kind of failure
	Type MCPFailureType

	// Description: human-readable explanation
	Description string

	// Direction: which 42i direction does this indicate?
	// Must be one of: unpredictable-behavior, coordinated-signaling, unauthorized-access, boundary-violation
	Direction string

	// DistanceWeight: how much 42i_distance this failure contributes
	// (like test failure weights in isotope)
	DistanceWeight int

	// Severity: 1-5, affects retry backoff and alert level
	// 1: ignorable (transient)
	// 3: moderate (concerning)
	// 5: critical (likely Byzantine)
	Severity int

	// Recoverable: can the server recover from this, or is it permanent?
	Recoverable bool
}

// DefaultMCPFailureModes maps failure types to their 42i costs.
var DefaultMCPFailureModes = map[MCPFailureType]MCPFailureMode{
	TimeoutFailure: {
		Type:            TimeoutFailure,
		Description:     "Server did not respond within SLA latency",
		Direction:       "unpredictable-behavior",
		DistanceWeight:  8, // Moderate cost (like a test failure)
		Severity:        2, // Transient issues common
		Recoverable:     true,
	},

	UnauthorizedAccessFailure: {
		Type:            UnauthorizedAccessFailure,
		Description:     "Server denied access to requested tool or resource",
		Direction:       "unauthorized-access",
		DistanceWeight:  24, // Higher cost (permission violation)
		Severity:        4, // Concerning—may be intentional restriction
		Recoverable:     false, // Usually permanent
	},

	CorruptedResponseFailure: {
		Type:            CorruptedResponseFailure,
		Description:     "Server response violated JSON-RPC protocol",
		Direction:       "boundary-violation",
		DistanceWeight:  32, // High cost (protocol violation)
		Severity:        4, // Significant—suggests compromise
		Recoverable:     false,
	},

	MalformedJSONFailure: {
		Type:            MalformedJSONFailure,
		Description:     "Server response contained invalid JSON",
		Direction:       "boundary-violation",
		DistanceWeight:  32,
		Severity:        4,
		Recoverable:     false,
	},

	ToolNotFoundFailure: {
		Type:            ToolNotFoundFailure,
		Description:     "Server listed tool in capabilities but then reported not found",
		Direction:       "coordinated-signaling",
		DistanceWeight:  40, // Very high (Byzantine: lying)
		Severity:        5, // Critical—dishonest server
		Recoverable:     false,
	},

	ResourceExhaustionFailure: {
		Type:            ResourceExhaustionFailure,
		Description:     "Server hit resource limits (rate limit, memory, connections)",
		Direction:       "boundary-violation",
		DistanceWeight:  16, // Moderate (can be transient)
		Severity:        3,
		Recoverable:     true, // Usually recovers after cooldown
	},

	PartialResponseFailure: {
		Type:            PartialResponseFailure,
		Description:     "Server sent incomplete message (connection dropped mid-stream)",
		Direction:       "unpredictable-behavior",
		DistanceWeight:  12,
		Severity:        2,
		Recoverable:     true,
	},

	CapabilityMismatchFailure: {
		Type:            CapabilityMismatchFailure,
		Description:     "Server's actual capabilities don't match its advertisement",
		Direction:       "coordinated-signaling",
		DistanceWeight:  48, // Very high (Byzantine: deception)
		Severity:        5,  // Critical
		Recoverable:     false,
	},
}

// String returns human-readable description.
func (mfm MCPFailureMode) String() string {
	return fmt.Sprintf(
		"%s [%s]: %s (distance=%d, severity=%d, recoverable=%v)",
		mfm.Type, mfm.Direction, mfm.Description,
		mfm.DistanceWeight, mfm.Severity, mfm.Recoverable,
	)
}

// GetFailureMode returns the 42i mapping for a failure type.
func GetFailureMode(ft MCPFailureType) MCPFailureMode {
	if mode, ok := DefaultMCPFailureModes[ft]; ok {
		return mode
	}
	// Fallback for unknown types
	return MCPFailureMode{
		Type:            ft,
		Description:     "Unknown MCP failure type",
		Direction:       "unpredictable-behavior",
		DistanceWeight:  4,
		Severity:        1,
		Recoverable:     true,
	}
}

// MCPFailureEvent records a single MCP failure and its context.
type MCPFailureEvent struct {
	// Which failure occurred
	Type MCPFailureType

	// Context: what was the agent doing?
	ServerID   string // MCP/ACP server that failed
	ToolName   string // which tool call failed (if applicable)
	MethodName string // JSON-RPC method (e.g., "tools/call")

	// Error details
	ErrorMessage string
	HTTPStatus   int // if HTTP-based

	// Timestamps and metrics
	AttemptNumber int       // which retry attempt was this?
	LatencyMs     int       // how long before timeout/error?
	RecoveredAt   int       // which attempt recovered? (-1 if never)
}

// String returns human-readable failure event.
func (mfe MCPFailureEvent) String() string {
	return fmt.Sprintf(
		"MCP failure [%s/%s]: %s on %s (attempt %d, %dms, recovered=%v)",
		mfe.ServerID, mfe.ToolName, mfe.Type, mfe.MethodName,
		mfe.AttemptNumber, mfe.LatencyMs, mfe.RecoveredAt >= 0,
	)
}

// MCPServerHealthSummary tracks aggregate health of an MCP server.
type MCPServerHealthSummary struct {
	ServerID         string
	TotalFailures    int
	FailuresByType   map[MCPFailureType]int
	TotalDistance    int          // 42i_distance from this server
	AverageLatencyMs int
	LastFailureTime  string
	IsCompromised    bool         // consensus: this server is Byzantine
}

// String returns human-readable summary.
func (mshs MCPServerHealthSummary) String() string {
	status := "HEALTHY"
	if mshs.IsCompromised {
		status = "COMPROMISED"
	} else if mshs.TotalFailures > 5 {
		status = "DEGRADED"
	}

	return fmt.Sprintf(
		"%s [%s]: %d failures, distance=%d, avg_latency=%dms",
		mshs.ServerID, status, mshs.TotalFailures, mshs.TotalDistance, mshs.AverageLatencyMs,
	)
}
