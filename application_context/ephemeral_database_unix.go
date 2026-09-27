//go:build !windows

package application_context

import (
	"errors"
	"os"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

// ephemeralDirectoryName is this user's shared ephemeral directory under the temp
// directory. The uid keeps users of one machine out of each other's.
func ephemeralDirectoryName() string {
	return "mahresources-ephemeral-" + strconv.Itoa(os.Getuid())
}

// ownedByCurrentUser reports whether this user owns the file info describes.
func ownedByCurrentUser(info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == os.Getuid()
}

// privateToCurrentUser reports whether this user owns the file and nobody else
// may use it.
func privateToCurrentUser(info os.FileInfo) bool {
	return ownedByCurrentUser(info) && info.Mode().Perm()&0o077 == 0
}

// lockEphemeralFile takes the exclusive lock that marks a database live, waiting
// out a sweep that holds it for a moment. flock belongs to the open file, not the
// process, so one server can hold several and still be refused its own.
func lockEphemeralFile(f *os.File) error {
	for {
		err := unix.Flock(int(f.Fd()), unix.LOCK_EX)
		if !errors.Is(err, unix.EINTR) {
			return err
		}
	}
}

// tryLockEphemeralFile takes that lock only if no one holds it.
func tryLockEphemeralFile(f *os.File) bool {
	return unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) == nil
}
