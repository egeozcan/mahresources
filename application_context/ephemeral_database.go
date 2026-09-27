package application_context

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"mahresources/download_queue"
)

// An ephemeral database (-memory-db, -ephemeral) is a scratch SQLite file that
// nothing reads once its server has gone. It lives in a directory of the system
// temp directory that this user's mahresources processes own
// (ephemeralDirectoryName, mode 0700, holding ephemeralDirectoryMarker), as
// <name>.db with SQLite's -wal and -shm beside it and a <name>.lock that the
// server holds an exclusive lock on from before the database exists until it has
// deleted it. The server deletes all of them on a graceful shutdown. A killed
// server cannot, but the kernel drops its lock, so every later ephemeral start
// deletes, in that directory only, the files of any database whose lock it can
// take. A file with no lock file beside it is never touched: nothing proves it is
// an ephemeral database. Windows takes no such lock here, so it sweeps nothing.

// ephemeralDirectoryMarker is the file that says a directory was made by
// mahresources for its ephemeral databases.
const ephemeralDirectoryMarker = ".mahresources-ephemeral"

const ephemeralDirectoryMarkerText = "Scratch databases of mahresources -ephemeral / -memory-db servers.\n" +
	"A database here is deleted once no process holds its .lock file.\n"

// ephemeralLockSuffix names a database's lock file; the database itself is the
// same name with ephemeralDatabaseSuffix.
const (
	ephemeralLockSuffix     = ".lock"
	ephemeralDatabaseSuffix = ".db"
)

// ephemeralDatabaseFiles are the files a database named <name> may leave, lock file
// last: it is deleted only after everything it protects.
var ephemeralDatabaseFiles = []string{".db", ".db-wal", ".db-shm", ".db-journal", ephemeralLockSuffix}

type ephemeralDatabase struct {
	path string
	// lock is the open lock file whose exclusive lock marks the database live.
	lock    *os.File
	release sync.Once
	err     error
}

// createEphemeralDatabase sweeps abandoned databases and reserves a new one for
// this process, locked before its file exists. It refuses when the ephemeral
// directory exists but is not one it can prove is mahresources': writing there
// could not be swept safely later, and a directory of its own per start would leak
// whenever a server was killed.
func createEphemeralDatabase() (*ephemeralDatabase, error) {
	dir := filepath.Join(os.TempDir(), ephemeralDirectoryName())
	if !claimEphemeralDirectory(dir) {
		return nil, fmt.Errorf("ephemeral database directory %s is not one mahresources made: it must be a "+
			"directory of this user that no one else may open, containing %s; remove it, or set TMPDIR "+
			"to another directory", dir, ephemeralDirectoryMarker)
	}
	sweepEphemeralDatabases(dir)

	for attempt := 1; ; attempt++ {
		lock, err := os.CreateTemp(dir, "*"+ephemeralLockSuffix)
		if err != nil {
			return nil, fmt.Errorf("create ephemeral database lock: %w", err)
		}
		if err := lockEphemeralFile(lock); err != nil {
			_ = lock.Close()
			_ = os.Remove(lock.Name())
			return nil, fmt.Errorf("lock ephemeral database: %w", err)
		}
		// A sweep that opened the new lock file before it was locked can have taken
		// the lock first and deleted it: a lock on a file no longer at its path
		// protects nothing.
		if !stillAt(lock, lock.Name()) {
			_ = lock.Close()
			if attempt < 3 {
				continue
			}
			return nil, errors.New("reserve ephemeral database: its lock file kept being removed")
		}
		database := &ephemeralDatabase{
			path: strings.TrimSuffix(lock.Name(), ephemeralLockSuffix) + ephemeralDatabaseSuffix,
			lock: lock,
		}
		file, err := os.OpenFile(database.path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			err = file.Close()
		}
		if err != nil {
			_ = database.remove()
			return nil, fmt.Errorf("create ephemeral database file: %w", err)
		}
		return database, nil
	}
}

// stillAt reports whether path still names the file f has open.
func stillAt(f *os.File, path string) bool {
	opened, err := f.Stat()
	if err != nil {
		return false
	}
	named, err := os.Lstat(path)
	return err == nil && os.SameFile(opened, named)
}

// claimEphemeralDirectory makes dir the shared ephemeral directory, or confirms it
// already is: a real directory (not a link), private to this user, holding the
// marker, or still empty. It answers false for anything else, which the caller
// must not touch.
func claimEphemeralDirectory(dir string) bool {
	created := os.Mkdir(dir, 0o700) == nil
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || !privateToCurrentUser(info) {
		return false
	}
	marker := filepath.Join(dir, ephemeralDirectoryMarker)
	if markerInfo, err := os.Lstat(marker); err == nil {
		return markerInfo.Mode().IsRegular()
	}
	// Made here, or left empty by a start that stopped before writing the marker:
	// either way nothing in it can be anyone else's.
	if entries, err := os.ReadDir(dir); !created && (err != nil || len(entries) > 0) {
		// A start racing this one writes the marker before anything else, so what
		// this read found may be that marker.
		markerInfo, err := os.Lstat(marker)
		return err == nil && markerInfo.Mode().IsRegular()
	}
	return os.WriteFile(marker, []byte(ephemeralDirectoryMarkerText), 0o600) == nil
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

// sweepEphemeralDatabases deletes, in the shared ephemeral directory, every
// database whose lock file no process holds: its owner released it or died. The
// files go while this sweep holds that lock, the lock file last, so a server that
// is still starting (its lock taken, its database not yet created or copied) is
// never touched. Only regular files are considered.
func sweepEphemeralDatabases(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		name, isLock := strings.CutSuffix(entry.Name(), ephemeralLockSuffix)
		if !isLock || name == "" || !entry.Type().IsRegular() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		lock, err := os.Open(path)
		if err != nil {
			continue
		}
		if tryLockEphemeralFile(lock) && stillAt(lock, path) {
			_ = removeEphemeralFiles(filepath.Join(dir, name))
		}
		_ = lock.Close()
	}
}

// removeEphemeralFiles deletes the files of the database named stem, lock file
// last.
func removeEphemeralFiles(stem string) error {
	var errs []error
	for _, suffix := range ephemeralDatabaseFiles {
		if err := os.Remove(stem + suffix); err != nil && !errors.Is(err, os.ErrNotExist) {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// remove deletes the database and its sidecar files while this process still holds
// its lock, then gives the lock up.
func (eph *ephemeralDatabase) remove() error {
	err := removeEphemeralFiles(strings.TrimSuffix(eph.path, ephemeralDatabaseSuffix))
	return errors.Join(err, eph.lock.Close())
}

// ReleaseEphemeralDatabase closes an ephemeral context's database handles and
// deletes its file. It is a no-op for any other context and after the first call.
// Nothing may use the database afterwards, so it belongs at the very end of
// shutdown.
//
// A queue execution's follower can still be writing when it is called (its Job
// reads terminal before the follower's last writes land), so it is waited out
// first, bounded like the queue's own drain. A write that outlived the bound
// fails against a closed handle, and the file it would have written is removed
// anyway.
func (ctx *MahresourcesContext) ReleaseEphemeralDatabase() error {
	eph := ctx.ephemeralDB
	if eph == nil {
		return nil
	}
	eph.release.Do(func() {
		if !ctx.WaitQueueFollowers(download_queue.ShutdownDrainTimeout) {
			log.Printf("warning: a queue execution was still publishing its outcome when the ephemeral database closed")
		}
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
