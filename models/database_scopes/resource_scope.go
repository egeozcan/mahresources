package database_scopes

import (
	"gorm.io/gorm"
	"mahresources/models/query_models"
	"mahresources/models/types"
	"regexp"
)

func ResourceQuery(query *query_models.ResourceSearchQuery, ignoreSort bool) func(db *gorm.DB) *gorm.DB {
	sortColumnMatcher := regexp.MustCompile("^[a-z_]+(\\s(desc|asc))?$")

	return func(db *gorm.DB) *gorm.DB {
		likeOperator := "LIKE"

		if db.Config.Dialector.Name() == "postgres" {
			likeOperator = "ILIKE"
		}

		dbQuery := db

		if !ignoreSort && query.SortBy != "" && sortColumnMatcher.MatchString(query.SortBy) {
			dbQuery = dbQuery.Order(query.SortBy)
		} else if !ignoreSort {
			dbQuery = dbQuery.Order("created_at desc")
		}

		if query.Tags != nil && len(query.Tags) > 0 {
			dbQuery = dbQuery.Where(
				"(SELECT Count(*) FROM resource_tags rt WHERE rt.tag_id IN ? AND rt.resource_id = resources.id) = ?",
				query.Tags,
				len(query.Tags),
			)
		}

		if query.Groups != nil && len(query.Groups) > 0 {
			dbQuery = dbQuery.Where(
				`
					(
						SELECT 
							Count(*) 
						FROM 
							groups_related_resources prr 
						WHERE 
							prr.group_id IN ? 
							AND prr.resource_id = resources.id
							AND resources.owner_id <> prr.group_id
					) + (
						SELECT
							CASE
								WHEN 
									resources.owner_id IN ?
								THEN 1
								ELSE 0
							END
					) = ?`,
				query.Groups,
				query.Groups,
				len(query.Groups),
			)
		}

		if query.Notes != nil && len(query.Notes) > 0 {
			dbQuery = dbQuery.Where(
				"(SELECT Count(*) FROM resource_notes ra WHERE ra.note_id IN ? AND ra.resource_id = resources.id) = ?",
				query.Notes,
				len(query.Notes),
			)
		}

		if query.Name != "" {
			dbQuery = dbQuery.Where("name "+likeOperator+" ?", "%"+query.Name+"%")
		}

		if query.Description != "" {
			dbQuery = dbQuery.Where("description "+likeOperator+" ?", "%"+query.Description+"%")
		}

		if query.OriginalName != "" {
			dbQuery = dbQuery.Where("original_name "+likeOperator+" ?", "%"+query.OriginalName+"%")
		}

		if query.OriginalLocation != "" {
			dbQuery = dbQuery.Where("original_location "+likeOperator+" ?", "%"+query.OriginalLocation+"%")
		}

		if query.OwnerId != 0 {
			dbQuery = dbQuery.Where("owner_id = ?", query.OwnerId)
		}

		if query.Hash != "" {
			dbQuery = dbQuery.Where("hash = ?", query.Hash)
		}

		if query.CreatedBefore != "" {
			dbQuery = dbQuery.Where("created_at <= ?", query.CreatedBefore)
		}

		if query.CreatedAfter != "" {
			dbQuery = dbQuery.Where("created_at >= ?", query.CreatedAfter)
		}

		if len(query.MetaQuery) > 0 {
			for _, v := range query.MetaQuery {
				if v.Key == "" {
					continue
				}

				dbQuery = dbQuery.Where(types.JSONQuery("meta").Operation(getOperationType(v.Operation), v.Value, v.Key))
			}
		}

		return dbQuery
	}
}

func getOperationType(operationStr string) types.JsonOperation {
	switch operationStr {
	case "EQ":
		return types.OperatorEquals
	case "LI":
		return types.OperatorLike
	case "NE":
		return types.OperatorNotEquals
	case "NL":
		return types.OperatorNotLike
	case "GT":
		return types.OperatorGreaterThan
	case "GE":
		return types.OperatorGreaterThanOrEquals
	case "LT":
		return types.OperatorLessThan
	case "LE":
		return types.OperatorLessThanOrEquals
	}
	return types.OperatorLike
}
