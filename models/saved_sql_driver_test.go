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

// joinBypassFixture adds to savedSQLFixture a table the allow-list does not
// classify, an index on users, a second content table and a join table, so the
// tests below can tell a read of a denied table from a read of an allowed one
// whichever b-tree (table or index) SQLite chooses to open.
func joinBypassFixture(t *testing.T) string {
	t.Helper()
	path := savedSQLFixture(t)
	writer, err := sqlx.Open(SQLiteDriverName, path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	for _, statement := range []string{
		"CREATE TABLE legacy_secrets (k INTEGER PRIMARY KEY, name TEXT)",
		"INSERT INTO legacy_secrets VALUES (7, 'hunter2')",
		"CREATE INDEX idx_users_username ON users(username)",
		"CREATE TABLE tags (id INTEGER PRIMARY KEY, name TEXT)",
		"INSERT INTO tags (id, name) VALUES (1, 'original'), (2, 'other')",
		"CREATE INDEX idx_tags_name ON tags(name)",
		"CREATE TABLE resource_tags (resource_id INTEGER, tag_id INTEGER, PRIMARY KEY (resource_id, tag_id))",
		"INSERT INTO resource_tags VALUES (1, 1)",
	} {
		if _, err := writer.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	return path
}

// A column a USING or NATURAL join merges is read without SQLite telling the
// authorizer (SQLite 3.53: no SQLITE_READ for the right-hand table's join
// columns, and no empty-column READ once a column of it is used), so the
// authorizer alone let these dump users.password_hash, test a guessed username
// and read tables the allow-list never mentions. The handle must refuse them
// from what the compiled statement opens, not from what the authorizer saw.
func TestSavedSQLHandleRefusesJoinColumnBypass(t *testing.T) {
	path := joinBypassFixture(t)
	ro := openSavedSQLHandle(t, "file:"+path+"?mode=ro")

	for _, query := range []string{
		// Dumps of a denied table.
		"SELECT username, password_hash FROM (SELECT 1 AS username, 1 AS password_hash WHERE 0) FULL JOIN users USING (username, password_hash)",
		"SELECT password_hash FROM (SELECT 1 AS password_hash WHERE 0) FULL JOIN users USING (password_hash)",
		"SELECT password_hash FROM (SELECT NULL AS password_hash WHERE 0) RIGHT JOIN users USING (password_hash)",
		"SELECT * FROM (SELECT 1 AS password_hash WHERE 0) NATURAL FULL JOIN users",
		"SELECT password_hash FROM resources FULL JOIN users USING (id)",
		// Oracles: does a user with this name exist?
		"SELECT count(*) FROM (SELECT 'ed' AS username) NATURAL JOIN users",
		"SELECT username FROM (SELECT 'ed' AS username) JOIN users USING (username)",
		"SELECT username FROM (SELECT 'ed' AS username) JOIN users INDEXED BY idx_users_username USING (username)",
		"SELECT 1 FROM (SELECT 1 AS id) NATURAL JOIN users",
		// A table the allow-list does not classify.
		"SELECT name FROM (SELECT 1 AS name WHERE 0) FULL JOIN legacy_secrets USING (name)",
		"SELECT name FROM (SELECT 'hunter2' AS name) JOIN legacy_secrets USING (name)",
		"SELECT count(*) FROM legacy_secrets",
		"SELECT 1 FROM legacy_secrets",
		"SELECT count(*) FROM resources, legacy_secrets",
		// The catalogue, and FTS over a denied table.
		"SELECT name FROM (SELECT 1 AS name WHERE 0) FULL JOIN sqlite_master USING (name)",
		"SELECT sql FROM (SELECT 1 AS sql WHERE 0) FULL JOIN sqlite_temp_master USING (sql)",
		"SELECT username FROM (SELECT 1 AS username WHERE 0) FULL JOIN users_fts USING (username)",
		"SELECT block FROM (SELECT 1 AS block WHERE 0) FULL JOIN users_fts_data USING (block)",
		// Modules that read pages directly are not compiled in.
		"SELECT data FROM (SELECT 1 AS data WHERE 0) FULL JOIN sqlite_dbpage USING (data)",
		"SELECT name FROM (SELECT 1 AS name WHERE 0) FULL JOIN dbstat USING (name)",
		"SELECT sql FROM (SELECT 1 AS sql WHERE 0) FULL JOIN sqlite_stmt USING (sql)",
		// The same, through a CTE and a subquery.
		"WITH u AS (SELECT password_hash FROM (SELECT 1 AS password_hash WHERE 0) FULL JOIN users USING (password_hash)) SELECT * FROM u",
		"SELECT (SELECT password_hash FROM (SELECT 1 AS password_hash WHERE 0) FULL JOIN users USING (password_hash) LIMIT 1)",
		// A second statement is not run quietly either.
		"SELECT 1; SELECT password_hash FROM (SELECT 1 AS password_hash WHERE 0) FULL JOIN users USING (password_hash)",
		"SELECT name FROM resources; SELECT name FROM resources",
	} {
		rows, err := ro.Queryx(query)
		if err == nil {
			var leaked [][]any
			for rows.Next() {
				row, _ := rows.SliceScan()
				leaked = append(leaked, row)
			}
			rows.Close()
			t.Errorf("%q ran on the read-only handle: %v", query, leaked)
			continue
		}
		if msg := err.Error(); !strings.Contains(msg, "not authorized") && !strings.Contains(msg, "prohibited") &&
			!strings.Contains(msg, "vtable constructor failed") && !strings.Contains(msg, "single statement") &&
			!strings.Contains(msg, "no such table") {
			t.Errorf("%q failed for the wrong reason: %v", query, err)
		}
	}

	// Positive controls: USING and NATURAL joins between allowed tables, index
	// lookups on allowed tables, joins through a join table, bound parameters
	// and a trailing semicolon or comment.
	for _, query := range []string{
		"SELECT name FROM resources JOIN tags USING (id, name)",
		"SELECT name FROM resources NATURAL JOIN tags",
		"SELECT name FROM (SELECT 1 AS name WHERE 0) FULL JOIN resources USING (name)",
		"SELECT name FROM resources FULL JOIN tags USING (name) WHERE name = 'original'",
		"SELECT name FROM tags WHERE name = 'original'",
		"SELECT name FROM tags INDEXED BY idx_tags_name WHERE name = 'original'",
		"SELECT r.name FROM resources r JOIN resource_tags rt ON rt.resource_id = r.id JOIN tags t ON t.id = rt.tag_id",
		"SELECT name FROM resources WHERE id IN (SELECT resource_id FROM resource_tags)",
		"SELECT name FROM resources WHERE id = 1;",
		"SELECT name FROM resources WHERE id = 1; -- trailing note\n",
		"SELECT name FROM resources WHERE id = 1 /* c */ ; /* c */ ;",
		"SELECT r.name FROM resources r JOIN resources_fts f ON f.rowid = r.id WHERE resources_fts MATCH 'original'",
	} {
		var name string
		if err := ro.Get(&name, query); err != nil || name != "original" {
			t.Errorf("%q: name=%q err=%v", query, name, err)
		}
	}
	var name string
	if err := ro.Get(&name, "SELECT name FROM resources WHERE id = ?", 1); err != nil || name != "original" {
		t.Errorf("positional parameter: name=%q err=%v", name, err)
	}
	rows, err := ro.NamedQuery("SELECT name FROM resources WHERE id = :id AND name = :name", map[string]any{"id": 1, "name": "original"})
	if err != nil {
		t.Fatalf("named parameters: %v", err)
	}
	if !rows.Next() {
		t.Error("named parameters: no row")
	}
	rows.Close()
}

// The check maps the b-trees a statement opens back to tables by root page, so
// the map must follow schema changes: a table created after the handle first
// ran a query must be readable if allowed, and a page a dropped allowed table
// gave back must not stay allowed once a denied table reuses it.
func TestSavedSQLHandleFollowsSchemaChanges(t *testing.T) {
	path := joinBypassFixture(t)
	ro := openSavedSQLHandle(t, "file:"+path+"?mode=ro")
	writer, err := sqlx.Open(SQLiteDriverName, path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	writer.SetMaxOpenConns(1)

	var n int
	if err := ro.Get(&n, "SELECT count(*) FROM resources"); err != nil || n != 1 {
		t.Fatalf("warm-up: n=%d err=%v", n, err)
	}
	for _, statement := range []string{
		"CREATE TABLE notes (id INTEGER PRIMARY KEY, name TEXT)",
		"INSERT INTO notes VALUES (1, 'note')",
	} {
		if _, err := writer.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	var note string
	if err := ro.Get(&note, "SELECT name FROM notes"); err != nil || note != "note" {
		t.Fatalf("a table created after the first query: name=%q err=%v", note, err)
	}
	var notesRoot int
	if err := writer.Get(&notesRoot, "SELECT rootpage FROM sqlite_master WHERE name = 'notes'"); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"DROP TABLE notes",
		"CREATE TABLE plugin_states (id INTEGER PRIMARY KEY, settings_json TEXT)",
		`INSERT INTO plugin_states VALUES (1, '{"api_key":"sk-leak"}')`,
	} {
		if _, err := writer.Exec(statement); err != nil {
			t.Fatalf("%s: %v", statement, err)
		}
	}
	var pluginRoot int
	if err := writer.Get(&pluginRoot, "SELECT rootpage FROM sqlite_master WHERE name = 'plugin_states'"); err != nil {
		t.Fatal(err)
	}
	if pluginRoot != notesRoot {
		t.Logf("plugin_states did not reuse notes' root page (%d vs %d); the test still checks the refusal", pluginRoot, notesRoot)
	}
	var leaked string
	err = ro.Get(&leaked, "SELECT settings_json FROM (SELECT 1 AS settings_json WHERE 0) FULL JOIN plugin_states USING (settings_json)")
	if err == nil || strings.Contains(leaked, "sk-leak") {
		t.Fatalf("plugin_states read after it took the place of an allowed table: %q err=%v", leaked, err)
	}
}
