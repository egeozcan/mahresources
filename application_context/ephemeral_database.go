package application_context

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"sync"
)

// An ephemeral database (-memory-db, -ephemeral) is a scratch SQLite file in the
// system temp directory, named after the process that owns it:
// mahresources_ephemeral_<pid>_<random>.db, with SQLite's -wal and -shm beside it.
// Nothing reads it once that process has gone, so the owner deletes it on
// shutdown, and every later ephemeral start deletes the files of owners that
// exited without doing so (a crash, a SIGKILL, a test binary that never shuts
// down). The random part keeps two contexts of one process apart, which tests do.
const ephemeralDatabasePrefix = "mahresources_ephemeral_"

// ephemeralDatabaseName matches every file an ephemeral database leaves, current
// or from a release that named it mahresources_ephemeral_<pid>.db, and captures
// the database's own file name and the owner's pid.
var ephemeralDatabaseName = regexp.MustCompile(`^(mahresources_ephemeral_([0-9]+)(?:_[0-9]+)?\.db)(?:-wal|-shm|-journal)?$`)

// ephemeralDatabaseSuffixes are the files SQLite keeps beside a database in WAL
// or rollback-journal mode.
var ephemeralDatabaseSuffixes = []string{"", "-wal", "-shm", "-journal"}

// legacyEphemeralDatabaseDir is where releases before the move to os.TempDir()
// put the file. It is swept too, or files already leaked there on a system whose
// temp directory is elsewhere (macOS) would never be reclaimed.
func legacyEphemeralDatabaseDir() string {
	if runtime.GOOS == "windows" {
		return ""
	}
	return "/tmp"
}

// ownPIDSweep runs once per process, before its first ephemeral database exists:
// at that point a file carrying this process's pid can only have been left by an
// earlier process that had the same pid, so it is removed as well.
var ownPIDSweep sync.Once

type ephemeralDatabase struct {
	path    string
	release sync.Once
	err     error
}

// createEphemeralDatabase sweeps the files of exited owners and reserves a new
// database file for this process.
func createEphemeralDatabase() (string, error) {
	dir := os.TempDir()
	dirs := []string{dir}
	if legacy := legacyEphemeralDatabaseDir(); legacy != "" && filepath.Clean(legacy) != filepath.Clean(dir) {
		dirs = append(dirs, legacy)
	}
	ownPIDSweep.Do(func() { sweepEphemeralDatabases(dirs, true) })
	sweepEphemeralDatabases(dirs, false)

	file, err := os.CreateTemp(dir, fmt.Sprintf("%s%d_*.db", ephemeralDatabasePrefix, os.Getpid()))
	if err != nil {
		return "", fmt.Errorf("create ephemeral database file: %w", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		removeEphemeralDatabaseFiles(path)
		return "", fmt.Errorf("create ephemeral database file: %w", err)
	}
	return path, nil
}

// sweepEphemeralDatabases deletes the ephemeral database files in dirs whose
// owning process has exited and that no process still has open. includeOwnPID
// also considers those that carry this process's pid (see ownPIDSweep). Only
// regular files are touched, and a file that cannot be removed (another user's, in
// a shared /tmp) is left alone.
func sweepEphemeralDatabases(dirs []string, includeOwnPID bool) {
	self := os.Getpid()
	exited := map[int]bool{}
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		// A database and its -wal, -shm and -journal files go together, decided once.
		abandoned := map[string]bool{}
		for _, entry := range entries {
			match := ephemeralDatabaseName.FindStringSubmatch(entry.Name())
			if match == nil || !entry.Type().IsRegular() {
				continue
			}
			pid, err := strconv.Atoi(match[2])
			if err != nil {
				continue
			}
			database := filepath.Join(dir, match[1])
			decided, seen := abandoned[database]
			if !seen {
				decided = ownerGone(pid, self, includeOwnPID, exited) && !ephemeralDatabaseOpen(database)
				abandoned[database] = decided
			}
			if decided {
				_ = os.Remove(filepath.Join(dir, entry.Name()))
			}
		}
	}
}

// ownerGone reports whether the process named by pid can no longer be using its
// database, remembering the answer for each pid in exited.
func ownerGone(pid, self int, includeOwnPID bool, exited map[int]bool) bool {
	if pid == self {
		return includeOwnPID
	}
	gone, known := exited[pid]
	if !known {
		gone = ephemeralOwnerExited(pid)
		exited[pid] = gone
	}
	return gone
}

func removeEphemeralDatabaseFiles(path string) error {
	var errs []error
	for _, suffix := range ephemeralDatabaseSuffixes {
		if err := os.Remove(path + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// ReleaseEphemeralDatabase closes an ephemeral context's database handles and
// deletes its file. It is a no-op for any other context and after the first call.
// Nothing may use the database afterwards, so it belongs at the very end of
// shutdown.
func (ctx *MahresourcesContext) ReleaseEphemeralDatabase() error {
	eph := ctx.ephemeralDB
	if eph == nil {
		return nil
	}
	eph.release.Do(func() {
		var errs []error
		if ctx.db != nil {
			if sqlDB, err := ctx.db.DB(); err == nil {
				errs = append(errs, sqlDB.Close())
			}
		}
		if ctx.readOnlyDB != nil {
			errs = append(errs, ctx.readOnlyDB.Close())
		}
		errs = append(errs, removeEphemeralDatabaseFiles(eph.path))
		eph.err = errors.Join(errs...)
	})
	return eph.err
}
