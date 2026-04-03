package stepdefinitions

// simulation_validator_steps.go — step definitions for features/simulation-validator.feature
// see: common_steps.go for shared steps

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
)

func InitializeSimulationValidatorScenario(ctx *godog.ScenarioContext) {
	// config input
	ctx.Step(`^the config file "([^"]*)" exists$`, configFileExists)
	ctx.Step(`^the config contains (\d+) targets$`, configContainsNTargets)
	ctx.Step(`^the config has federation enabled with base_port (\d+)$`, configHasFederationWithBasePort)
	ctx.Step(`^the config has alerts with aggressive set to (true|false)$`, configHasAggressiveAlerts)
	ctx.Step(`^the config has OAuth enabled with token_grace_period "([^"]*)"$`, configHasOAuthEnabled)

	// feature mapping
	ctx.Step(`^the validator discovers relevant feature files$`, validatorDiscoversFeatures)
	ctx.Step(`^the validator maps targets to federation feature steps$`, validatorMapsTargetsToFederation)
	ctx.Step(`^the validator finds (\d+) matching scenarios$`, validatorFindsMatchingScenarios)

	// validation
	ctx.Step(`^the simulation has participant for target "([^"]*)"$`, simulationHasParticipantForTarget)
	ctx.Step(`^the feature step "([^"]*)" exists$`, featureStepExists)
	ctx.Step(`^the validation result is "([^"]*)"$`, validationResultIs)

	// secure context verification
	ctx.Step(`^the validator checks OAuth configuration$`, validatorChecksOAuth)
	ctx.Step(`^the validator verifies scope "([^"]*)" is properly scoped$`, validatorVerifiesScope)
	ctx.Step(`^the validator checks redaction is enabled$`, validatorChecksRedaction)
	ctx.Step(`^the OAuth provider "([^"]*)" is marked as (OK|WARNING|FAIL)$`, oauthProviderMarkedAs)
	ctx.Step(`^scope "([^"]*)" is granted to (\d+) targets$`, scopeGrantedToNTargets)

	// gaps
	ctx.Step(`^the validator identifies (\d+) gaps from simulation to feature$`, validatorIdentifiesGapsSimulationToFeature)
	ctx.Step(`^the validator identifies (\d+) gaps from feature to simulation$`, validatorIdentifiesGapsFeatureToSimulation)
	ctx.Step(`^a gap exists: "([^"]*)"$`, gapExists)

	// TUI integration
	ctx.Step(`^the validator produces TUI integration guidance$`, validatorProducesTUIGuidance)
	ctx.Step(`^the TUI command is generated for the config$`, tuiCommandGenerated)
	ctx.Step(`^the keyboard shortcuts are documented$`, keyboardShortcutsDocumented)
	ctx.Step(`^the TUI elements map to feature scenarios$`, tuiElementsMapToFeatureScenarios)

	// validation report
	ctx.Step(`^the validator generates a validation report$`, validatorGeneratesReport)
	ctx.Step(`^the report includes feature coverage section$`, reportIncludesFeatureCoverage)
	ctx.Step(`^the report includes security verification section$`, reportIncludesSecuritySection)
	ctx.Step(`^the report includes TUI observation section$`, reportIncludesTUISection)
	ctx.Step(`^the overall validation status is "([^"]*)"$`, overallValidationStatusIs)

	// voice modes
	ctx.Step(`^the simulation is requested in warroom voice$`, simulationRequestedWarroomVoice)
	ctx.Step(`^the simulation is requested in mentor voice$`, simulationRequestedMentorVoice)
	ctx.Step(`^the output uses urgent command language$`, outputUsesUrgentCommandLanguage)
	ctx.Step(`^the output uses explanatory language with pauses$`, outputUsesExplanatoryLanguage)

	// report output
	ctx.Step(`^the report shows (\d+) passed validations$`, reportShowsPassedValidations)
	ctx.Step(`^the report shows (\d+) failed validations$`, reportShowsFailedValidations)

	// ── additional patterns ─────────────────────────────────────────────────
	ctx.Step(`^the validator reads the config file$`, theValidatorReadsTheConfigFile)
	ctx.Step(`^extracts service parameters$`, extractsServiceParameters)
	ctx.Step(`^the validator parses the config$`, theValidatorParsesTheConfig)
	ctx.Step(`^the validator performs gap analysis$`, theValidatorPerformsGapAnalysis)
	ctx.Step(`^the validator runs on that simulation$`, theValidatorRunsOnThatSimulation)
	ctx.Step(`^a simulation was generated from config "([^"]*)" in warroom mode$`, aSimulationWasGeneratedFromConfigInWarroomMode)
	ctx.Step(`^a config with (\d+) targets exists$`, aConfigWithTargetsExists)
	ctx.Step(`^a config with no matching features exists$`, aConfigWithNoMatchingFeaturesExists)
	ctx.Step(`^the redaction mask is "([^"]*)"$`, theRedactionMaskIs)
	ctx.Step(`^the validator identifies gaps from simulation to feature$`, theValidatorIdentifiesGapsFromSimulationToFeature)
	ctx.Step(`^the validator identifies gaps from feature to simulation$`, theValidatorIdentifiesGapsFromFeatureToSimulation)
}

