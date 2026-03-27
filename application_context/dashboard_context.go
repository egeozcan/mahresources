package application_context

import (
	"fmt"
	"mahresources/constants"
	"time"
)

// ActivityEntry represents a single item in the dashboard activity feed.
type ActivityEntry struct {
	EntityType string    `gorm:"column:entity_type"`
	EntityID   uint      `gorm:"column:entity_id"`
	Name       string    `gorm:"column:name"`
	Action     string    `gorm:"column:action"`
	Timestamp  time.Time `gorm:"column:timestamp"`
}

// GetRecentActivity returns a mixed timeline of recently created and updated entities.
func (ctx *MahresourcesContext) GetRecentActivity(limit int) ([]ActivityEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var entries []ActivityEntry

	// Use a 1-second threshold to avoid false "updated" entries from near-simultaneous
	// created_at/updated_at timestamps set during initial creation.
	var updatedFilter string
	if ctx.Config.DbType == constants.DbTypePosgres {
		updatedFilter = "updated_at > created_at + interval '1 second'"
	} else {
		updatedFilter = "datetime(updated_at) > datetime(created_at, '+1 second')"
	}

	query := fmt.Sprintf(`
		SELECT * FROM (
			SELECT * FROM (SELECT 'resource' AS entity_type, id AS entity_id, name, 'created' AS action, created_at AS timestamp FROM resources ORDER BY created_at DESC LIMIT ?)
			UNION ALL
			SELECT * FROM (SELECT 'resource' AS entity_type, id AS entity_id, name, 'updated' AS action, updated_at AS timestamp FROM resources WHERE %[1]s ORDER BY updated_at DESC LIMIT ?)
			UNION ALL
			SELECT * FROM (SELECT 'note' AS entity_type, id AS entity_id, name, 'created' AS action, created_at AS timestamp FROM notes ORDER BY created_at DESC LIMIT ?)
			UNION ALL
			SELECT * FROM (SELECT 'note' AS entity_type, id AS entity_id, name, 'updated' AS action, updated_at AS timestamp FROM notes WHERE %[1]s ORDER BY updated_at DESC LIMIT ?)
			UNION ALL
			SELECT * FROM (SELECT 'group' AS entity_type, id AS entity_id, name, 'created' AS action, created_at AS timestamp FROM "groups" ORDER BY created_at DESC LIMIT ?)
			UNION ALL
			SELECT * FROM (SELECT 'group' AS entity_type, id AS entity_id, name, 'updated' AS action, updated_at AS timestamp FROM "groups" WHERE %[1]s ORDER BY updated_at DESC LIMIT ?)
			UNION ALL
			SELECT * FROM (SELECT 'tag' AS entity_type, id AS entity_id, name, 'created' AS action, created_at AS timestamp FROM tags ORDER BY created_at DESC LIMIT ?)
			UNION ALL
			SELECT * FROM (SELECT 'tag' AS entity_type, id AS entity_id, name, 'updated' AS action, updated_at AS timestamp FROM tags WHERE %[1]s ORDER BY updated_at DESC LIMIT ?)
		) combined
		ORDER BY timestamp DESC
		LIMIT ?
	`, updatedFilter)

	// 8 sub-selects + 1 final LIMIT = 9 parameters, all the same value.
	params := make([]any, 9)
	for i := range params {
		params[i] = limit
	}
	err := ctx.db.Raw(query, params...).Scan(&entries).Error
	return entries, err
}
