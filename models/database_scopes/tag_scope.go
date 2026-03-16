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

// validMostUsedEntities are the entity types that have _tags junction tables.
var validMostUsedEntities = map[string]bool{
	"resource": true,
	"note":     true,
	"group":    true,
}

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
					// Validate against known junction tables (also prevents SQL injection)
					if !validMostUsedEntities[entityName] {
						continue
					}
					direction := "desc"
					if len(parts) > 1 && parts[1] == "asc" {
						direction = "asc"
					}
					tableName := fmt.Sprintf("%v_tags", entityName)
					dbQuery = dbQuery.Order(fmt.Sprintf("(SELECT count(*) FROM %v jt WHERE jt.tag_id = tags.id) %s", tableName, direction))
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
			p, esc := LikePattern(query.Name)
			dbQuery = dbQuery.Where("name "+likeOperator+" ?"+esc, p)
		}

		if query.Description != "" {
			p, esc := LikePattern(query.Description)
			dbQuery = dbQuery.Where("description "+likeOperator+" ?"+esc, p)
		}

		dbQuery = ApplyDateRange(dbQuery, "", query.CreatedBefore, query.CreatedAfter)

		return dbQuery
	}
}
