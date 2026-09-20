//go:build linux

package plugin_commands

import (
	"fmt"
	"os"
)

func CurrentBootSessionID() (string, error) {
	raw, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return "", fmt.Errorf("read Linux boot session id: %w", err)
	}
	return normalizeBootSessionID(string(raw))
}
