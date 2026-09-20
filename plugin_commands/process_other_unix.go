//go:build aix || dragonfly || freebsd || netbsd || openbsd || solaris

package plugin_commands

import (
	"errors"
	"syscall"
)

func (nativeProcessInspector) InspectGroup(pgid int, _ string) (GroupIdentity, error) {
	err := syscall.Kill(-pgid, 0)
	switch {
	case err == nil || errors.Is(err, syscall.EPERM):
		// These platforms have no implementation here which can bind every group
		// member to MAHR_COMMAND_RUN_ID or exclude a zombie-only group. Existence is
		// not ownership, and a zombie may keep the runtime quarantined until its
		// parent reaps it or the server restarts.
		return GroupIdentity{State: GroupAliveUnverified}, nil
	case errors.Is(err, syscall.ESRCH):
		return GroupIdentity{State: GroupDead}, nil
	default:
		return GroupIdentity{}, err
	}
}

func (nativeProcessInspector) KillGroup(pgid int) error {
	return syscall.Kill(-pgid, syscall.SIGKILL)
}
