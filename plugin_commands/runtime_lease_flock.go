//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package plugin_commands

import (
	"errors"

	"golang.org/x/sys/unix"
)

func claimRuntimeLeaseProcess(_ int) (func(), error) {
	return func() {}, nil
}

func lockRuntimeLease(fd int) error {
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return ErrRuntimeLeaseBusy
		}
		return err
	}
	return nil
}

func unlockRuntimeLease(fd int) error {
	return unix.Flock(fd, unix.LOCK_UN)
}
