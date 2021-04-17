package database_scopes

import (
	"gorm.io/gorm"
	"mahresources/http_query"
)

func TagQuery(query *http_query.TagQuery) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		dbQuery := db

		if query.Name != "" {
			dbQuery = dbQuery.Where("name LIKE ?", "%"+query.Name+"%")
		}

		return dbQuery
	}
}
