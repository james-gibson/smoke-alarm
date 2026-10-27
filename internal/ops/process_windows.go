//go:build windows

package ops

import (
	"errors"
	"syscall"
)

// processAlive reports whether pid names a running process.
//
// Windows has no signal 0 (Process.Signal rejects anything but Kill with
// EWINDOWS), so the existence check goes through OpenProcess. SYNCHRONIZE is
// the least right that answers the question: the handle is signaled once the
// process terminates, and a zero-timeout wait therefore tells "still running"
// apart from "exited but not yet reaped" — an exit-code check cannot, because a
// process that exits with code 259 is indistinguishable from STILL_ACTIVE.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}

	h, err := syscall.OpenProcess(syscall.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		// ERROR_ACCESS_DENIED means the pid exists but is not ours to open —
		// the EPERM case on unix, and alive for lock-file purposes.
		// ERROR_INVALID_PARAMETER means no such pid: the lock is stale.
		return errors.Is(err, syscall.ERROR_ACCESS_DENIED)
	}
	defer func() { _ = syscall.CloseHandle(h) }()

	event, err := syscall.WaitForSingleObject(h, 0)
	if err != nil {
		// Cannot tell; assume the lock is held rather than start a second
		// instance over the top of one that may still be running.
		return true
	}
	return event == syscall.WAIT_TIMEOUT
}
