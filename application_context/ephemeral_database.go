package application_context

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// An ephemeral database (-memory-db, -ephemeral) is a scratch SQLite file that
// nothing reads once its process has gone. It lives in a directory of the system
// temp directory that this user's mahresources processes own
// (ephemeralDirectoryName, mode 0700, holding ephemeralDirectoryMarker), named
// <pid>_<random>.db with SQLite's -wal and -shm beside it. The owner deletes it on
// a graceful shutdown, and every later ephemeral start deletes, in that directory
// only, the databases of owners that exited without doing so (a crash, a SIGKILL,
// a test binary that never shuts down). The random part keeps two contexts of one
// process apart, which tests do.

// ephemeralDirectoryMarker is the file that says a directory was made by
// mahresources for its ephemeral databases, and so may be swept.
const ephemeralDirectoryMarker = ".mahresources-ephemeral"

const ephemeralDirectoryMarkerText = "Scratch databases of mahresources -ephemeral / -memory-db servers.\n" +
	"A database here is deleted once the process named by its file name has exited.\n"

// ephemeralDatabaseName matches the files an ephemeral database leaves in that
// directory, and captures the database's own file name and the owner's pid.
var ephemeralDatabaseName = regexp.MustCompile(`^(([0-9]+)_[0-9]+\.db)(?:-wal|-shm|-journal)?$`)

// legacyEphemeralDatabaseName is where releases before the private directory put
// an ephemeral database: /tmp/mahresources_ephemeral_<pid>.db, and its -wal and
// -shm. It captures the database's file name and the owner's pid.
var legacyEphemeralDatabaseName = regexp.MustCompile(`^(mahresources_ephemeral_([0-9]+)\.db)(?:-wal|-shm)?$`)

// ephemeralDatabaseSuffixes are the files SQLite keeps beside a database in WAL
// or rollback-journal mode.
var ephemeralDatabaseSuffixes = []string{"", "-wal", "-shm", "-journal"}

// sqliteFileHeader opens every SQLite database file.
var sqliteFileHeader = []byte("SQLite format 3\x00")

// legacyEphemeralDatabaseDir is where releases before the private directory put
// their databases. Nothing in it is ours by construction, so it is only swept of
// files that prove to be one (see sweepLegacyEphemeralDatabases).
func legacyEphemeralDatabaseDir() string {
	if runtime.GOOS == "windows" {
		return ""
	}
	return "/tmp"
}

// ownPIDSweep runs once per process, before its first ephemeral database exists:
// at that point a database carrying this process's pid can only have been left by
// an earlier process that had the same pid, so it is removed as well.
var ownPIDSweep sync.Once

// legacySweep runs once per process.
var legacySweep sync.Once

type ephemeralDatabase struct {
	path string
	// ownDir is a directory made for this database alone, removed with it; empty
	// when the database lives in the shared ephemeral directory.
	ownDir  string
	release sync.Once
	err     error
}

// createEphemeralDatabase sweeps the databases of exited owners and reserves a new
// database file for this process.
func createEphemeralDatabase() (*ephemeralDatabase, error) {
	legacySweep.Do(func() {
		if legacy := legacyEphemeralDatabaseDir(); legacy != "" {
			sweepLegacyEphemeralDatabases(legacy)
		}
	})

	dir := filepath.Join(os.TempDir(), ephemeralDirectoryName())
	ownDir := ""
	if claimEphemeralDirectory(dir) {
		ownPIDSweep.Do(func() { sweepEphemeralDatabases(dir, true) })
		sweepEphemeralDatabases(dir, false)
	} else {
		// Someone else's directory, or one that is not private: never write into it,
		// and never sweep it. A directory of this database's own takes its place.
		private, err := os.MkdirTemp(os.TempDir(), ephemeralDirectoryName()+"-")
		if err != nil {
			return nil, fmt.Errorf("create ephemeral database directory: %w", err)
		}
		dir, ownDir = private, private
	}

	file, err := os.CreateTemp(dir, strconv.Itoa(os.Getpid())+"_*.db")
	if err != nil {
		if ownDir != "" {
			_ = os.Remove(ownDir)
		}
		return nil, fmt.Errorf("create ephemeral database file: %w", err)
	}
	database := &ephemeralDatabase{path: file.Name(), ownDir: ownDir}
	if err := file.Close(); err != nil {
		_ = database.remove()
		return nil, fmt.Errorf("create ephemeral database file: %w", err)
	}
	return database, nil
}

