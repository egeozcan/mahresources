package database_scopes

import (
	"fmt"

	"gorm.io/gorm"
	"mahresources/models/query_models"
)

func NoteQuery(query *query_models.NoteQuery, ignoreSort bool, originalDB *gorm.DB) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		likeOperator := GetLikeOperator(db)
		dbQuery := db

		if !ignoreSort {
			dbQuery = ApplySortColumns(dbQuery, query.SortBy, "", "created_at desc")
		}

		if len(query.Tags) > 0 {
			tags := deduplicateUints(query.Tags)
			subQuery := originalDB.
				Table("note_tags nt").
				Where("nt.tag_id IN ?", tags).
				Group("nt.note_id").
				Having("count(*) = ?", len(tags)).
				Select("nt.note_id")

			dbQuery = dbQuery.Where("notes.id IN (?)", subQuery)
		}

		if len(query.Ids) > 0 {
			dbQuery = dbQuery.Where("notes.id IN (?)", query.Ids)
		}

		if len(query.Groups) > 0 {
			groups := deduplicateUints(query.Groups)
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
							AND (notes.owner_id IS NULL OR notes.owner_id <> grn.group_id)
					) + (
						SELECT
							CASE
								WHEN
									notes.owner_id IN ?
								THEN 1
								ELSE 0
							END
					) = ?`,
				groups,
				groups,
				len(groups),
			)
		}

		if query.Name != "" {
			p, esc := LikePattern(query.Name)
			dbQuery = dbQuery.Where("notes.name "+likeOperator+" ?"+esc, p)
		}

		if query.Description != "" {
			p, esc := LikePattern(query.Description)
			dbQuery = dbQuery.Where("notes.description "+likeOperator+" ?"+esc, p)
		}

		if query.OwnerId != 0 {
			dbQuery = dbQuery.Where("owner_id = ?", query.OwnerId)
		}

		dbQuery = ApplyDateRange(dbQuery, "notes.", query.CreatedBefore, query.CreatedAfter)
		dbQuery = ApplyUpdatedDateRange(dbQuery, "notes.", query.UpdatedBefore, query.UpdatedAfter)

		if query.StartDateBefore != "" {
			if !ValidateDateString(query.StartDateBefore) {
				_ = dbQuery.AddError(fmt.Errorf("%w: startDateBefore=%q is not a valid date (expected YYYY-MM-DD or RFC 3339)", ErrInvalidDateFilter, query.StartDateBefore))
				return dbQuery
			}
			dbQuery = dbQuery.Where("start_date <= ?", query.StartDateBefore)
		}

		if query.StartDateAfter != "" {
			if !ValidateDateString(query.StartDateAfter) {
				_ = dbQuery.AddError(fmt.Errorf("%w: startDateAfter=%q is not a valid date (expected YYYY-MM-DD or RFC 3339)", ErrInvalidDateFilter, query.StartDateAfter))
				return dbQuery
			}
			dbQuery = dbQuery.Where("start_date >= ?", query.StartDateAfter)
		}

		if query.EndDateBefore != "" {
			if !ValidateDateString(query.EndDateBefore) {
				_ = dbQuery.AddError(fmt.Errorf("%w: endDateBefore=%q is not a valid date (expected YYYY-MM-DD or RFC 3339)", ErrInvalidDateFilter, query.EndDateBefore))
				return dbQuery
			}
			dbQuery = dbQuery.Where("end_date <= ?", query.EndDateBefore)
		}

		if query.EndDateAfter != "" {
			if !ValidateDateString(query.EndDateAfter) {
				_ = dbQuery.AddError(fmt.Errorf("%w: endDateAfter=%q is not a valid date (expected YYYY-MM-DD or RFC 3339)", ErrInvalidDateFilter, query.EndDateAfter))
				return dbQuery
			}
			dbQuery = dbQuery.Where("end_date >= ?", query.EndDateAfter)
		}

		if query.NoteTypeId != 0 {
			dbQuery = dbQuery.Where("note_type_id = ?", query.NoteTypeId)
		}

		dbQuery = ApplyMetaQuery(dbQuery, query.MetaQuery, "notes.meta")

		if query.Shared != nil {
			if *query.Shared {
				dbQuery = dbQuery.Where("share_token IS NOT NULL")
			} else {
				dbQuery = dbQuery.Where("share_token IS NULL")
			}
		}

		return dbQuery
	}
}

func NoteTypeQuery(query *query_models.NoteTypeQuery) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		dbQuery := db
		likeOperator := GetLikeOperator(db)
		if query.Name != "" {
			p, esc := LikePattern(query.Name)
			dbQuery = dbQuery.Where("name "+likeOperator+" ?"+esc, p)
		}
		if query.Description != "" {
			p, esc := LikePattern(query.Description)
			dbQuery = dbQuery.Where("description "+likeOperator+" ?"+esc, p)
		}
		return dbQuery
	}
}
