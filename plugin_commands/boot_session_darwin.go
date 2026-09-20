//go:build darwin

package plugin_commands

import (
	"fmt"

	"golang.org/x/sys/unix"
)

func CurrentBootSessionID() (string, error) {
	raw, err := unix.Sysctl("kern.bootsessionuuid")
	if err != nil {
		return "", fmt.Errorf("read Darwin boot session id: %w", err)
	}
	return normalizeBootSessionID(raw)
}
