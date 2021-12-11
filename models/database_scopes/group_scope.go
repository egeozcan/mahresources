package database_scopes

import (
	"gorm.io/gorm"
	"mahresources/models/query_models"
	"mahresources/models/types"
)

func GroupQuery(query *query_models.GroupQuery) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		dbQuery := db

		if query.Tags != nil && len(query.Tags) > 0 {
			dbQuery = dbQuery.Where(
				"(SELECT Count(*) FROM group_tags pt WHERE pt.tag_id IN ? AND pt.group_id = groups.id) = ?",
				query.Tags,
				len(query.Tags),
			)
		}

		if query.Notes != nil && len(query.Notes) > 0 {
			dbQuery = dbQuery.Where(
				`
					(
						SELECT 
							Count(*) 
						FROM 
							notes n
						JOIN
							groups_related_notes grn on n.id = grn.note_id
							AND n.owner_id <> grn.group_id
						WHERE 
							n.id IN ?
							AND grn.group_id = groups.id
					) + (
						SELECT
							Count(*) 
						FROM 
							notes n 
						WHERE 
							n.id IN ? 
							AND n.owner_id = groups.id
					) = ?`,
				query.Notes,
				query.Notes,
				len(query.Notes),
			)
		}

		if query.Resources != nil && len(query.Resources) > 0 {
			dbQuery = dbQuery.Where(
				`
					(
						SELECT 
							Count(*) 
						FROM 
							resources r
						JOIN
							groups_related_resources grr on r.id = grr.resource_id
							AND r.owner_id <> grr.group_id
						WHERE 
							r.id IN ?
							AND grr.group_id = groups.id
					) + (
						SELECT
							Count(*) 
						FROM 
							resources r 
						WHERE 
							r.id IN ? 
							AND r.owner_id = groups.id
					) = ?`,
				query.Resources,
				query.Resources,
				len(query.Resources),
			)
		}

		if query.Groups != nil && len(query.Groups) > 0 {
			dbQuery = dbQuery.Where(
				`
					(
						SELECT 
							Count(*) 
						FROM 
							group_related_groups grg
						WHERE 
							grg.related_group_id = groups.id
							AND grg.group_id IN ?
					) = ?`,
				query.Groups,
				len(query.Groups),
			).Or("owner_id IN ?", query.Groups)
		}

		if query.RelationTypeId != 0 {
			if query.RelationSide == 0 {
				dbQuery = dbQuery.Where(`
					groups.category_id = (
						SELECT
							from_category_id
						FROM
							group_relation_types grt
						WHERE
							grt.id = ?
					)
				`, query.RelationTypeId)
			} else {
				dbQuery = dbQuery.Where(`
					groups.category_id = (
						SELECT
							to_category_id
						FROM
							group_relation_types grt
						WHERE
							grt.id = ?
					)
				`, query.RelationTypeId)
			}
		}

		if query.Name != "" {
			dbQuery = dbQuery.Where("name LIKE ?", "%"+query.Name+"%")
		}

		if query.Description != "" {
			dbQuery = dbQuery.Where("description LIKE ?", "%"+query.Description+"%")
		}

		if query.CreatedBefore != "" {
			dbQuery = dbQuery.Where("created_at <= ?", query.CreatedBefore)
		}

		if query.CreatedAfter != "" {
			dbQuery = dbQuery.Where("created_at >= ?", query.CreatedAfter)
		}

		if query.CategoryId != 0 {
			dbQuery = dbQuery.Where("category_id >= ?", query.CategoryId)
		}

		if query.OwnerId != 0 {
			dbQuery = dbQuery.Where("owner_id >= ?", query.OwnerId)
		}

		if len(query.Categories) != 0 {
			dbQuery = dbQuery.Where("category_id IN ?", query.Categories)
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
