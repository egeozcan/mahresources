package database_scopes

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
	"mahresources/models/query_models"
)

func TagQuery(query *query_models.TagQuery, ignoreSort bool) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		likeOperator := GetLikeOperator(db)
		dbQuery := db

		if !ignoreSort && ValidateSortColumn(query.SortBy) {
			prefix := "most_used_"
			if strings.HasPrefix(query.SortBy, prefix) {
				tableName := fmt.Sprintf("%v_tags", strings.TrimPrefix(query.SortBy, prefix))
				dbQuery.Order(fmt.Sprintf("(SELECT count(*) FROM %v jt WHERE jt.tag_id = tags.id) desc", tableName)).Order("created_at desc")
			} else {
				dbQuery = dbQuery.Order(query.SortBy).Order("created_at desc")
			}
		} else if !ignoreSort {
			dbQuery = dbQuery.Order("created_at desc")
		}

		if query.Name != "" {
			dbQuery = dbQuery.Where("name "+likeOperator+" ?", "%"+query.Name+"%")
		}

		if query.Description != "" {
			dbQuery = dbQuery.Where("description "+likeOperator+" ?", "%"+query.Description+"%")
		}

		dbQuery = ApplyDateRange(dbQuery, "", query.CreatedBefore, query.CreatedAfter)

		return dbQuery
	}
}
