package application_context

import (
	"errors"
	"net/url"
	"strings"

	"gorm.io/gorm"
	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
	"mahresources/models/types"
	"mahresources/mrql"
)

func (ctx *MahresourcesContext) CreateGroup(groupQuery *query_models.GroupCreator) (*models.Group, error) {
	groupQuery.Name = strings.TrimSpace(groupQuery.Name)
	if groupQuery.Name == "" {
		return nil, errors.New("group name needed")
	}

	if err := ValidateEntityName(groupQuery.Name, "group"); err != nil {
		return nil, err
	}

	if groupQuery.Meta == "" {
		groupQuery.Meta = "{}"
	}

	if err := ValidateMeta(groupQuery.Meta); err != nil {
		return nil, err
	}

	hookData := map[string]any{
		"id":          float64(0),
		"name":        groupQuery.Name,
		"description": groupQuery.Description,
		"meta":        groupQuery.Meta,
	}
	hookData, hookErr := ctx.RunBeforePluginHooks("before_group_create", hookData)
	if hookErr != nil {
		return nil, hookErr
	}
	if name, ok := hookData["name"].(string); ok {
		groupQuery.Name = name
	}
	if desc, ok := hookData["description"].(string); ok {
		groupQuery.Description = desc
	}
	if hMeta, ok := hookData["meta"].(string); ok {
		groupQuery.Meta = hMeta
	}

	var groupUrl *types.URL
	if groupQuery.URL != "" {
		parsedURL, err := url.Parse(groupQuery.URL)
		if err != nil {
			return nil, err
		}
		groupUrl = (*types.URL)(parsedURL)
	}

	group := models.Group{
		Name:        groupQuery.Name,
		Description: groupQuery.Description,
		CategoryId:  uintPtrOrNil(groupQuery.CategoryId),
		Meta:        []byte(groupQuery.Meta),
		URL:         groupUrl,
	}

	// db.Transaction rather than db.Begin: on a handle that is already inside a
	// transaction, Begin returns ErrInvalidTransaction (a *sql.Tx is neither a
	// TxBeginner nor a ConnPoolBeginner) while Transaction issues a SAVEPOINT.
	// mah.db.transaction is the caller that needs that.
	//
	// Panics are handled differently now, deliberately. The deferred recover
	// this replaces rolled back and then *swallowed* the panic, so the function
	// returned (nil, nil) — a create that reports neither a group nor an error,
	// which every caller then dereferences. db.Transaction rolls back and lets
	// the panic continue to the recovery middleware.
	if err := ctx.db.Transaction(func(tx *gorm.DB) error {
		if groupQuery.OwnerId != 0 {
			var ownerCheck models.Group
			if err := tx.Select("id").First(&ownerCheck, groupQuery.OwnerId).Error; err != nil {
				return errors.New("owner group not found")
			}
			group.OwnerId = &groupQuery.OwnerId
		}

		if groupQuery.CategoryId != 0 {
			var catCheck models.Category
			if err := tx.Select("id").First(&catCheck, groupQuery.CategoryId).Error; err != nil {
				return errors.New("category not found")
			}
		}

		if err := tx.Create(&group).Error; err != nil {
			if isForeignKeyError(err) {
				return errors.New("referenced category or owner does not exist")
			}
			return err
		}

		// Reject self-ownership (can only check after Create assigns the ID)
		if group.OwnerId != nil && *group.OwnerId == group.ID {
			return errors.New("a group cannot be its own owner")
		}

		if len(groupQuery.Tags) > 0 {
			if err := ValidateAssociationIDs[models.Tag](tx, groupQuery.Tags, "tags"); err != nil {
				return err
			}
			tags := BuildAssociationSlice(groupQuery.Tags, TagFromID)

			if createTagsErr := tx.Model(&group).Association("Tags").Append(&tags); createTagsErr != nil {
				return createTagsErr
			}
		}

		if len(groupQuery.Groups) > 0 {
			if err := ValidateAssociationIDs[models.Group](tx, groupQuery.Groups, "groups"); err != nil {
				return err
			}
			groups := BuildAssociationSlice(groupQuery.Groups, GroupFromID)

			if createGroupsErr := tx.Model(&group).Association("RelatedGroups").Append(&groups); createGroupsErr != nil {
				return createGroupsErr
			}
		}

		return nil
	}); err != nil {
		return nil, err
	}

	ctx.syncMentionsForGroup(&group)

	ctx.Logger().Info(models.LogActionCreate, "group", &group.ID, group.Name, "Created group", nil)

	ctx.RunAfterPluginHooks("after_group_create", map[string]any{
		"id":          float64(group.ID),
		"name":        group.Name,
		"description": group.Description,
		"meta":        string(group.Meta),
	})

	ctx.InvalidateSearchCacheByType(EntityTypeGroup)
	return &group, nil
}

