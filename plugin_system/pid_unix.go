//go:build !windows

package plugin_system

import (
	"errors"
	"syscall"
)

// pidLiveness answers whether one pid exists on this host.
//
// Signal 0 is the classic existence probe: it delivers nothing and reports whether
// the process could be signalled. EPERM is a process that exists and belongs to
// somebody else — inside a container that is the ordinary case for a shared host —
// so it reads as Alive, and only ESRCH reads as Gone. Anything else is Unknown:
// a probe that failed for a reason this code does not understand has proved
// nothing, and guessing either way is how work gets interrupted by mistake.
func pidLiveness(pid int) RuntimeLiveness {
	if pid <= 0 {
		return RuntimeUnknown
	}
	err := syscall.Kill(pid, 0)
	switch {
	case err == nil, errors.Is(err, syscall.EPERM):
		return RuntimeAlive
	case errors.Is(err, syscall.ESRCH):
		return RuntimeGone
	default:
		return RuntimeUnknown
	}
}
