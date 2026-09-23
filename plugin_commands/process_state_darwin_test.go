//go:build darwin

package plugin_commands

import (
	"errors"
	"fmt"
	"syscall"

	"golang.org/x/sys/unix"
)

func testProcessHasLiveState(pid int) (bool, error) {
	process, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if errors.Is(err, unix.ENOENT) || errors.Is(err, unix.ESRCH) {
		return false, nil
	}
	if err != nil {
		if errors.Is(err, unix.EIO) {
			probeErr := syscall.Kill(pid, 0)
			if errors.Is(probeErr, syscall.ESRCH) {
				return false, nil
			}
			if probeErr == nil || errors.Is(probeErr, syscall.EPERM) {
				return false, fmt.Errorf("query process state for live pid %d: %w", pid, err)
			}
			return false, probeErr
		}
		return false, err
	}
	if process == nil || int(process.Proc.P_pid) != pid {
		return false, nil
	}
	return process.Proc.P_stat != darwinProcessStateZombie, nil
}
