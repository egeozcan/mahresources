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
	return SweepOrphanedExportsProtected(fs, dir, retention, nil)
}

// SweepOrphanedExportsProtected is SweepOrphanedExports with a veto: a path keep
// answers true for is left exactly where it is, whatever its age.
//
// Age is the wrong question for a staging file whose durable owner still needs it.
// A plan, an archive or a staged result is named after the Job that will read it,
// and a Job that has not finished may be older than the retention window for any
// number of honest reasons — an outage, a blocked Job waiting for a person, a
// deployment that only started the Job runtime after this sweep ran. Deleting its
// input leaves work that can never run and an error nobody can act on. The caller
// that owns the database is the only thing that knows which paths those are, so it
// supplies the predicate; the sweep stays a filesystem walk.
func SweepOrphanedExportsProtected(fs afero.Fs, dir string, retention time.Duration, keep func(path string) bool) (int, error) {
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
		if !info.ModTime().Before(cutoff) {
			return nil
		}
		// The veto is asked only about files the sweep would otherwise delete, so
		// the predicate's own cost is proportional to what it protects rather than
		// to the size of the tree.
		if keep != nil && keep(path) {
			return nil
		}
		if err := fs.Remove(path); err != nil {
			return err
		}
		removed++
		return nil
	}
	if err := afero.Walk(fs, dir, walkFn); err != nil {
		return removed, err
	}
	return removed, nil
}
