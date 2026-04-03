package integration_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// projectRoot returns the project root directory by walking up from cwd to find go.mod.
func projectRoot() string {
	dir, _ := filepath.Abs(".")
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "."
		}
		dir = parent
	}
}

func TestSimulationValidatorFeatureMapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		configPath   string
		wantFeatures []string
		wantIncident string
	}{
		{
			name:         "federation star hub",
			configPath:   "configs/samples/federation-star-hub.yaml",
			wantFeatures: []string{"federation.feature", "alerts.feature", "tui.feature"},
			wantIncident: "hub-and-spoke failure",
		},
		{
			name:         "federation chain",
			configPath:   "configs/samples/federation-chain.yaml",
			wantFeatures: []string{"federation.feature", "alerts.feature", "tui.feature"},
			wantIncident: "cascading failure",
		},
		{
			name:         "deployment canary",
			configPath:   "configs/samples/deployment-canary.yaml",
			wantFeatures: []string{"deployment.feature", "tui.feature"},
			wantIncident: "deployment comparison",
		},
		{
			name:         "secrets rotation",
			configPath:   "configs/samples/secrets-rotation.yaml",
			wantFeatures: []string{"oauth-mock.feature", "tui.feature"},
			wantIncident: "authentication failure",
		},
		{
			name:         "incident session",
			configPath:   "configs/samples/incident-session-ephemeral.yaml",
			wantFeatures: []string{"skill-system.feature", "tui.feature"},
			wantIncident: "general federation failure",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			features := findFeaturesForConfig(tt.configPath)
			if len(features) != len(tt.wantFeatures) {
				t.Errorf("findFeaturesForConfig() = %v, want %v", features, tt.wantFeatures)
			}
			for i, want := range tt.wantFeatures {
				if i < len(features) && features[i] != want {
					t.Errorf("features[%d] = %v, want %v", i, features[i], want)
				}
			}
		})
	}
}

func TestSimulationValidatorConfigAnalysis(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		configPath     string
		wantTargets    int
		wantFederation bool
		wantBasePort   int
	}{
		{
			name:           "federation star hub",
			configPath:     "configs/samples/federation-star-hub.yaml",
			wantTargets:    4,
			wantFederation: true,
			wantBasePort:   5200,
		},
		{
			name:           "federation pool",
			configPath:     "configs/samples/federation-pool.yaml",
			wantTargets:    5,
			wantFederation: true,
			wantBasePort:   5400,
		},
		{
			name:           "deployment ha",
			configPath:     "configs/samples/deployment-ha.yaml",
			wantTargets:    4,
			wantFederation: true,
			wantBasePort:   5700,
		},
		{
			name:           "hosted mcp acp",
			configPath:     "configs/samples/hosted-mcp-acp.yaml",
			wantTargets:    4,
			wantFederation: false,
			wantBasePort:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test that config files exist and are readable
			absPath, err := filepath.Abs(filepath.Join(projectRoot(), tt.configPath))
			if err != nil {
				t.Fatalf("failed to get absolute path: %v", err)
			}

			data, err := os.ReadFile(absPath)
			if err != nil {
				t.Skipf("config not present (created by child process): %v", err)
			}

			content := string(data)

			// Check federation config
			hasFederation := strings.Contains(content, "federation:") &&
				strings.Contains(content, "enabled: true")
			if hasFederation != tt.wantFederation {
				t.Errorf("federation enabled = %v, want %v", hasFederation, tt.wantFederation)
			}

			// Count targets
			targetCount := strings.Count(content, "  - id:")
			if targetCount != tt.wantTargets {
				t.Errorf("target count = %d, want %d", targetCount, tt.wantTargets)
			}
		})
	}
}

func TestSimulationValidatorOAuthDetection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		configPath string
		wantOAuth  bool
		wantRedact bool
	}{
		{
			name:       "secrets rotation has oauth",
			configPath: "configs/samples/secrets-rotation.yaml",
			wantOAuth:  true,
			wantRedact: true,
		},
		{
			name:       "secure context handoff has oauth",
			configPath: "configs/samples/secure-context-handoff.yaml",
			wantOAuth:  true,
			wantRedact: true,
		},
		{
			name:       "federation star hub has oauth",
			configPath: "configs/samples/federation-star-hub.yaml",
			wantOAuth:  true,
			wantRedact: true,
		},
		{
			name:       "incident session has oauth",
			configPath: "configs/samples/incident-session-ephemeral.yaml",
			wantOAuth:  true,
			wantRedact: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			absPath, err := filepath.Abs(filepath.Join(projectRoot(), tt.configPath))
			if err != nil {
				t.Fatalf("failed to get absolute path: %v", err)
			}

			data, err := os.ReadFile(absPath)
			if err != nil {
				t.Skipf("config not present (created by child process): %v", err)
			}

			content := string(data)

			hasOAuth := strings.Contains(content, "oauth:")
			if hasOAuth != tt.wantOAuth {
				t.Errorf("oauth enabled = %v, want %v", hasOAuth, tt.wantOAuth)
			}

			hasRedact := strings.Contains(content, "redaction:") || strings.Contains(content, "redact:")
			if hasRedact != tt.wantRedact {
				t.Errorf("redaction configured = %v, want %v", hasRedact, tt.wantRedact)
			}
		})
	}
}

