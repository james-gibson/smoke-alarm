# MCP/ACP Fault Injection: Testing Server Resilience Through 42i Distance

**Purpose**: Inject systematic failures into MCP/ACP protocol interactions and measure agent resilience via 42i_distance accumulation. Detect Byzantine MCP servers through failure pattern analysis.

## Overview

When an agent interacts with MCP/ACP servers, failures don't just cause retries—they're **42i events** that indicate server reliability issues:

- **Timeout** → unpredictable-behavior (distance: +8)
- **Unauthorized Access** → unauthorized-access (distance: +24)
- **Corrupted Response** → boundary-violation (distance: +32)
- **Tool Not Found** → coordinated-signaling / Byzantine (distance: +40)
- **Capability Mismatch** → coordinated-signaling / Byzantine (distance: +48)

An agent tracks each MCP server's accumulated distance. High distance = untrusted server = candidate for eviction.

## Core Concepts

### MCPFailureType

Each failure type maps to a 42i direction and weight:

```go
const (
    TimeoutFailure              // 8 distance, unpredictable-behavior
    UnauthorizedAccessFailure   // 24 distance, unauthorized-access
    CorruptedResponseFailure    // 32 distance, boundary-violation
    MalformedJSONFailure        // 32 distance, boundary-violation
    ToolNotFoundFailure         // 40 distance, coordinated-signaling (Byzantine!)
    ResourceExhaustionFailure   // 16 distance, boundary-violation
    PartialResponseFailure      // 12 distance, unpredictable-behavior
    CapabilityMismatchFailure   // 48 distance, coordinated-signaling (Byzantine!)
)
```

### FaultInjectionProfile

A profile defines which failures to inject and how often:

```go
profile := mcp_faults.TransientFailuresProfile()
// 10% timeout rate, 10% partial response rate
```

Pre-built profiles:
- `NoFaultsProfile()` — Baseline (no failures)
- `TransientFailuresProfile()` — Occasional timeouts/partial responses
- `ProtocolViolationProfile()` — Corrupted/malformed responses
- `ByzantineProfile()` — Deceptive failures (tool-not-found, capability mismatch)
- `StressProfile()` — Resource exhaustion (rate limiting)
- `UnauthorizedAccessProfile()` — Permission errors
- `ChaoticProfile()` — All fault types, high rates (worst-case)

### Progressive Compromise

```go
profile := mcp_faults.CompromiseProgressionProfile()
// 5 stages: Healthy → Transient → Protocol Violations → Byzantine → Complete Failure
```

Shows how an MCP server degrades over time. Each stage adds more fault types.

## Quick Start

### Example 1: Detect a Byzantine MCP Server

```go
agent := isotope.NewAgentState("test-agent")

// Simulate MCP server failures
failures := []mcp_faults.MCPFailureType{
    mcp_faults.TimeoutFailure,           // +8 distance
    mcp_faults.CorruptedResponseFailure, // +32 distance
    mcp_faults.ToolNotFoundFailure,      // +40 distance (Byzantine!)
}

for _, failureType := range failures {
    mode := mcp_faults.GetFailureMode(failureType)

    event := isotope.MCPFailureEvent{
        FailureType:    string(failureType),
        ServerID:       "mcp-server-1",
        ToolName:       "some-tool",
        MethodName:     "tools/call",
        DistanceWeight: mode.DistanceWeight,
        Direction:      mode.Direction,
        Severity:       mode.Severity,
        Recoverable:    mode.Recoverable,
    }

    agent.RecordMCPFailure(event)
}

// Result: agent.TotalDistance = 80, indicates Byzantine behavior
if agent.TotalDistance > 60 {
    fmt.Println("Server is compromised—evict it")
}
```

### Example 2: Test Agent Resilience Against Progressive Compromise

```go
agent := isotope.NewAgentState("resilient-agent")
profile := mcp_faults.CompromiseProgressionProfile()

for stage in profile.Stages {
    for cycle := 0; cycle < stage.Cycles; cycle++ {
        // Simulate MCP calls with fault injection
        for rule in stage.Profile.Faults {
            if stage.Profile.ShouldInject(rule) {
                // Record failure and update agent distance
                event := isotope.MCPFailureEvent{...}
                agent.RecordMCPFailure(event)
            }
        }
    }

    fmt.Printf("Stage %d: distance=%d, rung=%d\n",
        stage.Name, agent.TotalDistance, agent.Position.Rung)
}

// Track when agent recognizes the server as untrustworthy
if agent.TotalDistance > 100 {
    fmt.Println("Agent correctly detected progressive compromise")
}
```

### Example 3: Rank Servers by Health

```go
servers := map[string]*isotope.AgentState{
    "primary":      isotope.NewAgentState("primary"),
    "secondary":    isotope.NewAgentState("secondary"),
    "tertiary":     isotope.NewAgentState("tertiary"),
}

// Record failures for each server
// (in real scenario, these come from actual MCP interactions)
recordFailures(servers["secondary"], TransientFailuresProfile())
recordFailures(servers["tertiary"], ByzantineProfile())

// Rank by health
type ServerHealth struct {
    ID       string
    Distance int
    Status   string
}

healths := []ServerHealth{}
for id, agent := range servers {
    status := "HEALTHY"
    if agent.TotalDistance > 60 {
        status = "COMPROMISED"
    } else if agent.TotalDistance > 30 {
        status = "DEGRADED"
    }

    healths = append(healths, ServerHealth{
        ID:       id,
        Distance: agent.TotalDistance,
        Status:   status,
    })
}

// Use primary when possible, skip compromised servers
// Healthy:      primary (distance=0)
// Degraded:     secondary (distance=16, retry with backoff)
// Compromised:  tertiary (distance=80, evict)
```

