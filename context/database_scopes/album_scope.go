package database_scopes

import (
	"gorm.io/gorm"
	"mahresources/http_utils/http_query"
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
				`EXISTS (
							SELECT 1 FROM people p 
								JOIN people_related_resources prr 
									ON p.id = prr.person_id
								JOIN resources r
									ON r.id = prr.resource_id
								JOIN resource_albums ra
									ON ra.resource_id = r.id AND ra.album_id = albums.id
							WHERE p.id IN ?)
					  `,
				query.People,
			)
		}

		if query.Name != "" {
			dbQuery = dbQuery.Where("name LIKE ?", "%"+query.Name+"%")
		}

		if query.HasThumbnail {
			dbQuery = dbQuery.Where("name LIKE ?", "%"+query.Name+"%")
		}

		if query.OwnerId != 0 {
			dbQuery = dbQuery.Where("owner = ?", query.OwnerId)
		}

		return dbQuery
	}
}