func (ctx *MahresourcesContext) UpdateGroup(groupQuery *query_models.GroupEditor) (*models.Group, error) {
	if groupQuery.Name == "" {
		return nil, errors.New("group name needed")
	}

	if err := ValidateEntityName(groupQuery.Name, "group"); err != nil {
		return nil, err
	}

	// The scoped UPDATE below matches no rows for a group outside the subtree,
	// but RowsAffected is never consulted, and the relation cleanup that follows
	// is keyed on groupQuery.ID rather than on what was actually updated. Without
	// this check a group-limited caller — reaching here through mah.db.update_group
	// from a hook — deletes the constrained edges incident to a group it cannot
	// see, controlling NEITHER endpoint. That is a different and worse case than
	// re-categorising a group it does own, which is listed as known-open.
	//
	// Checked before the hook fires, so a refused update does not run
	// before_group_update either, matching DeleteCategory. visibleGroupIDs reads
	// the allow-list already on the db context and issues no query.
	if !ctx.visibleGroupIDs([]uint{groupQuery.ID})[groupQuery.ID] {
		return nil, gorm.ErrRecordNotFound
	}

	hookData := map[string]any{
		"id":          float64(groupQuery.ID),
		"name":        groupQuery.Name,
		"description": groupQuery.Description,
		"meta":        groupQuery.Meta,
	}
	hookData, hookErr := ctx.RunBeforePluginHooks("before_group_update", hookData)
	if hookErr != nil {
		return nil, hookErr
	}
	if name, ok := hookData["name"].(string); ok {
		groupQuery.Name = name
	}
	if desc, ok := hookData["description"].(string); ok {
		groupQuery.Description = desc
	}
	if hMeta, ok := hookData["meta"].(string); ok {
		groupQuery.Meta = hMeta
	}

	groups := BuildAssociationSlicePtr(groupQuery.Groups, GroupPtrFromID)
	tags := BuildAssociationSlicePtr(groupQuery.Tags, TagPtrFromID)

	if groupQuery.Meta == "" {
		groupQuery.Meta = "{}"
	}

	if err := ValidateMeta(groupQuery.Meta); err != nil {
		return nil, err
	}

	group := &models.Group{
		ID:          groupQuery.ID,
		Name:        groupQuery.Name,
		Description: groupQuery.Description,
		CategoryId:  uintPtrOrNil(groupQuery.CategoryId),
		Meta:        []byte(groupQuery.Meta),
	}

	if groupQuery.URL != "" {
		parsedURL, err := url.Parse(groupQuery.URL)

		if groupQuery.URL != "" && err != nil {
			return nil, err
		}

		groupUrl := (*types.URL)(parsedURL)
		group.URL = groupUrl
	} else {
		group.URL = nil
	}

	// See CreateGroup: db.Transaction so this path savepoint-nests inside a
	// transaction the caller already opened.
	if err := ctx.db.Transaction(func(tx *gorm.DB) error {
		if groupQuery.OwnerId != 0 {
			// Serialise re-parents against every other re-parent (the mass
			// edit's owner op included): the cycle walk below reads an
			// ancestor CHAIN that a concurrent re-parent could extend between
			// the walk and the write, and row locks cannot freeze a chain.
			// First operation in the transaction, before any group row is
			// read or locked — see group_tree_lock.go.
			if err := ctx.lockGroupTreeMutation(tx); err != nil {
				return err
			}
			if groupQuery.OwnerId == groupQuery.ID {
				return errors.New("a group cannot be its own owner")
			}
			// Verify the proposed owner exists, then walk up its ancestry to detect cycles.
			currentAncestor := groupQuery.OwnerId
			for i := 0; i < 100; i++ { // depth limit to prevent infinite loops
				var ancestor models.Group
				if err := tx.Select("id", "owner_id").First(&ancestor, currentAncestor).Error; err != nil {
					if i == 0 {
						// First iteration: the proposed owner itself doesn't exist
						return errors.New("owner group not found")
					}
					break // further ancestor not found, no cycle
				}
				if ancestor.OwnerId == nil {
					break // reached a root group, no cycle
				}
				if *ancestor.OwnerId == groupQuery.ID {
					return errors.New("setting this owner would create an ownership cycle")
				}
				currentAncestor = *ancestor.OwnerId
			}
			group.OwnerId = &groupQuery.OwnerId
			group.Owner = &models.Group{ID: groupQuery.OwnerId}
		} else if err := tx.Model(group).Association("Owner").Clear(); err != nil {
			return err
		}

		if groupQuery.CategoryId != 0 {
			var catCheck models.Category
			if err := tx.Select("id").First(&catCheck, groupQuery.CategoryId).Error; err != nil {
				return errors.New("category not found")
			}
		}

		if err := tx.Model(group).Select("Name", "Description", "Meta", "URL", "OwnerId", "Owner", "CategoryId").Updates(group).Error; err != nil {
			if isForeignKeyError(err) {
				return errors.New("referenced category or owner does not exist")
			}
			return err
		}

		// Clean up GroupRelation records that become invalid after category change.
		// A relation is invalid if the group's new category doesn't match what
		// the relation type requires for that position (from or to).
		//
		// Item 4.1. These four DELETEs were keyed only on group.ID, and
		// group_relations is not in scopeColumn, so no callback confined the far
		// endpoint: a group-limited caller re-categorising a group inside its own
		// subtree destroyed edges reaching groups it cannot see, created by an
		// operator it cannot be. The edge is spared now. What is left behind is a
		// row whose category no longer matches its relation type -- a create-time
		// constraint only, never enforced at read time, and one that MergeGroups
		// already produces routinely by copying edges with no revalidation. A
		// spared inconsistent row is repairable by an editor; a deleted edge is
		// not.
		//
		// The caller's own view does not change either way: scopeRelationQuery
		// and GetGroup's relCond already hide out-of-subtree edges from it, so it
		// never saw these and still cannot.
		//
		// Deliberately not the shape the roadmap card proposed. Appending an
		// `IN (<subtree ids>)` predicate is the construction visibleGroupIDs'
		// own comment rejects -- it can trip SQLITE_MAX_VARIABLE_NUMBER and
		// Postgres's parameter ceiling, and subtreeScopeIDs re-runs the recursive
		// group-tree CTE per call. Selecting the handful of edges incident to one
		// group and filtering them in Go bounds the id list by the group's own
		// degree instead, and reads an allow-list that is already materialised.
		newCategoryId := groupQuery.CategoryId
		if newCategoryId == 0 {
			// Category cleared: delete all relations where this group occupies a position
			// that has a non-NULL category constraint on the relation type.
			if err := ctx.deleteRelationsSparingUnseen(tx, group.ID,
				"from_group_id = ? AND relation_type_id IN (SELECT id FROM group_relation_types WHERE from_category_id IS NOT NULL)",
				group.ID,
			); err != nil {
				return err
			}
			if err := ctx.deleteRelationsSparingUnseen(tx, group.ID,
				"to_group_id = ? AND relation_type_id IN (SELECT id FROM group_relation_types WHERE to_category_id IS NOT NULL)",
				group.ID,
			); err != nil {
				return err
			}
		} else {
			// Category changed to a specific value: delete relations where the relation
			// type's category constraint for this group's position doesn't match.
			if err := ctx.deleteRelationsSparingUnseen(tx, group.ID,
				"from_group_id = ? AND relation_type_id IN (SELECT id FROM group_relation_types WHERE from_category_id IS NOT NULL AND from_category_id != ?)",
				group.ID, newCategoryId,
			); err != nil {
				return err
			}
			if err := ctx.deleteRelationsSparingUnseen(tx, group.ID,
				"to_group_id = ? AND relation_type_id IN (SELECT id FROM group_relation_types WHERE to_category_id IS NOT NULL AND to_category_id != ?)",
				group.ID, newCategoryId,
			); err != nil {
				return err
			}
		}

		if len(groupQuery.Tags) > 0 {
			if err := ValidateAssociationIDs[models.Tag](tx, groupQuery.Tags, "tags"); err != nil {
				return err
			}
		}

		if len(groupQuery.Groups) > 0 {
			if err := ValidateAssociationIDs[models.Group](tx, groupQuery.Groups, "groups"); err != nil {
				return err
			}
		}

		if err := tx.Model(group).Association("Tags").Replace(tags); err != nil {
			return err
		}

		if err := tx.Model(group).Association("RelatedGroups").Replace(groups); err != nil {
			return err
		}

		return nil
	}); err != nil {
		return nil, err
	}

	ctx.syncMentionsForGroup(group)

	ctx.Logger().Info(models.LogActionUpdate, "group", &group.ID, group.Name, "Updated group", nil)

	ctx.RunAfterPluginHooks("after_group_update", map[string]any{
		"id":          float64(group.ID),
		"name":        group.Name,
		"description": group.Description,
		"meta":        string(group.Meta),
	})

	ctx.InvalidateSearchCacheByType(EntityTypeGroup)
	return group, nil
}

