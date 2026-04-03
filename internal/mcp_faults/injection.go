package mcp_faults

import (
	"fmt"
	"math/rand"
	"time"
)

// FaultInjectionProfile defines which MCP failures to inject and how often.
type FaultInjectionProfile struct {
	Name        string
	Description string
	Faults      []FaultInjectionRule
	Random      *rand.Rand // Seeded RNG for reproducible tests
}

// FaultInjectionRule defines a specific fault and when to inject it.
type FaultInjectionRule struct {
	FailureType MCPFailureType
	Rate        float64 // 0.0-1.0: probability of injection
	Enabled     bool
	MaxRetries  int // How many times to retry before giving up
}

// NewFaultInjectionProfile creates a profile with a seeded RNG.
func NewFaultInjectionProfile(name string) *FaultInjectionProfile {
	return &FaultInjectionProfile{
		Name:   name,
		Faults: []FaultInjectionRule{},
		Random: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// ShouldInject returns true if a fault should be injected based on the rule's rate.
func (fip *FaultInjectionProfile) ShouldInject(rule FaultInjectionRule) bool {
	if !rule.Enabled {
		return false
	}
	return fip.Random.Float64() < rule.Rate
}

// String returns human-readable profile description.
func (fip *FaultInjectionProfile) String() string {
	enabledCount := 0
	for _, f := range fip.Faults {
		if f.Enabled {
			enabledCount++
		}
	}
	return fmt.Sprintf("%s: %d fault types, %d enabled", fip.Name, len(fip.Faults), enabledCount)
}

// Pre-built fault injection profiles for common MCP failure scenarios.

// NoFaultsProfile creates a profile with no faults (baseline).
func NoFaultsProfile() *FaultInjectionProfile {
	return NewFaultInjectionProfile("No Faults (Baseline)")
}

// TransientFailuresProfile injects occasional timeouts and partial responses.
func TransientFailuresProfile() *FaultInjectionProfile {
	profile := NewFaultInjectionProfile("Transient Failures")
	profile.Description = "Occasional timeouts and partial responses (10% each)"
	profile.Faults = []FaultInjectionRule{
		{
			FailureType: TimeoutFailure,
			Rate:        0.10,
			Enabled:     true,
			MaxRetries:  3, // Retryable
		},
		{
			FailureType: PartialResponseFailure,
			Rate:        0.10,
			Enabled:     true,
			MaxRetries:  3,
		},
	}
	return profile
}

// ProtocolViolationProfile injects corrupted and malformed responses.
func ProtocolViolationProfile() *FaultInjectionProfile {
	profile := NewFaultInjectionProfile("Protocol Violations")
	profile.Description = "Malformed JSON and corrupted responses (10% each)"
	profile.Faults = []FaultInjectionRule{
		{
			FailureType: CorruptedResponseFailure,
			Rate:        0.10,
			Enabled:     true,
			MaxRetries:  0, // Not retryable—indicates protocol violation
		},
		{
			FailureType: MalformedJSONFailure,
			Rate:        0.10,
			Enabled:     true,
			MaxRetries:  0,
		},
	}
	return profile
}

// ByzantineProfile injects deceptive failures: tool-not-found, capability mismatch.
func ByzantineProfile() *FaultInjectionProfile {
	profile := NewFaultInjectionProfile("Byzantine Behavior")
	profile.Description = "Deceptive failures: lying about capabilities (5% each)"
	profile.Faults = []FaultInjectionRule{
		{
			FailureType: ToolNotFoundFailure,
			Rate:        0.05,
			Enabled:     true,
			MaxRetries:  0, // Not retryable—indicates dishonesty
		},
		{
			FailureType: CapabilityMismatchFailure,
			Rate:        0.05,
			Enabled:     true,
			MaxRetries:  0,
		},
	}
	return profile
}

// StressProfile injects resource exhaustion (rate limiting, memory pressure).
func StressProfile() *FaultInjectionProfile {
	profile := NewFaultInjectionProfile("Resource Stress")
	profile.Description = "Server hitting limits (15% resource exhaustion)"
	profile.Faults = []FaultInjectionRule{
		{
			FailureType: ResourceExhaustionFailure,
			Rate:        0.15,
			Enabled:     true,
			MaxRetries:  3, // Retryable after cooldown
		},
	}
	return profile
}

// UnauthorizedAccessProfile injects permission/access errors.
func UnauthorizedAccessProfile() *FaultInjectionProfile {
	profile := NewFaultInjectionProfile("Unauthorized Access")
	profile.Description = "Server denying access to tools (10%)"
	profile.Faults = []FaultInjectionRule{
		{
			FailureType: UnauthorizedAccessFailure,
			Rate:        0.10,
			Enabled:     true,
			MaxRetries:  0, // Not retryable—indicates persistent restriction
		},
	}
	return profile
}

// ChaoticProfile injects all fault types frequently (worst-case scenario).
func ChaoticProfile() *FaultInjectionProfile {
	profile := NewFaultInjectionProfile("Chaotic/Hostile")
	profile.Description = "All fault types, high rates (20-40% each)"
	profile.Faults = []FaultInjectionRule{
		{
			FailureType: TimeoutFailure,
			Rate:        0.20,
			Enabled:     true,
			MaxRetries:  1,
		},
		{
			FailureType: CorruptedResponseFailure,
			Rate:        0.15,
			Enabled:     true,
			MaxRetries:  0,
		},
		{
			FailureType: ToolNotFoundFailure,
			Rate:        0.10,
			Enabled:     true,
			MaxRetries:  0,
		},
		{
			FailureType: UnauthorizedAccessFailure,
			Rate:        0.15,
			Enabled:     true,
			MaxRetries:  0,
		},
		{
			FailureType: ResourceExhaustionFailure,
			Rate:        0.25,
			Enabled:     true,
			MaxRetries:  1,
		},
	}
	return profile
}

// ProgressiveProfile simulates a server gradually becoming compromised.
// Starts clean, then introduces more faults over time.
type ProgressiveProfile struct {
	Name   string
	Stages []ProgressiveStage
}

type ProgressiveStage struct {
	Name    string
	Cycles  int
	Profile *FaultInjectionProfile
}

// NewProgressiveProfile creates a multi-stage failure scenario.
func NewProgressiveProfile(name string) *ProgressiveProfile {
	return &ProgressiveProfile{
		Name:   name,
		Stages: []ProgressiveStage{},
	}
}

// String returns the progressive profile description.
func (pp *ProgressiveProfile) String() string {
	return fmt.Sprintf("%s: %d stages", pp.Name, len(pp.Stages))
}

// CompromiseProgressionProfile shows how a server degrades over time.
func CompromiseProgressionProfile() *ProgressiveProfile {
	profile := NewProgressiveProfile("Server Compromise Progression")

	profile.Stages = []ProgressiveStage{
		{
			Name:    "Stage 1: Healthy (cycles 0-2)",
			Cycles:  3,
			Profile: NoFaultsProfile(),
		},
		{
			Name:    "Stage 2: Transient Issues (cycles 3-5)",
			Cycles:  3,
			Profile: TransientFailuresProfile(),
		},
		{
			Name:    "Stage 3: Protocol Violations (cycles 6-8)",
			Cycles:  3,
			Profile: ProtocolViolationProfile(),
		},
		{
			Name:    "Stage 4: Byzantine Behavior (cycles 9-11)",
			Cycles:  3,
			Profile: ByzantineProfile(),
		},
		{
			Name:    "Stage 5: Complete Failure (cycles 12-14)",
			Cycles:  3,
			Profile: ChaoticProfile(),
		},
	}

	return profile
}
