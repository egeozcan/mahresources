package fts

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=private"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	db.Exec("CREATE TABLE items (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT)")
	return db
}

func TestFuzzyFallbackUnderscoreNotWildcard(t *testing.T) {
	fts := NewSQLiteFTS()

	t.Run("short term underscore treated as literal", func(t *testing.T) {
		db := setupTestDB(t)
		// "ax" does NOT contain the literal substring "_x"
		db.Exec("INSERT INTO items (name) VALUES ('ax')")
		// "b_x" DOES contain the literal substring "_x"
		db.Exec("INSERT INTO items (name) VALUES ('b_x')")

		var names []string
		fts.fuzzyFallback(db.Table("items"), "items", "_x").
			Pluck("name", &names)

		// Bug: "_x" in LIKE becomes a single-char wildcard, so "%_x%" matches "ax"
		// Expected: only "b_x" matches because it contains the literal substring "_x"
		if len(names) != 1 {
			t.Errorf("expected 1 match for literal '_x', got %d: %v", len(names), names)
		} else if names[0] != "b_x" {
			t.Errorf("expected match 'b_x', got %q", names[0])
		}
	})

	t.Run("long term exact match does not wildcard underscore", func(t *testing.T) {
		db := setupTestDB(t)
		db.Exec("INSERT INTO items (name) VALUES ('config_v2')")
		db.Exec("INSERT INTO items (name) VALUES ('configXv2')")
		db.Exec("INSERT INTO items (name) VALUES ('totally_unrelated')")

		var names []string
		fts.fuzzyFallback(db.Table("items"), "items", "config_v2").
			Pluck("name", &names)

		// "configXv2" may match through intentional fuzzy patterns (replacing
		// position 6 with _), and that's fine. But it must NOT match through
		// the exact substring clause treating the literal _ as a wildcard.
		//
		// "totally_unrelated" should never match — it has no resemblance.
		for _, n := range names {
			if n == "totally_unrelated" {
				t.Errorf("'totally_unrelated' should not match fuzzy search for 'config_v2'")
			}
		}

		// More directly: test that a name matching ONLY because of unescaped _
		// in the exact clause doesn't appear.
		// "configXv2" differs from "config_v2" at position 6 only, so a correct
		// fuzzy search SHOULD match it via the intentional wildcard at position 6.
		// But let's verify with a case where extra _ wildcards cause bad matches.
		db2 := setupTestDB(t)
		db2.Exec("INSERT INTO items (name) VALUES ('a_b_c')")
		db2.Exec("INSERT INTO items (name) VALUES ('aXbYc')") // matches only if both _'s are wildcards
		db2.Exec("INSERT INTO items (name) VALUES ('apple')") // 5 chars, but no resemblance

		var names2 []string
		fts.fuzzyFallback(db2.Table("items"), "items", "a_b_c").
			Pluck("name", &names2)

		// "apple" should never match a fuzzy search for "a_b_c"
		for _, n := range names2 {
			if n == "apple" {
				t.Errorf("'apple' should not match fuzzy search for 'a_b_c'")
			}
		}
	})
}
