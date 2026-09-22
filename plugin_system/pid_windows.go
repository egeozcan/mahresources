//go:build windows

package plugin_system

// pidLiveness cannot tell an existing process from a reused pid on Windows:
// os.FindProcess succeeds for any pid, and there is no signal-0 equivalent that
// distinguishes "not there" from "not yours". It therefore proves nothing, which
// is the fail-safe answer — a closure-backed Job whose runtime cannot be
// inspected stays blocked rather than being interrupted or redispatched.
func pidLiveness(pid int) RuntimeLiveness {
	if pid <= 0 {
		return RuntimeUnknown
	}
	return RuntimeUnknown
}
