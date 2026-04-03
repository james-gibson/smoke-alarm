package mcp_faults

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// ToolProxy wraps an MCP tool with validation and honeypot capabilities.
// All legitimate tool invocations include a unique prefix; calls without it are invalid/fuzzed.
type ToolProxy struct {
	// Unique identifier for this proxy instance
	ProxyID string

	// The actual tool name this proxy wraps
	ToolName string

	// Unique prefix that all legitimate calls must include
	// Format: honeypot_<8-hex-chars>_
	Prefix string

	// Tracks invalid/fuzzed calls (calls without the prefix)
	InvalidCalls []InvalidToolCall

	// Whether this proxy is in honeypot mode (log all attempts)
	HoneypotMode bool

	// Severity multiplier for invalid calls (used for 42i distance)
	SuspicionLevel int // 1-5, affects distance weight for invalid calls

	// Timestamp of creation
	CreatedAt time.Time
}

// InvalidToolCall records an attempted tool invocation without proper prefix.
type InvalidToolCall struct {
	// The malformed call
	CallName string

	// What prefix was expected
	ExpectedPrefix string

	// What prefix was actually present (if any)
	ActualPrefix string

	// Whether it looks like a fuzz attempt (random chars) vs. direct call
	LikelyFuzzed bool

	// Timestamp
	Timestamp time.Time

	// Honeypot metadata
	Source string // "direct", "proxy", "fuzz"
}

// NewToolProxy creates a proxy for a tool with a unique prefix.
func NewToolProxy(toolName string, honeypotMode bool) *ToolProxy {
	// Generate unique 8-hex-char prefix
	prefixBytes := make([]byte, 4)
	rand.Read(prefixBytes)
	prefixHex := hex.EncodeToString(prefixBytes)

	prefix := fmt.Sprintf("honeypot_%s_", prefixHex)

	return &ToolProxy{
		ProxyID:        fmt.Sprintf("proxy_%s_%d", toolName, time.Now().UnixNano()),
		ToolName:       toolName,
		Prefix:         prefix,
		InvalidCalls:   []InvalidToolCall{},
		HoneypotMode:   honeypotMode,
		SuspicionLevel: 1,
		CreatedAt:      time.Now(),
	}
}

// PrefixedToolName returns the canonical tool name with prefix included.
// Legitimate callers should use this.
func (tp *ToolProxy) PrefixedToolName() string {
	return tp.Prefix + tp.ToolName
}

// ValidateCall checks if a tool call includes the expected prefix.
// Returns: (hasCorrectPrefix, suspicionReason)
//
// NOTE: Calls without the prefix are NOT necessarily invalid/rejected.
// They're SUSPICIOUS and worth scrutiny:
// - Direct calls: legitimate client bypassing proxy (maybe legacy code?)
// - Fuzzed calls: test framework or deliberate probing
// - Wrong prefix: possible misconfiguration or attack attempt
//
// The distinction matters for honeypot/monitoring vs. enforcement:
// - Log and track all suspicious calls (contributes to 42i distance)
// - But don't necessarily block them (allow investigation)
// - Pattern of suspicion (many unprefixed calls) triggers eviction
func (tp *ToolProxy) ValidateCall(callName string) (hasCorrectPrefix bool, suspicion string) {
	expected := tp.PrefixedToolName()

	// Check for exact match (expected call)
	if callName == expected {
		return true, ""
	}

	// Not using correct prefix - determine why and log it
	var reason string
	var likelyFuzzed, likelyDirect bool

	// Check if the bare tool name was used (direct call, bypassing proxy)
	if callName == tp.ToolName {
		likelyDirect = true
		reason = "direct-call-to-tool-bypasses-proxy"
	}

	// Check if there's a prefix but wrong one (fuzz test or attack attempt)
	if strings.Contains(callName, "_") && !likelyDirect {
		likelyFuzzed = true
		reason = "fuzzed-or-wrong-prefix"
	}

	// If no prefix at all and not the bare tool name, probably malformed
	if !likelyFuzzed && !likelyDirect && !strings.Contains(callName, "_") {
		reason = "unprefixed-malformed"
	}

	// Record the suspicious call
	suspiciousCall := InvalidToolCall{
		CallName:       callName,
		ExpectedPrefix: tp.Prefix,
		ActualPrefix:   extractPrefix(callName),
		LikelyFuzzed:   likelyFuzzed,
		Timestamp:      time.Now(),
		Source:         sourceFromCall(callName, tp.ToolName),
	}

	tp.InvalidCalls = append(tp.InvalidCalls, suspiciousCall)

	if tp.HoneypotMode {
		fmt.Printf("[HONEYPOT] Tool proxy %s detected suspicious call: %q (%s, expected %q)\n",
			tp.ProxyID, callName, reason, expected)
	}

	return false, reason
}