## Understanding Failure Types

### Transient Failures (Recoverable)

**Timeout** (+8 distance, 1 retry)
- Server didn't respond within SLA
- Indicates: unpredictable-behavior (latency variability)
- Suggests: temporary overload or network issue
- Action: Retry with exponential backoff

**Partial Response** (+12 distance, 3 retries)
- Connection dropped mid-stream
- Indicates: unpredictable-behavior (unreliable delivery)
- Suggests: network instability or crash
- Action: Retry and reconnect

**Resource Exhaustion** (+16 distance, 3 retries)
- Server hit rate limits or memory pressure
- Indicates: boundary-violation (exceeding contract)
- Suggests: server under stress (load spike, memory leak)
- Action: Backoff significantly, may recover

### Protocol Failures (Non-recoverable)

**Corrupted Response** (+32 distance, no retry)
- Server response violated JSON-RPC protocol
- Indicates: boundary-violation (protocol violation)
- Suggests: serious issue (corruption, wrong endpoint, compromise)
- Action: Stop using this server immediately

**Malformed JSON** (+32 distance, no retry)
- JSON parsing failed
- Indicates: boundary-violation (invalid format)
- Suggests: server bug or protocol mismatch
- Action: Investigate and skip server

### Byzantine Failures (Critical, Non-recoverable)

**Tool Not Found** (+40 distance, no retry)
- Server advertised tool but then reported not found
- Indicates: coordinated-signaling / Byzantine (dishonesty)
- Suggests: intentional deception or major state confusion
- Action: **Evict immediately**—server cannot be trusted

**Capability Mismatch** (+48 distance, no retry)
- Server's actual capabilities don't match advertisement
- Indicates: coordinated-signaling / Byzantine (deception)
- Suggests: server is lying about what it can do
- Action: **Evict immediately**—server is unreliable

**Unauthorized Access** (+24 distance, no retry)
- Server denied access to requested tool/resource
- Indicates: unauthorized-access (permission violation)
- Suggests: permission restriction or access control change
- Action: Check permissions; if intentional, skip server

## Eviction Policy

Based on 42i distance accumulation:

| Distance | Status | Action |
|----------|--------|--------|
| 0-20 | Healthy | Use normally |
| 21-60 | Degraded | Use with caution, retry with backoff |
| 61-100 | Concerning | Investigate, prefer other servers |
| >100 | Compromised | Evict (remove from pool) |

Special case: Any **Byzantine failure** (tool-not-found, capability-mismatch) immediately marks server as untrusted, regardless of total distance.

## Measuring Agent Resilience

Use the fault profiles to stress-test your agent:

```go
func BenchmarkAgentResilience(b *testing.B) {
    profiles := map[string]*mcp_faults.FaultInjectionProfile{
        "Transient":    mcp_faults.TransientFailuresProfile(),
        "Protocol":     mcp_faults.ProtocolViolationProfile(),
        "Byzantine":    mcp_faults.ByzantineProfile(),
        "Chaotic":      mcp_faults.ChaoticProfile(),
    }

    for name, profile := range profiles {
        b.Run(name, func(b *testing.B) {
            agent := isotope.NewAgentState("benchmark")

            for i := 0; i < b.N; i++ {
                for _, rule := range profile.Faults {
                    if profile.ShouldInject(rule) {
                        mode := mcp_faults.GetFailureMode(rule.FailureType)
                        event := isotope.MCPFailureEvent{...}
                        agent.RecordMCPFailure(event)
                    }
                }
            }

            b.ReportMetric(float64(agent.TotalDistance), "distance")
        })
    }
}
```

## Integration with Fire-Marshal

**In development (dev mode)**: Use fault injection profiles to validate retry logic and error handling before production.

**In production (fire-marshal)**: Use 42i distance thresholds to:
1. Monitor MCP server health in real-time
2. Automatically degrade servers to secondary pool when distance > threshold
3. Evict servers with Byzantine failures immediately
4. Prefer healthy servers (distance=0) for critical operations

## Testing Checklist

- [ ] Agent correctly accumulates distance from each failure type
- [ ] Direction is inferred correctly (transient vs. Byzantine vs. restricted)
- [ ] Eviction threshold triggers at distance > 60
- [ ] Recovery works: agent reconnects after backoff
- [ ] Byzantine failures trigger immediate eviction
- [ ] Agent prefers healthy servers over degraded ones
- [ ] No sensitive data in error logs/messages

## See Also

- **42I_INTEGRATION.md** — How 42i distance and rungs work
- **CHAOS.md** — Service-level chaos testing for isotopes
- **DEVMODE.md** — Dev mode simulator for testing clusters
