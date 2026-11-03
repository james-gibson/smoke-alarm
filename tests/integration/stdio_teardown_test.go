//go:build !windows

package integration_test

// Issue #44: StdioProber hung forever (past its own deadline) and leaked the
// real server whenever `stdio.command` was a wrapper — `uvx`, `npx`, a shell —
// because teardown killed only the direct child. The orphan kept the pipes open,
// so the stdout read and the stderr copy never saw EOF.
//
// These tests use `sh` as the wrapper so the grandchild is real, and assert both
// halves of the contract: Probe returns, and nothing survives it.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/james-gibson/smoke-alarm/internal/engine"
	"github.com/james-gibson/smoke-alarm/internal/targets"
)

// TestStdioProber_ReturnsWhenWrapperGrandchildHoldsThePipes covers the failure
// path: the server never answers, and a grandchild keeps stdout/stderr open. The
// probe must return at its own deadline instead of blocking on a pipe the direct
// child no longer holds.
func TestStdioProber_ReturnsWhenWrapperGrandchildHoldsThePipes(t *testing.T) {
	t.Parallel()

	pidFile := filepath.Join(t.TempDir(), "grandchild.pid")
	// `sleep` inherits the wrapper's stdout/stderr and outlives the `sh` that
	// spawned it — exactly the shape of `uvx`/`npx` with a real server.
	script := fmt.Sprintf("sleep 300 & echo $! > %s; wait", pidFile)
	target := stdioShellTarget("stdio-wrapper-holds-pipes", script, 2*time.Second)
	killWhenDone(t, pidFile)

	start := time.Now()
	res := probeStdio(t, target, 10*time.Second)
	elapsed := time.Since(start)

	if elapsed > 8*time.Second {
		t.Fatalf("probe outlived its deadline by too much: %s (timeout was 2s, message %q)", elapsed, res.Message)
	}
	if res.FailureClass != targets.FailureTimeout {
		t.Fatalf("expected timeout failure class, got %s (%s)", res.FailureClass, res.Message)
	}
	assertNoProcessSurvives(t, pidFile, 3*time.Second)
}

// TestStdioProber_TearsDownAfterSuccessfulHandshakeWhenServerIgnoresEOF covers
// the success path: the handshake passes, then the server refuses to exit when
// stdin closes. Teardown must kill the group rather than wait for an EOF that
// never comes.
func TestStdioProber_TearsDownAfterSuccessfulHandshakeWhenServerIgnoresEOF(t *testing.T) {
	t.Parallel()

	pidFile := filepath.Join(t.TempDir(), "server.pid")
	// The mock is a grandchild of the probe again: `sh` is the wrapper, the
	// built mock is what teardown has to reach. It is prebuilt rather than
	// `go run` so teardown is not racing a compiler.
	//
	// `exec 3<&0` is load-bearing rather than stylistic: a POSIX shell assigns
	// a background job's stdin from /dev/null *before* explicit redirections, so
	// a plain `cmd &` would start the server with closed stdin. Duplicating the
	// pipe to fd 3 first and redirecting from there keeps the handshake alive on
	// shells that follow the rule strictly (dash, i.e. CI's /bin/sh).
	bin := buildStubbornMock(t)
	script := fmt.Sprintf("exec 3<&0; %s <&3 & wait", bin)
	target := stdioShellTarget("stdio-stubborn-server", script, 10*time.Second)
	target.Stdio.Env = map[string]string{"STUBBORN_PID_FILE": pidFile}
	killWhenDone(t, pidFile)

	start := time.Now()
	res := probeStdio(t, target, 15*time.Second)
	elapsed := time.Since(start)

	if res.State != targets.StateHealthy {
		t.Fatalf("expected healthy handshake, got %s (%s)", res.State, res.Message)
	}
	if elapsed > 8*time.Second {
		t.Fatalf("teardown of a server that ignores EOF took %s; it must be bounded", elapsed)
	}
	assertNoProcessSurvives(t, pidFile, 3*time.Second)
}

