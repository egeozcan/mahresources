package database_scopes

import (
	"gorm.io/gorm"
	"mahresources/http_query"
)

func PersonQuery(query *http_query.PersonQuery) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		dbQuery := db

		if query.Tags != nil && len(query.Tags) > 0 {
			dbQuery = dbQuery.Where(
				"(SELECT Count(*) FROM person_tags pt WHERE pt.tag_id IN ? AND pt.person_id = people.id) = ?",
				query.Tags,
				len(query.Tags),
			)
		}

		if query.Name != "" {
			dbQuery = dbQuery.Where("name LIKE ?", "%"+query.Name+"%")
		}

		if query.Surname != "" {
			dbQuery = dbQuery.Where("surname LIKE ?", "%"+query.Surname+"%")
		}

		if query.Description != "" {
			dbQuery = dbQuery.Where("description LIKE ?", "%"+query.Description+"%")
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
