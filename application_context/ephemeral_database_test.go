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
	dir := t.TempDir()
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

func ephemeralFilesIn(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	var names []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "mahresources_ephemeral_") {
			names = append(names, entry.Name())
		}
	}
	return names
}

func TestEphemeralDatabaseLivesInTheSystemTempDirAndIsRemovedOnRelease(t *testing.T) {
	dir := isolateTempDir(t)

	ctx, db, _, err := OpenContextWithConfig(&MahresourcesInputConfig{MemoryDB: true, MemoryFS: true})
	if err != nil {
		t.Fatalf("opening an ephemeral context: %v", err)
	}
	if err := db.Exec("CREATE TABLE ephemeral_probe (id INTEGER PRIMARY KEY)").Error; err != nil {
		t.Fatalf("writing to the ephemeral database: %v", err)
	}

	files := ephemeralFilesIn(t, dir)
	if len(files) == 0 {
		t.Fatalf("no ephemeral database file in %s", dir)
	}
	for _, name := range files {
		if !strings.HasPrefix(name, fmt.Sprintf("mahresources_ephemeral_%d_", os.Getpid())) {
			t.Errorf("ephemeral file %q does not carry this process's pid", name)
		}
	}

	if err := ctx.ReleaseEphemeralDatabase(); err != nil {
		t.Fatalf("releasing the ephemeral database: %v", err)
	}
	if left := ephemeralFilesIn(t, dir); len(left) != 0 {
		t.Fatalf("files left after release: %v", left)
	}
	if err := ctx.ReleaseEphemeralDatabase(); err != nil {
		t.Fatalf("a second release should be a no-op, got %v", err)
	}
}

func TestTwoEphemeralContextsInOneProcessDoNotShareAFile(t *testing.T) {
	dir := isolateTempDir(t)

	first, firstDB, _, err := OpenContextWithConfig(&MahresourcesInputConfig{MemoryDB: true, MemoryFS: true})
	if err != nil {
		t.Fatalf("opening the first context: %v", err)
	}
	if err := firstDB.Exec("CREATE TABLE first_only (id INTEGER PRIMARY KEY)").Error; err != nil {
		t.Fatalf("writing to the first database: %v", err)
	}
	second, secondDB, _, err := OpenContextWithConfig(&MahresourcesInputConfig{MemoryDB: true, MemoryFS: true})
	if err != nil {
		t.Fatalf("opening the second context: %v", err)
	}
	t.Cleanup(func() { _ = second.ReleaseEphemeralDatabase() })

	if err := first.ReleaseEphemeralDatabase(); err != nil {
		t.Fatalf("releasing the first context: %v", err)
	}
	if err := secondDB.Exec("CREATE TABLE second_only (id INTEGER PRIMARY KEY)").Error; err != nil {
		t.Fatalf("the second database stopped working when the first was released: %v", err)
	}
	var tables int64
	if err := secondDB.Raw("SELECT count(*) FROM sqlite_master WHERE name = 'first_only'").Scan(&tables).Error; err != nil {
		t.Fatalf("reading the second database: %v", err)
	}
	if tables != 0 {
		t.Fatal("the second context opened the first context's database")
	}
	if len(ephemeralFilesIn(t, dir)) == 0 {
		t.Fatal("releasing the first context removed the second context's file")
	}
}

func TestOpeningAnEphemeralDatabaseSweepsFilesOfExitedProcesses(t *testing.T) {
	dir := isolateTempDir(t)
	dead := exitedPID(t)
	live := os.Getppid()

	write := func(name string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatalf("writing %s: %v", name, err)
		}
	}
	gone := []string{
		fmt.Sprintf("mahresources_ephemeral_%d.db", dead),
		fmt.Sprintf("mahresources_ephemeral_%d.db-wal", dead),
		fmt.Sprintf("mahresources_ephemeral_%d.db-shm", dead),
		fmt.Sprintf("mahresources_ephemeral_%d_123456.db", dead),
		fmt.Sprintf("mahresources_ephemeral_%d_123456.db-journal", dead),
	}
	kept := []string{
		fmt.Sprintf("mahresources_ephemeral_%d.db", live),
		fmt.Sprintf("mahresources_ephemeral_%d_42.db-wal", live),
		fmt.Sprintf("mahresources_ephemeral_%d.db.bak", dead),
		"mahresources_ephemeral_abc.db",
		"unrelated.db",
	}
	for _, name := range append(append([]string{}, gone...), kept...) {
		write(name)
	}
	if err := os.Symlink(filepath.Join(dir, "unrelated.db"), filepath.Join(dir, fmt.Sprintf("mahresources_ephemeral_%d_7.db", dead))); err != nil {
		t.Fatalf("creating a symlink: %v", err)
	}
	kept = append(kept, fmt.Sprintf("mahresources_ephemeral_%d_7.db", dead))

	ctx, _, _, err := OpenContextWithConfig(&MahresourcesInputConfig{MemoryDB: true, MemoryFS: true})
	if err != nil {
		t.Fatalf("opening an ephemeral context: %v", err)
	}
	t.Cleanup(func() { _ = ctx.ReleaseEphemeralDatabase() })

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

func TestSweepCoversTheLegacyLocationToo(t *testing.T) {
	current := t.TempDir()
	legacy := t.TempDir()
	dead := exitedPID(t)
	stale := filepath.Join(legacy, fmt.Sprintf("mahresources_ephemeral_%d.db", dead))
	if err := os.WriteFile(stale, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	sweepEphemeralDatabases([]string{current, legacy}, false)

	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Fatalf("a legacy file of an exited process survived the sweep (stat err %v)", err)
	}
}

func TestSweepRemovesFilesThatCarryThisProcessesPidOnlyWhenAskedTo(t *testing.T) {
	dir := t.TempDir()
	own := filepath.Join(dir, fmt.Sprintf("mahresources_ephemeral_%d.db", os.Getpid()))
	if err := os.WriteFile(own, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}

	sweepEphemeralDatabases([]string{dir}, false)
	if _, err := os.Stat(own); err != nil {
		t.Fatalf("a file carrying this pid was removed by a sweep that should skip it: %v", err)
	}

	sweepEphemeralDatabases([]string{dir}, true)
	if _, err := os.Stat(own); !os.IsNotExist(err) {
		t.Fatalf("a file left under this pid by an earlier process survived (stat err %v)", err)
	}
}

// A pid this process cannot see is not proof of an exit: a process in another PID
// namespace sharing the directory is invisible to signal 0. A database that some
// process still has open is kept whatever its name says.
func TestSweepKeepsADatabaseAnotherProcessHasOpen(t *testing.T) {
	dir := t.TempDir()
	stem := filepath.Join(dir, fmt.Sprintf("mahresources_ephemeral_%d_1.db", exitedPID(t)))

	holder := exec.Command(os.Args[0], "-test.run=^TestEphemeralSweepHelperHoldsADatabase$")
	holder.Env = append(os.Environ(), "MAHRES_HOLD_EPHEMERAL_DB="+stem)
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

	sweepEphemeralDatabases([]string{dir}, false)
	for _, suffix := range []string{"", "-shm"} {
		if _, err := os.Stat(stem + suffix); err != nil {
			t.Fatalf("swept %s while another process had the database open: %v", filepath.Base(stem+suffix), err)
		}
	}

	_ = stdin.Close()
	if err := holder.Wait(); err != nil {
		t.Fatalf("holder: %v", err)
	}
	sweepEphemeralDatabases([]string{dir}, false)
	if _, err := os.Stat(stem); !os.IsNotExist(err) {
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
