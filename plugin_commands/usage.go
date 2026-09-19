package plugin_commands

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	defaultPerRunQuota        int64 = 8 << 30
	defaultGlobalStagingQuota int64 = 50 << 30
)

type GroupState int

const (
	GroupDead GroupState = iota
	GroupAliveOwned
	GroupAliveUnverified
)

type GroupIdentity struct {
	State GroupState
	PIDs  []int
}

type ProcessInspector interface {
	InspectGroup(pgid int, runID string) (GroupIdentity, error)
	KillGroup(pgid int) error
}

type RunnerDependencies struct {
	Store     Store
	Settings  Settings
	Inspector ProcessInspector
	Logf      func(string, ...any)
}

func effectiveQuota(configured, fallback int64) int64 {
	if configured <= 0 {
		return fallback
	}
	return configured
}

// pathUsageNoSymlinks sums regular files rooted at path without traversing or
// charging symlinks. Missing paths have zero usage.
func pathUsageNoSymlinks(path string) (int64, error) {
	var total int64
	root := filepath.Clean(path)
	err := filepath.WalkDir(root, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if entry.Type()&os.ModeSymlink != 0 {
			if current == root {
				return fmt.Errorf("usage root is a symlink: %s", root)
			}
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	if errors.Is(err, os.ErrNotExist) {
		return 0, nil
	}
	return total, err
}

func runUsage(store Store, stagingRoot, runID, exchangeDir string) (int64, error) {
	total, err := pathUsageNoSymlinks(exchangeDir)
	if err != nil {
		return 0, fmt.Errorf("measure exchange directory: %w", err)
	}
	imports, err := store.NonterminalImports()
	if err != nil {
		return 0, fmt.Errorf("list import temps for quota: %w", err)
	}
	for _, record := range imports {
		if record.RunID != runID {
			continue
		}
		usage, err := pathUsageNoSymlinks(filepath.Join(stagingRoot, "import_tmp", record.ID))
		if err != nil {
			return 0, fmt.Errorf("measure import temp %s: %w", record.ID, err)
		}
		total += usage
	}
	return total, nil
}
