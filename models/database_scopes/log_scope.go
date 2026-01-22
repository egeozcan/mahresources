package database_scopes

import (
	"gorm.io/gorm"
	"mahresources/models/query_models"
)

// LogEntryQuery returns a GORM scope for filtering log entries.
func LogEntryQuery(query *query_models.LogEntryQuery, ignoreSort bool) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		likeOperator := GetLikeOperator(db)
		dbQuery := db

		if !ignoreSort {
			dbQuery = ApplySortColumns(dbQuery, query.SortBy, "", "created_at desc")
		}

		if query.Level != "" {
			dbQuery = dbQuery.Where("level = ?", query.Level)
		}

		if query.Action != "" {
			dbQuery = dbQuery.Where("action = ?", query.Action)
		}

		if query.EntityType != "" {
			dbQuery = dbQuery.Where("entity_type = ?", query.EntityType)
		}

		if query.EntityID != 0 {
			dbQuery = dbQuery.Where("entity_id = ?", query.EntityID)
		}

		if query.Message != "" {
			dbQuery = dbQuery.Where("message "+likeOperator+" ?", "%"+query.Message+"%")
		}

		if query.RequestPath != "" {
			dbQuery = dbQuery.Where("request_path "+likeOperator+" ?", "%"+query.RequestPath+"%")
		}

		dbQuery = ApplyDateRange(dbQuery, "", query.CreatedBefore, query.CreatedAfter)

		return dbQuery
	}
}

// EntityHistoryQuery returns a GORM scope for getting history of a specific entity.
func EntityHistoryQuery(query *query_models.EntityHistoryQuery) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		return db.Where("entity_type = ? AND entity_id = ?", query.EntityType, query.EntityID).
			Order("created_at desc")
	}
}