func (ctx *MahresourcesContext) GetGroup(id uint) (*models.Group, error) {
	var group models.Group

	// group_relations is not an owner-scoped table, so a group-limited principal
	// viewing an in-scope group could otherwise read its relations to (and the
	// IDs of) out-of-subtree groups. Confine each preloaded relation's far
	// endpoint to the subtree; fail-closed when the subtree is unresolvable.
	ids, scoped, deny := ctx.subtreeScopeIDs()
	relCond := func(farCol string) func(*gorm.DB) *gorm.DB {
		return func(tx *gorm.DB) *gorm.DB {
			switch {
			case deny:
				return tx.Where("1 = 0")
			case scoped:
				return tx.Where(farCol+" IN ?", ids)
			default:
				return tx
			}
		}
	}

	err := ctx.db.
		Preload("OwnGroups", pageLimit).
		Preload("OwnGroups.Category").
		Preload("OwnResources", pageLimitCustom(5)).
		Preload("OwnNotes", pageLimit).
		Preload("RelatedResources", pageLimitCustom(5)).
		Preload("RelatedNotes", pageLimit).
		Preload("RelatedGroups", pageLimit).
		Preload("Tags").
		Preload("Owner").
		Preload("Category", pageLimit).
		Preload("Relationships", relCond("to_group_id")).
		Preload("Relationships.ToGroup").
		Preload("Relationships.RelationType").
		Preload("BackRelations", relCond("from_group_id")).
		Preload("BackRelations.FromGroup").
		Preload("BackRelations.RelationType").
		First(&group, id).Error

	return &group, err
}

