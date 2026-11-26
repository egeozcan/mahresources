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
		// The incoming relations of one group (the [relations direction="in"]
		// shortcode, the reverse-relations panel). unique_rel leads with
		// from_group_id, so it serves only the outgoing side.
		"CREATE INDEX IF NOT EXISTS idx__group_relations__to_group_id ON group_relations(to_group_id)",
	}
	if db.Dialector.Name() == "postgres" {
		queries = append(queries, "CREATE INDEX IF NOT EXISTS idx__groups_related_resources__resource_id___hash ON groups_related_resources USING HASH (resource_id)")
		// The Reduction compute pages a content hash's holders by id (hash = ? AND id > ?
		// ORDER BY id LIMIT n). Postgres' hash index holds the hash alone, so without
		// the pair it walks the primary key filtering on the hash. SQLite's hash index
		// carries the rowid, which is the id, so it needs nothing more.
		queries = append(queries, "CREATE INDEX IF NOT EXISTS idx__resources__hash_id ON resources(hash, id)")
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
