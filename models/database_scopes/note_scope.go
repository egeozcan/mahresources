package database_scopes

import (
	"gorm.io/gorm"
	"mahresources/models/query_models"
	"mahresources/models/types"
)

func NoteQuery(query *query_models.NoteQuery, ignoreSort bool, originalDB *gorm.DB) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		likeOperator := GetLikeOperator(db)
		dbQuery := db

		if !ignoreSort {
			dbQuery = ApplySortColumns(dbQuery, query.SortBy, "", "created_at desc")
		}

		if len(query.Tags) > 0 {
			subQuery := originalDB.
				Table("note_tags nt").
				Where("nt.tag_id IN ?", query.Tags).
				Group("nt.note_id").
				Having("count(*) = ?", len(query.Tags)).
				Select("nt.note_id")

			dbQuery = dbQuery.Where("notes.id IN (?)", subQuery)
		}

		if len(query.Ids) > 0 {
			dbQuery = dbQuery.Where("notes.id IN (?)", query.Ids)
		}

		if len(query.Groups) > 0 {
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

		dbQuery = ApplyDateRange(dbQuery, "", query.CreatedBefore, query.CreatedAfter)

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

		if query.Shared != nil {
			dbQuery = dbQuery.Where("share_token IS NOT NULL")
		}

		return dbQuery
	}
}

func NoteTypeQuery(query *query_models.NoteTypeQuery) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		dbQuery := db
		likeOperator := GetLikeOperator(db)
		if query.Name != "" {
			dbQuery = dbQuery.Where("name "+likeOperator+" ?", "%"+query.Name+"%")
		}
		if query.Description != "" {
			dbQuery = dbQuery.Where("description "+likeOperator+" ?", "%"+query.Description+"%")
		}
		return dbQuery
	}
}
