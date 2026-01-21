package database_scopes

import (
	"fmt"
	"regexp"
	"strings"

	"gorm.io/gorm"
	"mahresources/models/query_models"
)

// validEntityName validates entity names used in most_used_ sort columns.
// Only allows lowercase letters to prevent SQL injection.
var validEntityName = regexp.MustCompile(`^[a-z]+$`)

func TagQuery(query *query_models.TagQuery, ignoreSort bool) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		likeOperator := GetLikeOperator(db)
		dbQuery := db

		if !ignoreSort {
			mostUsedPrefix := "most_used_"
			for _, sort := range query.SortBy {
				sort = strings.TrimSpace(sort)

				// Handle most_used_ prefix for sorting by tag usage count
				if strings.HasPrefix(sort, mostUsedPrefix) {
					remainder := strings.TrimPrefix(sort, mostUsedPrefix)
					// Extract entity name (first word), ignoring any direction suffix
					parts := strings.Fields(remainder)
					if len(parts) == 0 {
						continue
					}
					entityName := parts[0]
					// Validate entity name to prevent SQL injection
					if !validEntityName.MatchString(entityName) {
						continue
					}
					tableName := fmt.Sprintf("%v_tags", entityName)
					dbQuery = dbQuery.Order(fmt.Sprintf("(SELECT count(*) FROM %v jt WHERE jt.tag_id = tags.id) desc", tableName))
					continue
				}

				// Handle standard sort columns
				if ValidateSortColumn(sort) {
					dbQuery = dbQuery.Order(sort)
				}
			}
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
