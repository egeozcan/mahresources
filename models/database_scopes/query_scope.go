package database_scopes

import (
	"gorm.io/gorm"
	"mahresources/models/query_models"
)

func QueryQuery(query *query_models.QueryQuery) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		dbQuery := db
		likeOperator := GetLikeOperator(db)

		if query.Name != "" {
			dbQuery = dbQuery.Where("name "+likeOperator+" ?", "%"+query.Name+"%")
		}

		if query.Text != "" {
			dbQuery = dbQuery.Where("text "+likeOperator+" ?", "%"+query.Text+"%")
		}

		return dbQuery
	}
}
