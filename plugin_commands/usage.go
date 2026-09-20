package plugin_commands

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
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

// StagingUsageCache keeps global quota admission O(1). The runtime refreshes
// the sample at startup and after each retention sweep; admission never walks
// the staging tree while a plugin VM or dispatcher lock is held.
type StagingUsageCache struct {
	mu          sync.RWMutex
	bytes       int64
	initialized bool
	measure     func(string) (int64, error)
}

func NewStagingUsageCache() *StagingUsageCache {
	return &StagingUsageCache{measure: pathUsageNoSymlinks}
}

func (c *StagingUsageCache) Refresh(root string) error {
	if c == nil {
		return fmt.Errorf("plugin command staging usage cache is unavailable")
	}
	measure := c.measure
	if measure == nil {
		measure = pathUsageNoSymlinks
	}
	bytes, err := measure(root)
	if err != nil {
		return err
	}
	c.mu.Lock()
	c.bytes, c.initialized = bytes, true
	c.mu.Unlock()
	return nil
}

func (c *StagingUsageCache) Current() (int64, error) {
	if c == nil {
		return 0, fmt.Errorf("plugin command staging usage cache is unavailable")
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	if !c.initialized {
		return 0, fmt.Errorf("global staging usage is unavailable: no sample has been published")
	}
	return c.bytes, nil
}

type ProcessInspector interface {
	InspectGroup(pgid int, runID string) (GroupIdentity, error)
	KillGroup(pgid int) error
}

// nativeProcessInspector is implemented per platform. Ownership inspection is
// intentionally stronger than process-group existence: recovery must not signal
// a reused pgid belonging to another process.
type nativeProcessInspector struct{}

type RunnerDependencies struct {
	Store     Store
	Settings  Settings
	Inspector ProcessInspector
	Usage     *StagingUsageCache
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
