package mcp_faults

import (
	"fmt"
	"strings"
	"sync"
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
	//
	// No longer a draw: this is Schedule.PrefixAt(Epoch), so it belongs to one
	// epoch and stops being the route when the epoch closes.
	Prefix string

	// Schedule is the pre-planned rotation: (nonce, base, origin, period). Both
	// sides derive the same prefix from it, which is how the route moves without
	// being announced. A zero schedule means the route only moves on an explicit
	// Rotate.
	Schedule RouteSchedule

	// Epoch is the rotation sequence in force, not a timestamp (see RouteKey).
	// A stale prefix names a lower epoch; a prefix naming no epoch is a guess.
	Epoch int64

	// Retired is the bounded ledger of prefixes this instance presented and has
	// since rotated away from. Bounded by RetiredEpochWindow.
	Retired []RetiredRoute

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

	// mu guards the mutable half of the route: the epoch, the prefix in force,
	// the retired ledger, the stamps and the call log.
	//
	// Rotation used to happen only when a caller asked for it, so reading a proxy
	// needed no lock. A route that moves on its own clock is state: two concurrent
	// calls can both notice the epoch closed, and without this they would rotate
	// twice, retire the wrong prefix, or read a torn one.
	mu sync.Mutex

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
	Epoch    int64  // route epoch the stamp was issued in
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
	Source string // "direct", "proxy", "fuzz", "skill", "stale"

	// Skill this call was attributed to, if any (empty when unattributable)
	Skill string

	// Epoch is the route epoch in force when the call arrived.
	Epoch int64

	// CallEpoch is the epoch the call's prefix belonged to when we could derive
	// it (current, retired, or the client running ahead); -1 when the prefix
	// matched no epoch of ours. Without this field a stale client and an
	// enumerator are the same log line.
	CallEpoch int64
}

// NewToolProxy creates a proxy for a tool with a unique prefix.
//
// The prefix is derived from a fresh nonce at epoch zero, so the route has a
// schedule from the moment it exists: it is already tumbling, it just has not
// been asked to move yet.
func NewToolProxy(toolName string, honeypotMode bool) *ToolProxy {
	now := time.Now()
	schedule := NewRouteSchedule(now, ClusterRotationPeriod)

	return &ToolProxy{
		ProxyID:        fmt.Sprintf("proxy_%s_%d", toolName, now.UnixNano()),
		ToolName:       toolName,
		Prefix:         schedule.PrefixAt(0),
		Schedule:       schedule,
		Epoch:          0,
		InvalidCalls:   []InvalidToolCall{},
		HoneypotMode:   honeypotMode,
		SuspicionLevel: 1,
		CreatedAt:      now,
	}
}

// PrefixAt reports the prefix this route will present at epoch, so a client
// holding the nonce can compute where the route will be without being told.
func (tp *ToolProxy) PrefixAt(epoch int64) string {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	return tp.Schedule.PrefixAt(epoch)
}

// Key is this route's address at the current epoch: the shared address, the
// claimant speaking for it, and the rotation it is speaking from. Like
// PrefixedToolName it reads through the rotation clock, so the key never names an
// epoch the route has already left.
func (tp *ToolProxy) Key() RouteKey {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	tp.syncEpoch()
	return RouteKey{Address: tp.Prefix, Claimant: tp.ProxyID, Epoch: tp.Epoch}
}

// ensureSchedule gives a route that was built by hand - a decoy minted with an
// explicit prefix, a standby adopting a ledger - a nonce and a clock, so that
// rotating it produces a prefix rather than an empty string.
func (tp *ToolProxy) ensureSchedule() {
	if tp.Schedule.Valid() {
		return
	}
	tp.Schedule = RouteSchedule{
		Nonce:  newNonce(),
		Base:   tp.Epoch,
		Origin: tp.clock(),
		Period: ClusterRotationPeriod,
	}
}