// GetGroupByID returns a group without preloading associations.
// Use this for internal operations that only need the group entity itself.
func (ctx *MahresourcesContext) GetGroupByID(id uint) (*models.Group, error) {
	var group models.Group
	return &group, ctx.db.First(&group, id).Error
}

func (ctx *MahresourcesContext) GetGroups(offset, maxResults int, query *query_models.GroupQuery) ([]models.Group, error) {
	var groups []models.Group
	groupScope := database_scopes.GroupQuery(query, false, ctx.db)

	db := ctx.db.Scopes(groupScope)
	db, err := ctx.applyMRQLFilter(db, mrql.EntityGroup, query.MRQL)
	if err != nil {
		return nil, err
	}

	return groups, db.Limit(maxResults).
		Offset(offset).Preload("Tags").Preload("Category").Find(&groups).Error
}

func (ctx *MahresourcesContext) GetGroupsWithIds(ids *[]uint) ([]*models.Group, error) {
	var groups []*models.Group

	if len(*ids) == 0 {
		return groups, nil
	}

	return groups, ctx.db.Preload("Category").Find(&groups, ids).Error
}

func (ctx *MahresourcesContext) GetGroupsCount(query *query_models.GroupQuery) (int64, error) {
	var group models.Group
	var count int64

	db := ctx.db.Scopes(database_scopes.GroupQuery(query, true, ctx.db)).Model(&group)
	db, err := ctx.applyMRQLFilter(db, mrql.EntityGroup, query.MRQL)
	if err != nil {
		return 0, err
	}

	return count, db.Count(&count).Error
}

