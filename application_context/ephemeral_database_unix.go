//go:build !windows

package application_context

import (
	"errors"
	"io"
	"os"
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

// ephemeralDatabaseOpen reports whether another process still has the database at
// path open. Every SQLite connection in WAL mode holds a lock on the -shm file for
// as long as it is open, and a lock outlives nothing but its process, so this also
// sees an owner whose pid is invisible here (another PID namespace sharing the
// directory). A probe that fails proves nothing, so it answers yes.
func ephemeralDatabaseOpen(path string) bool {
	shm, err := os.Open(path + "-shm")
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	if err != nil {
		return true
	}
	defer shm.Close()
	probe := syscall.Flock_t{Type: syscall.F_WRLCK, Whence: io.SeekStart}
	if err := syscall.FcntlFlock(shm.Fd(), syscall.F_GETLK, &probe); err != nil {
		return true
	}
	return probe.Type != syscall.F_UNLCK
}
