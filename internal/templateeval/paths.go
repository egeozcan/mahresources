package templateeval

import (
	"errors"
	"os"
	"path/filepath"
	"time"
)

// ModuleRoot is the nearest directory at or above start that holds go.mod.
// The eval test runs in server/api_tests, so paths it writes are resolved
// against this, not the working directory.
func ModuleRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no go.mod above " + start)
		}
		dir = parent
	}
}

// ResolveOutPath is where a report is written: out (TEMPLATE_EVAL_OUT) when
// absolute, out under root when relative, and a timestamped file in
// benchmarks/template-generation/results under root when empty.
func ResolveOutPath(out, root string, now time.Time) string {
	if out == "" {
		return filepath.Join(root, "benchmarks", "template-generation", "results", "eval-"+now.UTC().Format("20060102T150405Z")+".json")
	}
	if filepath.IsAbs(out) {
		return out
	}
	return filepath.Join(root, out)
}
