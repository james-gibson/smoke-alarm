//go:build !windows

package engine

import (
	"errors"
	"os/exec"
	"syscall"
)

// stdioSysProcAttr puts the stdio server in its own process group.
//
// `stdio.command` is frequently a wrapper (`uvx`, `npx`, a shell) that forks or
// execs the real server. With Setpgid the direct child's pid is also its process
// group id, so teardown can signal everything the command spawned instead of
// leaving the real server reparented to PID 1 holding the pipes (issue #44).
func stdioSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// terminateStdioProcess SIGKILLs the server's whole process group.
//
// It reports success when the group is already gone: teardown and the context
// canceller both call it, and the second call must not turn into a probe error.
// If the group signal fails for any other reason it falls back to killing the
// direct child so teardown still makes progress.
func terminateStdioProcess(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}

	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return cmd.Process.Kill()
	}
	return nil
}
