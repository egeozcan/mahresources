package models

import "gorm.io/gorm"

// EnsureSupplementalIndexes creates indexes on generated many-to-many tables
// and cross-column lookup shapes that GORM model tags cannot express reliably.
// It is idempotent and shared by production startup and integration tests.
func EnsureSupplementalIndexes(db *gorm.DB) error {
	queries := []string{
		"CREATE INDEX IF NOT EXISTS idx__resource_notes__note_id ON resource_notes(note_id)",
		"CREATE INDEX IF NOT EXISTS idx__resource_notes__resource_id ON resource_notes(resource_id)",
		"CREATE INDEX IF NOT EXISTS idx__groups_related_resources__resource_id ON groups_related_resources(resource_id)",
		"CREATE INDEX IF NOT EXISTS idx__groups_related_resources__group_id ON groups_related_resources(group_id)",
		"CREATE INDEX IF NOT EXISTS idx__groups_related_notes__note_id ON groups_related_notes(note_id)",
		"CREATE INDEX IF NOT EXISTS idx__log_entries__entity_type_entity_id ON log_entries(entity_type, entity_id)",
		"CREATE INDEX IF NOT EXISTS idx__resource_tags__tag_id ON resource_tags(tag_id)",
		"CREATE INDEX IF NOT EXISTS idx__note_tags__tag_id ON note_tags(tag_id)",
		"CREATE INDEX IF NOT EXISTS idx__group_tags__tag_id ON group_tags(tag_id)",
	}
	if db.Dialector.Name() == "postgres" {
		queries = append(queries, "CREATE INDEX IF NOT EXISTS idx__groups_related_resources__resource_id___hash ON groups_related_resources USING HASH (resource_id)")
	} else {
		queries = append(queries, "CREATE INDEX IF NOT EXISTS idx__groups_related_resources__resource_id___hash ON groups_related_resources(resource_id)")
	}
	for _, query := range queries {
		if err := db.Exec(query).Error; err != nil {
			return err
		}
	}
	return nil
}
