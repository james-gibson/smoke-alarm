package stepdefinitions

// federation_strict_test.go — runs features/federation.feature on its own with
// godog Strict enabled.
//
// The repository-wide suite in suite_test.go deliberately tolerates pending and
// undefined steps, which is how #38 hid a feature whose every scenario returned
// godog.ErrPending. This run has no such tolerance: a pending, undefined or
// ambiguous step in the federation feature fails the build. Tag @wip scenarios
// are excluded, and every exclusion must point at the test that covers it.

import (
	"testing"

	"github.com/cucumber/godog"
)

func TestFederationFeaturesStrict(t *testing.T) {
	suite := godog.TestSuite{
		ScenarioInitializer: func(ctx *godog.ScenarioContext) {
			InitializeCommonSteps(ctx)
			InitializeFederationScenario(ctx)
		},
		Options: &godog.Options{
			Format: "pretty",
			Paths:  []string{"../../features/federation.feature"},
			Tags:   "~@wip",
			Strict: true,
		},
	}
	if suite.Run() != 0 {
		t.Fatal("non-zero exit from the strict federation feature run")
	}
}
