package database_scopes

import (
	"gorm.io/gorm"
	"mahresources/models/query_models"
	"mahresources/models/types"
	"regexp"
)

func ResourceQuery(query *query_models.ResourceSearchQuery, ignoreSort bool, originalDb *gorm.DB) func(db *gorm.DB) *gorm.DB {
	sortColumnMatcher := regexp.MustCompile("^(meta->>?'[a-z_]+'|[a-z_]+)(\\s(desc|asc))?$")

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
			subQuery := originalDb.
				Table("resource_tags rt").
				Where("rt.tag_id IN ?", query.Tags).
				Group("rt.resource_id").
				Having("count(*) = ?", len(query.Tags)).
				Select("rt.resource_id")

			dbQuery = dbQuery.Where(
				"resources.id IN (?)",
				subQuery,
			)
		}

		if query.Groups != nil && len(query.Groups) > 0 {
			subQuery := originalDb.
				Table("groups_related_resources grr").
				Select("grr.resource_id").
				Where("grr.group_id IN (?)", query.Groups).
				Group("grr.resource_id").
				Having("count(*) = ?", len(query.Groups))

			dbQuery = dbQuery.Where(`resources.id IN (?)`, subQuery)
		}

		if query.Notes != nil && len(query.Notes) > 0 {
			subQuery := originalDb.
				Table("resource_notes rn").
				Where("rn.note_id IN ?", query.Notes).
				Group("rn.resource_id").
				Select("rn.resource_id")

			dbQuery = dbQuery.Where("resources.id IN (?)", subQuery)
		}

		if query.ShowWithSimilar {
			subQuery := originalDb.
				Table("resources hasSimilarResource").
				Joins("JOIN image_hashes ih ON hasSimilarResource.id = ih.resource_id").
				Joins("JOIN image_hashes i ON ih.d_hash = i.d_hash").
				Having("count(*) > 1").
				Group("hasSimilarResource.id").
				Select("hasSimilarResource.id")
			dbQuery = dbQuery.Where("resources.id IN (?)", subQuery)
		}

		if query.Name != "" {
			dbQuery = dbQuery.Where("resources.name "+likeOperator+" ?", "%"+query.Name+"%")
		}

		if query.Description != "" {
			dbQuery = dbQuery.Where("resources.description "+likeOperator+" ?", "%"+query.Description+"%")
		}

		if query.ContentType != "" {
			dbQuery = dbQuery.Where("resources.content_type "+likeOperator+" ?", "%"+query.ContentType+"%")
		}

		if query.OriginalName != "" {
			dbQuery = dbQuery.Where("resources.original_name "+likeOperator+" ?", "%"+query.OriginalName+"%")
		}

		if query.OriginalLocation != "" {
			dbQuery = dbQuery.Where("resources.original_location "+likeOperator+" ?", "%"+query.OriginalLocation+"%")
		}

		if query.OwnerId != 0 {
			dbQuery = dbQuery.Where("resources.owner_id = ?", query.OwnerId)
		}

		if query.Hash != "" {
			dbQuery = dbQuery.Where("resources.hash = ?", query.Hash)
		}

		if query.CreatedBefore != "" {
			dbQuery = dbQuery.Where("resources.created_at <= ?", query.CreatedBefore)
		}

		if query.CreatedAfter != "" {
			dbQuery = dbQuery.Where("resources.created_at >= ?", query.CreatedAfter)
		}

		if query.ShowWithoutOwner {
			dbQuery = dbQuery.Where("resources.owner_id IS NULL")
		}

		if len(query.MetaQuery) > 0 {
			for _, v := range query.MetaQuery {
				if v.Key == "" {
					continue
				}

				dbQuery = dbQuery.Where(types.JSONQuery("resources.meta").Operation(getOperationType(v.Operation), v.Value, v.Key))
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
