package database_scopes

import (
	"gorm.io/gorm"
	"mahresources/models/query_models"
	"mahresources/models/types"
	"regexp"
	"strings"
)

func GroupQuery(query *query_models.GroupQuery, ignoreSort bool, originalDB *gorm.DB) func(db *gorm.DB) *gorm.DB {
	sortColumnMatcher := regexp.MustCompile("^[a-z_]+(\\s(desc|asc))?$")

	return func(db *gorm.DB) *gorm.DB {
		likeOperator := "LIKE"

		if db.Config.Dialector.Name() == "postgres" {
			likeOperator = "ILIKE"
		}

		dbQuery := db

		if !ignoreSort && query.SortBy != "" && sortColumnMatcher.MatchString(query.SortBy) {
			dbQuery = dbQuery.Order(query.SortBy)
		} else if !ignoreSort {
			dbQuery = dbQuery.Order("created_at desc")
		}

		if query.Tags != nil && len(query.Tags) > 0 {
			subSelectCondition := originalDB.
				Where("groups.id = gt.group_id")

			if query.SearchParentsForTags {
				dbQuery = dbQuery.
					Joins("LEFT JOIN groups parent ON parent.id = groups.owner_id")
				subSelectCondition = subSelectCondition.Or("parent.id = gt.group_id")
			}

			if query.SearchChildrenForTags {
				subSelectCondition = subSelectCondition.
					Or("gt.group_id IN (SELECT id FROM groups child WHERE child.owner_id = groups.id)")
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
			dbQuery = dbQuery.Where("groups.name "+likeOperator+" ?", "%"+query.Name+"%")
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
