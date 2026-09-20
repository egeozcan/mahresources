//go:build darwin

package plugin_commands

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"syscall"

	"golang.org/x/sys/unix"
)

const darwinProcessStateZombie = 5 // SZOMB from <sys/proc.h>

func (nativeProcessInspector) InspectGroup(pgid int, runID string) (GroupIdentity, error) {
	processes, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return GroupIdentity{}, fmt.Errorf("enumerate processes: %w", err)
	}
	pids := make([]int, 0)
	for _, process := range processes {
		// A zombie has exited and cannot write into the exchange directory. The
		// runner may be the only process able to reap its direct child, so counting
		// that child as a surviving group member would deadlock recovery before
		// cmd.Wait can run.
		if int(process.Eproc.Pgid) == pgid && process.Proc.P_stat != darwinProcessStateZombie {
			pids = append(pids, int(process.Proc.P_pid))
		}
	}
	if len(pids) == 0 {
		return GroupIdentity{State: GroupDead}, nil
	}
	sort.Ints(pids)
	want := "MAHR_COMMAND_RUN_ID=" + runID
	for _, pid := range pids {
		raw, err := unix.SysctlRaw("kern.procargs2", pid)
		if err != nil {
			continue
		}
		environment, err := darwinProcessEnvironment(raw)
		if err != nil {
			continue
		}
		for _, value := range environment {
			if value == want {
				// One marked member binds the process group to this run. A child may
				// scrub its own environment while remaining in that owned group.
				return GroupIdentity{State: GroupAliveOwned, PIDs: pids}, nil
			}
		}
	}
	return GroupIdentity{State: GroupAliveUnverified, PIDs: pids}, nil
}

func darwinProcessEnvironment(raw []byte) ([]string, error) {
	if len(raw) < 4 {
		return nil, errors.New("short KERN_PROCARGS2 response")
	}
	argc := int(binary.LittleEndian.Uint32(raw[:4]))
	if argc < 0 || argc > 1<<20 {
		return nil, errors.New("invalid KERN_PROCARGS2 argc")
	}
	index := 4
	var ok bool
	if index, ok = skipDarwinCString(raw, index); !ok { // executable path
		return nil, errors.New("missing executable path")
	}
	for index < len(raw) && raw[index] == 0 {
		index++
	}
	for i := 0; i < argc; i++ {
		if index, ok = skipDarwinCString(raw, index); !ok {
			return nil, errors.New("truncated argv")
		}
	}
	result := make([]string, 0)
	for index < len(raw) {
		if raw[index] == 0 {
			index++
			continue
		}
		start := index
		if index, ok = skipDarwinCString(raw, index); !ok {
			return nil, errors.New("truncated environment")
		}
		result = append(result, string(raw[start:index-1]))
	}
	return result, nil
}

func skipDarwinCString(raw []byte, index int) (int, bool) {
	for index < len(raw) {
		index++
		if raw[index-1] == 0 {
			return index, true
		}
	}
	return index, false
}

func (nativeProcessInspector) KillGroup(pgid int) error {
	return syscall.Kill(-pgid, syscall.SIGKILL)
}
