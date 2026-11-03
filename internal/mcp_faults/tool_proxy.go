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

	// The scope this route serves: the project (or session) whose tools are
	// exposed. Two projects can expose a tool of the same name; the scope is what
	// keeps their canonical route ids apart.
	Scope string

	// Unique prefix that all legitimate calls must include
	// Format: honeypot_<8-hex-chars>_
	Prefix string

	// Tracks invalid/fuzzed calls (calls without the prefix)
	InvalidCalls []InvalidToolCall

	// Whether this proxy is in honeypot mode (log all attempts)
	HoneypotMode bool

	// Severity multiplier for invalid calls (used for 42i distance)
	SuspicionLevel int // 1-5, affects distance weight for invalid calls

	// Skills stamped onto this route. A stamp that rotation has burned still
	// attributes a call back to the client we issued it to.
	Skills []SkillRoute

	// How many times the route has rotated (each rotation burns its skills)
	Rotations int

	// When this node last rotated. Nodes rotate independently; the cluster's
	// 5s cadence is a notification schedule, not a shared clock.
	LastRotation time.Time

	// How long a burned stamp still resolves after its rotation, covering
	// notification propagation to peers and clients. Zero means DefaultGrace.
	GraceWindow time.Duration

	// Clock seam for tests; nil means time.Now.
	now func() time.Time

	// Timestamp of creation
	CreatedAt time.Time
}

// SkillRoute is a one-time capability stamped onto a route. Rotation burns the
// stamp: the route stops resolving once the grace window expires, but a call
// carrying it stays attributable, which is what separates a stale client from a
// fuzzer.
type SkillRoute struct {
	Name     string
	Stamp    string // full stamped call name, prefix included
	Prefix   string // route prefix at stamp time; the part that proves capability
	Burned   bool
	BurnedAt time.Time // when rotation invalidated the stamp
}

// burnedSkillCost is what a call with an attributable-but-burned stamp costs
// once grace has expired. Rotation is an honest cost of the safe trail, not an
// attack signal, so it must stay below the fuzz tier.
const burnedSkillCost = 2

// Rotation cadence, as clarified by the topology:
//
//   - the cluster rotates every ClusterRotationPeriod, with a grace window;
//   - a mirror node observes each rotation before it is applied, so a single
//     node can rotate without a cluster rotation;
//   - the notification then fans out from that observation.
//
// The mirror node does not exist yet - it is another node, downstream of the
// workspace spike (james-gibson/lab-safety#9) - and whether it alerts cluster
// neighbors is configuration. Grace is therefore a notification-latency budget,
// not a second clock: a stamp stays valid until word of its rotation could
// plausibly have arrived.
//
// The load-bearing consequence: because alerting can be off, grace cannot be the
// safety net. Attribution must hold without any notification at all - a silent
// rotation still leaves a burned stamp attributable, which is why staleness is
// priced at burnedSkillCost rather than as fuzz.
const (
	ClusterRotationPeriod = 5 * time.Second
	DefaultGraceWindow    = 1 * time.Second
)

func (tp *ToolProxy) clock() time.Time {
	if tp.now != nil {
		return tp.now()
	}
	return time.Now()
}

func (tp *ToolProxy) grace() time.Duration {
	claimed := tp.GraceWindow
	if claimed <= 0 {
		claimed = DefaultGraceWindow
	}
	// A node cannot grant itself more grace than the cluster allows. This is a
	// self-check, not a control: a compromised node runs its own code, so peers
	// must apply the mirror's verdict (VerifyRotation) rather than the claim.
	return DefaultRotationPolicy().EffectiveGrace(claimed)
}

// withinGrace reports whether a burn is recent enough that notification may
// still be in flight.
func (tp *ToolProxy) withinGrace(burnedAt time.Time) bool {
	return tp.clock().Sub(burnedAt) <= tp.grace()
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
	Source string // "direct", "proxy", "fuzz", "skill"

	// Skill this call was attributed to, if any (empty when unattributable)
	Skill string
}

// newPrefix draws a fresh route prefix in the proxy's own shape. The marker is
// kept for log readability while the shipped format still has it; production
// decoys that carry no marker rotate through the same path.
func newPrefix() string {
	prefixBytes := make([]byte, 4)
	rand.Read(prefixBytes)
	return fmt.Sprintf("honeypot_%s_", hex.EncodeToString(prefixBytes))
}

// NewToolProxy creates a proxy for a tool with a unique prefix.
func NewToolProxy(toolName string, honeypotMode bool) *ToolProxy {
	return &ToolProxy{
		ProxyID:        fmt.Sprintf("proxy_%s_%d", toolName, time.Now().UnixNano()),
		ToolName:       toolName,
		Prefix:         newPrefix(),
		InvalidCalls:   []InvalidToolCall{},
		HoneypotMode:   honeypotMode,
		SuspicionLevel: 1,
		CreatedAt:      time.Now(),
	}
}

// IssueSkill stamps a one-time skill onto the current route and returns the
// stamped call name. The stamp is the skill's whole identity: it is what lets a
// call be attributed back to the client it was issued to.
func (tp *ToolProxy) IssueSkill(name string) string {
	stamp := tp.PrefixedToolName()
	tp.Skills = append(tp.Skills, SkillRoute{Name: name, Stamp: stamp, Prefix: tp.Prefix})
	return stamp
}

