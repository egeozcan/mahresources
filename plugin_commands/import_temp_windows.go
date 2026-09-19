//go:build windows

package plugin_commands

import (
	"errors"
	"os"
)

type importTempDir struct{}

func createImportTempDir(string, string) (*importTempDir, error) {
	return nil, errors.New("plugin command imports are unsupported on Windows")
}

func (*importTempDir) Create(string) (*os.File, func() error, error) {
	return nil, nil, errors.New("plugin command imports are unsupported on Windows")
}

func (*importTempDir) Cleanup() error { return nil }

func cleanupImportTemps(string) error { return nil }
