//go:build aix

package plugin_commands

import (
	"errors"

	"golang.org/x/sys/unix"
)

var errRuntimeLeaseBusy = errors.New("plugin command runtime lease is busy")

func lockRuntimeLease(fd int) error {
	lock := unix.Flock_t{Type: unix.F_WRLCK, Whence: 0, Start: 0, Len: 0}
	if err := unix.FcntlFlock(uintptr(fd), unix.F_SETLK, &lock); err != nil {
		if errors.Is(err, unix.EACCES) || errors.Is(err, unix.EAGAIN) {
			return errRuntimeLeaseBusy
		}
		return err
	}
	return nil
}

func unlockRuntimeLease(fd int) error {
	lock := unix.Flock_t{Type: unix.F_UNLCK, Whence: 0, Start: 0, Len: 0}
	return unix.FcntlFlock(uintptr(fd), unix.F_SETLK, &lock)
}