// BurnSkills invalidates every active stamp, reporting how many it burned.
func (tp *ToolProxy) BurnSkills() int {
	at := tp.clock()
	burned := 0
	for i := range tp.Skills {
		if !tp.Skills[i].Burned {
			tp.Skills[i].Burned = true
			tp.Skills[i].BurnedAt = at
			burned++
		}
	}
	return burned
}

// Rotate burns every active skill and moves this node's route to a fresh prefix.
// It is deliberately per-node: a single node can rotate without a cluster
// rotation, and the notification fans out from the observing mirror. Burning
// alone moves no score - only calls do.
func (tp *ToolProxy) Rotate() int {
	burned := tp.BurnSkills()
	tp.Prefix = newPrefix()
	tp.Rotations++
	tp.LastRotation = tp.clock()
	return burned
}

// attribute resolves a call back to a stamped skill, burned or not, reporting
// whether the match was the exact stamp. An exact stamp is the strongest
// evidence; failing that, the route prefix still proves the caller was issued the
// capability, even if a layer rewrote the suffix.
func (tp *ToolProxy) attribute(callName string) (skill SkillRoute, exact, ok bool) {
	for _, candidate := range tp.Skills {
		if candidate.Stamp == callName {
			return candidate, true, true
		}
	}
	for _, candidate := range tp.Skills {
		if candidate.Prefix != "" && strings.HasPrefix(callName, candidate.Prefix) {
			return candidate, false, true
		}
	}
	return SkillRoute{}, false, false
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
	var attributed string

	// Attribution first: a call carrying a stamp we issued is ours, however
	// stale. A burned stamp inside the grace window still resolves, because the
	// notification may not have reached the caller yet.
	if skill, exact, ok := tp.attribute(callName); ok {
		attributed = skill.Name
		switch {
		case !skill.Burned:
			reason = "skill-route-mismatch"
		case exact && tp.withinGrace(skill.BurnedAt):
			return true, "" // notification still in flight
		default:
			reason = "burned-skill-route"
		}
	}

	// Check if the bare tool name was used (direct call, bypassing proxy)
	if callName == tp.ToolName && attributed == "" {
		likelyDirect = true
		reason = "direct-call-to-tool-bypasses-proxy"
	}

	// Check if there's a prefix but wrong one (fuzz test or attack attempt).
	// Only unattributable calls count: several layers in, a mangled stamp is
	// fuzzing, an intact one is a client that has not caught up.
	if reason == "" && strings.Contains(callName, "_") {
		likelyFuzzed = true
		reason = "fuzzed-or-wrong-prefix"
	}

	// If no prefix at all and not the bare tool name, probably malformed
	if reason == "" && !strings.Contains(callName, "_") {
		reason = "unprefixed-malformed"
	}

	// Record the suspicious call
	suspiciousCall := InvalidToolCall{
		CallName:       callName,
		ExpectedPrefix: tp.Prefix,
		ActualPrefix:   extractPrefix(callName),
		LikelyFuzzed:   likelyFuzzed,
		Timestamp:      time.Now(),
		Source:         tp.callSource(callName, attributed, likelyDirect),
		Skill:          attributed,
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
	skillRouteCalls := 0

	for _, call := range tp.InvalidCalls {
		if call.LikelyFuzzed {
			fuzzAttempts++
		}
		if call.Source == "direct" {
			directCalls++
		}
		if call.Source == "skill" {
			skillRouteCalls++
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
	if skillRouteCalls > 0 {
		distanceCost += skillRouteCalls * burnedSkillCost // Attributable but stale: rotation cost
	}
	if rest := totalInvalid - fuzzAttempts - directCalls - skillRouteCalls; rest > 0 {
		// Other invalid calls (wrong prefix): +16 each (boundary-violation)
		distanceCost += rest * 16
	}

	return ToolProxyMetrics{
		ProxyID:         tp.ProxyID,
		ToolName:        tp.ToolName,
		Prefix:          tp.Prefix,
		TotalInvalid:    totalInvalid,
		FuzzAttempts:    fuzzAttempts,
		DirectCalls:     directCalls,
		SkillRouteCalls: skillRouteCalls,
		WrongPrefix:     totalInvalid - fuzzAttempts - directCalls - skillRouteCalls,
		Distance42i:     distanceCost,
		HoneypotMode:    tp.HoneypotMode,
		SuspicionLevel:  tp.SuspicionLevel,
	}
}

// ToolProxyMetrics summarizes a tool proxy's honeypot activity.
type ToolProxyMetrics struct {
	ProxyID         string
	ToolName        string
	Prefix          string
	TotalInvalid    int // Total invalid calls
	FuzzAttempts    int // Fuzzed/random prefix calls
	DirectCalls     int // Direct calls (bypass proxy)
	SkillRouteCalls int // Calls attributable to a burned skill (rotation cost)
	WrongPrefix     int // Wrong prefix
	Distance42i     int // 42i cost from invalid calls
	HoneypotMode    bool
	SuspicionLevel  int
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
func (tpc *ToolProxyCluster) ValidateToolCall(toolName, callName string) (hasCorrectPrefix bool, suspicion string, metrics ToolProxyMetrics) {
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

// callSource labels a call by where it came from, preferring attribution over
// shape: a stamped call is "skill" even though its name looks like fuzz.
func (tp *ToolProxy) callSource(callName, attributed string, direct bool) string {
	switch {
	case attributed != "":
		return "skill"
	case direct:
		return "direct"
	default:
		return sourceFromCall(callName, tp.ToolName)
	}
}
