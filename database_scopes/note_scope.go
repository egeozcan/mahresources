package database_scopes

import (
	"gorm.io/gorm"
	"mahresources/http_query"
)

func NoteQuery(query *http_query.NoteQuery) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		dbQuery := db

		if query.Tags != nil && len(query.Tags) > 0 {
			dbQuery = dbQuery.Where(
				"(SELECT Count(*) FROM note_tags at WHERE at.tag_id IN ? AND at.note_id = notes.id) = ?",
				query.Tags,
				len(query.Tags),
			)
		}

		if query.Groups != nil && len(query.Groups) > 0 {
			dbQuery = dbQuery.Where(
				`
					(
						SELECT 
							Count(*) 
						FROM 
							groups_related_notes grn 
						WHERE 
							grn.group_id IN ? 
							AND grn.note_id = notes.id
							AND notes.owner_id <> grn.group_id
					) + (
						SELECT
							CASE
								WHEN 
									notes.owner_id IN ?
								THEN 1
								ELSE 0
							END
					) = ?`,
				query.Groups,
				query.Groups,
				len(query.Groups),
			)
		}

		if query.Name != "" {
			dbQuery = dbQuery.Where("name LIKE ?", "%"+query.Name+"%")
		}

		if query.Description != "" {
			dbQuery = dbQuery.Where("description LIKE ?", "%"+query.Description+"%")
		}

		if query.OwnerId != 0 {
			dbQuery = dbQuery.Where("owner_id = ?", query.OwnerId)
		}

		if query.CreatedBefore != "" {
			dbQuery = dbQuery.Where("created_at <= ?", query.CreatedBefore)
		}

		if query.CreatedAfter != "" {
			dbQuery = dbQuery.Where("created_at >= ?", query.CreatedAfter)
		}

		return dbQuery
	}
}
