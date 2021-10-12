package database_scopes

import (
	"gorm.io/gorm"
	"mahresources/http_query"
)

func AlbumQuery(query *http_query.AlbumQuery) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		dbQuery := db

		if query.Tags != nil && len(query.Tags) > 0 {
			dbQuery = dbQuery.Where(
				"(SELECT Count(*) FROM album_tags at WHERE at.tag_id IN ? AND at.album_id = albums.id) = ?",
				query.Tags,
				len(query.Tags),
			)
		}

		if query.Groups != nil && len(query.Groups) > 0 {
			dbQuery = dbQuery.Where(
				"(SELECT Count(*) FROM groups_related_albums pra WHERE pra.group_id IN ? AND pra.album_id = albums.id) = ?",
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