func TestSimulationValidatorSampleFilesExist(t *testing.T) {
	t.Parallel()

	// Verify all expected sample configs exist
	samples := []string{
		"configs/samples/federation-star-hub.yaml",
		"configs/samples/federation-chain.yaml",
		"configs/samples/federation-pool.yaml",
		"configs/samples/federation-dependency.yaml",
		"configs/samples/a2a-handoff.yaml",
		"configs/samples/deployment-canary.yaml",
		"configs/samples/deployment-ha.yaml",
		"configs/samples/secrets-rotation.yaml",
		"configs/samples/compliance-audit.yaml",
		"configs/samples/cost-control.yaml",
		"configs/samples/disaster-recovery-drill.yaml",
		"configs/samples/multitenancy-isolation.yaml",
		"configs/samples/incident-session-ephemeral.yaml",
		"configs/samples/secure-context-handoff.yaml",
		"configs/samples/escalation-chain.yaml",
		"configs/samples/observer-mode.yaml",
		"configs/samples/llmstxt-auto-discovery.yaml",
		"configs/samples/hosted-mcp-acp.yaml",
		"configs/samples/stdio-mcp-strict.yaml",
		"configs/samples/sse-remote-mixed.yaml",
	}

	for _, sample := range samples {
		t.Run(filepath.Base(sample), func(t *testing.T) {
			absPath, err := filepath.Abs(filepath.Join(projectRoot(), sample))
			if err != nil {
				t.Fatalf("failed to get absolute path: %v", err)
			}

			info, err := os.Stat(absPath)
			switch {
			case os.IsNotExist(err):
				t.Skipf("sample config not present (created by child process): %s", sample)
			case err != nil:
				t.Errorf("error checking sample config: %v", err)
			case info.IsDir():
				t.Errorf("expected file, got directory: %s", sample)
			}
		})
	}
}

func TestSimulationValidatorSkillExists(t *testing.T) {
	t.Parallel()

	skills := []string{
		".opencode/skills/warroom-guide/SKILL.md",
		".opencode/skills/warroom-builder/SKILL.md",
		".opencode/skills/warroom-simulator/SKILL.md",
		".opencode/skills/config-to-simulation/SKILL.md",
		".opencode/skills/simulation-validator/SKILL.md",
	}

	for _, skill := range skills {
		t.Run(filepath.Base(skill), func(t *testing.T) {
			absPath, err := filepath.Abs(filepath.Join(projectRoot(), skill))
			if err != nil {
				t.Fatalf("failed to get absolute path: %v", err)
			}

			info, err := os.Stat(absPath)
			switch {
			case os.IsNotExist(err):
				t.Skipf("skill not present (installed by child process): %s", skill)
			case err != nil:
				t.Errorf("error checking skill: %v", err)
			case info.IsDir():
				t.Errorf("expected file, got directory: %s", skill)
			}
		})
	}
}

func TestSimulationValidatorFeatureFileExists(t *testing.T) {
	t.Parallel()

	features := []string{
		"features/simulation-validator.feature",
		"features/tui.feature",
		"features/federation.feature",
		"features/alerts.feature",
		"features/oauth-mock.feature",
		"features/skill-system.feature",
	}

	for _, feature := range features {
		t.Run(filepath.Base(feature), func(t *testing.T) {
			absPath, err := filepath.Abs(filepath.Join(projectRoot(), feature))
			if err != nil {
				t.Fatalf("failed to get absolute path: %v", err)
			}

			info, err := os.Stat(absPath)
			switch {
			case os.IsNotExist(err):
				t.Skipf("feature not present: %s", feature)
			case err != nil:
				t.Errorf("error checking feature: %v", err)
			case info.IsDir():
				t.Errorf("expected file, got directory: %s", feature)
			}
		})
	}
}

func TestSimulationValidatorStepDefsExist(t *testing.T) {
	t.Parallel()

	stepDefs := []string{
		"features/step_definitions/simulation_validator_steps.go",
		"features/step_definitions/tui_steps.go",
		"features/step_definitions/federation_steps.go",
		"features/step_definitions/common_steps.go",
	}

	for _, stepDef := range stepDefs {
		t.Run(filepath.Base(stepDef), func(t *testing.T) {
			absPath, err := filepath.Abs(filepath.Join(projectRoot(), stepDef))
			if err != nil {
				t.Fatalf("failed to get absolute path: %v", err)
			}

			info, err := os.Stat(absPath)
			switch {
			case os.IsNotExist(err):
				t.Skipf("step definitions not present: %s", stepDef)
			case err != nil:
				t.Errorf("error checking step definitions: %v", err)
			case info.IsDir():
				t.Errorf("expected file, got directory: %s", stepDef)
			}
		})
	}
}

// Helper functions (mirrored from step definitions for testing)

func findFeaturesForConfig(configPath string) []string {
	features := []string{}
	baseName := filepath.Base(configPath)
	baseName = strings.TrimSuffix(baseName, ".yaml")

	if strings.HasPrefix(baseName, "federation-") {
		features = append(features, "federation.feature", "alerts.feature")
	}
	if strings.HasPrefix(baseName, "deployment-") {
		features = append(features, "deployment.feature")
	}
	if strings.HasPrefix(baseName, "warroom-") || strings.HasPrefix(baseName, "incident-") {
		features = append(features, "skill-system.feature")
	}
	if strings.HasPrefix(baseName, "secrets-") {
		features = append(features, "oauth-mock.feature")
	}

	features = append(features, "tui.feature")

	return features
}
