package safety

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/targets"
)

// Outcome represents the result of a stop-gap safety test.
type Outcome string

const (
	OutcomePass Outcome = "pass"
	OutcomeFail Outcome = "fail"
	OutcomeSkip Outcome = "skip"
)

// Stage identifies the validation stage for this scanner.
const Stage = "pre_protocol_hurl"

// Result is one safety test execution result.
type Result struct {
	TargetID     string               `json:"target_id"`
	TestName     string               `json:"test_name"`
	Stage        string               `json:"stage"`
	Outcome      Outcome              `json:"outcome"`
	FailureClass targets.FailureClass `json:"failure_class,omitempty"`
	StatusCode   int                  `json:"status_code,omitempty"`
	Message      string               `json:"message,omitempty"`
	Duration     time.Duration        `json:"duration,omitempty"`
	StartedAt    time.Time            `json:"started_at"`
	FinishedAt   time.Time            `json:"finished_at"`
}

// Report aggregates results for one target.
type Report struct {
	TargetID     string    `json:"target_id"`
	StartedAt    time.Time `json:"started_at"`
	FinishedAt   time.Time `json:"finished_at"`
	Passed       int       `json:"passed"`
	Failed       int       `json:"failed"`
	Skipped      int       `json:"skipped"`
	HasBlocking  bool      `json:"has_blocking"`
	Results      []Result  `json:"results"`
	AnyTestsSeen bool      `json:"any_tests_seen"`
}

// CommandRunner allows command execution to be mocked in tests.
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

// Scanner runs registered HURL stop-gap tests before deeper protocol validation.
type Scanner struct {
	mu sync.RWMutex

	registered map[string][]targets.HURLTest

	client  *http.Client
	runner  CommandRunner
	hurlBin string
}

// Option configures Scanner.
type Option func(*Scanner)

// WithHTTPClient sets custom HTTP client for endpoint-based stop-gap checks.
func WithHTTPClient(c *http.Client) Option {
	return func(s *Scanner) {
		if c != nil {
			s.client = c
		}
	}
}

// WithCommandRunner sets a custom command runner (mainly for tests).
func WithCommandRunner(r CommandRunner) Option {
	return func(s *Scanner) {
		if r != nil {
			s.runner = r
		}
	}
}

// WithHURLBinary sets the HURL executable name/path (default: "hurl").
func WithHURLBinary(bin string) Option {
	return func(s *Scanner) {
		if strings.TrimSpace(bin) != "" {
			s.hurlBin = strings.TrimSpace(bin)
		}
	}
}

