package database_scopes

import (
	"gorm.io/gorm"
	"mahresources/models/query_models"
)

func RelationTypeQuery(query *query_models.RelationshipTypeQuery) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		likeOperator := "LIKE"

		if db.Config.Dialector.Name() == "postgres" {
			likeOperator = "ILIKE"
		}

		dbQuery := db

		if query.Name != "" {
			dbQuery = dbQuery.Where("name "+likeOperator+" ?", "%"+query.Name+"%")
		}

		if query.Description != "" {
			dbQuery = dbQuery.Where("description "+likeOperator+" ?", "%"+query.Description+"%")
		}

		if query.ForFromGroup != 0 {
			dbQuery = dbQuery.Where(`
				from_category_id = (
					SELECT 
						category_id 
					FROM 
						groups 
					WHERE 
						groups.id = ?
				)
			`, query.ForFromGroup)
		}

		if query.ForToGroup != 0 {
			dbQuery = dbQuery.Where(`
				to_category_id = (
					SELECT 
						category_id 
					FROM 
						groups 
					WHERE 
						groups.id = ?
				)
			`, query.ForToGroup)
		}

		if query.ToCategory != 0 {
			dbQuery = dbQuery.Where(`
				to_category_id = ?
			`, query.ToCategory)
		}

		if query.FromCategory != 0 {
			dbQuery = dbQuery.Where(`
				from_category_id = ?
			`, query.FromCategory)
		}

		return dbQuery
	}
}

func RelationQuery(query *query_models.GroupRelationshipQuery) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		dbQuery := db

		likeOperator := "LIKE"

		if db.Config.Dialector.Name() == "postgres" {
			likeOperator = "ILIKE"
		}

		if query.FromGroupId != 0 {
			dbQuery = dbQuery.Where("from_group_id = ?", query.FromGroupId)
		}

		if query.ToGroupId != 0 {
			dbQuery = dbQuery.Where("to_group_id = ?", query.ToGroupId)
		}

		if query.GroupRelationTypeId != 0 {
			dbQuery = dbQuery.Where("relation_type_id = ?", query.GroupRelationTypeId)
		}

		if query.Name != "" {
			dbQuery = dbQuery.Where("name "+likeOperator+" ?", "%"+query.Name+"%")
		}

		if query.Description != "" {
			dbQuery = dbQuery.Where("description "+likeOperator+" ?", "%"+query.Description+"%")
		}

		return dbQuery
	}
}
