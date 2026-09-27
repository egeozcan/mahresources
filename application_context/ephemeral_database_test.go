//go:build !windows

package application_context

import (
	"database/sql"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
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

// exitedPID returns the pid of a process that has already exited and been reaped.
func exitedPID(t *testing.T) int {
	t.Helper()
	cmd := exec.Command("true")
	if err := cmd.Run(); err != nil {
		t.Fatalf("running true: %v", err)
	}
	return cmd.Process.Pid
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

// writeSQLiteDatabase creates a real SQLite database at path.
func writeSQLiteDatabase(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE leftover (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
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
	files := databaseFilesIn(t, dir)
	if len(files) == 0 {
		t.Fatalf("no ephemeral database file in %s", dir)
	}
	for _, name := range files {
		if !strings.HasPrefix(name, fmt.Sprintf("%d_", os.Getpid())) {
			t.Errorf("ephemeral file %q does not carry this process's pid", name)
		}
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
	if !isOwnSQLiteDatabase(ctx.ephemeralDB.path) {
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

func TestOpeningAnEphemeralDatabaseSweepsTheDatabasesOfExitedProcesses(t *testing.T) {
	dir := filepath.Join(isolateTempDir(t), ephemeralDirectoryName())
	if !claimEphemeralDirectory(dir) {
		t.Fatal("could not create the ephemeral directory")
	}
	dead := exitedPID(t)
	live := os.Getppid()

	gone := []string{
		fmt.Sprintf("%d_123.db", dead),
		fmt.Sprintf("%d_123.db-wal", dead),
		fmt.Sprintf("%d_123.db-shm", dead),
		fmt.Sprintf("%d_456.db-journal", dead),
	}
	kept := []string{
		fmt.Sprintf("%d_42.db", live),
		fmt.Sprintf("%d_42.db-wal", live),
		fmt.Sprintf("%d_123.db.bak", dead),
		"notes.txt",
	}
	for _, name := range append(append([]string{}, gone...), kept...) {
		writeFile(t, filepath.Join(dir, name), "x")
	}
	link := fmt.Sprintf("%d_7.db", dead)
	if err := os.Symlink(filepath.Join(dir, "notes.txt"), filepath.Join(dir, link)); err != nil {
		t.Fatalf("creating a symlink: %v", err)
	}
	kept = append(kept, link)

	openEphemeralContext(t)

	for _, name := range gone {
		if _, err := os.Lstat(filepath.Join(dir, name)); !os.IsNotExist(err) {
			t.Errorf("%s belongs to an exited process and should have been swept (stat err %v)", name, err)
		}
	}
	for _, name := range kept {
		if _, err := os.Lstat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s should have been kept: %v", name, err)
		}
	}
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
// to others) is neither written to nor swept.
func TestAnEphemeralDirectoryThatIsNotOursIsNeitherUsedNorSwept(t *testing.T) {
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
			stranger := filepath.Join(dir, fmt.Sprintf("%d_1.db", exitedPID(t)))
			writeFile(t, stranger, "x")

			ctx, exec := openEphemeralContext(t)
			if err := exec("CREATE TABLE ephemeral_probe (id INTEGER PRIMARY KEY)"); err != nil {
				t.Fatalf("writing to the ephemeral database: %v", err)
			}
			if _, err := os.Stat(stranger); err != nil {
				t.Fatalf("swept a directory that is not ours: %v", err)
			}
			if filepath.Dir(ctx.ephemeralDB.path) == dir {
				t.Fatalf("wrote the database into a directory that is not ours: %s", ctx.ephemeralDB.path)
			}
			own := filepath.Dir(ctx.ephemeralDB.path)
			if err := ctx.ReleaseEphemeralDatabase(); err != nil {
				t.Fatalf("releasing: %v", err)
			}
			if _, err := os.Stat(own); !os.IsNotExist(err) {
				t.Fatalf("the database's own directory %s survived its release (stat err %v)", own, err)
			}
		})
	}
}

// The directory an older release wrote into is shared with everything else, so
// only a file that proves to be one of its databases is deleted there.
func TestLegacySweepDeletesOnlyVerifiedDatabasesOfExitedProcesses(t *testing.T) {
	legacy := t.TempDir()
	dead := exitedPID(t)
	live := os.Getppid()

	database := filepath.Join(legacy, fmt.Sprintf("mahresources_ephemeral_%d.db", dead))
	writeSQLiteDatabase(t, database)
	writeFile(t, database+"-wal", "")
	writeFile(t, database+"-shm", "")

	notADatabase := filepath.Join(legacy, fmt.Sprintf("mahresources_ephemeral_%d.db", exitedPID(t)))
	writeFile(t, notADatabase, "x")
	liveOwner := filepath.Join(legacy, fmt.Sprintf("mahresources_ephemeral_%d.db", live))
	writeSQLiteDatabase(t, liveOwner)
	notTheLegacyName := filepath.Join(legacy, fmt.Sprintf("mahresources_ephemeral_%d_9.db", dead))
	writeSQLiteDatabase(t, notTheLegacyName)

	sweepLegacyEphemeralDatabases(legacy)

	for _, path := range []string{database, database + "-wal", database + "-shm"} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s is a legacy database of an exited process and should have been swept (stat err %v)", filepath.Base(path), err)
		}
	}
	for _, path := range []string{notADatabase, liveOwner, notTheLegacyName} {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s should have been kept: %v", filepath.Base(path), err)
		}
	}
}

