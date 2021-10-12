package database_scopes

import (
	"gorm.io/gorm"
	"mahresources/http_query"
)

func ResourceQuery(query *http_query.ResourceQuery) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		dbQuery := db

		if query.Tags != nil && len(query.Tags) > 0 {
			dbQuery = dbQuery.Where(
				"(SELECT Count(*) FROM resource_tags rt WHERE rt.tag_id IN ? AND rt.resource_id = resources.id) = ?",
				query.Tags,
				len(query.Tags),
			)
		}

		if query.Groups != nil && len(query.Groups) > 0 {
			dbQuery = dbQuery.Where(
				`(
					SELECT Count(*) 
					FROM groups_related_resources prr WHERE prr.group_id IN ? AND prr.resource_id = resources.id) = ?`,
				query.Groups,
				len(query.Groups),
			)
		}

		if query.Notes != nil && len(query.Notes) > 0 {
			dbQuery = dbQuery.Where(
				"(SELECT Count(*) FROM resource_notes ra WHERE ra.note_id IN ? AND ra.resource_id = resources.id) = ?",
				query.Notes,
				len(query.Notes),
			)
		}

		if query.Name != "" {
			dbQuery = dbQuery.Where("name LIKE ?", "%"+query.Name+"%")
		}

		if query.Description != "" {
			dbQuery = dbQuery.Where("description LIKE ?", "%"+query.Description+"%")
		}

		if query.HasThumbnail {
			dbQuery = dbQuery.Where("preview IS NOT NULL")
		}

		if query.OwnerId != 0 {
			dbQuery = dbQuery.Where("owner = ?", query.OwnerId)
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
