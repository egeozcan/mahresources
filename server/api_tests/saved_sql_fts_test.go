//go:build json1 && fts5

package api_tests

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"mahresources/models"
)

// The saved-SQL handle's authorizer is an allow-list. FTS5 reads
// PRAGMA main.data_version internally on every MATCH, so the authorizer must let
// exactly that read through: saved SQL over the app's own full-text indexes has
// to keep working, while every other pragma stays refused.
func TestSavedSQLCanQueryFullTextIndexes(t *testing.T) {
	tc := setupAuthEnv(t)
	if err := tc.AppCtx.InitFTS(); err != nil {
		t.Fatalf("InitFTS: %v", err)
	}
	editor := roleBearer(t, tc, models.RoleEditor)
	user := roleBearer(t, tc, models.RoleUser)
	if err := tc.DB.Create(&models.Tag{Name: "findme"}).Error; err != nil {
		t.Fatal(err)
	}

	for i, text := range []string{
		"SELECT rowid FROM tags_fts WHERE tags_fts MATCH 'findme'",
		"SELECT rowid, bm25(tags_fts) AS score, highlight(tags_fts, 0, '[', ']') AS hl FROM tags_fts WHERE tags_fts MATCH 'findme' ORDER BY rank",
		"SELECT t.name FROM tags t JOIN tags_fts f ON f.rowid = t.id WHERE tags_fts MATCH 'find*'",
	} {
		name := editorSavesQuery(t, tc, editor, fmt.Sprintf("fts-%d", i), text)
		for role, bearer := range map[string]string{"editor": editor, "user": user} {
			code, body := runSavedQuery(tc, bearer, name)
			if code != http.StatusOK {
				t.Errorf("%s running %q = %d, want 200: %s", role, text, code, body)
				continue
			}
			if !strings.Contains(body, "findme") && !strings.Contains(body, "rowid") {
				t.Errorf("%s running %q returned no match: %s", role, text, body)
			}
		}
	}

	for i, text := range []string{
		"PRAGMA data_version = 1",
		"PRAGMA main.data_version = 1",
		"PRAGMA query_only = 0",
		"PRAGMA table_info(users)",
		"SELECT * FROM pragma_table_info('users')",
	} {
		name := editorSavesQuery(t, tc, editor, fmt.Sprintf("pragma-%d", i), text)
		if code, body := runSavedQuery(tc, editor, name); code == http.StatusOK {
			t.Errorf("%q ran: 200 %s", text, body)
		}
	}
}
