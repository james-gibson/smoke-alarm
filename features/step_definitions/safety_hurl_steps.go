package stepdefinitions

// safety_hurl_steps.go — step definitions for features/safety-hurl.feature
//
// Implemented in-process against safety.Scanner with mock HTTP transport and
// command runner. No binary or subprocess required.
//
// ErrPending scenarios:
//   TF-SAFETY-1: file+endpoint mutually exclusive in validateHURLTests; the
//     TEST_ENDPOINT variable injection scenario (file + endpoint together) is
//     unreachable via the public Register API.
//   TF-SAFETY-2: hurl_test with no endpoint field is rejected by validateHURLTests
//     (requires file or endpoint); the target-endpoint fallback in runEndpointHTTP
//     is unreachable via the public API.
//
// Registered steps owned by this file:
//   "a HURL safety scanner is initialized"
//   "a target {string} with hurl_test name {string} and endpoint {string}"
//   "the target is registered with the scanner"
//   "the scanner reports {string} has registered tests"
//   "a target {string} with hurl_test name {string} and file {string}"
//   "a target {string} with hurl_test name {string} and both file and endpoint set"
//   "a config error is returned containing {string}"
//   "a target {string} with hurl_test name {string} with no file or endpoint"
//   "a target {string} with a hurl_test that has an empty name"
//   "a target {string} with hurl_test method {string}"
//   "the target {string} is registered with hurl tests"
//   "the target is unregistered from the scanner"
//   "the scanner reports {string} has no registered tests"
//   "a hurl_test endpoint {string} returns status code {int}"
//   "the scanner runs tests for the target"
//   "the test outcome is {string}"
//   "the report {string} count is {int}"
//   "\"has_blocking\" is false"
//   "\"has_blocking\" is true"
//   "a hurl_test endpoint {string} is unreachable"
//   "a hurl_test with no method field"
//   "the scanner runs the test"
//   "the outbound request uses method {string}"
//   "a target with endpoint {string} and a hurl_test with no endpoint field" (ErrPending — TF-SAFETY-2)
//   "the outbound request is sent to {string}"
//   "a hurl_test with header {string} set to {string}"
//   "the outbound request contains header {string}"
//   "a hurl_test with file {string}"
//   "the {string} binary is available on PATH"
//   "the command executed is {string}"
//   "a target with endpoint {string}"
//   "a hurl_test with file {string} and endpoint {string}" (ErrPending — TF-SAFETY-1)
//   "the command includes {string}"
//   "the {string} binary exits with code {int} for {string}"
//   "the {string} binary is not available on PATH"
//   "the scanner runs a file-based test"
//   "the hurl_test endpoint error message contains {string}"
//   "the scanner classifies the failure"
//   "a target with {int} hurl_tests: {int} passing endpoints and {int} failing endpoint"
//   "the scanner runs all tests for the target"
//   "a target with no hurl_tests registered"
//   "\"any_tests_seen\" is false"
//   "a target with {int} hurl_tests registered"
//   "the context is canceled before any tests run"
//   "each test outcome is {string}"
// Note: "the failure class is {string}" is owned by engine_steps.go;
//   this file sets hurlState.lastFailClass so the shared step works.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/cucumber/godog"
	"github.com/james-gibson/smoke-alarm/internal/safety"
	"github.com/james-gibson/smoke-alarm/internal/targets"
)

// ── mock HTTP transport ──────────────────────────────────────────────────────

type mockHURLTransport struct {
	mu          sync.Mutex
	statusCodes map[string]int   // URL → HTTP status code (default 200)
	errors      map[string]error // URL → forced transport error
	lastMethod  string
	lastURL     string
	lastHeaders http.Header
}

func newMockHURLTransport() *mockHURLTransport {
	return &mockHURLTransport{
		statusCodes: make(map[string]int),
		errors:      make(map[string]error),
	}
}

