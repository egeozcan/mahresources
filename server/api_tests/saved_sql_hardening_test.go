package api_tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"mahresources/models"
)

// editorSavesQuery saves SQL as the editor through the API and returns its name.
func editorSavesQuery(t *testing.T, tc *TestContext, editor, name, text string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"name": name, "text": text})
	rr := doReq(tc, http.MethodPost, "/v1/query",
		map[string]string{"Authorization": editor, "Content-Type": "application/json", "Accept": "application/json"},
		nil, strings.NewReader(string(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("editor saving %q = %d, want 200 (editors keep authoring saved SQL): %s", name, rr.Code, rr.Body.String())
	}
	return name
}

func runSavedQuery(tc *TestContext, bearer, name string) (int, string) {
	rr := doReq(tc, http.MethodPost, "/v1/query/run?name="+url.QueryEscape(name),
		map[string]string{"Authorization": bearer, "Accept": "application/json"}, nil, nil)
	return rr.Code, rr.Body.String()
}

// GitHub #87: an editor may author saved SQL, but neither the editor nor an
// unscoped user running it may read credentials, plugin secrets or the audit log.
func TestSavedSQLCannotReadSensitiveTables(t *testing.T) {
	tc := setupAuthEnv(t)
	editor := roleBearer(t, tc, models.RoleEditor)
	user := roleBearer(t, tc, models.RoleUser)
	// Something secret-shaped in every table, so a leak would be visible.
	if err := tc.DB.Create(&models.PluginState{PluginName: "p", SettingsJSON: `{"api_key":"sk-leak"}`}).Error; err != nil {
		t.Fatal(err)
	}

	positive := editorSavesQuery(t, tc, editor, "content", "SELECT count(*) AS n FROM tags")
	for _, bearer := range []string{editor, user} {
		if code, body := runSavedQuery(tc, bearer, positive); code != http.StatusOK {
			t.Fatalf("positive control: a content query = %d, want 200: %s", code, body)
		}
	}

	for i, text := range []string{
		"SELECT username, role, password_hash FROM users",
		"SELECT token_hash, csrf_token FROM sessions",
		"SELECT user_id, token_hash FROM api_tokens",
		"SELECT plugin_name, settings_json FROM plugin_states",
		"SELECT * FROM log_entries",
		"SELECT * FROM runtime_settings",
		"SELECT * FROM plugin_kvs",
	} {
		name := editorSavesQuery(t, tc, editor, fmt.Sprintf("sensitive-%d", i), text)
		for role, bearer := range map[string]string{"editor": editor, "user": user} {
			code, body := runSavedQuery(tc, bearer, name)
			if code == http.StatusOK {
				t.Errorf("%s ran %q: 200 %s", role, text, body)
			}
			for _, leak := range []string{"$2a$", "sk-leak", "password_hash\":"} {
				if strings.Contains(body, leak) {
					t.Errorf("%s running %q leaked %q: %s", role, text, leak, body)
				}
			}
		}
	}
}

// GitHub #87 and #88: saved SQL cannot write, directly or through an attached
// copy of the database, so an editor cannot make themselves admin.
func TestSavedSQLCannotEscalateEditorToAdmin(t *testing.T) {
	tc := setupAuthEnv(t)
	editor := roleBearer(t, tc, models.RoleEditor)
	if !tc.AppCtx.IsReadOnlyDBEnforced() {
		t.Fatal("the harness's read-only handle reports not enforced; it must be opened like production's")
	}

	var dbFile string
	if err := tc.DB.Raw("SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&dbFile).Error; err != nil || dbFile == "" {
		t.Fatalf("locate database file: %q %v", dbFile, err)
	}

	for i, text := range []string{
		"UPDATE users SET role = 'admin' WHERE username = 'rb_editor'",
		"PRAGMA query_only = 0",
		"ATTACH DATABASE '" + dbFile + "' AS rw",
		"UPDATE rw.users SET role = 'admin' WHERE username = 'rb_editor'",
	} {
		name := editorSavesQuery(t, tc, editor, fmt.Sprintf("escalate-%d", i), text)
		if code, body := runSavedQuery(tc, editor, name); code == http.StatusOK {
			t.Errorf("%q ran: 200 %s", text, body)
		}
	}

	if rr := doReq(tc, http.MethodGet, "/v1/users",
		map[string]string{"Authorization": editor, "Accept": "application/json"}, nil, nil); rr.Code != http.StatusForbidden {
		t.Fatalf("editor GET /v1/users = %d after the attempts, want 403", rr.Code)
	}
	var role string
	tc.DB.Raw("SELECT role FROM users WHERE username = 'rb_editor'").Scan(&role)
	if role != string(models.RoleEditor) {
		t.Fatalf("rb_editor's role is %q, want editor", role)
	}
}

// GitHub #87 item 4: the schema listing lists only what saved SQL may read,
// and a group-limited principal, who cannot run saved SQL, does not get it.
func TestSavedSQLSchemaOmitsSensitiveTablesAndRefusesScoped(t *testing.T) {
	tc := setupAuthEnv(t)
	user := roleBearer(t, tc, models.RoleUser)
	guest := roleBearer(t, tc, models.RoleGuest)

	rr := doReq(tc, http.MethodGet, "/v1/query/schema",
		map[string]string{"Authorization": guest, "Accept": "application/json"}, nil, nil)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("guest GET /v1/query/schema = %d, want 403: %s", rr.Code, rr.Body.String())
	}

	rr = doReq(tc, http.MethodGet, "/v1/query/schema",
		map[string]string{"Authorization": user, "Accept": "application/json"}, nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("user GET /v1/query/schema = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var schema map[string][]string
	if err := json.Unmarshal(rr.Body.Bytes(), &schema); err != nil {
		t.Fatal(err)
	}
	if _, ok := schema["resources"]; !ok {
		t.Fatalf("positive control: resources missing from schema %v", schema)
	}
	for table := range schema {
		if !models.IsSavedSQLAllowedTable(table) {
			t.Errorf("schema lists %s", table)
		}
	}
	for _, table := range []string{"users", "sessions", "api_tokens", "plugin_states", "plugin_kvs",
		"runtime_settings", "log_entries", "user_settings", "saved_searches", "job_replay_envelopes"} {
		// These are real tables, not a guess at GORM's naming.
		var n int64
		tc.DB.Raw("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&n)
		if n != 1 {
			t.Errorf("no table named %s; the table list is out of date", table)
		}
		if _, ok := schema[table]; ok {
			t.Errorf("schema lists %s", table)
		}
	}
	if strings.Contains(rr.Body.String(), "password_hash") {
		t.Error("schema mentions password_hash")
	}
}

// GitHub #87: saved SQL reads content tables only. A table that holds no
// credential but is not content either (Jobs, downloads, template history,
// owner-restricted Reductions, the SQLite catalogue) is refused as well, because
// the handle allows reads of listed tables rather than denying listed ones.
func TestSavedSQLReadsOnlyAllowListedTables(t *testing.T) {
	tc := setupAuthEnv(t)
	editor := roleBearer(t, tc, models.RoleEditor)

	positive := editorSavesQuery(t, tc, editor, "content",
		"SELECT t.name, count(rt.resource_id) AS n FROM tags t LEFT JOIN resource_tags rt ON rt.tag_id = t.id GROUP BY t.name")
	if code, body := runSavedQuery(tc, editor, positive); code != http.StatusOK {
		t.Fatalf("positive control: a content query = %d, want 200: %s", code, body)
	}

	for i, text := range []string{
		"SELECT * FROM jobs",
		"SELECT * FROM job_events",
		"SELECT * FROM download_history_entries",
		"SELECT * FROM scheduled_downloads",
		"SELECT * FROM template_revisions",
		"SELECT * FROM resource_reductions",
		"SELECT * FROM plugin_command_runs",
		"SELECT sql FROM sqlite_master",
		"SELECT r.name FROM resources r JOIN jobs j ON j.id = r.id",
	} {
		name := editorSavesQuery(t, tc, editor, fmt.Sprintf("unlisted-%d", i), text)
		code, body := runSavedQuery(tc, editor, name)
		if code == http.StatusOK {
			t.Errorf("%q ran: 200 %s", text, body)
		} else if !strings.Contains(body, "not authorized") && !strings.Contains(body, "prohibited") {
			t.Errorf("%q failed for another reason than the authorizer: %d %s", text, code, body)
		}
	}
}

// SQLite does not tell the authorizer about the right-hand table's columns in
// a USING or NATURAL join, so before the bytecode check these returned every
// username and bcrypt hash, session token hashes and plugin secrets to anyone
// who could run saved SQL, answered "does this username exist", and read a
// table the allow-list never mentions.
func TestSavedSQLJoinColumnsCannotBypassAllowList(t *testing.T) {
	tc := setupAuthEnv(t)
	editor := roleBearer(t, tc, models.RoleEditor)
	user := roleBearer(t, tc, models.RoleUser)
	if err := tc.DB.Create(&models.PluginState{PluginName: "p", SettingsJSON: `{"api_key":"sk-leak"}`}).Error; err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"CREATE TABLE legacy_secrets (k INTEGER PRIMARY KEY, name TEXT)",
		"INSERT INTO legacy_secrets VALUES (7, 'hunter2')",
	} {
		if err := tc.DB.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}
	for i, text := range []string{
		"SELECT username, password_hash FROM (SELECT 1 AS username, 1 AS password_hash WHERE 0) FULL JOIN users USING (username, password_hash)",
		"SELECT token_hash FROM (SELECT 1 AS token_hash WHERE 0) FULL JOIN sessions USING (token_hash)",
		"SELECT settings_json FROM (SELECT 1 AS settings_json WHERE 0) FULL JOIN plugin_states USING (settings_json)",
		"SELECT token_hash FROM (SELECT 1 AS token_hash WHERE 0) FULL JOIN api_tokens USING (token_hash)",
		"SELECT count(*) AS n FROM (SELECT 'rb_editor' AS username) NATURAL JOIN users",
		"SELECT name FROM (SELECT 1 AS name WHERE 0) FULL JOIN legacy_secrets USING (name)",
	} {
		name := editorSavesQuery(t, tc, editor, fmt.Sprintf("join-bypass-%d", i), text)
		for role, bearer := range map[string]string{"editor": editor, "user": user} {
			code, body := runSavedQuery(tc, bearer, name)
			if code == http.StatusOK {
				t.Errorf("%s ran %q: 200 %s", role, text, body)
			} else if !strings.Contains(body, "not authorized") {
				t.Errorf("%s running %q failed for another reason than the allow-list: %d %s", role, text, code, body)
			}
			for _, leak := range []string{"$2a$", "sk-leak", "hunter2", "rb_editor"} {
				if strings.Contains(body, leak) {
					t.Errorf("%s running %q leaked %q: %s", role, text, leak, body)
				}
			}
		}
	}

	// Positive controls: USING and NATURAL joins between allowed tables.
	for i, text := range []string{
		"SELECT count(*) AS n FROM resource_tags JOIN groups_related_resources USING (resource_id)",
		"SELECT count(*) AS n FROM tags NATURAL JOIN categories",
		"SELECT name FROM (SELECT 'x' AS name WHERE 0) FULL JOIN tags USING (name)",
	} {
		name := editorSavesQuery(t, tc, editor, fmt.Sprintf("join-allowed-%d", i), text)
		if code, body := runSavedQuery(tc, user, name); code != http.StatusOK {
			t.Errorf("positive control %q = %d, want 200: %s", text, code, body)
		}
	}
}
