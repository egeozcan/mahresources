package database_scopes

import (
	"fmt"
	"gorm.io/gorm"
	"mahresources/models/query_models"
	"regexp"
	"strings"
)

func TagQuery(query *query_models.TagQuery, ignoreSort bool) func(db *gorm.DB) *gorm.DB {
	sortColumnMatcher := regexp.MustCompile("^(meta->>?'[a-z_]+'|[a-z_]+)(\\s(desc|asc))?$")

	return func(db *gorm.DB) *gorm.DB {
		likeOperator := "LIKE"

		if db.Config.Dialector.Name() == "postgres" {
			likeOperator = "ILIKE"
		}

		dbQuery := db

		if !ignoreSort && query.SortBy != "" && sortColumnMatcher.MatchString(query.SortBy) {
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

		if query.CreatedBefore != "" {
			dbQuery = dbQuery.Where("created_at <= ?", query.CreatedBefore)
		}

		if query.CreatedAfter != "" {
			dbQuery = dbQuery.Where("created_at >= ?", query.CreatedAfter)
		}

		return dbQuery
	}
}
