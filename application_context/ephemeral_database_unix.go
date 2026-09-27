//go:build !windows

package application_context

import (
	"errors"
	"syscall"
)

// ephemeralOwnerExited reports whether pid is certainly gone. Signal 0 probes
// without delivering anything; EPERM is a live process of another user, and any
// answer other than ESRCH proves nothing, so the file is kept.
func ephemeralOwnerExited(pid int) bool {
	if pid <= 0 {
		return false
	}
	return errors.Is(syscall.Kill(pid, 0), syscall.ESRCH)
}
