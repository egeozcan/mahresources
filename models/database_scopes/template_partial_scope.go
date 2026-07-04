package database_scopes

import (
	"mahresources/models/query_models"

	"gorm.io/gorm"
)

// TemplatePartialQuery filters template partials by a name and/or description
// LIKE match, mirroring the other simple taxonomy scopes.
func TemplatePartialQuery(query *query_models.TemplatePartialQuery) func(db *gorm.DB) *gorm.DB {
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
