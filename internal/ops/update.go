package ops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// CommandRunner executes shell commands for lifecycle steps.
type CommandRunner interface {
	Run(ctx context.Context, command string) (string, error)
}

// shellRunner executes commands using "sh -c".
type shellRunner struct{}

func (shellRunner) Run(ctx context.Context, command string) (string, error) {
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("command failed: %w; output=%s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// DeployStep applies an update payload and returns rollback metadata.
type DeployStep func(ctx context.Context) (DeployResult, error)

// DeployResult contains deployment metadata and optional rollback hook.
type DeployResult struct {
	NewVersion string
	Rollback   func(context.Context) error
}

// Plan configures unattended stop/update/restart/verify/rollback.
type Plan struct {
	LockFilePath        string
	JournalPath         string
	CurrentVersionPath  string
	PreviousVersionPath string

	StopCommand     string
	StartCommand    string
	VerifyCommand   string
	RollbackCommand string

	HealthURL    string
	ReadyURL     string
	RequireReady bool

	GracefulStopTimeout time.Duration
	StartTimeout        time.Duration
	VerifyTimeout       time.Duration
	PollInterval        time.Duration
}

// UpdateResult describes update execution outcome.
type UpdateResult struct {
	StartedAt      time.Time
	FinishedAt     time.Time
	OldVersion     string
	NewVersion     string
	RolledBack     bool
	Committed      bool
	FailureReason  string
	JournalEntries []JournalEntry
}

// JournalEntry is a durable structured event for unattended operation.
type JournalEntry struct {
	Timestamp time.Time         `json:"timestamp"`
	Step      string            `json:"step"`
	Status    string            `json:"status"`
	Message   string            `json:"message,omitempty"`
	Meta      map[string]string `json:"meta,omitempty"`
}

// LifecycleController orchestrates unattended update workflow.
type LifecycleController struct {
	plan   Plan
	runner CommandRunner
	client *http.Client
	now    func() time.Time

	mu sync.Mutex
}

// NewLifecycleController builds a controller with defaults.
func NewLifecycleController(plan Plan) *LifecycleController {
	if plan.PollInterval <= 0 {
		plan.PollInterval = 1 * time.Second
	}
	if plan.GracefulStopTimeout <= 0 {
		plan.GracefulStopTimeout = 20 * time.Second
	}
	if plan.StartTimeout <= 0 {
		plan.StartTimeout = 60 * time.Second
	}
	if plan.VerifyTimeout <= 0 {
		plan.VerifyTimeout = 60 * time.Second
	}

	return &LifecycleController{
		plan:   plan,
		runner: shellRunner{},
		client: &http.Client{Timeout: 5 * time.Second},
		now:    time.Now,
	}
}

// SetRunner swaps command execution implementation (useful for tests).
func (c *LifecycleController) SetRunner(r CommandRunner) {
	if r == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.runner = r
}

// Execute performs:
// 1) lock
// 2) stop
// 3) deploy
// 4) restart
// 5) verify
// 6) commit markers
// and rollback on failure.
func (c *LifecycleController) Execute(ctx context.Context, deploy DeployStep) (UpdateResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	res := UpdateResult{
		StartedAt: c.now().UTC(),
	}

	appendJournal := func(step, status, msg string, meta map[string]string) {
		entry := JournalEntry{
			Timestamp: c.now().UTC(),
			Step:      step,
			Status:    status,
			Message:   msg,
			Meta:      meta,
		}
		res.JournalEntries = append(res.JournalEntries, entry)
		_ = c.writeJournal(entry)
	}

	appendJournal("acquire_lock", "started", "", nil)
	unlock, err := c.acquireLock()
	if err != nil {
		appendJournal("acquire_lock", "failed", err.Error(), nil)
		res.FinishedAt = c.now().UTC()
		res.FailureReason = "LOCK_CONTENTION"
		return res, err
	}
	defer unlock()
	appendJournal("acquire_lock", "ok", "", nil)

	oldVersion, _ := c.readTrimmed(c.plan.CurrentVersionPath)
	res.OldVersion = oldVersion

	// Stop existing runtime.
	if cmd := strings.TrimSpace(c.plan.StopCommand); cmd != "" {
		appendJournal("stop", "started", cmd, nil)
		stopCtx, cancel := context.WithTimeout(ctx, c.plan.GracefulStopTimeout)
		_, stopErr := c.runner.Run(stopCtx, cmd)
		cancel()
		if stopErr != nil {
			appendJournal("stop", "failed", stopErr.Error(), nil)
			res.FinishedAt = c.now().UTC()
			res.FailureReason = "STOP_TIMEOUT"
			return res, stopErr
		}
		appendJournal("stop", "ok", "", nil)
	}

	// Deploy new version.
	appendJournal("deploy", "started", "", nil)
	deployResult := DeployResult{
		NewVersion: c.inferVersion(),
	}
	if deploy != nil {
		deployResult, err = deploy(ctx)
		if err != nil {
			appendJournal("deploy", "failed", err.Error(), nil)
			res.FinishedAt = c.now().UTC()
			res.FailureReason = "DEPLOY_FAILED"
			return res, err
		}
		if strings.TrimSpace(deployResult.NewVersion) == "" {
			deployResult.NewVersion = c.inferVersion()
		}
	}
	res.NewVersion = deployResult.NewVersion
	appendJournal("deploy", "ok", "new version prepared", map[string]string{"new_version": res.NewVersion})

	// Start runtime.
	if cmd := strings.TrimSpace(c.plan.StartCommand); cmd != "" {
		appendJournal("start", "started", cmd, nil)
		startCtx, cancel := context.WithTimeout(ctx, c.plan.StartTimeout)
		_, startErr := c.runner.Run(startCtx, cmd)
		cancel()
		if startErr != nil {
			appendJournal("start", "failed", startErr.Error(), nil)
			rbErr := c.rollback(ctx, deployResult, oldVersion, appendJournal)
			res.RolledBack = rbErr == nil
			res.FinishedAt = c.now().UTC()
			res.FailureReason = "START_FAILED"
			return res, errors.Join(startErr, rbErr)
		}
		appendJournal("start", "ok", "", nil)
	}

	// Verify health/readiness and optional command.
	appendJournal("verify", "started", "", nil)
	verifyErr := c.verify(ctx)
	if verifyErr == nil && strings.TrimSpace(c.plan.VerifyCommand) != "" {
		verifyCtx, cancel := context.WithTimeout(ctx, c.plan.VerifyTimeout)
		_, verifyErr = c.runner.Run(verifyCtx, c.plan.VerifyCommand)
		cancel()
	}
	if verifyErr != nil {
		appendJournal("verify", "failed", verifyErr.Error(), nil)
		rbErr := c.rollback(ctx, deployResult, oldVersion, appendJournal)
		res.RolledBack = rbErr == nil
		res.FinishedAt = c.now().UTC()
		res.FailureReason = "VERIFY_FAILED"
		return res, errors.Join(verifyErr, rbErr)
	}
	appendJournal("verify", "ok", "", nil)

	// Commit markers.
	appendJournal("commit", "started", "", nil)
	if oldVersion != "" {
		if err := c.writeTrimmed(c.plan.PreviousVersionPath, oldVersion); err != nil {
			appendJournal("commit", "failed", err.Error(), nil)
			res.FinishedAt = c.now().UTC()
			res.FailureReason = "COMMIT_FAILED"
			return res, err
		}
	}
	if err := c.writeTrimmed(c.plan.CurrentVersionPath, res.NewVersion); err != nil {
		appendJournal("commit", "failed", err.Error(), nil)
		res.FinishedAt = c.now().UTC()
		res.FailureReason = "COMMIT_FAILED"
		return res, err
	}
	appendJournal("commit", "ok", "", map[string]string{
		"old_version": oldVersion,
		"new_version": res.NewVersion,
	})

	res.Committed = true
	res.FinishedAt = c.now().UTC()
	return res, nil
}

func (c *LifecycleController) rollback(
	ctx context.Context,
	deployResult DeployResult,
	oldVersion string,
	appendJournal func(step, status, msg string, meta map[string]string),
) error {
	appendJournal("rollback", "started", "", map[string]string{"old_version": oldVersion})

	var errs []error

	// Stop failed runtime again before rollback start.
	if cmd := strings.TrimSpace(c.plan.StopCommand); cmd != "" {
		stopCtx, cancel := context.WithTimeout(ctx, c.plan.GracefulStopTimeout)
		_, err := c.runner.Run(stopCtx, cmd)
		cancel()
		if err != nil {
			errs = append(errs, fmt.Errorf("rollback stop failed: %w", err))
		}
	}

	// Custom rollback hook from deploy step.
	if deployResult.Rollback != nil {
		if err := deployResult.Rollback(ctx); err != nil {
			errs = append(errs, fmt.Errorf("deploy rollback hook failed: %w", err))
		}
	}

	// Optional rollback command.
	if cmd := strings.TrimSpace(c.plan.RollbackCommand); cmd != "" {
		rbCtx, cancel := context.WithTimeout(ctx, c.plan.StartTimeout)
		_, err := c.runner.Run(rbCtx, cmd)
		cancel()
		if err != nil {
			errs = append(errs, fmt.Errorf("rollback command failed: %w", err))
		}
	}

	// Best effort start old runtime.
	if cmd := strings.TrimSpace(c.plan.StartCommand); cmd != "" {
		startCtx, cancel := context.WithTimeout(ctx, c.plan.StartTimeout)
		_, err := c.runner.Run(startCtx, cmd)
		cancel()
		if err != nil {
			errs = append(errs, fmt.Errorf("restart previous runtime failed: %w", err))
		}
	}

	if len(errs) > 0 {
		joined := errors.Join(errs...)
		appendJournal("rollback", "failed", joined.Error(), nil)
		return joined
	}

	appendJournal("rollback", "ok", "", nil)
	return nil
}

func (c *LifecycleController) verify(ctx context.Context) error {
	deadline := c.plan.VerifyTimeout
	if deadline <= 0 {
		deadline = 60 * time.Second
	}
	verifyCtx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()

	if strings.TrimSpace(c.plan.HealthURL) != "" {
		if err := c.waitHTTP200(verifyCtx, c.plan.HealthURL); err != nil {
			return fmt.Errorf("health verification failed: %w", err)
		}
	}
	if c.plan.RequireReady && strings.TrimSpace(c.plan.ReadyURL) != "" {
		if err := c.waitHTTP200(verifyCtx, c.plan.ReadyURL); err != nil {
			return fmt.Errorf("readiness verification failed: %w", err)
		}
	}
	return nil
}

func (c *LifecycleController) waitHTTP200(ctx context.Context, rawURL string) error {
	reqURL := strings.TrimSpace(rawURL)
	if reqURL == "" {
		return errors.New("empty URL")
	}

	ticker := time.NewTicker(c.plan.PollInterval)
	defer ticker.Stop()

	for {
		ok, err := c.httpIs200(ctx, reqURL)
		if err == nil && ok {
			return nil
		}
		if ctx.Err() != nil {
			if err != nil {
				return fmt.Errorf("%w; last_error=%w", ctx.Err(), err)
			}
			return ctx.Err()
		}

		select {
		case <-ctx.Done():
			if err != nil {
				return fmt.Errorf("%w; last_error=%w", ctx.Err(), err)
			}
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (c *LifecycleController) httpIs200(ctx context.Context, rawURL string) (bool, error) {
	parsed, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, http.NoBody)
	if err != nil {
		return false, err
	}
	resp, err := c.client.Do(parsed)
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == http.StatusOK, nil
}

func (c *LifecycleController) acquireLock() (func(), error) {
	lockPath := strings.TrimSpace(c.plan.LockFilePath)
	if lockPath == "" {
		return func() {}, nil
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("create lock dir: %w", err)
	}
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("acquire lock %q: %w", lockPath, err)
	}
	_, _ = fmt.Fprintf(f, "%d\n", os.Getpid())
	_ = f.Close()

	return func() { _ = os.Remove(lockPath) }, nil
}

func (c *LifecycleController) writeJournal(entry JournalEntry) error {
	if strings.TrimSpace(c.plan.JournalPath) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(c.plan.JournalPath), 0o755); err != nil {
		return fmt.Errorf("create journal dir: %w", err)
	}
	f, err := os.OpenFile(c.plan.JournalPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open journal: %w", err)
	}
	defer func() { _ = f.Close() }()

	b, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

func (c *LifecycleController) readTrimmed(path string) (string, error) {
	if strings.TrimSpace(path) == "" {
		return "", nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func (c *LifecycleController) writeTrimmed(path, value string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(value)+"\n"), 0o644)
}

func (c *LifecycleController) inferVersion() string {
	if strings.TrimSpace(c.plan.CurrentVersionPath) != "" {
		if v, err := c.readTrimmed(c.plan.CurrentVersionPath); err == nil && v != "" {
			// caller should provide proper version; this is only fallback behavior.
			return "next-of-" + v
		}
	}
	return c.now().UTC().Format("20060102-150405")
}
