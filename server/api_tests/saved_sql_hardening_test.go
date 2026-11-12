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

// GitHub #87 item 4: the schema listing leaves out what saved SQL may not read,
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
		if models.IsSavedSQLDeniedTable(table) {
			t.Errorf("schema lists %s", table)
		}
	}
	for _, table := range []string{"users", "sessions", "api_tokens", "plugin_states", "plugin_kvs",
		"runtime_settings", "log_entries", "user_settings", "saved_searches", "job_replay_envelopes"} {
		// The deny list names real tables, not a guess at GORM's naming.
		var n int64
		tc.DB.Raw("SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&n)
		if n != 1 {
			t.Errorf("no table named %s; the deny list is out of date", table)
		}
		if _, ok := schema[table]; ok {
			t.Errorf("schema lists %s", table)
		}
	}
	if strings.Contains(rr.Body.String(), "password_hash") {
		t.Error("schema mentions password_hash")
	}
}
