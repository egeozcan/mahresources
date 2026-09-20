package plugin_commands

import (
	"errors"
	"fmt"
	"strings"
)

var ErrBootSessionIDUnavailable = errors.New("boot session id is unavailable")

func normalizeBootSessionID(raw string) (string, error) {
	identity := strings.TrimSpace(raw)
	if identity == "" {
		return "", fmt.Errorf("boot session id is empty")
	}
	return identity, nil
}