func (m *mockHURLTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	m.mu.Lock()
	m.lastMethod = req.Method
	m.lastURL = req.URL.String()
	m.lastHeaders = req.Header.Clone()
	m.mu.Unlock()

	url := req.URL.String()
	if err, ok := m.errors[url]; ok {
		return nil, err
	}
	code := 200
	if c, ok := m.statusCodes[url]; ok {
		code = c
	}
	return &http.Response{
		StatusCode: code,
		Body:       io.NopCloser(strings.NewReader("")),
		Header:     make(http.Header),
		Request:    req,
	}, nil
}

// ── mock command runner ──────────────────────────────────────────────────────

type mockHURLRunner struct {
	mu        sync.Mutex
	exitCodes map[string]int // file path → exit code
	notFound  bool
	lastArgs  []string
}

func newMockHURLRunner() *mockHURLRunner {
	return &mockHURLRunner{
		exitCodes: make(map[string]int),
	}
}

func (m *mockHURLRunner) Run(_ context.Context, name string, args ...string) (string, string, error) {
	m.mu.Lock()
	all := make([]string, 0, 1+len(args))
	all = append(all, name)
	all = append(all, args...)
	m.lastArgs = all
	notFound := m.notFound
	exitCodes := m.exitCodes
	m.mu.Unlock()

	if notFound {
		return "", "executable file not found in $PATH", errors.New("executable file not found in $PATH")
	}

	// Extract file argument (follows --test flag).
	file := ""
	for i, a := range args {
		if a == "--test" && i+1 < len(args) {
			file = args[i+1]
			break
		}
	}

	code := 0
	if c, ok := exitCodes[file]; ok {
		code = c
	}
	if code != 0 {
		return "", "test failed", fmt.Errorf("exit status %d", code)
	}
	return "", "", nil
}

// ── per-scenario state ───────────────────────────────────────────────────────

var hurlState struct {
	scanner        *safety.Scanner
	transport      *mockHURLTransport
	runner         *mockHURLRunner
	targetID       string
	targetEndpoint string
	pendingTests   []targets.HURLTest
	lastRegErr     error
	lastReport     safety.Report
	lastResult     safety.Result
	lastFailClass  targets.FailureClass // read by engine_steps.go theFailureClassIs
	runCtx         context.Context
}

func resetHURLState() {
	hurlState.scanner = nil
	hurlState.transport = nil
	hurlState.runner = nil
	hurlState.targetID = "test-target"
	hurlState.targetEndpoint = ""
	hurlState.pendingTests = nil
	hurlState.lastRegErr = nil
	hurlState.lastReport = safety.Report{}
	hurlState.lastResult = safety.Result{}
	hurlState.lastFailClass = ""
	hurlState.runCtx = nil
}

func initHURLScanner() {
	tr := newMockHURLTransport()
	rn := newMockHURLRunner()
	sc := safety.NewScanner(
		safety.WithHTTPClient(&http.Client{Transport: tr}),
		safety.WithCommandRunner(rn),
		safety.WithHURLBinary("hurl"),
	)
	hurlState.scanner = sc
	hurlState.transport = tr
	hurlState.runner = rn
}

// runTargetWithScanner runs the scanner for the current pending target and
// stores the report and first result for subsequent assertion steps.
func runTargetWithScanner() {
	ctx := hurlState.runCtx
	if ctx == nil {
		ctx = context.Background()
	}
	t := targets.Target{
		ID:       hurlState.targetID,
		Endpoint: hurlState.targetEndpoint,
	}
	r := hurlState.scanner.RunTarget(ctx, t)
	hurlState.lastReport = r
	if len(r.Results) > 0 {
		hurlState.lastResult = r.Results[0]
		hurlState.lastFailClass = r.Results[0].FailureClass
	}
}

