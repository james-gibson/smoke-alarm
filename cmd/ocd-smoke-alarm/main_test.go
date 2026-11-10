package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/james-gibson/smoke-alarm/internal/config"
)

// TestToTargetCopiesHeaders pins the #85 one-shot-path fix: toTarget must carry
// the static targets[].headers map onto the compiled Target, matching
// engine.compileTargets, so the two config conversions cannot drift again.
func TestToTargetCopiesHeaders(t *testing.T) {
	target, err := toTarget(config.TargetConfig{
		ID:        "t1",
		Enabled:   true,
		Protocol:  "http",
		Endpoint:  "http://127.0.0.1:8099/mcp",
		Transport: "http",
		Headers:   map[string]string{"X-Custom": "value"},
		Check:     config.TargetCheckConfig{Interval: "1m", Timeout: "5s"},
	}, config.Config{})
	if err != nil {
		t.Fatalf("toTarget: %v", err)
	}
	if got := target.Headers["X-Custom"]; got != "value" {
		t.Fatalf("target.Headers[X-Custom] = %q, want %q", got, "value")
	}
}

func TestCmdOpsHelp(t *testing.T) {
	for _, arg := range []string{"-h", "--help", "help"} {
		if err := cmdOps([]string{arg}); err != nil {
			t.Errorf("cmdOps(%q) = %v, want nil", arg, err)
		}
	}
}

func TestCmdOpsSubcommandHelp(t *testing.T) {
	cases := [][]string{
		{"stop", "-h"},
		{"stop", "--help"},
		{"status", "-h"},
		{"status", "--help"},
		{"reload", "-h"},
		{"reload", "--help"},
		{"self-check", "-h"},
		{"self-check", "--help"},
	}
	for _, args := range cases {
		if err := cmdOps(args); err != nil {
			t.Errorf("cmdOps(%v) = %v, want nil", args, err)
		}
	}
}

func TestCmdOpsUnknownSubcommand(t *testing.T) {
	if err := cmdOps([]string{"bogus"}); err == nil {
		t.Error("cmdOps(bogus) = nil, want error")
	}
}

func TestSignalReloadMissingPidFile(t *testing.T) {
	if err := signalReload(filepath.Join(t.TempDir(), "missing.pid")); err == nil {
		t.Error("signalReload(missing pid file) = nil, want error")
	}
}

func TestSignalReloadSendsSIGHUP(t *testing.T) {
	// Trap SIGHUP so the helper survives the signal and stays observable.
	cmd := exec.Command("sh", "-c", `trap "" HUP; sleep 30`)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	pidFile := filepath.Join(t.TempDir(), "sa.pid")
	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(cmd.Process.Pid)), 0o600); err != nil {
		t.Fatalf("write pid file: %v", err)
	}

	if err := signalReload(pidFile); err != nil {
		t.Fatalf("signalReload: %v", err)
	}
	if !isProcessRunning(cmd.Process.Pid) {
		t.Fatalf("helper pid=%d should still be running after SIGHUP", cmd.Process.Pid)
	}
}
