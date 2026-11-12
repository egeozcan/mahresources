//go:build postgres

package api_tests

import (
	"fmt"
	"net/http"
	"testing"

	"mahresources/models"
)

// GitHub #87: on Postgres a saved query runs as one prepared statement in a READ
// ONLY transaction that has already taken its snapshot, whatever role
// DB_READONLY_DSN connects as (the harness uses the writable test role). It can
// neither write, nor switch the transaction to read-write, nor end it and write.
func TestPG_SavedSQLCannotWrite(t *testing.T) {
	tc := SetupPostgresTestEnv(t)
	tag := &models.Tag{Name: "original"}
	if err := tc.DB.Create(tag).Error; err != nil {
		t.Fatal(err)
	}

	positive := createSavedQuery(t, tc, "pg read", "SELECT name FROM tags")
	if resp := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/query/run?id=%d", positive.ID), map[string]any{}); resp.Code != http.StatusOK {
		t.Fatalf("positive control: SELECT = %d: %s", resp.Code, resp.Body.String())
	}

	for i, text := range []string{
		fmt.Sprintf("UPDATE tags SET name = 'pwned' WHERE id = %d", tag.ID),
		fmt.Sprintf("WITH x AS (UPDATE tags SET name = 'pwned' WHERE id = %d RETURNING id) SELECT * FROM x", tag.ID),
		"SET TRANSACTION READ WRITE",
		fmt.Sprintf("COMMIT; UPDATE tags SET name = 'pwned' WHERE id = %d", tag.ID),
	} {
		q := createSavedQuery(t, tc, fmt.Sprintf("pg write %d", i), text)
		resp := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/query/run?id=%d", q.ID), map[string]any{})
		if resp.Code == http.StatusOK {
			t.Errorf("%q ran: %s", text, resp.Body.String())
		}
	}

	var name string
	tc.DB.Raw("SELECT name FROM tags WHERE id = ?", tag.ID).Scan(&name)
	if name != "original" {
		t.Fatalf("tag name = %q, want it unchanged", name)
	}
}
