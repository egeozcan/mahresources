package database_scopes

import (
	"gorm.io/gorm"
	"mahresources/models/query_models"
)

// RelationTypeQuery builds the relation type list filter. ForFromGroup and
// ForToGroup read a group's category; under vis a group the caller cannot see
// resolves to no category, exactly like a group that does not exist.
func RelationTypeQuery(query *query_models.RelationshipTypeQuery, vis Visibility) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		likeOperator := GetLikeOperator(db)
		dbQuery := db

		if query.Name != "" {
			p, esc := LikePattern(query.Name)
			dbQuery = dbQuery.Where("name "+likeOperator+" ?"+esc, p)
		}

		if query.Description != "" {
			p, esc := LikePattern(query.Description)
			dbQuery = dbQuery.Where("description "+likeOperator+" ?"+esc, p)
		}

		if query.ForFromGroup != 0 {
			dbQuery = dbQuery.Where(`
				from_category_id = (
					SELECT 
						category_id 
					FROM 
						groups 
					WHERE 
						`+and("groups.id = ?", vis.groupVisible("groups.id"))+`
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
						`+and("groups.id = ?", vis.groupVisible("groups.id"))+`
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
		likeOperator := GetLikeOperator(db)

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