// Step implementations

func configFileExists(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		return godog.ErrPending
	}
	return nil
}

func configContainsNTargets(n int) error {
	// This would parse the YAML and count targets
	// For now, return pending
	return godog.ErrPending
}

func configHasFederationWithBasePort(port int) error {
	return godog.ErrPending
}

func configHasAggressiveAlerts(aggressive string) error {
	return godog.ErrPending
}

func configHasOAuthEnabled(gracePeriod string) error {
	return godog.ErrPending
}

func validatorDiscoversFeatures() error {
	return godog.ErrPending
}

func validatorMapsTargetsToFederation() error {
	return godog.ErrPending
}

func validatorFindsMatchingScenarios(n int) error {
	return godog.ErrPending
}

func simulationHasParticipantForTarget(target string) error {
	return godog.ErrPending
}

func featureStepExists(step string) error {
	// Check if step exists in relevant feature files
	return godog.ErrPending
}

func validationResultIs(result string) error {
	return godog.ErrPending
}

func validatorChecksOAuth() error {
	return godog.ErrPending
}

func validatorVerifiesScope(scope string) error {
	return godog.ErrPending
}

func validatorChecksRedaction() error {
	return godog.ErrPending
}

func oauthProviderMarkedAs(provider, status string) error {
	return godog.ErrPending
}

func scopeGrantedToNTargets(scope string, n int) error {
	return godog.ErrPending
}

func validatorIdentifiesGapsSimulationToFeature(n int) error {
	return godog.ErrPending
}

func validatorIdentifiesGapsFeatureToSimulation(n int) error {
	return godog.ErrPending
}

func gapExists(description string) error {
	return godog.ErrPending
}

func validatorProducesTUIGuidance() error {
	return godog.ErrPending
}

func tuiCommandGenerated() error {
	return godog.ErrPending
}

func keyboardShortcutsDocumented() error {
	return godog.ErrPending
}

func tuiElementsMapToFeatureScenarios() error {
	return godog.ErrPending
}

func validatorGeneratesReport() error {
	return godog.ErrPending
}

func reportIncludesFeatureCoverage() error {
	return godog.ErrPending
}

func reportIncludesSecuritySection() error {
	return godog.ErrPending
}

func reportIncludesTUISection() error {
	return godog.ErrPending
}

func overallValidationStatusIs(status string) error {
	return godog.ErrPending
}

func simulationRequestedWarroomVoice() error {
	return godog.ErrPending
}

func simulationRequestedMentorVoice() error {
	return godog.ErrPending
}

func outputUsesUrgentCommandLanguage() error {
	return godog.ErrPending
}

func outputUsesExplanatoryLanguage() error {
	return godog.ErrPending
}

func reportShowsPassedValidations(n int) error {
	return godog.ErrPending
}

func reportShowsFailedValidations(n int) error {
	return godog.ErrPending
}

func theValidatorReadsTheConfigFile() error                         { return godog.ErrPending }
func extractsServiceParameters() error                              { return godog.ErrPending }
func theValidatorParsesTheConfig() error                            { return godog.ErrPending }
func theValidatorPerformsGapAnalysis() error                        { return godog.ErrPending }
func theValidatorRunsOnThatSimulation() error                       { return godog.ErrPending }
func aSimulationWasGeneratedFromConfigInWarroomMode(c string) error { return godog.ErrPending }
func aConfigWithTargetsExists(n int) error                          { return godog.ErrPending }
func aConfigWithNoMatchingFeaturesExists() error                    { return godog.ErrPending }
func theRedactionMaskIs(mask string) error                          { return godog.ErrPending }
func theValidatorIdentifiesGapsFromSimulationToFeature() error      { return godog.ErrPending }
func theValidatorIdentifiesGapsFromFeatureToSimulation() error      { return godog.ErrPending }

// Helper functions for validation logic

func findFeaturesForConfig(configPath string) []string {
	// Parse config to determine relevant features
	// Federation configs → federation.feature
	// Deployment configs → deployment.feature
	// etc.
	features := []string{}

	baseName := filepath.Base(configPath)
	baseName = strings.TrimSuffix(baseName, ".yaml")

	if strings.HasPrefix(baseName, "federation-") {
		features = append(features, "federation.feature")
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

	// Always include TUI for runtime validation
	features = append(features, "tui.feature")

	return features
}

func mapConfigToIncidentType(configPath string) string {
	baseName := filepath.Base(configPath)
	baseName = strings.TrimSuffix(baseName, ".yaml")

	switch {
	case strings.Contains(baseName, "star"):
		return "hub-and-spoke failure"
	case strings.Contains(baseName, "chain"):
		return "cascading failure"
	case strings.Contains(baseName, "pool"):
		return "pool depletion"
	case strings.Contains(baseName, "secrets"):
		return "authentication failure"
	case strings.Contains(baseName, "canary"):
		return "deployment comparison"
	default:
		return "general federation failure"
	}
}
