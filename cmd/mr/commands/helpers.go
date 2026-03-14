package commands

import (
	"fmt"
	"strconv"
	"strings"
)

// parseUintList parses a comma-separated string of unsigned integers.
func parseUintList(s string) ([]uint, error) {
	if s == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	var result []uint
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		n, err := strconv.ParseUint(p, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid ID %q: %w", p, err)
		}
		result = append(result, uint(n))
	}
	return result, nil
}
