package models

import (
	"strings"
	"testing"

	"mahresources/constants"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// [adjacent] seeks one owner's and one category's rows in created_at, id
// order. The resources index joins the resource list set (and its lock, since
// both build on one table); the notes and groups ones are their own set.
func TestSiblingNavigationIndexes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:sibling_nav_indexes?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if sqlDB, err := db.DB(); err == nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	for _, statement := range []string{
		"CREATE TABLE resources (id integer primary key, name text, owner_id integer, resource_category_id integer, created_at datetime)",
		"CREATE TABLE notes (id integer primary key, name text, owner_id integer, note_type_id integer, created_at datetime)",
		"CREATE TABLE groups (id integer primary key, name text, owner_id integer, category_id integer, created_at datetime)",
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("create test table: %v", err)
		}
	}

	want := map[string]string{
		"idx_resources_adjacent_created": "owner_id, resource_category_id, created_at, id",
		"idx_notes_adjacent_created":     "owner_id, note_type_id, created_at, id",
		"idx_groups_adjacent_created":    "owner_id, category_id, created_at, id",
	}
	check := func(pass string) {
		t.Helper()
		for name, columns := range want {
			var got []string
			if err := db.Raw("SELECT name FROM pragma_index_info(?) ORDER BY seqno", name).Scan(&got).Error; err != nil {
				t.Fatalf("inspect %s: %v", name, err)
			}
			if strings.Join(got, ", ") != columns {
				t.Errorf("%s: %s covers %v, want %s", pass, name, got, columns)
			}
		}
	}
	for _, pass := range []string{"first pass", "second pass"} {
		if err := EnsureResourceListIndexes(db); err != nil {
			t.Fatalf("%s: resource list indexes: %v", pass, err)
		}
		if err := EnsureSiblingNavIndexes(db); err != nil {
			t.Fatalf("%s: sibling navigation indexes: %v", pass, err)
		}
		check(pass)
	}

	// The FTS prefix build also builds CONCURRENTLY on notes and groups, so the
	// notes and groups set must take the same lock or the two builds deadlock.
	if siblingNavIndexLockKey != constants.ResourceTableIndexBuildLockKey {
		t.Fatal("the notes/groups sibling indexes must share the FTS prefix build's lock")
	}
}
