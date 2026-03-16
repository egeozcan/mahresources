package database_scopes

import (
	"testing"

	"mahresources/models/query_models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestTagQueryMostUsedInvalidEntityIgnored(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	db.Exec("CREATE TABLE tags (id INTEGER PRIMARY KEY, name TEXT, description TEXT, created_at DATETIME, updated_at DATETIME)")
	db.Exec("INSERT INTO tags (id, name, created_at, updated_at) VALUES (1, 'alpha', datetime('now'), datetime('now'))")

	// "most_used_foo" references non-existent table "foo_tags" — should be
	// silently ignored (like other invalid sort columns), not cause a SQL error.
	query := &query_models.TagQuery{SortBy: []string{"most_used_foo"}}
	scope := TagQuery(query, false)

	type tagResult struct {
		ID   uint
		Name string
	}
	var results []tagResult
	err = scope(db).Table("tags").Find(&results).Error
	if err != nil {
		t.Errorf("most_used_ with invalid entity name should be ignored, not cause error: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("expected 1 tag, got %d", len(results))
	}
}

func TestTagQueryMostUsedRespectsDirection(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}

	// Create minimal schema
	db.Exec("CREATE TABLE tags (id INTEGER PRIMARY KEY, name TEXT, description TEXT, created_at DATETIME, updated_at DATETIME)")
	db.Exec("CREATE TABLE resource_tags (tag_id INTEGER, resource_id INTEGER)")

	// Insert tags and usage data: tag1 used 3 times, tag2 used 1 time
	db.Exec("INSERT INTO tags (id, name, created_at, updated_at) VALUES (1, 'popular', datetime('now'), datetime('now'))")
	db.Exec("INSERT INTO tags (id, name, created_at, updated_at) VALUES (2, 'rare', datetime('now'), datetime('now'))")
	db.Exec("INSERT INTO resource_tags (tag_id, resource_id) VALUES (1, 10), (1, 20), (1, 30)")
	db.Exec("INSERT INTO resource_tags (tag_id, resource_id) VALUES (2, 10)")

	// Sort by most_used_resource asc — should return least-used first (rare before popular)
	query := &query_models.TagQuery{SortBy: []string{"most_used_resource asc"}}
	scope := TagQuery(query, false)

	type tagResult struct {
		ID   uint
		Name string
	}
	var results []tagResult
	if err := scope(db).Table("tags").Find(&results).Error; err != nil {
		t.Fatal(err)
	}

	if len(results) < 2 {
		t.Fatalf("expected 2 tags, got %d", len(results))
	}

	// With "asc" direction, the least-used tag ("rare", 1 usage) should come first
	if results[0].Name != "rare" {
		t.Errorf("most_used_resource asc: expected first result 'rare' (1 usage), got %q — direction was ignored", results[0].Name)
	}
}
