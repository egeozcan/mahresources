package database_scopes

import (
	"gorm.io/gorm"
	"mahresources/models/query_models"
	"mahresources/models/types"
	"regexp"
)

func NoteQuery(query *query_models.NoteQuery, ignoreSort bool) func(db *gorm.DB) *gorm.DB {
	sortColumnMatcher := regexp.MustCompile("^(meta->>?'[a-z_]+'|[a-z_]+)(\\s(desc|asc))?$")

	return func(db *gorm.DB) *gorm.DB {
		likeOperator := "LIKE"

		if db.Config.Dialector.Name() == "postgres" {
			likeOperator = "ILIKE"
		}

		dbQuery := db

		if !ignoreSort && query.SortBy != "" && sortColumnMatcher.MatchString(query.SortBy) {
			dbQuery = dbQuery.Order(query.SortBy).Order("created_at desc")
		} else if !ignoreSort {
			dbQuery = dbQuery.Order("created_at desc")
		}

		if query.Tags != nil && len(query.Tags) > 0 {
			dbQuery = dbQuery.Where(
				"(SELECT Count(*) FROM note_tags at WHERE at.tag_id IN ? AND at.note_id = notes.id) = ?",
				query.Tags,
				len(query.Tags),
			)
		}

		if query.Ids != nil && len(query.Ids) > 0 {
			dbQuery = dbQuery.Where("notes.id IN (?)", query.Ids)
		}

		if query.Groups != nil && len(query.Groups) > 0 {
			dbQuery = dbQuery.Where(
				`
					(
						SELECT 
							Count(*) 
						FROM 
							groups_related_notes grn 
						WHERE 
							grn.group_id IN ? 
							AND grn.note_id = notes.id
							AND notes.owner_id <> grn.group_id
					) + (
						SELECT
							CASE
								WHEN 
									notes.owner_id IN ?
								THEN 1
								ELSE 0
							END
					) = ?`,
				query.Groups,
				query.Groups,
				len(query.Groups),
			)
		}

		if query.Name != "" {
			dbQuery = dbQuery.Where("name "+likeOperator+" ?", "%"+query.Name+"%")
		}

		if query.Description != "" {
			dbQuery = dbQuery.Where("description "+likeOperator+" ?", "%"+query.Description+"%")
		}

		if query.OwnerId != 0 {
			dbQuery = dbQuery.Where("owner_id = ?", query.OwnerId)
		}

		if query.CreatedBefore != "" {
			dbQuery = dbQuery.Where("created_at <= ?", query.CreatedBefore)
		}

		if query.CreatedAfter != "" {
			dbQuery = dbQuery.Where("created_at >= ?", query.CreatedAfter)
		}

		if query.StartDateBefore != "" {
			dbQuery = dbQuery.Where("start_date <= ?", query.StartDateBefore)
		}

		if query.StartDateAfter != "" {
			dbQuery = dbQuery.Where("start_date >= ?", query.StartDateAfter)
		}

		if query.EndDateBefore != "" {
			dbQuery = dbQuery.Where("end_date <= ?", query.EndDateBefore)
		}

		if query.EndDateAfter != "" {
			dbQuery = dbQuery.Where("end_date >= ?", query.EndDateAfter)
		}

		if query.NoteTypeId != 0 {
			dbQuery = dbQuery.Where("note_type_id = ?", query.NoteTypeId)
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
