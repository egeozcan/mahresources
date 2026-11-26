package models

import (
	"slices"
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
		"CREATE TABLE group_relations (from_group_id integer, to_group_id integer, relation_type_id integer)",
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

func TestEnsureSupplementalIndexesAddsIncomingRelationIndex(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:supplemental_relation_index?mode=memory&cache=shared"), &gorm.Config{})
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
		"CREATE TABLE group_relations (from_group_id integer, to_group_id integer, relation_type_id integer)",
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("create test table: %v", err)
		}
	}
	if err := EnsureSupplementalIndexes(db); err != nil {
		t.Fatalf("create supplemental indexes: %v", err)
	}
	var names []string
	if err := db.Raw("SELECT name FROM pragma_index_list('group_relations')").Scan(&names).Error; err != nil {
		t.Fatalf("list indexes: %v", err)
	}
	if !slices.Contains(names, "idx__group_relations__to_group_id") {
		t.Fatalf("group_relations indexes = %v, want idx__group_relations__to_group_id", names)
	}
}
