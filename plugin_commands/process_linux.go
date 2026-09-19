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
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return GroupIdentity{}, fmt.Errorf("enumerate /proc: %w", err)
	}
	pids := make([]int, 0)
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		data, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return GroupIdentity{}, fmt.Errorf("read process %d stat: %w", pid, err)
		}
		memberPGID, err := linuxStatProcessGroup(data)
		if err != nil {
			return GroupIdentity{}, fmt.Errorf("parse process %d stat: %w", pid, err)
		}
		if memberPGID == pgid {
			pids = append(pids, pid)
		}
	}
	if len(pids) == 0 {
		return GroupIdentity{State: GroupDead}, nil
	}
	sort.Ints(pids)
	want := []byte("MAHR_COMMAND_RUN_ID=" + runID)
	for _, pid := range pids {
		environ, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "environ"))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return GroupIdentity{State: GroupAliveUnverified, PIDs: pids}, nil
			}
			return GroupIdentity{State: GroupAliveUnverified, PIDs: pids}, nil
		}
		owned := false
		for _, item := range bytes.Split(environ, []byte{0}) {
			if bytes.Equal(item, want) {
				owned = true
				break
			}
		}
		if !owned {
			return GroupIdentity{State: GroupAliveUnverified, PIDs: pids}, nil
		}
	}
	return GroupIdentity{State: GroupAliveOwned, PIDs: pids}, nil
}

func linuxStatProcessGroup(data []byte) (int, error) {
	// comm is parenthesized and may contain spaces or ')' characters. The last
	// ')' is the only stable delimiter before state, ppid and pgrp.
	end := strings.LastIndexByte(string(data), ')')
	if end < 0 || end+2 >= len(data) {
		return 0, errors.New("missing comm delimiter")
	}
	fields := strings.Fields(string(data[end+1:]))
	if len(fields) < 3 {
		return 0, errors.New("missing pgrp field")
	}
	pgid, err := strconv.Atoi(fields[2])
	if err != nil {
		return 0, fmt.Errorf("parse pgrp: %w", err)
	}
	return pgid, nil
}

func (nativeProcessInspector) KillGroup(pgid int) error {
	return syscall.Kill(-pgid, syscall.SIGKILL)
}
