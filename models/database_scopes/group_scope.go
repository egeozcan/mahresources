package database_scopes

import (
	"gorm.io/gorm"
	"mahresources/models/query_models"
	"mahresources/models/types"
	"regexp"
	"strings"
)

func GroupQuery(query *query_models.GroupQuery, ignoreSort bool, originalDB *gorm.DB) func(db *gorm.DB) *gorm.DB {
	sortColumnMatcher := regexp.MustCompile("^(meta->>?'[a-z_]+'|[a-z_]+)(\\s(desc|asc))?$")

	return func(db *gorm.DB) *gorm.DB {
		likeOperator := "LIKE"

		if db.Config.Dialector.Name() == "postgres" {
			likeOperator = "ILIKE"
		}

		dbQuery := db

		if !ignoreSort && query.SortBy != "" && sortColumnMatcher.MatchString(query.SortBy) {
			dbQuery = dbQuery.Order("groups." + query.SortBy).Order("groups.created_at desc")
		} else if !ignoreSort {
			dbQuery = dbQuery.Order("groups.created_at desc")
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

		if query.Ids != nil && len(query.Ids) > 0 {
			dbQuery = dbQuery.Where("groups.id IN (?)", query.Ids)
		}

		if query.Tags != nil && len(query.Tags) > 0 {
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

		if query.Notes != nil && len(query.Notes) > 0 {
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

		if query.Resources != nil && len(query.Resources) > 0 {
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
			var padCharacter = "%"

			// if query name starts and ends with a quote, we will search for exact match and replace \" with ", while removing the quotes
			if strings.HasPrefix(query.Name, "\"") && strings.HasSuffix(query.Name, "\"") {
				operator = "="
				query.Name = strings.ReplaceAll(query.Name[1:len(query.Name)-1], "\\\"", "\"")
				padCharacter = ""
			}

			// Base subselect condition for "g.name"
			subselectCondition := originalDB.Where("g.name "+operator+" ?", padCharacter+query.Name+padCharacter).Where("groups.id = g.id")

			// Check if parent name should be included
			if query.SearchParentsForName {
				addParentSubquery()
				subselectCondition = subselectCondition.Or("parent.name "+operator+" ?", padCharacter+query.Name+padCharacter).Where("groups.owner_id = parent.id")
			}

			// Check if child name should be included
			if query.SearchChildrenForName {
				addChildSubquery()
				subselectCondition = subselectCondition.Or("child.name "+operator+" ?", padCharacter+query.Name+padCharacter).Where("groups.id = child.owner_id")
			}

			// Construct the final subquery
			subSelect := originalDB.
				Table("groups g").
				Select("count(*)").
				Where(subselectCondition)

			// Apply subselect condition to main query
			dbQuery = dbQuery.Where("(?) > 0", subSelect)
		}

		if query.Description != "" {
			dbQuery = dbQuery.Where("groups.description "+likeOperator+" ?", "%"+query.Description+"%")
		}

		if query.URL != "" {
			dbQuery = dbQuery.Where("groups.url "+likeOperator+" ?", "%"+query.URL+"%")
		}

		if query.CreatedBefore != "" {
			dbQuery = dbQuery.Where("groups.created_at <= ?", query.CreatedBefore)
		}

		if query.CreatedAfter != "" {
			dbQuery = dbQuery.Where("groups.created_at >= ?", query.CreatedAfter)
		}

		if query.CategoryId != 0 {
			dbQuery = dbQuery.Where("groups.category_id >= ?", query.CategoryId)
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