// InitializeSafetyHURLSteps registers all step definitions for safety-hurl.feature.
func InitializeSafetyHURLSteps(ctx *godog.ScenarioContext) {
	ctx.BeforeScenario(func(_ *godog.Scenario) { resetHURLState() })
	ctx.AfterScenario(func(_ *godog.Scenario, _ error) { resetHURLState() })

	// ── registration ───────────────────────────────────────────────────────
	ctx.Step(`^a HURL safety scanner is initialized$`, aHURLSafetyScannerIsInitialised)
	ctx.Step(`^a target "([^"]*)" with hurl_test name "([^"]*)" and endpoint "([^"]*)"$`, aTargetWithHurlTestEndpoint)
	ctx.Step(`^the target is registered with the scanner$`, theTargetIsRegisteredWithScanner)
	ctx.Step(`^the scanner reports "([^"]*)" has registered tests$`, theScannerReportsHasRegisteredTests)
	ctx.Step(`^a target "([^"]*)" with hurl_test name "([^"]*)" and file "([^"]*)"$`, aTargetWithHurlTestFile)
	ctx.Step(`^a target "([^"]*)" with hurl_test name "([^"]*)" and both file and endpoint set$`, aTargetWithHurlTestBothSet)
	ctx.Step(`^a config error is returned containing "([^"]*)"$`, aConfigErrorIsReturnedContaining)
	ctx.Step(`^a target "([^"]*)" with hurl_test name "([^"]*)" with no file or endpoint$`, aTargetWithHurlTestNeitherSet)
	ctx.Step(`^a target "([^"]*)" with a hurl_test that has an empty name$`, aTargetWithHurlTestEmptyName)
	ctx.Step(`^a target "([^"]*)" with hurl_test method "([^"]*)"$`, aTargetWithHurlTestMethod)
	ctx.Step(`^the target "([^"]*)" is registered with hurl tests$`, theTargetIsRegisteredWithHurlTests)
	ctx.Step(`^the target is unregistered from the scanner$`, theTargetIsUnregisteredFromScanner)
	ctx.Step(`^the scanner reports "([^"]*)" has no registered tests$`, theScannerReportsHasNoRegisteredTests)

	// ── endpoint-based mode ────────────────────────────────────────────────
	ctx.Step(`^a hurl_test endpoint "([^"]*)" returns status code (\d+)$`, aHurlTestEndpointReturnsStatusCode)
	ctx.Step(`^the scanner runs tests for the target$`, theScannerRunsTestsForTarget)
	ctx.Step(`^the test outcome is "([^"]*)"$`, theTestOutcomeIs)
	ctx.Step(`^the report "([^"]*)" count is (\d+)$`, theReportCountIs)
	ctx.Step(`^"has_blocking" is false$`, hasBlockingIsFalse)
	ctx.Step(`^"has_blocking" is true$`, hasBlockingIsTrue)
	ctx.Step(`^a hurl_test endpoint "([^"]*)" is unreachable$`, aHurlTestEndpointIsUnreachable)
	// "the failure class is ..." owned by engine_steps.go; this file sets hurlState.lastFailClass.
	ctx.Step(`^a hurl_test with no method field$`, aHurlTestWithNoMethodField)
	ctx.Step(`^the scanner runs the test$`, theScannerRunsTheTest)
	ctx.Step(`^the outbound request uses method "([^"]*)"$`, theOutboundRequestUsesMethod)
	ctx.Step(`^a target with endpoint "([^"]*)" and a hurl_test with no endpoint field$`, aTargetWithEndpointAndHurlTestNoEndpoint)
	ctx.Step(`^the outbound request is sent to "([^"]*)"$`, theOutboundRequestIsSentTo)
	ctx.Step(`^a hurl_test with header "([^"]*)" set to "([^"]*)"$`, aHurlTestWithHeader)
	ctx.Step(`^the outbound request contains header "([^"]*)"$`, theOutboundRequestContainsHeader)

	// ── file-based mode ────────────────────────────────────────────────────
	ctx.Step(`^a hurl_test with file "([^"]*)"$`, aHurlTestWithFile)
	ctx.Step(`^the "([^"]*)" binary is available on PATH$`, theBinaryIsAvailableOnPATH)
	ctx.Step(`^the command executed is "([^"]*)"$`, theCommandExecutedIs)
	ctx.Step(`^a target with endpoint "([^"]*)"$`, aTargetWithEndpoint)
	ctx.Step(`^a hurl_test with file "([^"]*)" and endpoint "([^"]*)"$`, aHurlTestWithFileAndEndpoint)
	ctx.Step(`^the command includes "([^"]*)"$`, theCommandIncludes)
	ctx.Step(`^the "([^"]*)" binary exits with code (\d+) for "([^"]*)"$`, theBinaryExitsWithCode)
	ctx.Step(`^the "([^"]*)" binary is not available on PATH$`, theBinaryIsNotAvailableOnPATH)
	ctx.Step(`^the scanner runs a file-based test$`, theScannerRunsFileBasedTest)

	// ── failure class mapping ──────────────────────────────────────────────
	ctx.Step(`^the hurl_test endpoint error message contains "([^"]*)"$`, theHurlTestEndpointErrorMessageContains)
	ctx.Step(`^the scanner classifies the failure$`, theScannerClassifiesFailure)

	// ── report aggregation ─────────────────────────────────────────────────
	ctx.Step(`^a target with (\d+) hurl_tests: (\d+) passing endpoints and (\d+) failing endpoint$`, aTargetWithMixedHurlTests)
	ctx.Step(`^the scanner runs all tests for the target$`, theScannerRunsAllTests)
	ctx.Step(`^a target with no hurl_tests registered$`, aTargetWithNoHurlTests)
	ctx.Step(`^"any_tests_seen" is false$`, anyTestsSeenIsFalse)
	ctx.Step(`^a target with (\d+) hurl_tests registered$`, aTargetWithNHurlTests)
	ctx.Step(`^the context is canceled before any tests run$`, theContextIsCancelledBeforeTests)
	ctx.Step(`^each test outcome is "([^"]*)"$`, eachTestOutcomeIs)
}

