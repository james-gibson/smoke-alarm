package hurl

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// fakeRunner records the invocation and returns a canned outcome.
type fakeRunner struct {
	calls  int
	name   string
	args   []string
	stdout string
	stderr string
	err    error
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) (string, string, error) {
	f.calls++
	f.name = name
	f.args = append([]string(nil), args...)
	return f.stdout, f.stderr, f.err
}

func TestFileBackedPass(t *testing.T) {
	run := &fakeRunner{}

	report, err := New(WithCommandRunner(run)).Run(context.Background(), "t1", []Test{
		{Name: "health", File: "tests/hurl/health.hurl"},
	}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !report.AnyTestsSeen {
		t.Error("any_tests_seen: got false, want true")
	}
	if report.Passed != 1 || report.Failed != 0 || report.Skipped != 0 {
		t.Errorf("counts: got pass=%d fail=%d skip=%d, want 1/0/0",
			report.Passed, report.Failed, report.Skipped)
	}
	if report.HasBlocking {
		t.Error("has_blocking: got true, want false")
	}
	if got := report.Results[0].Outcome; got != OutcomePass {
		t.Errorf("outcome: got %q, want %q", got, OutcomePass)
	}
	if got := report.Results[0].FailureClass; got != FailureNone {
		t.Errorf("failure_class: got %q, want %q", got, FailureNone)
	}
	if report.Results[0].Duration <= 0 {
		t.Error("duration: got zero, want a measured duration")
	}
}

func TestFileBackedFail(t *testing.T) {
	run := &fakeRunner{
		stderr: "curl: (7) connection refused",
		err:    errors.New("exit status 1"),
	}

	report, err := New(WithCommandRunner(run)).Run(context.Background(), "t1", []Test{
		{Name: "probe", File: "tests/hurl/probe.hurl"},
	}, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Failed != 1 {
		t.Fatalf("failed: got %d, want 1", report.Failed)
	}
	if !report.HasBlocking {
		t.Error("has_blocking: got false, want true — a failed test blocks")
	}
	got := report.Results[0]
	if got.Outcome != OutcomeFail {
		t.Errorf("outcome: got %q, want %q", got.Outcome, OutcomeFail)
	}
	if got.FailureClass != FailureNetwork {
		t.Errorf("failure_class: got %q, want %q", got.FailureClass, FailureNetwork)
	}
	if !strings.Contains(got.Message, "connection refused") {
		t.Errorf("message: got %q, want the command output", got.Message)
	}
}

func TestMissingBinaryIsConfigFailure(t *testing.T) {
	run := &fakeRunner{
		err: errors.New(`exec: "hurl": executable file not found in $PATH`),
	}

	report, _ := New(WithCommandRunner(run)).Run(context.Background(), "t1", []Test{
		{Name: "probe", File: "probe.hurl"},
	}, nil)

	if got := report.Results[0].FailureClass; got != FailureConfig {
		t.Errorf("failure_class: got %q, want %q", got, FailureConfig)
	}
	if !report.HasBlocking {
		t.Error("has_blocking: got false, want true")
	}
}

func TestArgvHasNoVariablesWhenNoneAreDeclared(t *testing.T) {
	run := &fakeRunner{}

	if _, err := New(WithCommandRunner(run)).Run(context.Background(), "t1", []Test{
		{Name: "health", File: "tests/hurl/health.hurl"},
	}, nil); err != nil {
		t.Fatalf("Run: %v", err)
	}

	// No endpoint, no vars: the invocation is exactly --test and the file. A
	// stray argument here would be a new capability, not a formatting nit.
	want := []string{"--test", "tests/hurl/health.hurl"}
	if !reflect.DeepEqual(run.args, want) {
		t.Errorf("args: got %v, want %v", run.args, want)
	}
	if run.name != "hurl" {
		t.Errorf("binary: got %q, want %q", run.name, "hurl")
	}
}

func TestVariablesAreSortedAndKeepHistoricDefaults(t *testing.T) {
	run := &fakeRunner{}

	// The caller supplies ENDPOINT (the endpoint being checked); the test
	// supplies TEST_ENDPOINT. Both are passed, in sorted order.
	if _, err := New(WithCommandRunner(run)).Run(context.Background(), "t1", []Test{
		{Name: "probe", File: "probe.hurl", Endpoint: "https://example.com/healthz"},
	}, map[string]string{"ENDPOINT": "https://example.com/mcp"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := []string{
		"--test", "probe.hurl",
		"--variable", "ENDPOINT=https://example.com/mcp",
		"--variable", "TEST_ENDPOINT=https://example.com/healthz",
	}
	if !reflect.DeepEqual(run.args, want) {
		t.Errorf("args: got %v, want %v", run.args, want)
	}
}

func TestCallerOverridesTestEndpointVariable(t *testing.T) {
	run := &fakeRunner{}

	if _, err := New(WithCommandRunner(run)).Run(context.Background(), "t1", []Test{
		{Name: "probe", File: "probe.hurl", Endpoint: "https://from-test.example"},
	}, map[string]string{"TEST_ENDPOINT": "https://from-caller.example"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	if got := strings.Join(run.args, " "); !strings.Contains(got, "TEST_ENDPOINT=https://from-caller.example") {
		t.Errorf("args: got %q, want the caller's TEST_ENDPOINT", got)
	}
	if got := strings.Join(run.args, " "); strings.Contains(got, "from-test.example") {
		t.Errorf("args: got %q, the test endpoint should have been overridden", got)
	}
}

func TestTestWithoutFileFailsClosed(t *testing.T) {
	run := &fakeRunner{}

	report, _ := New(WithCommandRunner(run)).Run(context.Background(), "t1", []Test{
		{Name: "endpoint-only", Endpoint: "https://example.com"},
	}, nil)

	if run.calls != 0 {
		t.Errorf("runner calls: got %d, want 0 — no file means no invocation", run.calls)
	}
	if report.Results[0].Outcome != OutcomeFail {
		t.Errorf("outcome: got %q, want %q", report.Results[0].Outcome, OutcomeFail)
	}
	if report.Results[0].FailureClass != FailureConfig {
		t.Errorf("failure_class: got %q, want %q", report.Results[0].FailureClass, FailureConfig)
	}
	if !report.HasBlocking {
		t.Error("has_blocking: got false, want true")
	}
}

func TestContextCancelSkipsAndRunFailsClosed(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	report, err := Run(ctx, "t1", []Test{
		{Name: "probe", File: "probe.hurl"},
	}, nil)

	if !errors.Is(err, ErrNothingRan) {
		t.Fatalf("Run: got %v, want ErrNothingRan", err)
	}
	if report.Skipped != 1 {
		t.Errorf("skipped: got %d, want 1", report.Skipped)
	}
	if report.HasBlocking {
		t.Error("has_blocking: got true, want false — a skip is unknown, and the error carries that")
	}
}

func TestZeroTestsFailsClosedFromRun(t *testing.T) {
	report, err := Run(context.Background(), "t1", nil, nil)

	if !errors.Is(err, ErrNothingRan) {
		t.Fatalf("Run: got %v, want ErrNothingRan", err)
	}
	if report.AnyTestsSeen {
		t.Error("any_tests_seen: got true, want false")
	}
	if report.HasBlocking {
		t.Error("has_blocking: got true, want false")
	}
}

// A monitored target with no registered tests is legitimately "nothing to
// check", so Executor.Run must not apply the blocking policy. This is the
// split that lets internal/safety keep its current behaviour.
func TestExecutorRunDoesNotApplyNothingRanPolicy(t *testing.T) {
	report, err := New().Run(context.Background(), "t1", nil, nil)
	if err != nil {
		t.Fatalf("Executor.Run: got %v, want nil", err)
	}
	if report.AnyTestsSeen {
		t.Error("any_tests_seen: got true, want false")
	}
}