// NewScanner creates a pre-protocol safety scanner.
func NewScanner(opts ...Option) *Scanner {
	s := &Scanner{
		registered: make(map[string][]targets.HURLTest),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		runner:  execRunner{},
		hurlBin: "hurl",
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// RegisterTarget registers a target's HURL tests.
// Existing tests for the target are replaced.
func (s *Scanner) RegisterTarget(t targets.Target) error {
	if strings.TrimSpace(t.ID) == "" {
		return errors.New("target id is required")
	}
	if err := validateHURLTests(t.Check.HURLTests); err != nil {
		return fmt.Errorf("target %q hurl_tests invalid: %w", t.ID, err)
	}

	tests := cloneHURLTests(t.Check.HURLTests)

	s.mu.Lock()
	s.registered[t.ID] = tests
	s.mu.Unlock()
	return nil
}

// Register explicitly registers HURL tests for a target ID.
func (s *Scanner) Register(targetID string, tests []targets.HURLTest) error {
	if strings.TrimSpace(targetID) == "" {
		return errors.New("target id is required")
	}
	if err := validateHURLTests(tests); err != nil {
		return fmt.Errorf("target %q hurl_tests invalid: %w", targetID, err)
	}

	s.mu.Lock()
	s.registered[targetID] = cloneHURLTests(tests)
	s.mu.Unlock()
	return nil
}

// Unregister removes HURL tests for target ID.
func (s *Scanner) Unregister(targetID string) {
	s.mu.Lock()
	delete(s.registered, targetID)
	s.mu.Unlock()
}

// HasRegistered reports whether target has any registered HURL tests.
func (s *Scanner) HasRegistered(targetID string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tests := s.registered[targetID]
	return len(tests) > 0
}

// RunTarget executes registered HURL stop-gap tests for a target.
// This should be run before protocol validation for the same target.
func (s *Scanner) RunTarget(ctx context.Context, t targets.Target) Report {
	start := time.Now().UTC()
	report := Report{
		TargetID:  t.ID,
		StartedAt: start,
		Results:   []Result{},
	}

	tests := s.getTests(t.ID)
	if len(tests) == 0 {
		report.FinishedAt = time.Now().UTC()
		report.AnyTestsSeen = false
		return report
	}
	report.AnyTestsSeen = true

	for _, tt := range tests {
		select {
		case <-ctx.Done():
			report.Results = append(report.Results, Result{
				TargetID:     t.ID,
				TestName:     tt.Name,
				Stage:        Stage,
				Outcome:      OutcomeSkip,
				FailureClass: targets.FailureUnknown,
				Message:      "context canceled before test execution",
				StartedAt:    time.Now().UTC(),
				FinishedAt:   time.Now().UTC(),
			})
			report.Skipped++
			report.HasBlocking = report.HasBlocking || false
			continue
		default:
		}

		r := s.runSingle(ctx, t, tt)
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
	return report
}

func (s *Scanner) runSingle(ctx context.Context, t targets.Target, ht targets.HURLTest) Result {
	start := time.Now().UTC()

	// File-backed HURL test: run actual "hurl --test <file>".
	if strings.TrimSpace(ht.File) != "" {
		res := s.runHURLFile(ctx, t, ht)
		res.StartedAt = start
		res.FinishedAt = time.Now().UTC()
		res.Duration = res.FinishedAt.Sub(start)
		return res
	}

	// Endpoint-backed stop-gap HTTP check (hurl-like registration, lightweight in-process).
	res := s.runEndpointHTTP(ctx, t, ht)
	res.StartedAt = start
	res.FinishedAt = time.Now().UTC()
	res.Duration = res.FinishedAt.Sub(start)
	return res
}

func (s *Scanner) runHURLFile(ctx context.Context, t targets.Target, ht targets.HURLTest) Result {
	if s.runner == nil {
		return Result{
			TargetID:     t.ID,
			TestName:     ht.Name,
			Stage:        Stage,
			Outcome:      OutcomeSkip,
			FailureClass: targets.FailureConfig,
			Message:      "no command runner configured for hurl file test",
		}
	}

	args := []string{"--test", ht.File}
	// Optional variable injection: ENDPOINT defaults to target endpoint.
	if strings.TrimSpace(t.Endpoint) != "" {
		args = append(args, "--variable", "ENDPOINT="+t.Endpoint)
	}
	if strings.TrimSpace(ht.Endpoint) != "" {
		args = append(args, "--variable", "TEST_ENDPOINT="+ht.Endpoint)
	}

	stdout, stderr, err := s.runner.Run(ctx, s.hurlBin, args...)
	if err != nil {
		msg := compactCmdOutput(stdout, stderr)
		if msg == "" {
			msg = err.Error()
		}
		return Result{
			TargetID:     t.ID,
			TestName:     ht.Name,
			Stage:        Stage,
			Outcome:      OutcomeFail,
			FailureClass: classifyCmdFailure(err, msg),
			Message:      "hurl file test failed: " + msg,
		}
	}

	return Result{
		TargetID:     t.ID,
		TestName:     ht.Name,
		Stage:        Stage,
		Outcome:      OutcomePass,
		FailureClass: targets.FailureNone,
		Message:      "hurl file test passed",
	}
}

func (s *Scanner) runEndpointHTTP(ctx context.Context, t targets.Target, ht targets.HURLTest) Result {
	endpoint := strings.TrimSpace(ht.Endpoint)
	if endpoint == "" {
		endpoint = strings.TrimSpace(t.Endpoint)
	}
	if endpoint == "" {
		return Result{
			TargetID:     t.ID,
			TestName:     ht.Name,
			Stage:        Stage,
			Outcome:      OutcomeFail,
			FailureClass: targets.FailureConfig,
			Message:      "no endpoint available for endpoint-based hurl test",
		}
	}

	method := strings.ToUpper(strings.TrimSpace(ht.Method))
	if method == "" {
		method = http.MethodGet
	}

	var body io.Reader
	if ht.Body != "" {
		body = strings.NewReader(ht.Body)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return Result{
			TargetID:     t.ID,
			TestName:     ht.Name,
			Stage:        Stage,
			Outcome:      OutcomeFail,
			FailureClass: targets.FailureConfig,
			Message:      "invalid endpoint request: " + err.Error(),
		}
	}
	for k, v := range ht.Headers {
		req.Header.Set(k, v)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return Result{
			TargetID:     t.ID,
			TestName:     ht.Name,
			Stage:        Stage,
			Outcome:      OutcomeFail,
			FailureClass: classifyHTTPError(err),
			Message:      "endpoint safety check failed: " + err.Error(),
		}
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return Result{
			TargetID:     t.ID,
			TestName:     ht.Name,
			Stage:        Stage,
			Outcome:      OutcomeFail,
			FailureClass: targets.FailureProtocol,
			StatusCode:   resp.StatusCode,
			Message:      fmt.Sprintf("endpoint safety check returned status %d", resp.StatusCode),
		}
	}

	return Result{
		TargetID:     t.ID,
		TestName:     ht.Name,
		Stage:        Stage,
		Outcome:      OutcomePass,
		FailureClass: targets.FailureNone,
		StatusCode:   resp.StatusCode,
		Message:      "endpoint safety check passed",
	}
}

func (s *Scanner) getTests(targetID string) []targets.HURLTest {
	s.mu.RLock()
	defer s.mu.RUnlock()
	src := s.registered[targetID]
	return cloneHURLTests(src)
}

func validateHURLTests(tests []targets.HURLTest) error {
	for i, ht := range tests {
		if strings.TrimSpace(ht.Name) == "" {
			return fmt.Errorf("hurl_tests[%d].name is required", i)
		}
		hasFile := strings.TrimSpace(ht.File) != ""
		hasEndpoint := strings.TrimSpace(ht.Endpoint) != ""
		if !hasFile && !hasEndpoint {
			return fmt.Errorf("hurl_tests[%d] requires either file or endpoint", i)
		}
		if hasFile && hasEndpoint {
			return fmt.Errorf("hurl_tests[%d].file and .endpoint are mutually exclusive", i)
		}
		if m := strings.ToUpper(strings.TrimSpace(ht.Method)); m != "" {
			switch m {
			case http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodHead, http.MethodOptions:
			default:
				return fmt.Errorf("hurl_tests[%d].method %q is unsupported", i, ht.Method)
			}
		}
	}
	return nil
}

func cloneHURLTests(in []targets.HURLTest) []targets.HURLTest {
	if len(in) == 0 {
		return nil
	}
	out := make([]targets.HURLTest, len(in))
	for i := range in {
		out[i] = targets.HURLTest{
			Name:     in[i].Name,
			File:     in[i].File,
			Endpoint: in[i].Endpoint,
			Method:   in[i].Method,
			Body:     in[i].Body,
		}
		if len(in[i].Headers) > 0 {
			out[i].Headers = make(map[string]string, len(in[i].Headers))
			for k, v := range in[i].Headers {
				out[i].Headers[k] = v
			}
		}
	}
	return out
}

func classifyCmdFailure(err error, msg string) targets.FailureClass {
	if err == nil {
		return targets.FailureNone
	}
	lmsg := strings.ToLower(msg)
	switch {
	case strings.Contains(lmsg, "not found"), strings.Contains(lmsg, "executable file not found"):
		return targets.FailureConfig
	case strings.Contains(lmsg, "timed out"), strings.Contains(lmsg, "timeout"):
		return targets.FailureTimeout
	case strings.Contains(lmsg, "tls"), strings.Contains(lmsg, "x509"):
		return targets.FailureTLS
	case strings.Contains(lmsg, "unauthorized"), strings.Contains(lmsg, "forbidden"):
		return targets.FailureAuth
	case strings.Contains(lmsg, "connection refused"), strings.Contains(lmsg, "network"):
		return targets.FailureNetwork
	default:
		return targets.FailureUnknown
	}
}

func classifyHTTPError(err error) targets.FailureClass {
	if err == nil {
		return targets.FailureNone
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "timeout"):
		return targets.FailureTimeout
	case strings.Contains(msg, "tls"), strings.Contains(msg, "x509"):
		return targets.FailureTLS
	case strings.Contains(msg, "connection refused"), strings.Contains(msg, "no such host"), strings.Contains(msg, "dial tcp"):
		return targets.FailureNetwork
	default:
		return targets.FailureUnknown
	}
}

func compactCmdOutput(stdout, stderr string) string {
	parts := []string{}
	if strings.TrimSpace(stdout) != "" {
		parts = append(parts, strings.TrimSpace(stdout))
	}
	if strings.TrimSpace(stderr) != "" {
		parts = append(parts, strings.TrimSpace(stderr))
	}
	return strings.Join(parts, " | ")
}
