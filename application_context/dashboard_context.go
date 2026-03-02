package application_context

import (
	"time"
)

// ActivityEntry represents a single item in the dashboard activity feed.
type ActivityEntry struct {
	EntityType string
	EntityID   uint
	Name       string
	Action     string
	Timestamp  time.Time
}

// GetRecentActivity returns a mixed timeline of recently created and updated entities.
func (ctx *MahresourcesContext) GetRecentActivity(limit int) ([]ActivityEntry, error) {
	var entries []ActivityEntry

	query := `
		SELECT 'resource' AS entity_type, id AS entity_id, name, 'created' AS action, created_at AS timestamp FROM resources
		UNION ALL
		SELECT 'resource', id, name, 'updated', updated_at FROM resources WHERE updated_at != created_at
		UNION ALL
		SELECT 'note' AS entity_type, id, name, 'created', created_at FROM notes
		UNION ALL
		SELECT 'note', id, name, 'updated', updated_at FROM notes WHERE updated_at != created_at
		UNION ALL
		SELECT 'group' AS entity_type, id, name, 'created', created_at FROM "groups"
		UNION ALL
		SELECT 'group', id, name, 'updated', updated_at FROM "groups" WHERE updated_at != created_at
		UNION ALL
		SELECT 'tag' AS entity_type, id, name, 'created', created_at FROM tags
		UNION ALL
		SELECT 'tag', id, name, 'updated', updated_at FROM tags WHERE updated_at != created_at
		ORDER BY timestamp DESC
		LIMIT ?
	`

	err := ctx.db.Raw(query, limit).Scan(&entries).Error
	return entries, err
}
