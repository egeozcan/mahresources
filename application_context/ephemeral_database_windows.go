//go:build windows

package application_context

// ephemeralOwnerExited never claims a file is orphaned on Windows: the sweep only
// deletes what it can prove is abandoned, and it has no liveness probe there.
func ephemeralOwnerExited(pid int) bool {
	return false
}
