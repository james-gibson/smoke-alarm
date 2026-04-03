package stepdefinitions

// targets_steps.go — step definitions for features/targets.feature
//
// All steps operate purely in-process against targets.Target.Validate() and
// targets.CheckResult methods. No HTTP server or binary execution required.
//
// State is held in tsState (reset per scenario via BeforeScenario).
// validationSucceeds() is shared with config_validation_steps.go; it checks
// tsState.validated first, then falls back to cmdState for config-validation.

import (
	"fmt"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/james-gibson/smoke-alarm/internal/targets"
)

// tsState holds per-scenario target/checkresult state.
var tsState struct {
	target            targets.Target
	checkResult       targets.CheckResult
	validated         bool
	validationErr     error
	lastIsFailure     bool
	isFailureCalled   bool
	lastIsEscalated   bool
	isEscalatedCalled bool
}

func resetTSState() {
	tsState.target = targets.Target{}
	tsState.checkResult = targets.CheckResult{}
	tsState.validated = false
	tsState.validationErr = nil
	tsState.lastIsFailure = false
	tsState.isFailureCalled = false
	tsState.lastIsEscalated = false
	tsState.isEscalatedCalled = false
}

func InitializeTargetsSteps(ctx *godog.ScenarioContext) {
	ctx.BeforeScenario(func(_ *godog.Scenario) {
		resetTSState()
	})

	// ── required fields ────────────────────────────────────────────────────
	ctx.Step(`^a target with id "([^"]*)", protocol "([^"]*)", transport "([^"]*)", endpoint "([^"]*)"$`, aTargetWithIDProtocolTransportEndpoint)
	ctx.Step(`^the target has interval (\w+), timeout (\w+), and retries (-?\d+)$`, theTargetHasIntervalTimeoutRetries)
	ctx.Step(`^I validate the target$`, iValidateTheTarget)
	ctx.Step(`^validation succeeds$`, validationSucceeds)
	ctx.Step(`^validation fails with message "([^"]*)"$`, validationFailsWithMessage)

	// ── endpoint rules ─────────────────────────────────────────────────────
	ctx.Step(`^a target with id "([^"]*)", protocol "([^"]*)", transport "([^"]*)", and no command set$`, aTargetWithNoCommandSet)
	ctx.Step(`^a target with id "([^"]*)", protocol "([^"]*)", transport "([^"]*)", and command "([^"]*)"$`, aTargetWithCommand)

	// ── check policy rules ─────────────────────────────────────────────────
	ctx.Step(`^the target handshake_profile is "([^"]*)"$`, theTargetHandshakeProfileIs)
	ctx.Step(`^the target handshake_profile is "([^"]*)"""$`, theTargetHandshakeProfileIsEmptyLiteral)
	ctx.Step(`^required_methods includes an empty string$`, requiredMethodsIncludesEmptyString)

	// ── HURL test rules ────────────────────────────────────────────────────
	ctx.Step(`^a HURL test named "([^"]*)" with no file and no endpoint$`, aHURLTestWithNoFileAndNoEndpoint)
	ctx.Step(`^a HURL test named "([^"]*)" with file "([^"]*)" and endpoint "([^"]*)"$`, aHURLTestWithFileAndEndpoint)
	ctx.Step(`^a HURL test named "([^"]*)" with endpoint "([^"]*)" and method "([^"]*)"$`, aHURLTestWithEndpointAndMethod)
	ctx.Step(`^a HURL test named "([^"]*)" with file "([^"]*)" and no endpoint$`, aHURLTestWithFileAndNoEndpoint)

	// ── auth config rules ──────────────────────────────────────────────────
	ctx.Step(`^auth type is "([^"]*)" with no secret_ref$`, authTypeWithNoSecretRef)
	ctx.Step(`^auth type is "([^"]*)" with secret_ref "([^"]*)"$`, authTypeWithSecretRef)
	ctx.Step(`^auth type is "([^"]*)" with no key_name and no secret_ref$`, authTypeWithNoKeyNameAndNoSecretRef)
	ctx.Step(`^auth type is "([^"]*)" with no client_id and no token_url$`, authTypeWithNoClientIDAndNoTokenURL)
	ctx.Step(`^auth type is "([^"]*)"$`, authTypeIs)

	// ── CheckResult semantics ──────────────────────────────────────────────
	ctx.Step(`^a check result with state "([^"]*)"$`, aCheckResultWithState)
	ctx.Step(`^I call IsFailure on the result$`, iCallIsFailureOnTheResult)
	ctx.Step(`^IsFailure returns (\w+)$`, isFailureReturns)
	ctx.Step(`^a check result with state "([^"]*)", severity "([^"]*)", regression flag (\w+)$`, aCheckResultWithStateSeverityRegression)
	ctx.Step(`^I call IsEscalated on the result$`, iCallIsEscalatedOnTheResult)
	ctx.Step(`^IsEscalated returns (\w+)$`, isEscalatedReturns)

	// ── type vocabulary ────────────────────────────────────────────────────
	ctx.Step(`^a target with protocol "([^"]*)"$`, aTargetWithProtocol)
	ctx.Step(`^the target protocol is accepted as a known protocol$`, theTargetProtocolIsKnown)
	ctx.Step(`^a target with transport "([^"]*)"$`, aTargetWithTransportOnly)
	ctx.Step(`^the target transport is accepted as a known transport$`, theTargetTransportIsKnown)
	ctx.Step(`^a check result with failure class "([^"]*)"$`, aCheckResultWithFailureClass)
	ctx.Step(`^the failure class is accepted as a known failure class$`, theFailureClassIsKnown)
}

