// Package hurl is the importable execution core for HURL preflight checks.
//
// Two binaries need the same runner: smoke-alarm's safety scanner runs a
// pre-protocol check before it validates a monitored target, and lezz's
// sidecars run a blocking preflight before spending a queue batch or a cache
// fetch. The runner used to live in internal/safety, which is internal/ and
// therefore unreachable from another module — the only alternatives were
// duplicating runHURLFile or registering a sidecar as a monitored target, and
// both are rejected by the sidecar contract (lezz.go/docs/mcp-wrapper.md).
//
// This package depends on nothing under internal/. That is a hard requirement,
// not an accident: a host module must be able to `go get` it. It is also why it
// carries its own FailureClass instead of reusing internal/targets — the
// classification vocabulary is part of the contract, so it has to be visible
// here.
//
// Nothing in this package decides policy. It runs hurl, reports what happened,
// and never interprets a pass as authorisation: a preflight asserts that the
// environment looked right before a call, not that the call is safe.
package hurl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// Outcome is the result of one test.
type Outcome string

const (
	OutcomePass Outcome = "pass"
	OutcomeFail Outcome = "fail"
	OutcomeSkip Outcome = "skip"
)

// FailureClass normalizes root-cause classes so a caller can route an alert
// without parsing messages. The values match smoke-alarm's target-facing
// vocabulary; internal/safety converts rather than re-exporting the type, so
// this package stays free of internal/ imports.
type FailureClass string

const (
	FailureNone        FailureClass = "none"
	FailureNetwork     FailureClass = "network"
	FailureTimeout     FailureClass = "timeout"
	FailureAuth        FailureClass = "auth"
	FailureProtocol    FailureClass = "protocol"
	FailureConfig      FailureClass = "config"
	FailureTLS         FailureClass = "tls"
	FailureRateLimited FailureClass = "rate_limited"
	FailureUnknown     FailureClass = "unknown"
)

// Test is one HURL check. File is the executable input: it becomes
// `hurl --test <file>`. Endpoint supplies the TEST_ENDPOINT variable when the
// caller has not set one. Method is carried for hosts that keep one test shape
// across the file-backed runner here and their own HTTP endpoint check; this
// package does not use it.
type Test struct {
	Name     string
	File     string
	Endpoint string
	Method   string
}

// Result is one test execution.
type Result struct {
	ID           string
	TestName     string
	Outcome      Outcome
	FailureClass FailureClass
	Message      string
	Duration     time.Duration
	StartedAt    time.Time
	FinishedAt   time.Time
}

// Report aggregates the results of one Run.
//
// HasBlocking is set only by a failure. A run that executed nothing is not
// "not blocking" — it is unknown, which is why Run returns ErrNothingRan rather
// than a clean report. Callers that legitimately have nothing to check (a
// monitored target with no registered tests) use Executor.Run and read
// AnyTestsSeen instead.
type Report struct {
	ID           string
	StartedAt    time.Time
	FinishedAt   time.Time
	Passed       int
	Failed       int
	Skipped      int
	HasBlocking  bool
	Results      []Result
	AnyTestsSeen bool
}

// ErrNothingRan reports that a Run produced no executed test: no tests were
// given, or every result was a skip. It is the fail-closed signal for blocking
// consumers, and exists because the previous shape ("skipped" ⇒ not blocking)
// let a preflight that never ran admit the workload it was meant to gate.
var ErrNothingRan = errors.New("hurl: no test executed")

// CommandRunner allows command execution to be mocked in tests and lets a host
// inject its own process policy.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) (stdout string, stderr string, err error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var outB bytes.Buffer
	var errB bytes.Buffer
	cmd.Stdout = &outB
	cmd.Stderr = &errB
	err := cmd.Run()
	return outB.String(), errB.String(), err
}

// Executor is a configured runner: which binary, and through what runner.
type Executor struct {
	binary string
	runner CommandRunner
}

// Option configures an Executor.
type Option func(*Executor)

// WithBinary sets the hurl executable name or path (default "hurl").
func WithBinary(bin string) Option {
	return func(e *Executor) {
		if strings.TrimSpace(bin) != "" {
			e.binary = strings.TrimSpace(bin)
		}
	}
}

// WithCommandRunner injects a command runner (tests, or a host's process
// policy). A nil runner is ignored rather than accepted, so an injected
// executor can never end up unable to run anything.
func WithCommandRunner(r CommandRunner) Option {
	return func(e *Executor) {
		if r != nil {
			e.runner = r
		}
	}
}

