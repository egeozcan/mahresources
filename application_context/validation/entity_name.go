// Package validation holds pure validators for user-supplied entity data.
//
// These helpers have no dependencies on the rest of the application_context
// package, so they can be unit tested in isolation.
package validation

import (
	"fmt"
	"strings"
)

// SanitizeEntityName validates and trims a user-supplied entity name.
//
// It returns an error if the name contains NUL bytes, C0/C1 control
// characters (except TAB), Unicode directional overrides/isolates, or
// embedded newlines/carriage returns, or if it is empty after trimming.
//
// Background (BH-019): these characters cause UI spoofing (RTL-override
// disguised filenames), CSV / log-line injection, and C-library truncation
// (e.g. ffmpeg shelling out to a path containing a NUL byte). The input
// layer is the right place to reject them.
//
// The returned string is the input with leading/trailing ASCII whitespace
// stripped; otherwise it is byte-identical.
func SanitizeEntityName(name string) (string, error) {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return "", fmt.Errorf("name must not be empty")
	}

	for i, r := range trimmed {
		switch {
		case r == 0x00:
			return "", fmt.Errorf("name contains NUL byte at position %d", i)
		case r == '\n' || r == '\r':
			return "", fmt.Errorf("name contains newline at position %d", i)
		case r == '\t':
			// TAB is permitted: it shows up in legitimate labels and is
			// harmless in our rendering pipeline.
		case r < 0x20 || r == 0x7F:
			return "", fmt.Errorf("name contains control character U+%04X at position %d", r, i)
		case r >= 0x80 && r < 0xA0:
			return "", fmt.Errorf("name contains C1 control character U+%04X at position %d", r, i)
		case r >= 0x202A && r <= 0x202E:
			return "", fmt.Errorf("name contains directional override U+%04X at position %d", r, i)
		case r >= 0x2066 && r <= 0x2069:
			return "", fmt.Errorf("name contains directional isolate U+%04X at position %d", r, i)
		}
	}

	return trimmed, nil
}
