//go:build windows

package application_context

import "os"

// ephemeralDirectoryName is the shared ephemeral directory under the temp
// directory, which Windows keeps per user unless TEMP says otherwise.
func ephemeralDirectoryName() string {
	return "mahresources-ephemeral"
}

// privateToCurrentUser answers yes: Windows reports no Unix permission bits, and
// its ACLs are not checked here.
func privateToCurrentUser(info os.FileInfo) bool {
	return true
}

// lockEphemeralFile takes no lock on Windows.
func lockEphemeralFile(f *os.File) error {
	return nil
}

// tryLockEphemeralFile never answers that a database is abandoned on Windows, so
// the sweep deletes nothing there.
func tryLockEphemeralFile(f *os.File) bool {
	return false
}
