package database_scopes

import (
	"gorm.io/gorm"
	"mahresources/models/query_models"
)

func ResourceCategoryQuery(query *query_models.ResourceCategoryQuery) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		dbQuery := db
		likeOperator := GetLikeOperator(db)

		if query.Name != "" {
			dbQuery = dbQuery.Where("name "+likeOperator+" ?", "%"+query.Name+"%")
		}

		if query.Description != "" {
			dbQuery = dbQuery.Where("description "+likeOperator+" ?", "%"+query.Description+"%")
		}

		return dbQuery
	}
}