func (ctx *MahresourcesContext) GetPopularGroupTags(query *query_models.GroupQuery) ([]PopularTag, error) {
	var res []PopularTag

	db := ctx.db.Table("groups").
		Scopes(database_scopes.GroupQuery(query, true, ctx.db)).
		Joins("INNER JOIN group_tags pt ON pt.group_id = groups.id").
		Joins("INNER JOIN tags t ON t.id = pt.tag_id").
		Select("t.id AS id, t.name AS name, count(*) AS count").
		Group("t.id, t.name").
		Order("count DESC").
		Limit(20)

	db, err := ctx.applyMRQLFilter(db, mrql.EntityGroup, query.MRQL)
	if err != nil {
		return nil, err
	}

	return res, db.Scan(&res).Error
}

type groupDeleteEffect struct {
	ID   uint
	Name string
}

type groupDeleteEffectSink interface {
	LogDeleted(*MahresourcesContext, groupDeleteEffect)
	RunAfterHook(*MahresourcesContext, groupDeleteEffect)
	InvalidateCache(*MahresourcesContext, groupDeleteEffect)
}

type defaultGroupDeleteEffectSink struct{}

func (defaultGroupDeleteEffectSink) LogDeleted(ctx *MahresourcesContext, event groupDeleteEffect) {
	ctx.Logger().Info(models.LogActionDelete, "group", &event.ID, event.Name, "Deleted group", nil)
}
func (defaultGroupDeleteEffectSink) RunAfterHook(ctx *MahresourcesContext, event groupDeleteEffect) {
	ctx.RunAfterPluginHooks("after_group_delete", map[string]any{"id": float64(event.ID), "name": event.Name})
}
func (defaultGroupDeleteEffectSink) InvalidateCache(ctx *MahresourcesContext, _ groupDeleteEffect) {
	ctx.InvalidateSearchCacheByType(EntityTypeGroup)
}

func (ctx *MahresourcesContext) emitGroupDeleteEffects(events []groupDeleteEffect) {
	sink := ctx.groupDeleteEffectSink
	if sink == nil {
		sink = defaultGroupDeleteEffectSink{}
	}
	for _, event := range events {
		sink.LogDeleted(ctx, event)
		sink.RunAfterHook(ctx, event)
		sink.InvalidateCache(ctx, event)
	}
}

func (ctx *MahresourcesContext) prepareGroupDelete(groupID uint) error {
	_, err := ctx.RunBeforePluginHooks("before_group_delete", map[string]any{"id": float64(groupID)})
	return err
}