// GetInvalidCallMetrics returns honeypot metrics for this proxy.
func (tp *ToolProxy) GetInvalidCallMetrics() ToolProxyMetrics {
	totalInvalid := len(tp.InvalidCalls)
	fuzzAttempts := 0
	directCalls := 0

	for _, call := range tp.InvalidCalls {
		if call.LikelyFuzzed {
			fuzzAttempts++
		}
		if call.Source == "direct" {
			directCalls++
		}
	}

	// Calculate 42i cost: invalid calls indicate fuzz/attack/compromise
	distanceCost := 0
	if fuzzAttempts > 0 {
		distanceCost += fuzzAttempts * 8 // Fuzz attempts: +8 each (unpredictable-behavior)
	}
	if directCalls > 0 {
		distanceCost += directCalls * 24 // Direct calls (bypass proxy): +24 each (suspicious)
	}
	if totalInvalid-fuzzAttempts-directCalls > 0 {
		// Other invalid calls (wrong prefix): +16 each (boundary-violation)
		distanceCost += (totalInvalid - fuzzAttempts - directCalls) * 16
	}

	return ToolProxyMetrics{
		ProxyID:        tp.ProxyID,
		ToolName:       tp.ToolName,
		Prefix:         tp.Prefix,
		TotalInvalid:   totalInvalid,
		FuzzAttempts:   fuzzAttempts,
		DirectCalls:    directCalls,
		WrongPrefix:    totalInvalid - fuzzAttempts - directCalls,
		Distance42i:    distanceCost,
		HoneypotMode:   tp.HoneypotMode,
		SuspicionLevel: tp.SuspicionLevel,
	}
}

// ToolProxyMetrics summarizes a tool proxy's honeypot activity.
type ToolProxyMetrics struct {
	ProxyID        string
	ToolName       string
	Prefix         string
	TotalInvalid   int // Total invalid calls
	FuzzAttempts   int // Fuzzed/random prefix calls
	DirectCalls    int // Direct calls (bypass proxy)
	WrongPrefix    int // Wrong prefix
	Distance42i    int // 42i cost from invalid calls
	HoneypotMode   bool
	SuspicionLevel int
}

// String returns human-readable metrics.
func (tpm ToolProxyMetrics) String() string {
	status := "CLEAN"
	if tpm.TotalInvalid > 0 {
		status = "SUSPICIOUS"
	}
	if tpm.FuzzAttempts > 0 || tpm.DirectCalls > 0 {
		status = "UNDER ATTACK"
	}

	return fmt.Sprintf(
		"Tool proxy %s/%s [%s]: %d invalid calls (fuzz=%d, direct=%d, wrong=%d) | distance=%d",
		tpm.ProxyID, tpm.ToolName, status,
		tpm.TotalInvalid, tpm.FuzzAttempts, tpm.DirectCalls, tpm.WrongPrefix, tpm.Distance42i,
	)
}

// ToolProxyCluster manages multiple tool proxies and detects coordinated attacks.
type ToolProxyCluster struct {
	// Map of tool name → proxy
	Proxies map[string]*ToolProxy

	// Cluster-wide metrics
	TotalInvalidCalls int
	AttackDetected    bool
	SuspiciousTools   []string

	// Timestamp
	CreatedAt time.Time
}

