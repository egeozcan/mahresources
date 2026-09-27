//go:build !windows

package application_context

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// isolateTempDir points os.TempDir at a fresh directory for the test.
func isolateTempDir(t *testing.T) string {
	t.Helper()
	return isolateTempDirAt(t, t.TempDir())
}

func isolateTempDirAt(t *testing.T, dir string) string {
	t.Helper()
	t.Setenv("TMPDIR", dir)
	if got := os.TempDir(); got != dir {
		t.Fatalf("os.TempDir() = %q, want %q", got, dir)
	}
	return dir
}

// databaseFilesIn lists the ephemeral database files in dir, leaving out the
// directory's marker.
func databaseFilesIn(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	var names []string
	for _, entry := range entries {
		if entry.Name() != ephemeralDirectoryMarker {
			names = append(names, entry.Name())
		}
	}
	return names
}

// startsWithSQLiteHeader reports whether the file at path is a written SQLite
// database.
func startsWithSQLiteHeader(t *testing.T, path string) bool {
	t.Helper()
	content, err := os.ReadFile(path)
	return err == nil && strings.HasPrefix(string(content), "SQLite format 3\x00")
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing %s: %v", path, err)
	}
}

func openEphemeralContext(t *testing.T) (*MahresourcesContext, func(string) error) {
	t.Helper()
	ctx, db, _, err := OpenContextWithConfig(&MahresourcesInputConfig{MemoryDB: true, MemoryFS: true})
	if err != nil {
		t.Fatalf("opening an ephemeral context: %v", err)
	}
	t.Cleanup(func() { _ = ctx.ReleaseEphemeralDatabase() })
	return ctx, func(statement string) error { return db.Exec(statement).Error }
}

func TestEphemeralDatabaseLivesInAPrivateDirectoryAndIsRemovedOnRelease(t *testing.T) {
	dir := filepath.Join(isolateTempDir(t), ephemeralDirectoryName())

	ctx, exec := openEphemeralContext(t)
	if err := exec("CREATE TABLE ephemeral_probe (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("writing to the ephemeral database: %v", err)
	}

	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0o700 {
		t.Fatalf("ephemeral directory %s: %v, %v", dir, info, err)
	}
	if _, err := os.Stat(filepath.Join(dir, ephemeralDirectoryMarker)); err != nil {
		t.Fatalf("the ephemeral directory carries no marker: %v", err)
	}
	if filepath.Dir(ctx.ephemeralDB.path) != dir {
		t.Fatalf("database at %s, want it in %s", ctx.ephemeralDB.path, dir)
	}
	stem := strings.TrimSuffix(ctx.ephemeralDB.path, ephemeralDatabaseSuffix)
	for _, name := range databaseFilesIn(t, dir) {
		if !strings.HasPrefix(filepath.Join(dir, name), stem+".") {
			t.Errorf("%s is not one of the database's files", name)
		}
	}
	if heldBy(t, stem+ephemeralLockSuffix) != "someone" {
		t.Fatal("a live database's lock file is not locked")
	}

	if err := ctx.ReleaseEphemeralDatabase(); err != nil {
		t.Fatalf("releasing the ephemeral database: %v", err)
	}
	if left := databaseFilesIn(t, dir); len(left) != 0 {
		t.Fatalf("files left after release: %v", left)
	}
	if err := ctx.ReleaseEphemeralDatabase(); err != nil {
		t.Fatalf("a second release should be a no-op, got %v", err)
	}
}

// A queue execution's follower goes on writing after its Job reads terminal, so
// releasing the database waits for it rather than closing the handle under a
// write in flight.
func TestReleasingTheEphemeralDatabaseWaitsForAQueueFollower(t *testing.T) {
	isolateTempDir(t)
	ctx, exec := openEphemeralContext(t)
	if err := exec("CREATE TABLE follower_probe (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("writing to the ephemeral database: %v", err)
	}

	ctx.queueFollowers.Add(1)
	released := make(chan error, 1)
	go func() { released <- ctx.ReleaseEphemeralDatabase() }()
	select {
	case err := <-released:
		ctx.queueFollowers.Done()
		t.Fatalf("the database was released with a follower still running (%v)", err)
	case <-time.After(200 * time.Millisecond):
	}
	writeErr := exec("INSERT INTO follower_probe (id) VALUES (1)")
	ctx.queueFollowers.Done()
	if writeErr != nil {
		t.Fatalf("the follower's write failed: %v", writeErr)
	}
	if err := <-released; err != nil {
		t.Fatalf("releasing the ephemeral database: %v", err)
	}
}