// syncEpoch advances the route to the epoch the schedule says is current. This is
// the rotation clock, and it is lazy on purpose: a route with no traffic needs no
// rotation, and the first call after an epoch closes is the moment the route is
// actually at risk.
//
// It only ever moves forward. An explicit Rotate re-bases the schedule, so a
// node that rotates on a neighbour's instruction does not then get dragged back
// to where its own clock thought it should be.
func (tp *ToolProxy) syncEpoch() {
	if !tp.Schedule.Valid() {
		return
	}
	scheduled := tp.Schedule.EpochAt(tp.clock())
	if scheduled > tp.Epoch {
		// The closed epoch ended when the schedule says it did, which may be well
		// before this call arrived.
		tp.rotateTo(scheduled, tp.Schedule.EpochEnd(tp.Epoch))
	}
}

// rotateTo retires the prefix in force, burns the stamps issued under it, and
// opens target as the new epoch. closedAt is when the old epoch stopped being the
// route, which is the schedule's boundary on a clock-driven rotation and now on
// an explicit one.
func (tp *ToolProxy) rotateTo(target int64, closedAt time.Time) int {
	tp.ensureSchedule()

	at := tp.clock()
	if closedAt.IsZero() || closedAt.After(at) {
		closedAt = at // a rotation cannot date a closure in the future
	}

	burned := tp.burnSkillsAt(closedAt)

	if tp.Prefix != "" {
		tp.Retired = append(tp.Retired, RetiredRoute{Epoch: tp.Epoch, Prefix: tp.Prefix, RetiredAt: closedAt})
		if over := len(tp.Retired) - RetiredEpochWindow; over > 0 {
			tp.Retired = tp.Retired[over:]
		}
	}

	tp.Epoch = target
	tp.Prefix = tp.Schedule.PrefixAt(target)
	tp.Rotations++
	tp.LastRotation = at

	// Re-base so the schedule keeps describing the epoch we just opened. Without
	// this, an out-of-band rotation would leave the clock pointing at a timeline
	// the route has already left.
	tp.Schedule.Base = target
	tp.Schedule.Origin = at

	return burned
}

// retiredRoute resolves a call back to a prefix this instance used to present.
func (tp *ToolProxy) retiredRoute(callName string) (RetiredRoute, bool) {
	for i := len(tp.Retired) - 1; i >= 0; i-- {
		if tp.Retired[i].Prefix != "" && strings.HasPrefix(callName, tp.Retired[i].Prefix) {
			return tp.Retired[i], true
		}
	}
	return RetiredRoute{}, false
}

// epochSkewTolerance is how far ahead of us a client's clock may be and still be
// read as skewed rather than as guessing. One epoch is the smallest tolerance
// that covers ordinary NTP drift without turning the near future into a second
// valid route: an enumerator gains nothing here, because a prefix for an epoch we
// have not reached still has to be derived from the nonce.
const epochSkewTolerance = 1

// aheadRoute resolves a call carrying a prefix from an epoch we have not reached
// yet, reporting the epoch it was derived for.
func (tp *ToolProxy) aheadRoute(callName string) (int64, string) {
	if !tp.Schedule.Valid() {
		return -1, ""
	}
	for ahead := int64(1); ahead <= epochSkewTolerance; ahead++ {
		epoch := tp.Epoch + ahead
		prefix := tp.Schedule.PrefixAt(epoch)
		if prefix != "" && strings.HasPrefix(callName, prefix) {
			return epoch, "ahead-route-epoch"
		}
	}
	return -1, ""
}

// IssueSkill stamps a one-time skill onto the current route and returns the
// stamped call name. The stamp is the skill's whole identity: it is what lets a
// call be attributed back to the client it was issued to.
func (tp *ToolProxy) IssueSkill(name string) string {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	stamp := tp.prefixedToolName()
	tp.Skills = append(tp.Skills, SkillRoute{Name: name, Stamp: stamp, Prefix: tp.Prefix, Epoch: tp.Epoch})
	return stamp
}

// BurnSkills invalidates every active stamp, reporting how many it burned.
func (tp *ToolProxy) BurnSkills() int {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	return tp.burnSkillsAt(tp.clock())
}