func TestSweepRemovesDatabasesThatCarryThisProcessesPidOnlyWhenAskedTo(t *testing.T) {
	dir := t.TempDir()
	own := filepath.Join(dir, fmt.Sprintf("%d_1.db", os.Getpid()))
	writeFile(t, own, "x")

	sweepEphemeralDatabases(dir, false)
	if _, err := os.Stat(own); err != nil {
		t.Fatalf("a file carrying this pid was removed by a sweep that should skip it: %v", err)
	}

	sweepEphemeralDatabases(dir, true)
	if _, err := os.Stat(own); !os.IsNotExist(err) {
		t.Fatalf("a file left under this pid by an earlier process survived (stat err %v)", err)
	}
}

// A pid this process cannot see is not proof of an exit: a process in another PID
// namespace sharing the directory is invisible to signal 0. A database that some
// process still has open is kept whatever its name says.
func TestSweepKeepsADatabaseAnotherProcessHasOpen(t *testing.T) {
	dir := t.TempDir()
	database := filepath.Join(dir, fmt.Sprintf("%d_1.db", exitedPID(t)))

	holder := exec.Command(os.Args[0], "-test.run=^TestEphemeralSweepHelperHoldsADatabase$")
	holder.Env = append(os.Environ(), "MAHRES_HOLD_EPHEMERAL_DB="+database)
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
	t.Cleanup(func() { _ = stdin.Close(); _ = holder.Wait() })
	ready := make([]byte, 64)
	if n, err := stdout.Read(ready); err != nil || !strings.Contains(string(ready[:n]), "holding") {
		t.Fatalf("the holder never opened the database: %q, %v", ready[:n], err)
	}

	sweepEphemeralDatabases(dir, false)
	for _, suffix := range []string{"", "-shm"} {
		if _, err := os.Stat(database + suffix); err != nil {
			t.Fatalf("swept %s while another process had the database open: %v", filepath.Base(database+suffix), err)
		}
	}

	_ = stdin.Close()
	if err := holder.Wait(); err != nil {
		t.Fatalf("holder: %v", err)
	}
	sweepEphemeralDatabases(dir, false)
	if _, err := os.Stat(database); !os.IsNotExist(err) {
		t.Fatalf("the database survived the sweep after its holder closed it (stat err %v)", err)
	}
}

// TestEphemeralSweepHelperHoldsADatabase is the other process for the test above:
// it opens a WAL database, says so, and keeps it open until its stdin closes.
func TestEphemeralSweepHelperHoldsADatabase(t *testing.T) {
	path := os.Getenv("MAHRES_HOLD_EPHEMERAL_DB")
	if path == "" {
		t.Skip("runs only as the helper process of TestSweepKeepsADatabaseAnotherProcessHasOpen")
	}
	db, err := sql.Open("sqlite3", "file:"+path+"?_journal_mode=WAL")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec("CREATE TABLE held (id INTEGER PRIMARY KEY)"); err != nil {
		t.Fatal(err)
	}
	fmt.Println("holding")
	_, _ = io.Copy(io.Discard, os.Stdin)
}
