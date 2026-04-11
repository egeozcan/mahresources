package download_queue

import (
	"errors"
	"os"
	"time"

	"github.com/spf13/afero"
)

// SweepOrphanedExports walks dir and removes files whose modtime is older
// than the retention window. Used at server startup to clean up tars left
// behind by exports that crashed mid-write or whose owning manager was lost
// to a server restart.
//
// Returns the count of removed files. A missing directory is not an error
// (returns 0, nil). A non-positive retention (<=0) short-circuits to 0 with
// no error — callers that want "never sweep" can pass 0.
func SweepOrphanedExports(fs afero.Fs, dir string, retention time.Duration) (int, error) {
	if retention <= 0 {
		return 0, nil
	}
	exists, err := afero.DirExists(fs, dir)
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, nil
	}
	cutoff := time.Now().Add(-retention)

	removed := 0
	walkFn := func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrNotExist) {
				return nil
			}
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		if info.ModTime().Before(cutoff) {
			if err := fs.Remove(path); err != nil {
				return err
			}
			removed++
		}
		return nil
	}
	if err := afero.Walk(fs, dir, walkFn); err != nil {
		return removed, err
	}
	return removed, nil
}