// ── Background ───────────────────────────────────────────────────────────────

func aHURLSafetyScannerIsInitialised() error {
	initHURLScanner()
	return nil
}

// ── registration ─────────────────────────────────────────────────────────────

func aTargetWithHurlTestEndpoint(id, name, endpoint string) error {
	hurlState.targetID = id
	hurlState.pendingTests = []targets.HURLTest{{Name: name, Endpoint: endpoint}}
	return nil
}

func theTargetIsRegisteredWithScanner() error {
	hurlState.lastRegErr = hurlState.scanner.Register(hurlState.targetID, hurlState.pendingTests)
	return nil
}

func theScannerReportsHasRegisteredTests(id string) error {
	if !hurlState.scanner.HasRegistered(id) {
		return fmt.Errorf("scanner has no registered tests for %q", id)
	}
	return nil
}

func aTargetWithHurlTestFile(id, name, file string) error {
	hurlState.targetID = id
	hurlState.pendingTests = []targets.HURLTest{{Name: name, File: file}}
	return nil
}

func aTargetWithHurlTestBothSet(id, name string) error {
	hurlState.targetID = id
	hurlState.pendingTests = []targets.HURLTest{{
		Name:     name,
		File:     "tests/hurl/health.hurl",
		Endpoint: "http://localhost/healthz",
	}}
	return nil
}

func aConfigErrorIsReturnedContaining(substr string) error {
	if hurlState.lastRegErr == nil {
		return fmt.Errorf("expected a config error containing %q, got nil", substr)
	}
	if !strings.Contains(hurlState.lastRegErr.Error(), substr) {
		return fmt.Errorf("config error %q does not contain %q", hurlState.lastRegErr.Error(), substr)
	}
	return nil
}

func aTargetWithHurlTestNeitherSet(id, name string) error {
	hurlState.targetID = id
	hurlState.pendingTests = []targets.HURLTest{{Name: name}}
	return nil
}

func aTargetWithHurlTestEmptyName(id string) error {
	hurlState.targetID = id
	hurlState.pendingTests = []targets.HURLTest{{Name: "", Endpoint: "http://localhost/healthz"}}
	return nil
}