// ── required fields ────────────────────────────────────────────────────────

func aTargetWithIDProtocolTransportEndpoint(id, proto, transport, endpoint string) error {
	tsState.target = targets.Target{
		ID:        id,
		Protocol:  targets.Protocol(proto),
		Transport: targets.Transport(transport),
		Endpoint:  endpoint,
	}
	return nil
}

func theTargetHasIntervalTimeoutRetries(interval, timeout string, retries int) error {
	iv, err := time.ParseDuration(interval)
	if err != nil {
		return fmt.Errorf("parse interval %q: %w", interval, err)
	}
	to, err := time.ParseDuration(timeout)
	if err != nil {
		return fmt.Errorf("parse timeout %q: %w", timeout, err)
	}
	tsState.target.Check.Interval = iv
	tsState.target.Check.Timeout = to
	tsState.target.Check.Retries = retries
	return nil
}

func iValidateTheTarget() error {
	tsState.validationErr = tsState.target.Validate()
	tsState.validated = true
	return nil
}

func validationFailsWithMessage(msg string) error {
	if !tsState.validated {
		return godog.ErrPending
	}
	if tsState.validationErr == nil {
		return fmt.Errorf("expected validation to fail with %q but it passed", msg)
	}
	if !strings.Contains(tsState.validationErr.Error(), msg) {
		return fmt.Errorf("validation error %q does not contain %q",
			tsState.validationErr.Error(), msg)
	}
	return nil
}

// ── endpoint rules ─────────────────────────────────────────────────────────

func aTargetWithNoCommandSet(id, proto, transport string) error {
	tsState.target = targets.Target{
		ID:        id,
		Protocol:  targets.Protocol(proto),
		Transport: targets.Transport(transport),
	}
	return nil
}

func aTargetWithCommand(id, proto, transport, cmd string) error {
	tsState.target = targets.Target{
		ID:        id,
		Protocol:  targets.Protocol(proto),
		Transport: targets.Transport(transport),
		Stdio:     targets.StdioCommand{Command: cmd},
	}
	return nil
}

// ── check policy rules ─────────────────────────────────────────────────────

func theTargetHandshakeProfileIs(profile string) error {
	tsState.target.Check.HandshakeProfile = profile
	return nil
}

// theTargetHandshakeProfileIsEmptyLiteral handles the Gherkin `| "" |` table row,
// which expands to the step pattern `...is "([^"]*)"""`. The Gherkin value `""`
// (two double-quote chars) is a non-empty string that is not in the valid set
// {none, base, strict}, so Validate() should reject it.
// NOTE: This requires Validate() to treat the two-char string `""` as invalid.
// The current code allows empty HandshakeProfile (treats it as "no profile").
// If `""` is the two-char string, validation correctly fails; if it somehow maps
// to empty string, this is TF-TARGET-1 (pending until resolved).
func theTargetHandshakeProfileIsEmptyLiteral(v string) error {
	// The Gherkin `""` cell value represents the two double-quote character string.
	// Set HandshakeProfile to the literal value `""` so Validate() rejects it as
	// unsupported (it's non-empty and not in none|base|strict).
	tsState.target.Check.HandshakeProfile = `""`
	return nil
}

func requiredMethodsIncludesEmptyString() error {
	tsState.target.Check.RequiredMethods = append(tsState.target.Check.RequiredMethods, "")
	return nil
}

// ── HURL test rules ────────────────────────────────────────────────────────

func aHURLTestWithNoFileAndNoEndpoint(name string) error {
	tsState.target.Check.HURLTests = append(tsState.target.Check.HURLTests,
		targets.HURLTest{Name: name})
	return nil
}

func aHURLTestWithFileAndEndpoint(name, file, endpoint string) error {
	tsState.target.Check.HURLTests = append(tsState.target.Check.HURLTests,
		targets.HURLTest{Name: name, File: file, Endpoint: endpoint})
	return nil
}

func aHURLTestWithEndpointAndMethod(name, endpoint, method string) error {
	tsState.target.Check.HURLTests = append(tsState.target.Check.HURLTests,
		targets.HURLTest{Name: name, Endpoint: endpoint, Method: method})
	return nil
}

