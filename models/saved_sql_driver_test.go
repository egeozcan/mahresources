package models

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
)

// savedSQLFixture creates a WAL database file with a content table and a users
// table, written through the server's own driver, and returns its path.
func savedSQLFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "main.db")
	db, _, err := CreateDatabaseConnection("SQLITE", path, "", 0)
	if err != nil {
		t.Fatalf("open writer: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { _ = sqlDB.Close() })
	for _, statement := range []string{
		"CREATE TABLE resources (id INTEGER PRIMARY KEY, name TEXT)",
		"INSERT INTO resources (id, name) VALUES (1, 'original')",
		"CREATE TABLE users (id INTEGER PRIMARY KEY, username TEXT, role TEXT, password_hash TEXT)",
		"INSERT INTO users (id, username, role, password_hash) VALUES (1, 'ed', 'editor', '$2a$10$secret')",
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	return path
}

// openSavedSQLHandle opens the read-only handle the way the server does, pinned to
// one connection so that anything a statement leaves on the connection (an
// attachment, a PRAGMA) is still there for the next one.
func openSavedSQLHandle(t *testing.T, dsn string) *sqlx.DB {
	t.Helper()
	ro, err := CreateReadOnlyDatabaseConnection("sqlite", dsn)
	if err != nil {
		t.Fatalf("open read-only handle: %v", err)
	}
	ro.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = ro.Close() })
	return ro
}

func resourceName(t *testing.T, path string) string {
	t.Helper()
	check, err := sqlx.Open(SQLiteDriverName, path)
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	var name string
	if err := check.Get(&name, "SELECT name FROM resources WHERE id = 1"); err != nil {
		t.Fatalf("read back through a writable handle: %v", err)
	}
	return name
}

// GitHub #88: an attached database opens read-write whatever the main schema's
// mode=ro says, so the read-only handle must refuse ATTACH itself.
func TestSavedSQLHandleRefusesAttachAndWritesThroughIt(t *testing.T) {
	path := savedSQLFixture(t)
	ro := openSavedSQLHandle(t, "file:"+path+"?mode=ro")

	var n int
	if err := ro.Get(&n, "SELECT count(*) FROM resources"); err != nil || n != 1 {
		t.Fatalf("positive control: a plain SELECT must work, got n=%d err=%v", n, err)
	}

	if _, err := ro.Exec("ATTACH DATABASE ? AS rw", path); err == nil {
		t.Fatal("ATTACH of the primary database file succeeded on the read-only handle")
	}
	if _, err := ro.Exec("UPDATE rw.resources SET name = 'pwned' WHERE id = 1"); err == nil {
		t.Fatal("a write through an attached schema succeeded")
	}
	created := filepath.Join(t.TempDir(), "new.db")
	if _, err := ro.Exec("ATTACH DATABASE ? AS fresh", created); err == nil {
		t.Fatal("ATTACH of a new database file succeeded on the read-only handle")
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Fatalf("ATTACH created %s on disk (stat err %v)", created, err)
	}
	if got := resourceName(t, path); got != "original" {
		t.Fatalf("resources.name = %q after the attach attempts, want it unchanged", got)
	}
}

func TestSavedSQLHandleCannotTurnQueryOnlyOff(t *testing.T) {
	path := savedSQLFixture(t)
	// No mode=ro, and no file: prefix either: the handle alone must refuse writes.
	ro := openSavedSQLHandle(t, path)

	if _, err := ro.Exec("PRAGMA query_only = 0"); err == nil {
		t.Fatal("PRAGMA query_only = 0 succeeded on the read-only handle")
	}
	for _, statement := range []string{
		"UPDATE resources SET name = 'pwned' WHERE id = 1",
		"INSERT INTO resources (id, name) VALUES (2, 'x')",
		"DELETE FROM resources",
		"CREATE TABLE evil (id INTEGER)",
		"CREATE TEMP TABLE evil (id INTEGER)",
		"DROP TABLE resources",
	} {
		if _, err := ro.Exec(statement); err == nil {
			t.Errorf("%q succeeded on the read-only handle", statement)
		}
	}
	if got := resourceName(t, path); got != "original" {
		t.Fatalf("resources.name = %q, want it unchanged", got)
	}
}

// GitHub #87: saved SQL must not reach credentials or other admin-only tables.
func TestSavedSQLHandleCannotReadSensitiveTables(t *testing.T) {
	path := savedSQLFixture(t)
	ro := openSavedSQLHandle(t, "file:"+path+"?mode=ro")

	for _, query := range []string{
		"SELECT password_hash FROM users",
		"SELECT * FROM users",
		"SELECT count(*) FROM users",
		"SELECT r.name FROM resources r JOIN users u ON u.id = r.id",
		"WITH x AS (SELECT password_hash FROM users) SELECT * FROM x",
		"SELECT (SELECT password_hash FROM main.users LIMIT 1)",
	} {
		rows, err := ro.Queryx(query)
		if err == nil {
			rows.Close()
			t.Errorf("%q ran on the read-only handle", query)
			continue
		}
		// SQLite says "access to users.<column> is prohibited" for a column it
		// was refused and "not authorized" for a bare table reference.
		if msg := err.Error(); !strings.Contains(msg, "prohibited") && !strings.Contains(msg, "not authorized") {
			t.Errorf("%q failed for the wrong reason: %v", query, err)
		}
	}

	// Positive controls: ordinary read-only SQL still works.
	var n int
	if err := ro.Get(&n, "WITH RECURSIVE c(x) AS (SELECT 1 UNION ALL SELECT x+1 FROM c WHERE x < 3) SELECT count(*) FROM c"); err != nil || n != 3 {
		t.Fatalf("recursive CTE: n=%d err=%v", n, err)
	}
	var name string
	if err := ro.Get(&name, "SELECT upper(name) FROM resources WHERE id = 1"); err != nil || name != "ORIGINAL" {
		t.Fatalf("function call: name=%q err=%v", name, err)
	}
}

func TestSavedSQLDeniedTablesCoverEveryAuthAndSecretTable(t *testing.T) {
	for _, table := range []string{
		"users", "sessions", "api_tokens", "plugin_states", "plugin_kvs",
		"runtime_settings", "log_entries", "user_settings", "saved_searches", "job_replay_envelopes",
	} {
		if !IsSavedSQLDeniedTable(table) {
			t.Errorf("%s is readable by saved SQL", table)
		}
	}
	if IsSavedSQLDeniedTable("resources") {
		t.Error("resources must stay readable by saved SQL")
	}
}
