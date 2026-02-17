package database_scopes

import (
	"strings"

	"gorm.io/gorm"
	"mahresources/models/query_models"
	"mahresources/models/types"
)

func GroupQuery(query *query_models.GroupQuery, ignoreSort bool, originalDB *gorm.DB) func(db *gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		likeOperator := GetLikeOperator(db)
		dbQuery := db

		if !ignoreSort {
			dbQuery = ApplySortColumns(dbQuery, query.SortBy, "groups.", "groups.created_at desc")
		}

		var parentAdded = false
		var childAdded = false

		var addParentSubquery = func() {
			if parentAdded {
				return
			}
			dbQuery = dbQuery.
				Joins("LEFT JOIN groups parent ON parent.id = groups.owner_id")
			parentAdded = true
		}

		var addChildSubquery = func() {
			if childAdded {
				return
			}
			dbQuery = dbQuery.
				Joins("LEFT JOIN groups child ON child.owner_id = groups.id")
			childAdded = true
		}

		if len(query.Ids) > 0 {
			dbQuery = dbQuery.Where("groups.id IN (?)", query.Ids)
		}

		if len(query.Tags) > 0 {
			subSelectCondition := originalDB.
				Where("groups.id = gt.group_id")

			if query.SearchParentsForTags {
				addParentSubquery()
				subSelectCondition = subSelectCondition.Or("parent.id = gt.group_id")
			}

			if query.SearchChildrenForTags {
				addChildSubquery()
				subSelectCondition = subSelectCondition.Or("child.id = gt.group_id")
			}

			subSelect := originalDB.
				Table("group_tags gt").
				Select("count(distinct tag_id)").
				Where("gt.tag_id IN ?", query.Tags).
				Where(subSelectCondition)

			dbQuery = dbQuery.Where("(?) = ?", subSelect, len(query.Tags))
		}

		if len(query.Notes) > 0 {
			justRelatedNotesSubQuery := originalDB.
				Table("notes n").
				Select("count(*)").
				Joins("JOIN groups_related_notes grn on n.id = grn.note_id").
				// filter out the ones of which the group is the owner
				// prevents counting 2 times when we are both related AND the owner
				Where("n.owner_id <> grn.group_id").
				Where("n.id IN ?", query.Notes).
				Where("grn.group_id = groups.id")

			justOwnedNotesSubquery := originalDB.
				Table("notes n").
				Select("count(*)").
				Where("n.id IN ?", query.Notes).
				Where("n.owner_id = groups.id")

			dbQuery = dbQuery.Where("(?) + (?) = ?", justRelatedNotesSubQuery, justOwnedNotesSubquery, len(query.Notes))
		}

		if len(query.Resources) > 0 {
			justRelatedResourcesQuery := originalDB.
				Table("resources r").
				Select("count(*)").
				Joins("JOIN groups_related_resources grr on r.id = grr.resource_id").
				// filter out the ones of which the group is the owner
				// prevents counting 2 times when we are both related AND the owner
				Where("grr.group_id <> r.owner_id").
				Where("grr.group_id = groups.id").
				Where("r.id IN ?", query.Resources)

			justOwnedResourcesQuery := originalDB.
				Table("resources r").
				Select("count(*)").
				Where("r.owner_id = groups.id").
				Where("r.id IN ?", query.Resources)

			dbQuery = dbQuery.Where("(?) + (?) = ?", justRelatedResourcesQuery, justOwnedResourcesQuery, len(query.Resources))
		}

		if len(query.Groups) > 0 {
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
			).Or("groups.owner_id IN ?", query.Groups)
		}

		if query.RelationTypeId != 0 {
			relationSubquery := originalDB.
				Table("group_relation_types grt").
				Where("grt.id = ?", query.RelationTypeId)

			if query.RelationSide == 0 {
				relationSubquery = relationSubquery.Select("grt.from_category_id")
			} else {
				relationSubquery = relationSubquery.Select("grt.to_category_id")
			}

			dbQuery = dbQuery.Where("groups.category_id = (?)", relationSubquery)
		}

		if query.Name != "" {
			var operator = likeOperator
			var name = "%" + query.Name + "%"

			if strings.HasPrefix(query.Name, "\"") && strings.HasSuffix(query.Name, "\"") {
				operator = "="
				name = strings.ReplaceAll(query.Name[1:len(query.Name)-1], "\\\"", "\"")
			}

			conditions := []string{"groups.name " + operator + " ?"}
			params := []interface{}{name}

			if query.SearchParentsForName {
				addParentSubquery()
				conditions = append(conditions, "parent.name "+operator+" ?")
				params = append(params, name)
			}

			if query.SearchChildrenForName {
				addChildSubquery()
				conditions = append(conditions, "child.name "+operator+" ?")
				params = append(params, name)
			}

			conditionString := strings.Join(conditions, " OR ")

			dbQuery = dbQuery.Where(conditionString, params...)
		}

		if query.Description != "" {
			dbQuery = dbQuery.Where("groups.description "+likeOperator+" ?", "%"+query.Description+"%")
		}

		if query.URL != "" {
			dbQuery = dbQuery.Where("groups.url "+likeOperator+" ?", "%"+query.URL+"%")
		}

		dbQuery = ApplyDateRange(dbQuery, "groups.", query.CreatedBefore, query.CreatedAfter)

		if query.CategoryId != 0 {
			dbQuery = dbQuery.Where("groups.category_id = ?", query.CategoryId)
		}

		if query.OwnerId != 0 {
			dbQuery = dbQuery.Where("groups.owner_id = ?", query.OwnerId)
		}

		if len(query.Categories) != 0 {
			dbQuery = dbQuery.Where("groups.category_id IN ?", query.Categories)
		}

		if len(query.MetaQuery) > 0 {
			for _, v := range query.MetaQuery {
				if v.Key == "" {
					continue
				}

				parentPrefix := "parent."
				childPrefix := "child."

				if strings.HasPrefix(v.Key, parentPrefix) {
					key := strings.TrimPrefix(v.Key, parentPrefix)

					subSelect := originalDB.
						Table("groups p").
						Select("count(*)").
						Where(types.JSONQuery("p.meta").Operation(getOperationType(v.Operation), v.Value, key)).
						Where("groups.owner_id = p.id")

					dbQuery = dbQuery.Where("(?) = 1", subSelect)
				} else if strings.HasPrefix(v.Key, childPrefix) {
					key := strings.TrimPrefix(v.Key, childPrefix)

					subSelect := originalDB.
						Table("groups p").
						Select("count(*)").
						Where(types.JSONQuery("p.meta").Operation(getOperationType(v.Operation), v.Value, key)).
						Where("groups.id = p.owner_id")

					dbQuery = dbQuery.Where("(?) = 1", subSelect)
				} else {
					dbQuery = dbQuery.Where(types.JSONQuery("groups.meta").Operation(getOperationType(v.Operation), v.Value, v.Key))
				}
			}
		}

		return dbQuery
	}
}
