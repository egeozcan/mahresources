//go:build windows

package application_context

import "os"

// ephemeralOwnerExited never claims a file is orphaned on Windows: the sweep only
// deletes what it can prove is abandoned, and it has no liveness probe there.
func ephemeralOwnerExited(pid int) bool {
	return false
}

// ephemeralDatabaseOpen answers yes on Windows, where the sweep deletes nothing.
func ephemeralDatabaseOpen(path string) bool {
	return true
}

// ephemeralDirectoryName is the shared ephemeral directory under the temp
// directory, which Windows already keeps per user.
func ephemeralDirectoryName() string {
	return "mahresources-ephemeral"
}

// ownedByCurrentUser cannot be answered from a FileInfo on Windows; nothing that
// asks deletes anything there.
func ownedByCurrentUser(info os.FileInfo) bool {
	return false
}

// privateToCurrentUser answers yes: the per-user temp directory is private
// already, and Windows reports no Unix permission bits to check.
func privateToCurrentUser(info os.FileInfo) bool {
	return true
}