func aTargetWithHurlTestMethod(id, method string) error {
	hurlState.targetID = id
	hurlState.pendingTests = []targets.HURLTest{{
		Name:     "method-test",
		Endpoint: "http://localhost/healthz",
		Method:   method,
	}}
	return nil
}

func theTargetIsRegisteredWithHurlTests(id string) error {
	hurlState.targetID = id
	return hurlState.scanner.Register(id, []targets.HURLTest{{Name: "test", Endpoint: "http://localhost/healthz"}})
}

func theTargetIsUnregisteredFromScanner() error {
	hurlState.scanner.Unregister(hurlState.targetID)
	return nil
}

func theScannerReportsHasNoRegisteredTests(id string) error {
	if hurlState.scanner.HasRegistered(id) {
		return fmt.Errorf("scanner still has registered tests for %q", id)
	}
	return nil
}

// ── endpoint-based mode ──────────────────────────────────────────────────────

func aHurlTestEndpointReturnsStatusCode(endpoint string, code int) error {
	hurlState.transport.statusCodes[endpoint] = code
	hurlState.pendingTests = []targets.HURLTest{{Name: "status-check", Endpoint: endpoint}}
	return nil
}

// theScannerRunsTestsForTarget registers any pending tests then runs the scanner.
// If pendingTests is nil (tests already registered by a prior step), it skips
// registration and runs directly.
func theScannerRunsTestsForTarget() error {
	if len(hurlState.pendingTests) > 0 {
		if err := hurlState.scanner.Register(hurlState.targetID, hurlState.pendingTests); err != nil {
			return fmt.Errorf("register: %w", err)
		}
	}
	runTargetWithScanner()
	return nil
}

func theTestOutcomeIs(outcome string) error {
	if string(hurlState.lastResult.Outcome) != outcome {
		return fmt.Errorf("outcome=%q want=%q", hurlState.lastResult.Outcome, outcome)
	}
	return nil
}

func theReportCountIs(field string, n int) error {
	var got int
	switch field {
	case "passed":
		got = hurlState.lastReport.Passed
	case "failed":
		got = hurlState.lastReport.Failed
	case "skipped":
		got = hurlState.lastReport.Skipped
	default:
		return fmt.Errorf("unknown report count field %q", field)
	}
	if got != n {
		return fmt.Errorf("%s count=%d want=%d", field, got, n)
	}
	return nil
}

func hasBlockingIsFalse() error {
	if hurlState.lastReport.HasBlocking {
		return fmt.Errorf("expected has_blocking=false, got true")
	}
	return nil
}

func hasBlockingIsTrue() error {
	if !hurlState.lastReport.HasBlocking {
		return fmt.Errorf("expected has_blocking=true, got false")
	}
	return nil
}

func aHurlTestEndpointIsUnreachable(endpoint string) error {
	hurlState.transport.errors[endpoint] = fmt.Errorf("dial tcp: connection refused")
	hurlState.pendingTests = []targets.HURLTest{{Name: "unreachable-check", Endpoint: endpoint}}
	return nil
}

func aHurlTestWithNoMethodField() error {
	endpoint := "http://localhost/no-method-test"
	hurlState.transport.statusCodes[endpoint] = 200
	hurlState.pendingTests = []targets.HURLTest{{Name: "no-method-check", Endpoint: endpoint}}
	return nil
}

// theScannerRunsTheTest registers pending tests and runs the scanner for a single
// test scenario (contrast with theScannerRunsTestsForTarget which skips registration
// when tests are already registered).
func theScannerRunsTheTest() error {
	if err := hurlState.scanner.Register(hurlState.targetID, hurlState.pendingTests); err != nil {
		return fmt.Errorf("register: %w", err)
	}
	runTargetWithScanner()
	return nil
}

func theOutboundRequestUsesMethod(method string) error {
	hurlState.transport.mu.Lock()
	got := hurlState.transport.lastMethod
	hurlState.transport.mu.Unlock()
	if got != method {
		return fmt.Errorf("method=%q want=%q", got, method)
	}
	return nil
}

