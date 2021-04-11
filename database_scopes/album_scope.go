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

		if query.People != nil && len(query.People) > 0 {
			dbQuery = dbQuery.Where(
				"(SELECT Count(*) FROM people_related_albums pra WHERE pra.person_id IN ? AND pra.album_id = albums.id) = ?",
				query.People,
				len(query.People),
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
