package application_context

import (
	"testing"

	"github.com/jmoiron/sqlx"
	"mahresources/models"
)

// GitHub #88: with the read-only DSN the ephemeral (-memory-db) start builds, a
// saved query must not attach the primary database file and write through the
// attachment. One connection, so the second query would reuse the first's.
func TestRunReadOnlyQueryCannotWriteThroughAnAttachedDatabase(t *testing.T) {
	isolateTempDir(t)
	ctx, exec := openEphemeralContext(t)
	ctx.readOnlyDB.SetMaxOpenConns(1)
	if err := ctx.db.AutoMigrate(&models.Query{}); err != nil {
		t.Fatal(err)
	}
	if err := exec("CREATE TABLE tags (id INTEGER PRIMARY KEY, v TEXT)"); err != nil {
		t.Fatal(err)
	}
	if err := exec("INSERT INTO tags (id, v) VALUES (1, 'original')"); err != nil {
		t.Fatal(err)
	}

	drain := func(rows *sqlx.Rows) error {
		for rows.Next() {
		}
		return rows.Err()
	}
	save := func(name, text string) uint {
		q := &models.Query{Name: name, Text: text}
		if err := ctx.db.Create(q).Error; err != nil {
			t.Fatal(err)
		}
		return q.ID
	}

	positive := save("positive", "SELECT v FROM tags")
	if err := ctx.RunReadOnlyQuery(positive, map[string]any{}, drain); err != nil {
		t.Fatalf("positive control: a plain SELECT failed: %v", err)
	}

	attach := save("attach", "ATTACH DATABASE '"+ctx.ephemeralDB.path+"' AS rw")
	if err := ctx.RunReadOnlyQuery(attach, map[string]any{}, drain); err == nil {
		t.Error("ATTACH of the primary database ran")
	}
	write := save("write", "UPDATE rw.tags SET v = 'pwned' WHERE id = 1")
	_ = ctx.RunReadOnlyQuery(write, map[string]any{}, drain)
	query := save("query_only", "PRAGMA query_only = 0")
	if err := ctx.RunReadOnlyQuery(query, map[string]any{}, drain); err == nil {
		t.Error("PRAGMA query_only = 0 ran")
	}

	var v string
	if err := ctx.db.Raw("SELECT v FROM tags WHERE id = 1").Scan(&v).Error; err != nil {
		t.Fatal(err)
	}
	if v != "original" {
		t.Fatalf("tags.v = %q through the main handle, want it unchanged", v)
	}
	if !ctx.IsReadOnlyDBEnforced() {
		t.Error("the ephemeral read-only handle reported not enforced")
	}
}
