//go:build windows

package engine

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// stdioSysProcAttr is nil on Windows: syscall.SysProcAttr has no process-group
// equivalent, and job objects would be needed to signal a wrapper's children.
func stdioSysProcAttr() *syscall.SysProcAttr { return nil }

// terminateStdioProcess kills the direct child only.
//
// A wrapper's children (npx.cmd → node, cmd /c → …) can therefore survive a
// probe on Windows. Teardown still returns: the stderr wait and cmd.Wait are
// both time-limited, so this leaks a process at worst instead of hanging the
// monitor (issue #44). Closing stdin first means a well-behaved server usually
// exits by itself anyway.
func terminateStdioProcess(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}
