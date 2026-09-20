//go:build linux

package plugin_commands

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
)

func (nativeProcessInspector) InspectGroup(pgid int, runID string) (GroupIdentity, error) {
	return inspectLinuxGroup("/proc", pgid, runID, func(pid int) error {
		return syscall.Kill(pid, 0)
	})
}

func inspectLinuxGroup(procRoot string, pgid int, runID string, probe func(int) error) (GroupIdentity, error) {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return GroupIdentity{}, fmt.Errorf("enumerate %s: %w", procRoot, err)
	}
	pids := make([]int, 0)
	targetZombies := 0
	unresolved := false
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		memberPGID, zombie, observed := inspectLinuxProcess(procRoot, pid)
		if !observed {
			unresolved = true
			continue
		}
		if memberPGID != pgid {
			continue
		}
		if zombie {
			targetZombies++
			continue
		}
		pids = append(pids, pid)
	}

	if len(pids) > 0 {
		sort.Ints(pids)
		want := []byte("MAHR_COMMAND_RUN_ID=" + runID)
		for _, pid := range pids {
			environ, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "environ"))
			if err != nil {
				continue
			}
			for _, item := range bytes.Split(environ, []byte{0}) {
				if bytes.Equal(item, want) {
					// One marked member binds the process group to this run. Descendants
					// may deliberately scrub their environments without becoming a new
					// process group or invalidating that ownership proof.
					return GroupIdentity{State: GroupAliveOwned, PIDs: pids}, nil
				}
			}
		}
		return GroupIdentity{State: GroupAliveUnverified, PIDs: pids}, nil
	}

	// A zombie has exited and can no longer write into the exchange folder.
	// PID 1 may leave orphaned descendants unreaped, so signal zero must not
	// revive a group whose only observed target members are zombies.
	if targetZombies > 0 && !unresolved {
		return GroupIdentity{State: GroupDead}, nil
	}
	// A sample whose group and state remained unreadable may be a live target
	// member. Keep recovery fail-closed rather than infer death from the other
	// target members or from a process-group probe.
	if unresolved {
		return GroupIdentity{State: GroupAliveUnverified}, nil
	}

	err = probe(-pgid)
	switch {
	case err == nil || errors.Is(err, syscall.EPERM):
		return GroupIdentity{State: GroupAliveUnverified}, nil
	case errors.Is(err, syscall.ESRCH):
		return GroupIdentity{State: GroupDead}, nil
	default:
		return GroupIdentity{}, fmt.Errorf("probe process group %d: %w", pgid, err)
	}
}

func inspectLinuxProcess(procRoot string, pid int) (pgid int, zombie bool, observed bool) {
	statPath := filepath.Join(procRoot, strconv.Itoa(pid), "stat")
	for attempt := 0; attempt < 2; attempt++ {
		data, err := os.ReadFile(statPath)
		if err == nil {
			pgid, zombie, err = linuxStatProcess(data)
			if err == nil {
				return pgid, zombie, true
			}
		}
	}

	data, err := os.ReadFile(filepath.Join(procRoot, strconv.Itoa(pid), "status"))
	if err != nil {
		// A process which vanished during the bounded retry is no longer a
		// candidate. Any other failure leaves a possible target unresolved.
		return 0, false, errors.Is(err, os.ErrNotExist)
	}
	pgid, zombie, err = linuxStatusProcess(data)
	if err != nil {
		return 0, false, false
	}
	return pgid, zombie, true
}

func linuxStatProcess(data []byte) (pgid int, zombie bool, err error) {
	// comm is parenthesized and may contain spaces or ')' characters. The last
	// ')' is the only stable delimiter before state, ppid and pgrp.
	end := strings.LastIndexByte(string(data), ')')
	if end < 0 || end+2 >= len(data) {
		return 0, false, errors.New("missing comm delimiter")
	}
	fields := strings.Fields(string(data[end+1:]))
	if len(fields) < 3 {
		return 0, false, errors.New("missing state or pgrp field")
	}
	pgid, err = strconv.Atoi(fields[2])
	if err != nil {
		return 0, false, fmt.Errorf("parse pgrp: %w", err)
	}
	return pgid, fields[0] == "Z", nil
}

func linuxStatusProcess(data []byte) (pgid int, zombie bool, err error) {
	var state string
	var namespacePGIDs []string
	for _, line := range strings.Split(string(data), "\n") {
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch name {
		case "State":
			fields := strings.Fields(value)
			if len(fields) > 0 {
				state = fields[0]
			}
		case "NSpgid":
			namespacePGIDs = strings.Fields(value)
		}
	}
	if state == "" || len(namespacePGIDs) == 0 {
		return 0, false, errors.New("missing state or NSpgid field")
	}
	// NSpgid is ordered from the procfs mount's PID namespace through nested
	// namespaces. The first value is visible alongside the process IDs enumerated
	// from this procfs mount.
	pgid, err = strconv.Atoi(namespacePGIDs[0])
	if err != nil || pgid <= 0 {
		if err == nil {
			err = errors.New("non-positive process group")
		}
		return 0, false, fmt.Errorf("parse NSpgid: %w", err)
	}
	return pgid, state == "Z", nil
}

func (nativeProcessInspector) KillGroup(pgid int) error {
	return syscall.Kill(-pgid, syscall.SIGKILL)
}
