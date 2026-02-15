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
			for _, sort := range query.SortBy {
				if ValidateSortColumn(sort) {
					dbQuery = dbQuery.Order(sort)
				}
			}
			dbQuery = dbQuery.Order("created_at desc")
		}

		if query.Name != "" {
			dbQuery = dbQuery.Where("name "+likeOperator+" ?", "%"+query.Name+"%")
		}

		if query.Slug != "" {
			dbQuery = dbQuery.Where("slug = ?", query.Slug)
		}

		dbQuery = ApplyDateRange(dbQuery, "", query.CreatedBefore, query.CreatedAfter)

		return dbQuery
	}
}
