package models

import (
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEnsureSupplementalIndexesAddsReverseJunctionIndexes(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:supplemental_indexes?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	for _, statement := range []string{
		"CREATE TABLE resource_notes (resource_id integer, note_id integer)",
		"CREATE TABLE groups_related_resources (group_id integer, resource_id integer)",
		"CREATE TABLE groups_related_notes (group_id integer, note_id integer)",
		"CREATE TABLE log_entries (entity_type text, entity_id integer)",
		"CREATE TABLE resource_tags (resource_id integer, tag_id integer)",
		"CREATE TABLE note_tags (note_id integer, tag_id integer)",
		"CREATE TABLE group_tags (group_id integer, tag_id integer)",
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("create test table: %v", err)
		}
	}
	if err := EnsureSupplementalIndexes(db); err != nil {
		t.Fatalf("create supplemental indexes: %v", err)
	}

	rows, err := db.Raw("EXPLAIN QUERY PLAN SELECT group_id FROM groups_related_notes WHERE note_id = ?", 7).Rows()
	if err != nil {
		t.Fatalf("explain reverse note lookup: %v", err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatalf("scan plan: %v", err)
		}
		plan.WriteString(detail)
	}
	if !strings.Contains(plan.String(), "idx__groups_related_notes__note_id") {
		t.Fatalf("expected reverse junction index in query plan, got %q", plan.String())
	}
}
