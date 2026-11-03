//go:build !windows

package ops

import (
	"errors"
	"syscall"
)

// processAlive reports whether pid names a process this user can signal.
//
// Signal 0 checks existence and permission without delivering a signal. EPERM
// means the pid exists but belongs to someone else, which still counts as
// alive: the lock file is held either way.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