// aTargetWithEndpointAndHurlTestNoEndpoint is ErrPending (TF-SAFETY-2):
// validateHURLTests requires either file or endpoint; a hurl_test with no
// endpoint field cannot be registered via the public API. The target-endpoint
// fallback in runEndpointHTTP is unreachable from step definitions.
func aTargetWithEndpointAndHurlTestNoEndpoint(_ string) error {
	return godog.ErrPending
}

func theOutboundRequestIsSentTo(url string) error {
	hurlState.transport.mu.Lock()
	got := hurlState.transport.lastURL
	hurlState.transport.mu.Unlock()
	if got != url {
		return fmt.Errorf("URL=%q want=%q", got, url)
	}
	return nil
}

func aHurlTestWithHeader(header, val string) error {
	endpoint := "http://localhost/header-test"
	hurlState.transport.statusCodes[endpoint] = 200
	hurlState.pendingTests = []targets.HURLTest{{
		Name:     "header-check",
		Endpoint: endpoint,
		Headers:  map[string]string{header: val},
	}}
	return nil
}

func theOutboundRequestContainsHeader(headerPair string) error {
	// headerPair format: "Name: Value"
	parts := strings.SplitN(headerPair, ": ", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid header pair format %q (expected 'Name: Value')", headerPair)
	}
	name, wantVal := parts[0], parts[1]
	hurlState.transport.mu.Lock()
	headers := hurlState.transport.lastHeaders
	hurlState.transport.mu.Unlock()
	if headers == nil {
		return fmt.Errorf("no outbound request captured")
	}
	got := headers.Get(name)
	if got != wantVal {
		return fmt.Errorf("header %q=%q want=%q", name, got, wantVal)
	}
	return nil
}

// ── file-based mode ──────────────────────────────────────────────────────────

func aHurlTestWithFile(file string) error {
	hurlState.pendingTests = []targets.HURLTest{{Name: "file-test", File: file}}
	return nil
}

// theBinaryIsAvailableOnPATH is a no-op: the mock runner succeeds by default.
func theBinaryIsAvailableOnPATH(_ string) error {
	hurlState.runner.mu.Lock()
	hurlState.runner.notFound = false
	hurlState.runner.mu.Unlock()
	return nil
}

func theCommandExecutedIs(cmd string) error {
	hurlState.runner.mu.Lock()
	args := hurlState.runner.lastArgs
	hurlState.runner.mu.Unlock()
	got := strings.Join(args, " ")
	if got != cmd {
		return fmt.Errorf("command=%q want=%q", got, cmd)
	}
	return nil
}

func aTargetWithEndpoint(endpoint string) error {
	hurlState.targetEndpoint = endpoint
	return nil
}

// aHurlTestWithFileAndEndpoint is ErrPending (TF-SAFETY-1):
// validateHURLTests rejects file+endpoint as mutually exclusive. The TEST_ENDPOINT
// variable injection path in runHURLFile is unreachable via the public Register API.
func aHurlTestWithFileAndEndpoint(_, _ string) error {
	return godog.ErrPending
}

func theCommandIncludes(flag string) error {
	hurlState.runner.mu.Lock()
	args := hurlState.runner.lastArgs
	hurlState.runner.mu.Unlock()
	full := strings.Join(args, " ")
	if !strings.Contains(full, flag) {
		return fmt.Errorf("command %q does not include %q", full, flag)
	}
	return nil
}

// theBinaryExitsWithCode configures the mock runner exit code for the given file
// and sets up pending tests so theScannerRunsTheTest can register and run them.
func theBinaryExitsWithCode(cmd string, code int, file string) error {
	hurlState.runner.mu.Lock()
	hurlState.runner.exitCodes[file] = code
	hurlState.runner.mu.Unlock()
	hurlState.pendingTests = []targets.HURLTest{{Name: "exit-code-test", File: file}}
	return nil
}

func theBinaryIsNotAvailableOnPATH(_ string) error {
	hurlState.runner.mu.Lock()
	hurlState.runner.notFound = true
	hurlState.runner.mu.Unlock()
	return nil
}

