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
		// An external-content FTS5 index, as fts/sqlite.go creates them.
		"CREATE VIRTUAL TABLE resources_fts USING fts5(name, content='resources', content_rowid='id')",
		"INSERT INTO resources_fts(resources_fts) VALUES('rebuild')",
		// A full-text index over a denied table; none exists in the app today.
		"CREATE VIRTUAL TABLE users_fts USING fts5(username, password_hash)",
		"INSERT INTO users_fts(username, password_hash) VALUES ('ed', 'secret')",
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

	// The authorizer lets FTS5's internal read of data_version through; no
	// other pragma, and no pragma that sets a value, may run.
	for _, statement := range []string{
		"PRAGMA query_only = 0",
		"PRAGMA query_only = OFF",
		"PRAGMA main.query_only = 0",
		"PRAGMA data_version = 1",
		"PRAGMA main.data_version = 1",
		"PRAGMA data_version(1)",
		"PRAGMA query_only",
		"PRAGMA table_info(users)",
		"SELECT * FROM pragma_table_info('users')",
		"PRAGMA journal_mode = DELETE",
		"PRAGMA writable_schema = ON",
	} {
		if _, err := ro.Exec(statement); err == nil {
			t.Errorf("%q succeeded on the read-only handle", statement)
		}
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
		"SELECT * FROM users_fts WHERE users_fts MATCH 'ed'",
		"SELECT * FROM users_fts_content",
		"SELECT * FROM users_fts_data",
	} {
		rows, err := ro.Queryx(query)
		if err == nil {
			rows.Close()
			t.Errorf("%q ran on the read-only handle", query)
			continue
		}
		// SQLite says "access to users.<column> is prohibited" for a column it
		// was refused and "not authorized" for a bare table reference; an FTS5
		// table over a denied table fails to construct because its own read of
		// <name>_fts_config is refused.
		if msg := err.Error(); !strings.Contains(msg, "prohibited") && !strings.Contains(msg, "not authorized") &&
			!strings.Contains(msg, "vtable constructor failed") {
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
	// FTS5 MATCH issues PRAGMA main.data_version internally while filtering;
	// the authorizer must let that read through or every full-text query fails
	// at row iteration.
	var rowid int
	if err := ro.Get(&rowid, "SELECT rowid FROM resources_fts WHERE resources_fts MATCH 'original'"); err != nil || rowid != 1 {
		t.Fatalf("FTS5 MATCH: rowid=%d err=%v", rowid, err)
	}
	var hit struct {
		Score     float64 `db:"score"`
		Highlight string  `db:"hl"`
		Snippet   string  `db:"sn"`
	}
	if err := ro.Get(&hit, `SELECT bm25(resources_fts) AS score,
		highlight(resources_fts, 0, '[', ']') AS hl,
		snippet(resources_fts, 0, '[', ']', '...', 5) AS sn
		FROM resources_fts WHERE resources_fts MATCH 'original' ORDER BY rank`); err != nil || hit.Highlight != "[original]" {
		t.Fatalf("FTS5 bm25/highlight/snippet: %+v err=%v", hit, err)
	}
	var total int
	if err := ro.Get(&total, `SELECT (SELECT count(*) FROM json_each('[1,2]')) + (SELECT count(*) FROM json_tree('{"a":[1]}'))`); err != nil || total != 5 {
		t.Fatalf("json_each/json_tree: total=%d err=%v", total, err)
	}
}

func TestSavedSQLTableAllowList(t *testing.T) {
	for _, table := range []string{
		"users", "sessions", "api_tokens", "plugin_states", "plugin_kvs",
		"runtime_settings", "log_entries", "user_settings", "saved_searches", "job_replay_envelopes",
		"jobs", "template_revisions", "resource_reductions", "download_history_entries",
	} {
		if IsSavedSQLAllowedTable(table) {
			t.Errorf("%s is readable by saved SQL", table)
		}
		if !IsSavedSQLClassifiedTable(table) {
			t.Errorf("%s is not classified", table)
		}
	}
	for _, table := range []string{"users_fts", "users_fts_data", "SESSIONS_FTS_content", "api_tokens_fts_idx",
		"sqlite_master", "sqlite_schema", "sqlite_sequence", "unknown_table", "resources_ftsx", "pragma_table_info"} {
		if IsSavedSQLAllowedTable(table) {
			t.Errorf("%s is readable by saved SQL", table)
		}
	}
	for _, table := range []string{"resources", "Resources", "resources_fts", "resources_fts_data", "resources_fts_idx",
		"resources_fts_docsize", "resources_fts_config", "notes_fts", "tags_fts", "resource_tags", "groups_related_notes"} {
		if !IsSavedSQLAllowedTable(table) {
			t.Errorf("%s must stay readable by saved SQL", table)
		}
	}
}

// Reads go through the allow-list however they are spelled: a table the list
// does not name is refused directly, through a view, a join, a CTE or a
// subquery, and so is the schema catalogue. A view is refused even over allowed
// tables; the server creates none.
func TestSavedSQLHandleReadsOnlyAllowListedTables(t *testing.T) {
	path := savedSQLFixture(t)
	writer, err := sqlx.Open(SQLiteDriverName, path)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"CREATE TABLE probe (id INTEGER PRIMARY KEY, v TEXT)",
		"INSERT INTO probe (id, v) VALUES (1, 'unlisted')",
		"CREATE VIEW probe_values AS SELECT v FROM probe",
	} {
		if _, err := writer.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	_ = writer.Close()
	ro := openSavedSQLHandle(t, "file:"+path+"?mode=ro")

	for _, query := range []string{
		"SELECT * FROM probe",
		"SELECT count(*) FROM users",
		"SELECT count(*) FROM main.users",
		"SELECT count(*) FROM sqlite_master",
		"SELECT * FROM probe_values",
		"SELECT r.name FROM resources r JOIN probe p ON p.id = r.id",
		"WITH x AS (SELECT v FROM probe) SELECT * FROM x",
		"SELECT name FROM resources WHERE id IN (SELECT id FROM probe)",
		"SELECT sql FROM sqlite_master",
		"SELECT name FROM sqlite_schema",
		"SELECT * FROM sqlite_temp_master",
	} {
		rows, err := ro.Queryx(query)
		if err == nil {
			rows.Close()
			t.Errorf("%q ran on the read-only handle", query)
			continue
		}
		if msg := err.Error(); !strings.Contains(msg, "prohibited") && !strings.Contains(msg, "not authorized") {
			t.Errorf("%q failed for the wrong reason: %v", query, err)
		}
	}

	// Positive controls: CTEs and subqueries over allowed tables work, and so
	// does counting a CTE, whose name SQLite reports like a table's.
	var n int
	if err := ro.Get(&n, "WITH x AS MATERIALIZED (SELECT name FROM resources) SELECT count(*) FROM x"); err != nil || n != 1 {
		t.Errorf("count(*) over a CTE: n=%d err=%v", n, err)
	}
	for _, query := range []string{
		"WITH x AS (SELECT name FROM resources) SELECT name FROM x",
		"SELECT name FROM (SELECT name FROM resources) AS sub",
		"SELECT name FROM resources WHERE id IN (SELECT rowid FROM resources_fts WHERE resources_fts MATCH 'original')",
	} {
		var name string
		if err := ro.Get(&name, query); err != nil || name != "original" {
			t.Errorf("%q: name=%q err=%v", query, name, err)
		}
	}
}
