package database_scopes

import (
	"gorm.io/gorm"
	"mahresources/models/query_models"
)

func SeriesQuery(query *query_models.SeriesQuery, ignoreSort bool) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		likeOperator := GetLikeOperator(db)
		dbQuery := db

		if !ignoreSort {
			dbQuery = ApplySortColumns(dbQuery, query.SortBy, "", "created_at desc")
		}

		if query.Name != "" {
			p, esc := LikePattern(query.Name)
			dbQuery = dbQuery.Where("name "+likeOperator+" ?"+esc, p)
		}

		if query.Slug != "" {
			dbQuery = dbQuery.Where("slug = ?", query.Slug)
		}

		dbQuery = ApplyDateRange(dbQuery, "", query.CreatedBefore, query.CreatedAfter)

		return dbQuery
	}
}
