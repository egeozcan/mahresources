//go:build postgres

package api_tests

import (
	"context"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"mahresources/application_context"
	"mahresources/models"
)

const readOnlyRolePassword = "ro-test-pw"

// pgRoleName returns a role name no other test uses; roles are cluster-wide
// and the container is shared by the package.
func pgRoleName(prefix string) string {
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano()%1e9, rand.Intn(1e6))
}

// dsnAs is testDSN with its user and password replaced.
func dsnAs(t *testing.T, testDSN, role string) string {
	t.Helper()
	u, err := url.Parse(testDSN)
	if err != nil {
		t.Fatal(err)
	}
	u.User = url.UserPassword(role, readOnlyRolePassword)
	return u.String()
}

// openAsTestOwner opens testDSN (the superuser that owns the tables) on lib/pq,
// which runs a multi-statement script in one Exec.
func openAsTestOwner(t *testing.T, testDSN string) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("postgres", testDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// createScriptRole runs the SQL `mahresources -print-readonly-role-sql` prints,
// then gives the role a password, as an administrator would.
func createScriptRole(t *testing.T, owner *sqlx.DB, role string) {
	t.Helper()
	script, err := models.PostgresReadOnlyRoleSQL(role)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(script); err != nil {
		t.Fatalf("run the generated role script: %v\n%s", err, script)
	}
	if _, err := owner.Exec(fmt.Sprintf(`ALTER ROLE %q PASSWORD '%s'`, role, readOnlyRolePassword)); err != nil {
		t.Fatal(err)
	}
}

// setupPostgresAuthEnvAsRole is SetupPostgresTestEnv with -auth on and the
// read-only handle connecting as the role prepare sets up on the migrated
// database. prepare gets the table owner's connection and the role name.
func setupPostgresAuthEnvAsRole(t *testing.T, prepare func(owner *sqlx.DB, role string)) (*TestContext, string) {
	t.Helper()
	role := pgRoleName("ro")
	tc := setupPostgresTestEnvWith(t,
		func(c *application_context.MahresourcesConfig) {
			c.AuthEnabled = true
			c.SessionTTL = time.Hour
		},
		func(testDSN string) string {
			prepare(openAsTestOwner(t, testDSN), role)
			return dsnAs(t, testDSN, role)
		})
	if _, err := tc.AppCtx.EnsureAdminUser("admin", "adminpw1"); err != nil {
		t.Fatalf("bootstrap admin: %v", err)
	}
	return tc, role
}

var pgSensitiveQueries = []string{
	"SELECT username, role, password_hash FROM users",
	"SELECT token_hash, csrf_token FROM sessions",
	"SELECT user_id, token_hash FROM api_tokens",
	"SELECT plugin_name, settings_json FROM plugin_states",
	"SELECT * FROM log_entries",
	"SELECT * FROM plugin_kvs",
	"SELECT * FROM jobs",
}

// GitHub #87: the role the generated script creates passes the server's check,
// runs content queries, and cannot read a credential, secret or audit table.
func TestPG_GeneratedReadOnlyRoleHidesSensitiveTables(t *testing.T) {
	tc, role := setupPostgresAuthEnvAsRole(t, func(owner *sqlx.DB, role string) { createScriptRole(t, owner, role) })
	if err := tc.AppCtx.CheckSavedSQLRole(); err != nil {
		t.Fatalf("the generated role %s failed the check: %v", role, err)
	}
	if !tc.AppCtx.IsReadOnlyDBEnforced() {
		t.Fatal("the generated role reported not enforced")
	}
	editor := roleBearer(t, tc, models.RoleEditor)
	user := roleBearer(t, tc, models.RoleUser)
	if err := tc.DB.Create(&models.PluginState{PluginName: "p", SettingsJSON: `{"api_key":"sk-leak"}`}).Error; err != nil {
		t.Fatal(err)
	}

	positive := editorSavesQuery(t, tc, editor, "content",
		"SELECT t.name, count(rt.resource_id) AS n FROM tags t LEFT JOIN resource_tags rt ON rt.tag_id = t.id GROUP BY t.name")
	for _, bearer := range []string{editor, user} {
		if code, body := runSavedQuery(tc, bearer, positive); code != http.StatusOK {
			t.Fatalf("positive control: a content query = %d, want 200: %s", code, body)
		}
	}

	for i, text := range pgSensitiveQueries {
		name := editorSavesQuery(t, tc, editor, fmt.Sprintf("sensitive-%d", i), text)
		code, body := runSavedQuery(tc, editor, name)
		if code == http.StatusOK {
			t.Errorf("%q ran: 200 %s", text, body)
		}
		if !strings.Contains(body, "permission denied") {
			t.Errorf("%q failed for another reason than the role's grants: %d %s", text, code, body)
		}
		for _, leak := range []string{"$2a$", "sk-leak"} {
			if strings.Contains(body, leak) {
				t.Errorf("%q leaked %q: %s", text, leak, body)
			}
		}
	}

	rr := doReq(tc, http.MethodGet, "/v1/query/schema",
		map[string]string{"Authorization": user, "Accept": "application/json"}, nil, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("schema = %d: %s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), `"resources"`) || strings.Contains(rr.Body.String(), "password_hash") {
		t.Fatalf("schema should list resources and not users: %s", rr.Body.String())
	}
}

// GitHub #87: a read-only role that can SELECT from users read every password
// hash through saved SQL. Under -auth the server now refuses saved SQL until
// the role is fixed, and says which check failed.
func TestPG_ReadOnlyRoleThatCanReadUsersDisablesSavedSQL(t *testing.T) {
	tc, _ := setupPostgresAuthEnvAsRole(t, func(owner *sqlx.DB, role string) {
		createScriptRole(t, owner, role)
		if _, err := owner.Exec(fmt.Sprintf(`GRANT SELECT ON users TO %q`, role)); err != nil {
			t.Fatal(err)
		}
	})
	editor := roleBearer(t, tc, models.RoleEditor)
	user := roleBearer(t, tc, models.RoleUser)
	guest := roleBearer(t, tc, models.RoleGuest)

	leak := editorSavesQuery(t, tc, editor, "hashes", "SELECT username, password_hash FROM users")
	content := editorSavesQuery(t, tc, editor, "content", "SELECT count(*) AS n FROM tags")
	for _, name := range []string{leak, content} {
		for _, bearer := range []string{editor, user} {
			code, body := runSavedQuery(tc, bearer, name)
			if code != http.StatusServiceUnavailable {
				t.Errorf("running %s = %d, want 503: %s", name, code, body)
			}
			if strings.Contains(body, "$2a$") {
				t.Errorf("running %s leaked a password hash: %s", name, body)
			}
			if !strings.Contains(body, "public.users") {
				t.Errorf("the refusal does not name the table the role can read: %s", body)
			}
		}
	}
	// A group-limited principal is still told no for who it is, first.
	if code, body := runSavedQuery(tc, guest, content); code != http.StatusForbidden {
		t.Errorf("guest run = %d, want 403: %s", code, body)
	}

	rr := doReq(tc, http.MethodGet, "/v1/query/schema",
		map[string]string{"Authorization": user, "Accept": "application/json"}, nil, nil)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("schema = %d, want 503: %s", rr.Code, rr.Body.String())
	}
	if tc.AppCtx.IsReadOnlyDBEnforced() {
		t.Error("a role that can read users reported enforced")
	}
	if problem := tc.AppCtx.ReadOnlyDBProblem(); !strings.Contains(problem, "public.users") {
		t.Errorf("ReadOnlyDBProblem = %q, want it to name public.users", problem)
	}
}

// A GRANT made while the server runs is noticed: the verdict is re-checked,
// not fixed at startup.
func TestPG_ReadOnlyRoleGrantAfterStartupDisablesSavedSQL(t *testing.T) {
	var owner *sqlx.DB
	var roleName string
	tc, _ := setupPostgresAuthEnvAsRole(t, func(o *sqlx.DB, role string) {
		owner, roleName = o, role
		createScriptRole(t, o, role)
	})
	editor := roleBearer(t, tc, models.RoleEditor)
	name := editorSavesQuery(t, tc, editor, "content", "SELECT count(*) AS n FROM tags")
	if code, body := runSavedQuery(tc, editor, name); code != http.StatusOK {
		t.Fatalf("before the grant = %d: %s", code, body)
	}
	if _, err := owner.Exec(fmt.Sprintf(`GRANT SELECT ON sessions TO %q`, roleName)); err != nil {
		t.Fatal(err)
	}
	// What the cache expiry does a minute later.
	if err := tc.AppCtx.CheckSavedSQLRole(); err == nil {
		t.Fatal("the check passed after GRANT SELECT ON sessions")
	}
	if code, body := runSavedQuery(tc, editor, name); code != http.StatusServiceUnavailable {
		t.Fatalf("after the grant = %d, want 503: %s", code, body)
	}
}

// Each way a role can do more than read the allowed tables fails the check,
// with a message naming it.
func TestPG_ReadOnlyRoleVerificationFailures(t *testing.T) {
	var testDSN string
	tc := setupPostgresTestEnvWith(t, nil, func(dsn string) string { testDSN = dsn; return dsn })
	sqlDB, _ := tc.DB.DB()
	var mainRole, mainDatabase string
	if err := sqlDB.QueryRow("SELECT current_user, current_database()").Scan(&mainRole, &mainDatabase); err != nil {
		t.Fatal(err)
	}
	owner := openAsTestOwner(t, testDSN)

	verifyAs := func(t *testing.T, dsn string) error {
		t.Helper()
		ro, err := models.CreateReadOnlyDatabaseConnection("postgres", dsn)
		if err != nil {
			t.Fatal(err)
		}
		defer ro.Close()
		return models.VerifyPostgresReadOnlyRole(context.Background(), ro, mainRole, mainDatabase)
	}

	t.Run("the generated role passes", func(t *testing.T) {
		role := pgRoleName("ok")
		createScriptRole(t, owner, role)
		if err := verifyAs(t, dsnAs(t, testDSN, role)); err != nil {
			t.Fatalf("generated role failed: %v", err)
		}
	})
	t.Run("the main role", func(t *testing.T) {
		err := verifyAs(t, testDSN)
		for _, want := range []string{"it is the main DB_DSN role", "it is a superuser"} {
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("err = %v, want it to contain %q", err, want)
			}
		}
	})

	for _, tt := range []struct {
		name  string
		extra string // %[1]s is the role
		want  string
	}{
		{"superuser", `ALTER ROLE %[1]q SUPERUSER`, "it is a superuser"},
		{"bypassrls", `ALTER ROLE %[1]q BYPASSRLS`, "it has BYPASSRLS"},
		{"insert on a content table", `GRANT INSERT ON tags TO %[1]q`, "it can write to public.tags"},
		{"update on one column", `GRANT UPDATE (name) ON tags TO %[1]q`, "it can write to public.tags"},
		{"select on users", `GRANT SELECT ON users TO %[1]q`, "it can SELECT from public.users"},
		{"select on one column of users", `GRANT SELECT (password_hash) ON users TO %[1]q`, "it can SELECT from public.users"},
		{"select granted to PUBLIC", `GRANT SELECT ON api_tokens TO PUBLIC`, "it can SELECT from public.api_tokens"},
		{"a sequence", `GRANT USAGE ON SEQUENCE users_id_seq TO %[1]q`, "public.users_id_seq"},
		{"create on the schema", `GRANT CREATE ON SCHEMA public TO %[1]q`, "it has CREATE on schema public"},
		{"member of the main role", `GRANT "` + mainRole + `" TO %[1]q`, "member of the main DB_DSN role"},
		{"member of pg_read_all_data", `GRANT pg_read_all_data TO %[1]q`, "it is a member of pg_read_all_data"},
		{"member of pg_read_server_files", `GRANT pg_read_server_files TO %[1]q`, "pg_read_server_files"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			role := pgRoleName("bad")
			createScriptRole(t, owner, role)
			statement := tt.extra
			if strings.Contains(statement, "%[1]") {
				statement = fmt.Sprintf(statement, role)
			}
			if _, err := owner.Exec(statement); err != nil {
				t.Fatalf("%s: %v", statement, err)
			}
			if strings.Contains(tt.extra, "PUBLIC") {
				t.Cleanup(func() { _, _ = owner.Exec(`REVOKE SELECT ON api_tokens FROM PUBLIC`) })
			}
			err := verifyAs(t, dsnAs(t, testDSN, role))
			if err == nil || !models.IsReadOnlyRoleCheckFailure(err) || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want a role check failure containing %q", err, tt.want)
			}
		})
	}
}

// With -auth off every request is the administrator's, so a role that can read
// more only warns; saved SQL keeps running as before.
func TestPG_ReadOnlyRoleFailureWithAuthOffOnlyWarns(t *testing.T) {
	role := pgRoleName("ro")
	tc := setupPostgresTestEnvWith(t, nil, func(testDSN string) string {
		owner := openAsTestOwner(t, testDSN)
		createScriptRole(t, owner, role)
		if _, err := owner.Exec(fmt.Sprintf(`GRANT SELECT ON users TO %q`, role)); err != nil {
			t.Fatal(err)
		}
		return dsnAs(t, testDSN, role)
	})
	if tc.AppCtx.IsReadOnlyDBEnforced() {
		t.Error("a role that can read users reported enforced")
	}
	q := createSavedQuery(t, tc, "content", "SELECT count(*) AS n FROM tags")
	if resp := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/query/run?id=%d", q.ID), map[string]any{}); resp.Code != http.StatusOK {
		t.Fatalf("run with auth off = %d, want 200: %s", resp.Code, resp.Body.String())
	}
}