// deleteGroupInTransaction performs only database work and returns the effect
// payload to emit after the owning transaction commits.
func (ctx *MahresourcesContext) deleteGroupInTransaction(groupID uint) (groupDeleteEffect, error) {
	group, err := ctx.lockScopeGroup(ctx.db, groupID, "delete")
	if err != nil {
		return groupDeleteEffect{}, err
	}
	ctx.EnsureForeignKeysActive(ctx.db)
	if err := rejectGroupDeletionIfUserScope(ctx.db, groupID); err != nil {
		return groupDeleteEffect{}, err
	}

	if err := ctx.db.Model(&models.Group{}).Where("owner_id = ?", groupID).Update("owner_id", nil).Error; err != nil {
		return groupDeleteEffect{}, err
	}
	if err := ctx.db.Model(&models.Note{}).Where("owner_id = ?", groupID).Update("owner_id", nil).Error; err != nil {
		return groupDeleteEffect{}, err
	}
	if err := ctx.db.Model(&models.Resource{}).Where("owner_id = ?", groupID).Update("owner_id", nil).Error; err != nil {
		return groupDeleteEffect{}, err
	}
	if err := ctx.db.Exec("DELETE FROM group_related_groups WHERE related_group_id = ?", groupID).Error; err != nil {
		return groupDeleteEffect{}, err
	}
	if err := ctx.db.
		Select("RelatedResources", "RelatedNotes", "RelatedGroups", "Relationships", "BackRelations", "Tags").
		Delete(group).Error; err != nil {
		return groupDeleteEffect{}, err
	}
	if err := ScrubGroupFromBlocks(ctx.db, groupID); err != nil {
		return groupDeleteEffect{}, err
	}
	return groupDeleteEffect{ID: groupID, Name: group.Name}, nil
}

func (ctx *MahresourcesContext) DeleteGroup(groupID uint) error {
	if err := ctx.prepareGroupDelete(groupID); err != nil {
		return err
	}
	var event groupDeleteEffect
	err := ctx.WithTransaction(func(txCtx *MahresourcesContext) error {
		if err := txCtx.lockUserManagementMutation(txCtx.db); err != nil {
			return err
		}
		var err error
		event, err = txCtx.deleteGroupInTransaction(groupID)
		return err
	})
	if err != nil {
		return err
	}
	ctx.emitGroupDeleteEffects([]groupDeleteEffect{event})
	return nil
}

// deleteRelationsSparingUnseen deletes the edges `query` selects, except any
// whose far endpoint the caller cannot see.
//
// nearGroupID is the endpoint the caller is acting on; the other column is the
// far one. For an unscoped principal visibleGroupIDs reports every id visible,
// so every selected edge is deleted and behaviour is unchanged.
//
// Selecting first and deleting by id, rather than appending a subtree predicate
// to the DELETE, keeps the bound parameter list the size of one group's edge
// degree instead of the size of its subtree.
func (ctx *MahresourcesContext) deleteRelationsSparingUnseen(tx *gorm.DB, nearGroupID uint, query string, args ...any) error {
	var edges []models.GroupRelation
	if err := tx.Where(query, args...).Find(&edges).Error; err != nil {
		return err
	}
	if len(edges) == 0 {
		return nil
	}

	endpoints := make([]uint, 0, len(edges)*2)
	for _, edge := range edges {
		if edge.FromGroupId != nil {
			endpoints = append(endpoints, *edge.FromGroupId)
		}
		if edge.ToGroupId != nil {
			endpoints = append(endpoints, *edge.ToGroupId)
		}
	}
	visible := ctx.visibleGroupIDs(endpoints)

	deletable := make([]uint, 0, len(edges))
	for _, edge := range edges {
		far := edge.ToGroupId
		if far != nil && *far == nearGroupID {
			far = edge.FromGroupId
		}
		// A dangling endpoint is not a group the caller is being protected
		// from; treat it as deletable so cleanup still collects the row.
		if far == nil || visible[*far] {
			deletable = append(deletable, edge.ID)
		}
	}
	if len(deletable) == 0 {
		return nil
	}

	return tx.Where("id IN ?", deletable).Delete(&models.GroupRelation{}).Error
}