// TestStdioProber_DirectChildStillTearsDown is the control: a server that exits
// on stdin EOF must not pay the grace window or get killed needlessly. It guards
// the process-group change against making every probe slower.
func TestStdioProber_DirectChildStillTearsDown(t *testing.T) {
	t.Parallel()

	target := stdioMockTarget("stdio-direct-teardown", mockStdioServer(t))
	target.Check.Timeout = 15 * time.Second

	start := time.Now()
	res := probeStdio(t, target, 20*time.Second)
	elapsed := time.Since(start)

	if res.State != targets.StateHealthy {
		t.Fatalf("expected healthy handshake, got %s (%s)", res.State, res.Message)
	}
	if elapsed > 5*time.Second {
		t.Fatalf("direct-child probe took %s; teardown should not add a grace penalty", elapsed)
	}
}

// buildStubbornMock compiles the fixture that ignores stdin EOF, so the test
// measures teardown instead of the Go toolchain.
func buildStubbornMock(t *testing.T) string {
	t.Helper()

	bin := filepath.Join(t.TempDir(), "mock_stubborn_stdio_server")
	cmd := exec.Command("go", "build", "-o", bin, "../../tests/testdata/mock_stubborn_stdio_server")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build stubborn mock: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return bin
}

func stdioShellTarget(id, script string, timeout time.Duration) targets.Target {
	return targets.Target{
		ID:        id,
		Enabled:   true,
		Protocol:  targets.ProtocolMCP,
		Name:      id,
		Transport: targets.TransportStdio,
		Endpoint:  "stdio://local",
		Stdio: targets.StdioCommand{
			Command: "sh",
			Args:    []string{"-c", script},
		},
		Check: targets.CheckPolicy{
			Interval: time.Second,
			Timeout:  timeout,
		},
	}
}

// probeStdio runs a probe and fails the test if it does not return in time,
// so a regression reports "did not return" instead of hanging the suite.
func probeStdio(t *testing.T, target targets.Target, budget time.Duration) targets.CheckResult {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), budget)
	defer cancel()

	type outcome struct {
		res targets.CheckResult
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		res, err := engine.NewStdioProber().Probe(ctx, target, nil)
		done <- outcome{res: res, err: err}
	}()

	select {
	case got := <-done:
		if got.err != nil {
			t.Fatalf("stdio probe returned error: %v", got.err)
		}
		return got.res
	case <-time.After(budget + 5*time.Second):
		t.Fatalf("stdio probe did not return within %s of its %s budget — teardown is blocked (issue #44)", budget+5*time.Second, budget)
		return targets.CheckResult{}
	}
}

// assertNoProcessSurvives waits for the pid the grandchild recorded to disappear.
func assertNoProcessSurvives(t *testing.T, pidFile string, within time.Duration) {
	t.Helper()

	pid := readPID(t, pidFile)
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if !processExists(pid) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("process %d (recorded in %s) survived the probe: the wrapper's grandchild was orphaned", pid, pidFile)
}

// killWhenDone makes sure a failing assertion does not leave the fixture running.
func killWhenDone(t *testing.T, pidFile string) {
	t.Helper()
	t.Cleanup(func() {
		pid, err := strconv.Atoi(strings.TrimSpace(readFileOrEmpty(pidFile)))
		if err != nil || pid <= 0 {
			return
		}
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		_ = syscall.Kill(pid, syscall.SIGKILL)
	})
}

func readPID(t *testing.T, pidFile string) int {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		raw := strings.TrimSpace(readFileOrEmpty(pidFile))
		if raw != "" {
			pid, err := strconv.Atoi(raw)
			if err != nil {
				t.Fatalf("pid file %s is not a number: %q", pidFile, raw)
			}
			return pid
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("pid file %s was never written; the fixture did not start", pidFile)
	return 0
}

func readFileOrEmpty(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func processExists(pid int) bool {
	// Signal 0 checks existence without delivering anything.
	err := syscall.Kill(pid, 0)
	return err == nil || err == syscall.EPERM
}
