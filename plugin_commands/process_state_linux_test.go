//go:build linux

package plugin_commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
)

func testProcessHasLiveState(pid int) (bool, error) {
	path := filepath.Join("/proc", strconv.Itoa(pid), "stat")
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	_, zombie, err := linuxStatProcess(data)
	if err != nil {
		return false, fmt.Errorf("parse %s: %w", path, err)
	}
	return !zombie, nil
}
