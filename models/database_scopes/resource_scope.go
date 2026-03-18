package database_scopes

import (
	"gorm.io/gorm"
	"mahresources/models/query_models"
	"mahresources/models/types"
)

func ResourceQuery(query *query_models.ResourceSearchQuery, ignoreSort bool, originalDb *gorm.DB) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		likeOperator := GetLikeOperator(db)
		dbQuery := db

		if !ignoreSort {
			dbQuery = ApplySortColumns(dbQuery, query.SortBy, "", "created_at desc")
		}

		if len(query.Ids) > 0 {
			dbQuery = dbQuery.Where("resources.id IN (?)", query.Ids)
		}

		if len(query.Tags) > 0 {
			tags := deduplicateUints(query.Tags)
			subQuery := originalDb.
				Table("resource_tags rt").
				Where("rt.tag_id IN ?", tags).
				Group("rt.resource_id").
				Having("count(*) = ?", len(tags)).
				Select("rt.resource_id")

			dbQuery = dbQuery.Where(
				"resources.id IN (?)",
				subQuery,
			)
		}

		if len(query.Groups) > 0 {
			groups := deduplicateUints(query.Groups)
			dbQuery = dbQuery.Where(`
				resources.id IN (
					WITH cte AS (
					  SELECT "grr".resource_id res_id, "grr".group_id src_group
					  FROM groups_related_resources grr
					  WHERE grr.group_id IN ?
					  UNION ALL
					  SELECT id AS res_id, owner_id AS src_group FROM resources WHERE owner_id IN ?
					)
					SELECT res_id FROM cte GROUP BY res_id HAVING count(DISTINCT src_group) = ?
				)`,
				groups,
				groups,
				len(groups),
			)
		}

		if len(query.Notes) > 0 {
			notes := deduplicateUints(query.Notes)
			subQuery := originalDb.
				Table("resource_notes rn").
				Where("rn.note_id IN ?", notes).
				Group("rn.resource_id").
				Having("count(*) = ?", len(notes)).
				Select("rn.resource_id")

			dbQuery = dbQuery.Where("resources.id IN (?)", subQuery)
		}

		if query.ShowWithSimilar {
			findDifferentHashToCurrent := originalDb.
				Table("image_hashes i").
				Where("ih.d_hash = i.d_hash").
				Where("ih.id <> i.id").
				Select("1")

			hashAndHashDuplicateExists := originalDb.
				Table("image_hashes ih").
				Where("resources.id = ih.resource_id").
				Where("EXISTS (?)", findDifferentHashToCurrent).
				Select("1")

			dbQuery = dbQuery.Where("EXISTS (?)", hashAndHashDuplicateExists)
		}

		if query.Name != "" {
			p, esc := LikePattern(query.Name)
			dbQuery = dbQuery.Where("resources.name "+likeOperator+" ?"+esc, p)
		}

		if query.Description != "" {
			p, esc := LikePattern(query.Description)
			dbQuery = dbQuery.Where("resources.description "+likeOperator+" ?"+esc, p)
		}

		if query.ContentType != "" {
			p, esc := LikePattern(query.ContentType)
			dbQuery = dbQuery.Where("resources.content_type "+likeOperator+" ?"+esc, p)
		}

		if query.OriginalName != "" {
			p, esc := LikePattern(query.OriginalName)
			dbQuery = dbQuery.Where("resources.original_name "+likeOperator+" ?"+esc, p)
		}

		if query.OriginalLocation != "" {
			p, esc := LikePattern(query.OriginalLocation)
			dbQuery = dbQuery.Where("resources.original_location "+likeOperator+" ?"+esc, p)
		}

		if query.OwnerId != 0 {
			dbQuery = dbQuery.Where("resources.owner_id = ?", query.OwnerId)
		}

		if query.ResourceCategoryId != 0 {
			dbQuery = dbQuery.Where("resources.resource_category_id = ?", query.ResourceCategoryId)
		}

		if query.Hash != "" {
			dbQuery = dbQuery.Where("resources.hash = ?", query.Hash)
		}

		dbQuery = ApplyDateRange(dbQuery, "resources.", query.CreatedBefore, query.CreatedAfter)

		if query.ShowWithoutOwner {
			dbQuery = dbQuery.Where("resources.owner_id IS NULL")
		}

		if query.MinWidth > 0 {
			dbQuery = dbQuery.Where("resources.width >= ?", query.MinWidth)
		}

		if query.MaxWidth > 0 {
			dbQuery = dbQuery.Where("resources.width <= ?", query.MaxWidth)
		}

		if query.MinHeight > 0 {
			dbQuery = dbQuery.Where("resources.height >= ?", query.MinHeight)
		}

		if query.MaxHeight > 0 {
			dbQuery = dbQuery.Where("resources.height <= ?", query.MaxHeight)
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
