package application_context

import (
	"fmt"
	"mahresources/constants"
	"time"

	"gorm.io/gorm"
)

// ActivityEntry represents a single item in the dashboard activity feed.
type ActivityEntry struct {
	EntityType  string    `gorm:"column:entity_type"`
	DisplayType string    `gorm:"-"`
	EntityID    uint      `gorm:"column:entity_id"`
	Name        string    `gorm:"column:name"`
	Action      string    `gorm:"column:action"`
	Timestamp   time.Time `gorm:"column:timestamp"`
}

// GetRecentActivity returns a mixed timeline of recently created and updated entities.
func (ctx *MahresourcesContext) GetRecentActivity(limit int) ([]ActivityEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	// This is a raw-SQL UNION, so the GORM scope callback does not apply. Apply
	// the principal's group-subtree confinement explicitly: a group-limited user
	// must not see names/IDs of recently changed resources, notes, or groups
	// outside its subtree. Tags are global labels and stay visible (consistent
	// with search and meta-keys). Fail-closed: an unresolved scope returns empty.
	scopeIDs, scoped, deny := ctx.subtreeScopeIDs()
	if deny {
		return []ActivityEntry{}, nil
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

	// Per-table scope fragments. Empty for unscoped principals (no filtering).
	// %[2]s/%[4]s/%[6]s are the "created" WHERE clauses; %[3]s/%[5]s/%[7]s are the
	// extra AND clauses appended to the "updated" sub-selects.
	resWhere, resAnd := "", ""
	noteWhere, noteAnd := "", ""
	grpWhere, grpAnd := "", ""
	if scoped {
		resWhere, resAnd = "WHERE owner_id IN ?", "AND owner_id IN ?"
		noteWhere, noteAnd = "WHERE owner_id IN ?", "AND owner_id IN ?"
		grpWhere, grpAnd = "WHERE id IN ?", "AND id IN ?"
	}

	query := fmt.Sprintf(`
		SELECT * FROM (
			SELECT * FROM (SELECT 'resource' AS entity_type, id AS entity_id, name, 'created' AS action, created_at AS timestamp FROM resources %[2]s ORDER BY created_at DESC LIMIT ?)
			UNION ALL
			SELECT * FROM (SELECT 'resource' AS entity_type, id AS entity_id, name, 'updated' AS action, updated_at AS timestamp FROM resources WHERE %[1]s %[3]s ORDER BY updated_at DESC LIMIT ?)
			UNION ALL
			SELECT * FROM (SELECT 'note' AS entity_type, id AS entity_id, name, 'created' AS action, created_at AS timestamp FROM notes %[4]s ORDER BY created_at DESC LIMIT ?)
			UNION ALL
			SELECT * FROM (SELECT 'note' AS entity_type, id AS entity_id, name, 'updated' AS action, updated_at AS timestamp FROM notes WHERE %[1]s %[5]s ORDER BY updated_at DESC LIMIT ?)
			UNION ALL
			SELECT * FROM (SELECT 'group' AS entity_type, id AS entity_id, name, 'created' AS action, created_at AS timestamp FROM "groups" %[6]s ORDER BY created_at DESC LIMIT ?)
			UNION ALL
			SELECT * FROM (SELECT 'group' AS entity_type, id AS entity_id, name, 'updated' AS action, updated_at AS timestamp FROM "groups" WHERE %[1]s %[7]s ORDER BY updated_at DESC LIMIT ?)
			UNION ALL
			SELECT * FROM (SELECT 'tag' AS entity_type, id AS entity_id, name, 'created' AS action, created_at AS timestamp FROM tags ORDER BY created_at DESC LIMIT ?)
			UNION ALL
			SELECT * FROM (SELECT 'tag' AS entity_type, id AS entity_id, name, 'updated' AS action, updated_at AS timestamp FROM tags WHERE %[1]s ORDER BY updated_at DESC LIMIT ?)
		) combined
		ORDER BY timestamp DESC
		LIMIT ?
	`, updatedFilter, resWhere, resAnd, noteWhere, noteAnd, grpWhere, grpAnd)

	// Build params in placeholder order. The six owner/group sub-selects each take
	// the scope-id slice (when scoped) before their LIMIT; the two tag sub-selects
	// and the final LIMIT take only the limit.
	params := make([]any, 0, 15)
	for i := 0; i < 6; i++ {
		if scoped {
			params = append(params, scopeIDs)
		}
		params = append(params, limit)
	}
	params = append(params, limit, limit, limit) // tag created, tag updated, final

	if err := ctx.db.Raw(query, params...).Scan(&entries).Error; err != nil {
		return nil, err
	}
	if err := ctx.populateActivityDisplayTypes(entries); err != nil {
		return nil, err
	}
	return entries, nil
}

type activityDisplayTypeRow struct {
	ID          uint   `gorm:"column:id"`
	DisplayType string `gorm:"column:display_type"`
}

// populateActivityDisplayTypes resolves taxonomy labels in at most one query
// per storage kind, regardless of feed length. EntityType remains unchanged so
// links and colour classes retain their stable routing key.
func (ctx *MahresourcesContext) populateActivityDisplayTypes(entries []ActivityEntry) error {
	ids := map[string][]uint{"resource": {}, "note": {}, "group": {}}
	for i := range entries {
		switch entries[i].EntityType {
		case "resource", "note", "group":
			ids[entries[i].EntityType] = append(ids[entries[i].EntityType], entries[i].EntityID)
		case "tag":
			entries[i].DisplayType = "Tag"
		default:
			entries[i].DisplayType = entries[i].EntityType
		}
	}

	labels := make(map[string]map[uint]string, 3)
	queries := map[string]*gorm.DB{
		"resource": ctx.db.Table("resources r").Select("r.id, COALESCE(rc.name, 'Resource') AS display_type").Joins("LEFT JOIN resource_categories rc ON rc.id = r.resource_category_id"),
		"note":     ctx.db.Table("notes n").Select("n.id, COALESCE(nt.name, 'Note') AS display_type").Joins("LEFT JOIN note_types nt ON nt.id = n.note_type_id"),
		"group":    ctx.db.Table("groups g").Select("g.id, COALESCE(c.name, 'Group') AS display_type").Joins("LEFT JOIN categories c ON c.id = g.category_id"),
	}
	for _, entityType := range []string{"resource", "note", "group"} {
		uniqueIDs := uniqueNonZeroIDs(ids[entityType])
		if len(uniqueIDs) == 0 {
			continue
		}
		var rows []activityDisplayTypeRow
		if err := queries[entityType].Where(entityTypeTableIDColumn(entityType)+" IN ?", uniqueIDs).Scan(&rows).Error; err != nil {
			return err
		}
		labels[entityType] = make(map[uint]string, len(rows))
		for _, row := range rows {
			labels[entityType][row.ID] = row.DisplayType
		}
	}
	for i := range entries {
		if entries[i].DisplayType != "" {
			continue
		}
		entries[i].DisplayType = labels[entries[i].EntityType][entries[i].EntityID]
		if entries[i].DisplayType == "" {
			entries[i].DisplayType = map[string]string{"resource": "Resource", "note": "Note", "group": "Group"}[entries[i].EntityType]
		}
	}
	return nil
}

func entityTypeTableIDColumn(entityType string) string {
	switch entityType {
	case "resource":
		return "r.id"
	case "note":
		return "n.id"
	default:
		return "g.id"
	}
}