// claimEphemeralDirectory makes dir the shared ephemeral directory, or confirms it
// already is: a real directory (not a link), private to this user, holding the
// marker. It answers false for anything else, which the caller must not touch.
func claimEphemeralDirectory(dir string) bool {
	created := os.Mkdir(dir, 0o700) == nil
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || !privateToCurrentUser(info) {
		return false
	}
	marker := filepath.Join(dir, ephemeralDirectoryMarker)
	if created {
		return os.WriteFile(marker, []byte(ephemeralDirectoryMarkerText), 0o600) == nil
	}
	markerInfo, err := os.Lstat(marker)
	return err == nil && markerInfo.Mode().IsRegular()
}

// ephemeralDatabaseDSN is the SQLite URI for the database at path, with the path
// escaped: a temp directory may contain '?', '#' or '%', which a URI would
// otherwise read as its query, fragment or an escape, and open another file.
func ephemeralDatabaseDSN(path, params string) string {
	slashed := filepath.ToSlash(path)
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed // a Windows drive letter
	}
	return (&url.URL{Scheme: "file", Path: slashed, RawQuery: params}).String()
}

// sweepEphemeralDatabases deletes the databases in the shared ephemeral directory
// whose owning process has exited and that no process still has open.
// includeOwnPID also considers those that carry this process's pid (see
// ownPIDSweep). Only regular files are touched.
func sweepEphemeralDatabases(dir string, includeOwnPID bool) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	self := os.Getpid()
	exited := map[int]bool{}
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

// sweepLegacyEphemeralDatabases deletes databases an older release left in dir,
// which is shared with everything else on the machine, so a name is not enough:
// the database must be named exactly as that release named it, be a regular file
// of this user that starts with SQLite's header, and belong to a process that has
// exited and that nobody has open. Its -wal and -shm go with it; anything that
// fails a check stays.
func sweepLegacyEphemeralDatabases(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	self := os.Getpid()
	abandoned := map[string]bool{}
	for _, entry := range entries {
		match := legacyEphemeralDatabaseName.FindStringSubmatch(entry.Name())
		if match == nil {
			continue
		}
		database := filepath.Join(dir, match[1])
		decided, seen := abandoned[database]
		if !seen {
			pid, err := strconv.Atoi(match[2])
			// This process never wrote there, so a database under its own pid is an
			// earlier process's.
			decided = err == nil && (pid == self || ephemeralOwnerExited(pid)) &&
				isOwnSQLiteDatabase(database) && !ephemeralDatabaseOpen(database)
			abandoned[database] = decided
		}
		if !decided {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if info, err := os.Lstat(path); err == nil && info.Mode().IsRegular() && ownedByCurrentUser(info) {
			_ = os.Remove(path)
		}
	}
}

// isOwnSQLiteDatabase reports whether path is a regular file of this user that
// starts with SQLite's header.
func isOwnSQLiteDatabase(path string) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || !ownedByCurrentUser(info) {
		return false
	}
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	header := make([]byte, len(sqliteFileHeader))
	if _, err := io.ReadFull(file, header); err != nil {
		return false
	}
	return bytes.Equal(header, sqliteFileHeader)
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

// remove deletes the database, its sidecar files and, when it had one, its own
// directory.
func (eph *ephemeralDatabase) remove() error {
	var errs []error
	for _, suffix := range ephemeralDatabaseSuffixes {
		if err := os.Remove(eph.path + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	if eph.ownDir != "" {
		if err := os.Remove(eph.ownDir); err != nil && !errors.Is(err, os.ErrNotExist) {
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
		errs = append(errs, eph.remove())
		eph.err = errors.Join(errs...)
	})
	return eph.err
}