// A temp directory may contain characters a URI reads as syntax; the database must
// still be the file in that directory, for the writer and for the read-only
// connection alike.
func TestEphemeralDatabaseOpensTheFileInATempDirWithURISyntaxInItsPath(t *testing.T) {
	base := t.TempDir()
	odd := filepath.Join(base, "odd?dir#part%41")
	if err := os.Mkdir(odd, 0o700); err != nil {
		t.Fatal(err)
	}
	isolateTempDirAt(t, odd)

	ctx, exec := openEphemeralContext(t)
	if err := exec("CREATE TABLE ephemeral_probe (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("writing to the ephemeral database: %v", err)
	}
	if !strings.HasPrefix(ctx.ephemeralDB.path, odd+string(filepath.Separator)) {
		t.Fatalf("database at %s, want it under %s", ctx.ephemeralDB.path, odd)
	}
	if !startsWithSQLiteHeader(t, ctx.ephemeralDB.path) {
		t.Fatalf("%s is not the database that was written", ctx.ephemeralDB.path)
	}
	var tables int
	if err := ctx.readOnlyDB.Get(&tables, "SELECT count(*) FROM sqlite_master WHERE name = 'ephemeral_probe'"); err != nil || tables != 1 {
		t.Fatalf("the read-only connection opened another database: tables=%d err=%v", tables, err)
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("files outside the temp directory: %v", entries)
	}
}

func TestTwoEphemeralContextsInOneProcessDoNotShareAFile(t *testing.T) {
	dir := filepath.Join(isolateTempDir(t), ephemeralDirectoryName())

	first, firstExec := openEphemeralContext(t)
	if err := firstExec("CREATE TABLE first_only (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("writing to the first database: %v", err)
	}
	_, secondExec := openEphemeralContext(t)

	if err := first.ReleaseEphemeralDatabase(); err != nil {
		t.Fatalf("releasing the first context: %v", err)
	}
	if err := secondExec("CREATE TABLE second_only (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("the second database stopped working when the first was released: %v", err)
	}
	if err := secondExec("CREATE TABLE first_only (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatalf("the second context opened the first context's database: %v", err)
	}
	if len(databaseFilesIn(t, dir)) == 0 {
		t.Fatal("releasing the first context removed the second context's file")
	}
}

func TestOpeningAnEphemeralDatabaseSweepsOnlyDatabasesWhoseLockIsFree(t *testing.T) {
	dir := filepath.Join(isolateTempDir(t), ephemeralDirectoryName())
	if !claimEphemeralDirectory(dir) {
		t.Fatal("could not create the ephemeral directory")
	}
	at := func(name string) string { return filepath.Join(dir, name) }

	// abandoned: its owner is gone, so no one holds its lock.
	gone := []string{"abandoned.db", "abandoned.db-wal", "abandoned.db-shm", "abandoned.db-journal", "abandoned.lock"}
	for _, name := range gone {
		writeFile(t, at(name), "x")
	}
	// live: an owner holds its lock; starting: locked, its database not created yet.
	kept := []string{"live.db", "live.db-wal", "live.lock", "starting.lock"}
	for _, name := range kept {
		writeFile(t, at(name), "x")
	}
	holdLock(t, at("live.lock"))
	holdLock(t, at("starting.lock"))
	// No lock file, or a lock file that is a link: nothing proves these are ours.
	writeFile(t, at("unproven.db"), "x")
	writeFile(t, at("unproven.db-wal"), "x")
	writeFile(t, at("notes.txt"), "x")
	writeFile(t, at("linked.db"), "x")
	if err := os.Symlink(at("notes.txt"), at("linked.lock")); err != nil {
		t.Fatalf("creating a symlink: %v", err)
	}
	kept = append(kept, "unproven.db", "unproven.db-wal", "notes.txt", "linked.db", "linked.lock")

	openEphemeralContext(t)

	for _, name := range gone {
		if _, err := os.Lstat(at(name)); !os.IsNotExist(err) {
			t.Errorf("%s belongs to a database no one holds and should have been swept (stat err %v)", name, err)
		}
	}
	for _, name := range kept {
		if _, err := os.Lstat(at(name)); err != nil {
			t.Errorf("%s should have been kept: %v", name, err)
		}
	}
}

// holdLock takes the lock a server holds on a live database's lock file, on an
// open file of its own, until the test ends.
func holdLock(t *testing.T, path string) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.Close() })
	if err := lockEphemeralFile(f); err != nil {
		t.Fatal(err)
	}
}

// heldBy reports "someone" when a lock is held on path, and "no one" otherwise.
func heldBy(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if tryLockEphemeralFile(f) {
		return "no one"
	}
	return "someone"
}

// An empty private directory of that name is one a start left before writing the
// marker; it is claimed.
func TestAnEmptyPrivateEphemeralDirectoryWithoutTheMarkerIsClaimed(t *testing.T) {
	dir := filepath.Join(isolateTempDir(t), ephemeralDirectoryName())
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	ctx, _ := openEphemeralContext(t)
	if filepath.Dir(ctx.ephemeralDB.path) != dir {
		t.Fatalf("database at %s, want it in %s", ctx.ephemeralDB.path, dir)
	}
	if _, err := os.Stat(filepath.Join(dir, ephemeralDirectoryMarker)); err != nil {
		t.Fatalf("the claimed directory carries no marker: %v", err)
	}
}