// theScannerRunsFileBasedTest registers a generic file-based test and runs the
// scanner. Uses pending tests if already set; otherwise creates a default test.
func theScannerRunsFileBasedTest() error {
	tests := hurlState.pendingTests
	if len(tests) == 0 {
		tests = []targets.HURLTest{{Name: "file-test", File: "tests/hurl/health.hurl"}}
	}
	if err := hurlState.scanner.Register(hurlState.targetID, tests); err != nil {
		return fmt.Errorf("register: %w", err)
	}
	runTargetWithScanner()
	return nil
}

// ── failure class mapping ─────────────────────────────────────────────────────

// theHurlTestEndpointErrorMessageContains sets up the mock transport to return
// an error whose message contains the given fragment for classifyHTTPError.
func theHurlTestEndpointErrorMessageContains(errFragment string) error {
	endpoint := "http://localhost/classify-test"
	hurlState.transport.errors[endpoint] = fmt.Errorf("%s: simulated error", errFragment)
	hurlState.pendingTests = []targets.HURLTest{{Name: "classify-test", Endpoint: endpoint}}
	return nil
}

func theScannerClassifiesFailure() error {
	return theScannerRunsTheTest()
}

// ── report aggregation ────────────────────────────────────────────────────────

// aTargetWithMixedHurlTests registers a target with the specified distribution
// of passing (2xx) and failing (5xx) endpoint-based tests.
func aTargetWithMixedHurlTests(_, passing, failing int) error {
	tests := make([]targets.HURLTest, 0, passing+failing)
	for i := range passing {
		ep := fmt.Sprintf("http://localhost/pass/%d/healthz", i)
		tests = append(tests, targets.HURLTest{Name: fmt.Sprintf("pass-%d", i), Endpoint: ep})
		hurlState.transport.statusCodes[ep] = 200
	}
	for i := range failing {
		ep := fmt.Sprintf("http://localhost/fail/%d/healthz", i)
		tests = append(tests, targets.HURLTest{Name: fmt.Sprintf("fail-%d", i), Endpoint: ep})
		hurlState.transport.statusCodes[ep] = 503
	}
	hurlState.targetID = "mixed-target"
	return hurlState.scanner.Register(hurlState.targetID, tests)
}

// theScannerRunsAllTests runs the scanner for the current target (tests already
// registered by a prior step such as aTargetWithMixedHurlTests).
func theScannerRunsAllTests() error {
	runTargetWithScanner()
	return nil
}

func aTargetWithNoHurlTests() error {
	hurlState.targetID = "no-tests-target"
	// Do not register any tests — scanner will return AnyTestsSeen=false.
	return nil
}

func anyTestsSeenIsFalse() error {
	if hurlState.lastReport.AnyTestsSeen {
		return fmt.Errorf("expected any_tests_seen=false, got true")
	}
	return nil
}

// aTargetWithNHurlTests registers n endpoint-based tests for the current target.
func aTargetWithNHurlTests(n int) error {
	tests := make([]targets.HURLTest, n)
	for i := range tests {
		ep := fmt.Sprintf("http://localhost/t/%d/healthz", i)
		tests[i] = targets.HURLTest{Name: fmt.Sprintf("test-%d", i), Endpoint: ep}
		hurlState.transport.statusCodes[ep] = 200
	}
	return hurlState.scanner.Register(hurlState.targetID, tests)
}

// theContextIsCancelledBeforeTests creates an already-canceled context so that
// RunTarget skips all tests via the ctx.Done() guard at the top of the test loop.
func theContextIsCancelledBeforeTests() error {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	hurlState.runCtx = ctx
	return nil
}

func eachTestOutcomeIs(outcome string) error {
	if len(hurlState.lastReport.Results) == 0 {
		return fmt.Errorf("no results in report")
	}
	for i, r := range hurlState.lastReport.Results {
		if string(r.Outcome) != outcome {
			return fmt.Errorf("result[%d] outcome=%q want=%q", i, r.Outcome, outcome)
		}
	}
	return nil
}