// burnSkillsAt is BurnSkills with the burn time supplied, so a clock-driven
// rotation can date the burn at the boundary the epoch crossed rather than at the
// moment the next call arrived.
func (tp *ToolProxy) burnSkillsAt(at time.Time) int {
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

// Rotate burns every active skill and moves this node's route to a fresh epoch.
// It is deliberately per-node: a single node can rotate without a cluster
// rotation, and the notification fans out from the observing mirror. Burning
// alone moves no score - only calls do.
func (tp *ToolProxy) Rotate() int {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	return tp.rotateTo(tp.Epoch+1, tp.clock())
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
// Legitimate callers should use this. It syncs the rotation clock first, so a
// long-lived process hands out the current epoch rather than the one it started
// in.
func (tp *ToolProxy) PrefixedToolName() string {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	return tp.prefixedToolName()
}

// prefixedToolName is PrefixedToolName for callers that already hold mu.
func (tp *ToolProxy) prefixedToolName() string {
	tp.syncEpoch()
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
	tp.mu.Lock()
	defer tp.mu.Unlock()

	tp.syncEpoch()
	expected := tp.prefixedToolName()

	// Check for exact match (expected call)
	if callName == expected {
		return true, ""
	}

	// Not using correct prefix - determine why and log it
	var reason string
	var likelyFuzzed, likelyDirect bool
	var attributed string
	callEpoch := int64(-1)

	// Attribution first: a call carrying a stamp we issued is ours, however
	// stale. A burned stamp inside the grace window still resolves, because the
	// notification may not have reached the caller yet.
	if skill, exact, ok := tp.attribute(callName); ok {
		attributed = skill.Name
		callEpoch = skill.Epoch
		switch {
		case !skill.Burned:
			reason = "skill-route-mismatch"
		case exact && tp.withinGrace(skill.BurnedAt):
			return true, "" // notification still in flight
		default:
			reason = "burned-skill-route"
		}
	}

	// A prefix this instance used to present, but no stamp names it: the caller
	// derived the route from the schedule and has not caught up. That is a
	// refresh problem, and it must not read as enumeration - a prefix that
	// matches nothing of ours is a guess, and the two must not share a reason or
	// a price.
	if reason == "" {
		if retired, ok := tp.retiredRoute(callName); ok {
			callEpoch = retired.Epoch
			if callName == retired.Prefix+tp.ToolName && tp.withinGrace(retired.RetiredAt) {
				return true, "" // client is one rotation behind, inside notification latency
			}
			reason = "stale-route-epoch"
		}
	}

	// The same problem with the sign flipped: a client whose clock runs ahead
	// derives the next epoch before we reach it. Its skew is bounded by the same
	// tolerance a lagging client gets, so it is attributable too - otherwise the
	// safe map would be a liability to every agent whose clock is fast.
	if reason == "" {
		callEpoch, reason = tp.aheadRoute(callName)
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
		Source:         tp.callSource(callName, attributed, likelyDirect, reason),
		Skill:          attributed,
		Epoch:          tp.Epoch,
		CallEpoch:      callEpoch,
	}

	tp.InvalidCalls = append(tp.InvalidCalls, suspiciousCall)

	if tp.HoneypotMode {
		fmt.Printf("[HONEYPOT] Tool proxy %s detected suspicious call: %q (%s, expected %q)\n",
			tp.ProxyID, callName, reason, expected)
	}

	return false, reason
}

// InvalidCount reports how many invalid calls this proxy has recorded.
func (tp *ToolProxy) InvalidCount() int {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	return len(tp.InvalidCalls)
}

// GetInvalidCallMetrics returns honeypot metrics for this proxy.
func (tp *ToolProxy) GetInvalidCallMetrics() ToolProxyMetrics {
	tp.mu.Lock()
	defer tp.mu.Unlock()

	totalInvalid := len(tp.InvalidCalls)
	fuzzAttempts := 0
	directCalls := 0
	skillRouteCalls := 0
	staleRouteCalls := 0

	for _, call := range tp.InvalidCalls {
		if call.LikelyFuzzed {
			fuzzAttempts++
		}
		switch call.Source {
		case "direct":
			directCalls++
		case "skill":
			skillRouteCalls++
		case "stale":
			staleRouteCalls++
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
	// Attributable staleness - a stamp we burned, or a prefix we retired - is a
	// refresh cost, not an attack signal, so it prices at the rotation tier.
	if attributable := skillRouteCalls + staleRouteCalls; attributable > 0 {
		distanceCost += attributable * burnedSkillCost
	}
	if rest := totalInvalid - fuzzAttempts - directCalls - skillRouteCalls - staleRouteCalls; rest > 0 {
		// Other invalid calls (wrong prefix): +16 each (boundary-violation)
		distanceCost += rest * 16
	}

	return ToolProxyMetrics{
		ProxyID:         tp.ProxyID,
		ToolName:        tp.ToolName,
		Prefix:          tp.Prefix,
		Epoch:           tp.Epoch,
		Rotations:       tp.Rotations,
		TotalInvalid:    totalInvalid,
		FuzzAttempts:    fuzzAttempts,
		DirectCalls:     directCalls,
		SkillRouteCalls: skillRouteCalls,
		StaleRouteCalls: staleRouteCalls,
		WrongPrefix:     totalInvalid - fuzzAttempts - directCalls - skillRouteCalls - staleRouteCalls,
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
	Epoch           int64 // rotation sequence the metrics were taken at
	Rotations       int   // how many times the route has moved
	TotalInvalid    int   // Total invalid calls
	FuzzAttempts    int   // Fuzzed/random prefix calls
	DirectCalls     int   // Direct calls (bypass proxy)
	SkillRouteCalls int   // Calls attributable to a burned skill (rotation cost)
	StaleRouteCalls int   // Calls carrying a retired epoch's prefix (rotation cost)
	WrongPrefix     int   // Wrong prefix
	Distance42i     int   // 42i cost from invalid calls
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
		"Tool proxy %s/%s [%s]: epoch=%d, %d invalid calls (fuzz=%d, direct=%d, skill=%d, stale=%d, wrong=%d) | distance=%d",
		tpm.ProxyID, tpm.ToolName, status, tpm.Epoch,
		tpm.TotalInvalid, tpm.FuzzAttempts, tpm.DirectCalls, tpm.SkillRouteCalls, tpm.StaleRouteCalls, tpm.WrongPrefix, tpm.Distance42i,
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

	// mu guards the counters above against concurrent validations of the same
	// cluster. The proxies themselves are created once and never replaced.
	mu sync.Mutex
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

	tpc.mu.Lock()
	recorded := proxy.InvalidCount()
	tpc.TotalInvalidCalls += recorded

	// Detect coordinated attack: multiple tools getting suspicious calls
	if !isCorrect {
		metrics = proxy.GetInvalidCallMetrics()

		// Mark as under attack if suspicious calls exceed threshold
		if recorded > 3 {
			tpc.AttackDetected = true
			tpc.SuspiciousTools = append(tpc.SuspiciousTools, toolName)
		}
	}
	tpc.mu.Unlock()

	return isCorrect, suspicion, metrics
}

// GetClusterMetrics returns aggregate honeypot metrics.
func (tpc *ToolProxyCluster) GetClusterMetrics() ClusterHoneypotMetrics {
	tpc.mu.Lock()
	defer tpc.mu.Unlock()

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
// shape: a stamped call is "skill" even though its name looks like fuzz, and a
// retired-epoch prefix is "stale" rather than "fuzz" for the same reason.
func (tp *ToolProxy) callSource(callName, attributed string, direct bool, reason string) string {
	switch {
	case attributed != "":
		return "skill"
	case reason == "stale-route-epoch" || reason == "ahead-route-epoch":
		return "stale"
	case direct:
		return "direct"
	default:
		return sourceFromCall(callName, tp.ToolName)
	}
}
