package stepdefinitions

import (
	"testing"

	"github.com/cucumber/godog"
)

func TestFeatures(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: func(ctx *godog.ScenarioContext) {
			InitializeCommonSteps(ctx)
			InitializeConfigValidationScenario(ctx)
			InitializeEngineSteps(ctx)
			InitializeTargetsSteps(ctx)
			InitializeKnownStateSteps(ctx)
			InitializeHealthSteps(ctx)
			InitializeStdioMCPSteps(ctx)
			InitializeDiscoveryLlmstxtSteps(ctx)
			InitializeSSETransportSteps(ctx)
			InitializeSafetyHURLSteps(ctx)
			InitializeOAuthMockSteps(ctx)
			InitializeHostedServerSteps(ctx)
			InitializeDynamicConfigSteps(ctx)
			InitializeAlertsSteps(ctx)
			InitializeSelfDescriptionSteps(ctx)
			InitializeSkillSystemSteps(ctx)
			InitializeMetaConfigSteps(ctx)
			InitializeFederationScenario(ctx)
			InitializeFederatedSkillsScenario(ctx)
			InitializeOpsScenario(ctx)
			InitializeTelemetryScenario(ctx)
			InitializeTUIScenario(ctx)
			InitializeTunerIntegrationSteps(ctx)
			InitializeOpenThePickleJarScenario(ctx)
			InitializeSimulationValidatorScenario(ctx)
			InitializeStartHereSteps(ctx)
			InitializeDemoCapabilitiesSteps(ctx)
			InitializeOpencodeStatusReportSteps(ctx)
			InitializeWarroomBuilderSteps(ctx)
			InitializeWarroomGuideSteps(ctx)
			InitializeWarroomSimulatorSteps(ctx)
			InitializeConfigToSimulationSteps(ctx)
			InitializeMDNSSteps(ctx)
		},
		// Tag filter presets:
		//   core only:   Tags: "@core && ~@wip"
		//   skills only: Tags: "@skill && ~@wip"
		//   full suite:  Tags: "~@wip"   ← current
		Options: &godog.Options{
			Format: "pretty",
			Paths:  []string{"../.."},
			Tags:   "~@wip",
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero exit from godog suite")
	}
}
