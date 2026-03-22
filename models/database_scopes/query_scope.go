package database_scopes

import (
	"gorm.io/gorm"
	"mahresources/models/query_models"
)

func QueryQuery(query *query_models.QueryQuery, ignoreSort bool) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		dbQuery := db
		likeOperator := GetLikeOperator(db)

		if query.Name != "" {
			p, esc := LikePattern(query.Name)
			dbQuery = dbQuery.Where("name "+likeOperator+" ?"+esc, p)
		}

		if query.Text != "" {
			p, esc := LikePattern(query.Text)
			dbQuery = dbQuery.Where("text "+likeOperator+" ?"+esc, p)
		}

		dbQuery = ApplyDateRange(dbQuery, "", query.CreatedBefore, query.CreatedAfter)
		dbQuery = ApplyUpdatedDateRange(dbQuery, "", query.UpdatedBefore, query.UpdatedAfter)

		if !ignoreSort {
			dbQuery = ApplySortColumns(dbQuery, query.SortBy, "", "created_at desc")
		}

		return dbQuery
	}
}