// A directory of that name the process did not make its own (no marker, or open
// to others) is neither written to nor swept, and the start is refused: a
// directory of its own instead would leak whenever the server was killed.
func TestAnEphemeralDirectoryThatIsNotOursRefusesTheStart(t *testing.T) {
	for name, prepare := range map[string]func(t *testing.T, dir string){
		"without the marker": func(t *testing.T, dir string) {
			if err := os.Mkdir(dir, 0o700); err != nil {
				t.Fatal(err)
			}
		},
		"readable by others": func(t *testing.T, dir string) {
			if err := os.Mkdir(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			writeFile(t, filepath.Join(dir, ephemeralDirectoryMarker), "x")
		},
	} {
		t.Run(name, func(t *testing.T) {
			tempDir := isolateTempDir(t)
			dir := filepath.Join(tempDir, ephemeralDirectoryName())
			prepare(t, dir)
			stranger := filepath.Join(dir, "stranger.db")
			writeFile(t, stranger, "x")
			before := databaseFilesIn(t, dir)

			ctx, _, _, err := OpenContextWithConfig(&MahresourcesInputConfig{MemoryDB: true, MemoryFS: true})
			if err == nil {
				_ = ctx.ReleaseEphemeralDatabase()
				t.Fatal("started with an ephemeral directory that is not ours")
			}
			if !strings.Contains(err.Error(), dir) {
				t.Errorf("the refusal does not name the directory: %v", err)
			}
			if after := databaseFilesIn(t, dir); strings.Join(after, ",") != strings.Join(before, ",") {
				t.Fatalf("touched a directory that is not ours: %v, then %v", before, after)
			}
			entries, err := os.ReadDir(tempDir)
			if err != nil || len(entries) != 1 {
				t.Fatalf("made something else in the temp directory: %v, %v", entries, err)
			}
		})
	}
}

// The lock is the kernel's to release: a database whose owner is another process
// is kept for exactly as long as that process lives, whatever it could or could
// not be seen as by pid.
func TestSweepKeepsADatabaseWhoseOwnerIsAliveAndReclaimsItWhenTheOwnerDies(t *testing.T) {
	dir := t.TempDir()
	stem := filepath.Join(dir, "owned")
	writeFile(t, stem+ephemeralLockSuffix, "")
	writeFile(t, stem+ephemeralDatabaseSuffix, "x")

	holder := exec.Command(os.Args[0], "-test.run=^TestEphemeralSweepHelperHoldsALock$")
	holder.Env = append(os.Environ(), "MAHRES_HOLD_EPHEMERAL_LOCK="+stem+ephemeralLockSuffix)
	stdin, err := holder.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := holder.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := holder.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = holder.Process.Kill(); _ = holder.Wait() })
	ready := make([]byte, 64)
	if n, err := stdout.Read(ready); err != nil || !strings.Contains(string(ready[:n]), "holding") {
		t.Fatalf("the holder never took the lock: %q, %v", ready[:n], err)
	}

	sweepEphemeralDatabases(dir)
	for _, suffix := range []string{ephemeralDatabaseSuffix, ephemeralLockSuffix} {
		if _, err := os.Stat(stem + suffix); err != nil {
			t.Fatalf("swept %s while its owner was alive: %v", filepath.Base(stem+suffix), err)
		}
	}

	// Killed, not shut down: it releases nothing itself.
	_ = stdin.Close()
	if err := holder.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = holder.Wait()
	sweepEphemeralDatabases(dir)
	for _, suffix := range []string{ephemeralDatabaseSuffix, ephemeralLockSuffix} {
		if _, err := os.Stat(stem + suffix); !os.IsNotExist(err) {
			t.Fatalf("%s survived its owner's death (stat err %v)", filepath.Base(stem+suffix), err)
		}
	}
}

// TestEphemeralSweepHelperHoldsALock is the owner process for the test above: it
// locks the file it is given, says so, and waits to be killed.
func TestEphemeralSweepHelperHoldsALock(t *testing.T) {
	path := os.Getenv("MAHRES_HOLD_EPHEMERAL_LOCK")
	if path == "" {
		t.Skip("runs only as the helper process of TestSweepKeepsADatabaseWhoseOwnerIsAliveAndReclaimsItWhenTheOwnerDies")
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := lockEphemeralFile(f); err != nil {
		t.Fatal(err)
	}
	fmt.Println("holding")
	_, _ = io.Copy(io.Discard, os.Stdin)
	select {}
}