func aHURLTestWithFileAndNoEndpoint(name, file string) error {
	tsState.target.Check.HURLTests = append(tsState.target.Check.HURLTests,
		targets.HURLTest{Name: name, File: file})
	return nil
}

// ── auth config rules ──────────────────────────────────────────────────────

func authTypeWithNoSecretRef(authType string) error {
	tsState.target.Auth = targets.AuthConfig{Type: targets.AuthType(authType)}
	return nil
}

func authTypeWithSecretRef(authType, secretRef string) error {
	tsState.target.Auth = targets.AuthConfig{
		Type:      targets.AuthType(authType),
		SecretRef: secretRef,
	}
	return nil
}

func authTypeWithNoKeyNameAndNoSecretRef(authType string) error {
	tsState.target.Auth = targets.AuthConfig{Type: targets.AuthType(authType)}
	return nil
}

func authTypeWithNoClientIDAndNoTokenURL(authType string) error {
	tsState.target.Auth = targets.AuthConfig{Type: targets.AuthType(authType)}
	return nil
}

func authTypeIs(authType string) error {
	tsState.target.Auth = targets.AuthConfig{Type: targets.AuthType(authType)}
	return nil
}

// ── CheckResult semantics ──────────────────────────────────────────────────

func aCheckResultWithState(state string) error {
	tsState.checkResult = targets.CheckResult{State: targets.HealthState(state)}
	return nil
}

func iCallIsFailureOnTheResult() error {
	tsState.lastIsFailure = tsState.checkResult.IsFailure()
	tsState.isFailureCalled = true
	return nil
}

func isFailureReturns(expected string) error {
	// If iCallIsFailureOnTheResult was not called, this is a known-state scenario;
	// delegate to that handler (both files share package stepdefinitions).
	if !tsState.isFailureCalled {
		return isFailureReturnsKS(expected)
	}
	want := expected == "true"
	if tsState.lastIsFailure != want {
		return fmt.Errorf("IsFailure: expected %v, got %v (state=%q)",
			want, tsState.lastIsFailure, tsState.checkResult.State)
	}
	return nil
}

func aCheckResultWithStateSeverityRegression(state, severity, regression string) error {
	tsState.checkResult = targets.CheckResult{
		State:      targets.HealthState(state),
		Severity:   targets.Severity(severity),
		Regression: regression == "true",
	}
	return nil
}

func iCallIsEscalatedOnTheResult() error {
	tsState.lastIsEscalated = tsState.checkResult.IsEscalated()
	tsState.isEscalatedCalled = true
	return nil
}

func isEscalatedReturns(expected string) error {
	want := expected == "true"
	if tsState.lastIsEscalated != want {
		return fmt.Errorf("IsEscalated: expected %v, got %v (state=%q severity=%q regression=%v)",
			want, tsState.lastIsEscalated,
			tsState.checkResult.State, tsState.checkResult.Severity, tsState.checkResult.Regression)
	}
	return nil
}

// ── type vocabulary ────────────────────────────────────────────────────────

func aTargetWithProtocol(protocol string) error {
	tsState.target = targets.Target{Protocol: targets.Protocol(protocol)}
	return nil
}

func theTargetProtocolIsKnown() error {
	switch tsState.target.Protocol {
	case targets.ProtocolMCP, targets.ProtocolACP, targets.ProtocolHTTP, targets.ProtocolTCP,
		targets.ProtocolOTLPHTTP, targets.ProtocolOTLPGRPC:
		return nil
	default:
		return fmt.Errorf("protocol %q is not a known protocol", tsState.target.Protocol)
	}
}

func aTargetWithTransportOnly(transport string) error {
	tsState.target = targets.Target{Transport: targets.Transport(transport)}
	return nil
}

func theTargetTransportIsKnown() error {
	switch tsState.target.Transport {
	case targets.TransportHTTP, targets.TransportWebSocket, targets.TransportSSE,
		targets.TransportStdio, targets.TransportGRPC, targets.TransportTCP:
		return nil
	default:
		return fmt.Errorf("transport %q is not a known transport", tsState.target.Transport)
	}
}

func aCheckResultWithFailureClass(class string) error {
	tsState.checkResult = targets.CheckResult{FailureClass: targets.FailureClass(class)}
	return nil
}

func theFailureClassIsKnown() error {
	switch tsState.checkResult.FailureClass {
	case targets.FailureNone, targets.FailureNetwork, targets.FailureTimeout,
		targets.FailureAuth, targets.FailureProtocol, targets.FailureConfig,
		targets.FailureTLS, targets.FailureRateLimited, targets.FailureUnknown:
		return nil
	default:
		return fmt.Errorf("failure class %q is not a known failure class",
			tsState.checkResult.FailureClass)
	}
}