// New creates an Executor. The default binary is "hurl" and the default runner
// executes it as a child process.
func New(opts ...Option) *Executor {
	e := &Executor{binary: "hurl", runner: execRunner{}}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// Run executes tests with a default Executor and applies the fail-closed
// policy: a report with no executed test is an error, not a verdict.
//
// This is the entry point a blocking consumer wants. A host that legitimately
// has nothing to check calls New().Run directly and reads AnyTestsSeen.
func Run(ctx context.Context, id string, tests []Test, vars map[string]string) (Report, error) {
	report, err := New().Run(ctx, id, tests, vars)
	if err != nil {
		return report, err
	}
	if report.Passed+report.Failed == 0 {
		return report, fmt.Errorf("%w: %s", ErrNothingRan, id)
	}
	return report, nil
}

// Run executes tests with this Executor and reports what happened.
//
// It returns an error only when a test could not be attempted for a reason the
// package owns (none today: a bad test is a failed Result, because a preflight
// that cannot start must fail closed rather than abort the caller). Skipped and
// absent tests are described by the Report, not by an error; Run is where the
// nothing-ran policy lives.
func (e *Executor) Run(ctx context.Context, id string, tests []Test, vars map[string]string) (Report, error) {
	report := Report{
		ID:        id,
		StartedAt: time.Now().UTC(),
		Results:   []Result{},
	}

	if len(tests) == 0 {
		report.FinishedAt = time.Now().UTC()
		return report, nil
	}
	report.AnyTestsSeen = true

	for _, t := range tests {
		r := e.runOne(ctx, id, t, vars)
		report.Results = append(report.Results, r)
		switch r.Outcome {
		case OutcomePass:
			report.Passed++
		case OutcomeFail:
			report.Failed++
			report.HasBlocking = true
		case OutcomeSkip:
			report.Skipped++
		}
	}

	report.FinishedAt = time.Now().UTC()
	return report, nil
}

// runOne executes a single test. Every failure path is a Result, never a
// returned error: a preflight that cannot start has to look like a refusal to
// the caller, not like a crash.
func (e *Executor) runOne(ctx context.Context, id string, t Test, vars map[string]string) Result {
	start := time.Now().UTC()

	res := Result{ID: id, TestName: t.Name, StartedAt: start}

	if ctx.Err() != nil {
		res.Outcome = OutcomeSkip
		res.FailureClass = FailureUnknown
		res.Message = "context canceled before test execution"
		return stamp(res)
	}

	file := strings.TrimSpace(t.File)
	if file == "" {
		res.Outcome = OutcomeFail
		res.FailureClass = FailureConfig
		res.Message = "no hurl file configured for test"
		return stamp(res)
	}

	args := make([]string, 0, 2+2*len(vars)+2)
	args = append(args, "--test", file)
	for _, kv := range variableArgs(t, vars) {
		args = append(args, "--variable", kv)
	}

	stdout, stderr, err := e.runner.Run(ctx, e.binary, args...)
	if err != nil {
		msg := compactOutput(stdout, stderr)
		if msg == "" {
			msg = err.Error()
		}
		res.Outcome = OutcomeFail
		res.FailureClass = classifyCmdFailure(err, msg)
		res.Message = "hurl file test failed: " + msg
		return stamp(res)
	}

	res.Outcome = OutcomePass
	res.FailureClass = FailureNone
	res.Message = "hurl file test passed"
	return stamp(res)
}

// stamp closes out a result's timing.
func stamp(res Result) Result {
	res.FinishedAt = time.Now().UTC()
	res.Duration = res.FinishedAt.Sub(res.StartedAt)
	return res
}

// variableArgs renders the --variable payload.
//
// The keys are sorted so argv is deterministic — a test that asserts on the
// command line should not depend on map iteration order. ENDPOINT and
// TEST_ENDPOINT keep their historical meaning: the caller supplies ENDPOINT (the
// endpoint being checked), and a test's own Endpoint supplies TEST_ENDPOINT
// unless the caller overrode it.
func variableArgs(t Test, vars map[string]string) []string {
	merged := make(map[string]string, len(vars)+1)
	for k, v := range vars {
		if strings.TrimSpace(k) == "" {
			continue
		}
		merged[k] = v
	}
	if endpoint := strings.TrimSpace(t.Endpoint); endpoint != "" {
		if _, ok := merged["TEST_ENDPOINT"]; !ok {
			merged["TEST_ENDPOINT"] = endpoint
		}
	}

	keys := make([]string, 0, len(merged))
	for k := range merged {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	out := make([]string, 0, len(keys))
	for _, k := range keys {
		out = append(out, k+"="+merged[k])
	}
	return out
}

// classifyCmdFailure maps a failed hurl invocation onto a failure class. It
// reads the combined output because hurl reports most of these on stderr and
// the process error alone does not say which one happened.
func classifyCmdFailure(err error, msg string) FailureClass {
	if err == nil {
		return FailureNone
	}
	lmsg := strings.ToLower(msg)
	switch {
	case strings.Contains(lmsg, "not found"), strings.Contains(lmsg, "executable file not found"):
		return FailureConfig
	case strings.Contains(lmsg, "timed out"), strings.Contains(lmsg, "timeout"):
		return FailureTimeout
	case strings.Contains(lmsg, "tls"), strings.Contains(lmsg, "x509"):
		return FailureTLS
	case strings.Contains(lmsg, "unauthorized"), strings.Contains(lmsg, "forbidden"):
		return FailureAuth
	case strings.Contains(lmsg, "connection refused"), strings.Contains(lmsg, "network"):
		return FailureNetwork
	default:
		return FailureUnknown
	}
}

// compactOutput joins the two streams into one message.
func compactOutput(stdout, stderr string) string {
	parts := []string{}
	if strings.TrimSpace(stdout) != "" {
		parts = append(parts, strings.TrimSpace(stdout))
	}
	if strings.TrimSpace(stderr) != "" {
		parts = append(parts, strings.TrimSpace(stderr))
	}
	return strings.Join(parts, " | ")
}