// NewToolProxyCluster creates a cluster of tool proxies.
func NewToolProxyCluster(toolNames []string, honeypotMode bool) *ToolProxyCluster {
	cluster := &ToolProxyCluster{
		Proxies:           make(map[string]*ToolProxy),
		TotalInvalidCalls: 0,
		AttackDetected:    false,
		SuspiciousTools:   []string{},
		CreatedAt:         time.Now(),
	}

	for _, toolName := range toolNames {
		cluster.Proxies[toolName] = NewToolProxy(toolName, honeypotMode)
	}

	return cluster
}

// ValidateToolCall checks a tool call against its proxy.
// Returns: (hasCorrectPrefix, suspicionReason, metrics)
//
// All calls are processed. Calls without correct prefix are marked as suspicious
// but still tracked. Multiple suspicious calls across tools indicate possible attack.
func (tpc *ToolProxyCluster) ValidateToolCall(toolName string, callName string) (hasCorrectPrefix bool, suspicion string, metrics ToolProxyMetrics) {
	proxy, ok := tpc.Proxies[toolName]
	if !ok {
		// Unknown tool - this is also suspicious
		return false, "unknown-tool", ToolProxyMetrics{
			ToolName:    toolName,
			Distance42i: 32, // Unknown tool: high suspicion (boundary-violation)
		}
	}

	isCorrect, suspicion := proxy.ValidateCall(callName)
	tpc.TotalInvalidCalls += len(proxy.InvalidCalls)

	// Detect coordinated attack: multiple tools getting suspicious calls
	if !isCorrect {
		metrics = proxy.GetInvalidCallMetrics()

		// Mark as under attack if suspicious calls exceed threshold
		if len(proxy.InvalidCalls) > 3 {
			tpc.AttackDetected = true
			tpc.SuspiciousTools = append(tpc.SuspiciousTools, toolName)
		}
	}

	return isCorrect, suspicion, metrics
}

// GetClusterMetrics returns aggregate honeypot metrics.
func (tpc *ToolProxyCluster) GetClusterMetrics() ClusterHoneypotMetrics {
	totalDistance := 0
	totalInvalid := 0
	toolStatus := make(map[string]ToolProxyMetrics)

	for toolName, proxy := range tpc.Proxies {
		metrics := proxy.GetInvalidCallMetrics()
		toolStatus[toolName] = metrics
		totalDistance += metrics.Distance42i
		totalInvalid += metrics.TotalInvalid
	}

	return ClusterHoneypotMetrics{
		TotalTools:        len(tpc.Proxies),
		TotalInvalidCalls: totalInvalid,
		TotalDistance42i:  totalDistance,
		AttackDetected:    tpc.AttackDetected,
		SuspiciousTools:   tpc.SuspiciousTools,
		ToolMetrics:       toolStatus,
	}
}

// ClusterHoneypotMetrics summarizes honeypot activity across all tool proxies.
type ClusterHoneypotMetrics struct {
	TotalTools        int
	TotalInvalidCalls int
	TotalDistance42i  int
	AttackDetected    bool
	SuspiciousTools   []string
	ToolMetrics       map[string]ToolProxyMetrics
}

// String returns human-readable cluster metrics.
func (chm ClusterHoneypotMetrics) String() string {
	status := "HEALTHY"
	if chm.TotalInvalidCalls > 0 {
		status = "SUSPICIOUS"
	}
	if chm.AttackDetected {
		status = "ATTACK DETECTED"
	}

	return fmt.Sprintf(
		"Tool proxy cluster [%s]: %d tools, %d invalid calls, distance=%d, suspicious=%d",
		status, chm.TotalTools, chm.TotalInvalidCalls, chm.TotalDistance42i, len(chm.SuspiciousTools),
	)
}

// Helper functions

// extractPrefix extracts the prefix from a tool call name.
func extractPrefix(callName string) string {
	parts := strings.Split(callName, "_")
	if len(parts) >= 2 {
		return parts[0] + "_"
	}
	return ""
}

// sourceFromCall determines the likely source of the invalid call.
func sourceFromCall(callName, expectedToolName string) string {
	if callName == expectedToolName {
		return "direct"
	}
	if strings.Contains(callName, "honeypot") || strings.Contains(callName, "_") {
		return "fuzz"
	}
	return "other"
}
