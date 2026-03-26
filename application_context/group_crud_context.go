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
)

func (ctx *MahresourcesContext) CreateGroup(groupQuery *query_models.GroupCreator) (*models.Group, error) {
	groupQuery.Name = strings.TrimSpace(groupQuery.Name)
	if groupQuery.Name == "" {
		return nil, errors.New("group name needed")
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

	tx := ctx.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

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

	if groupQuery.OwnerId != 0 {
		var ownerCheck models.Group
		if err := tx.Select("id").First(&ownerCheck, groupQuery.OwnerId).Error; err != nil {
			tx.Rollback()
			return nil, errors.New("owner group not found")
		}
		group.OwnerId = &groupQuery.OwnerId
	}

	if err := tx.Create(&group).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// Reject self-ownership (can only check after Create assigns the ID)
	if group.OwnerId != nil && *group.OwnerId == group.ID {
		tx.Rollback()
		return nil, errors.New("a group cannot be its own owner")
	}

	if len(groupQuery.Tags) > 0 {
		tags := BuildAssociationSlice(groupQuery.Tags, TagFromID)

		if createTagsErr := tx.Model(&group).Association("Tags").Append(&tags); createTagsErr != nil {
			tx.Rollback()
			return nil, createTagsErr
		}
	}

	if len(groupQuery.Groups) > 0 {
		groups := BuildAssociationSlice(groupQuery.Groups, GroupFromID)

		if createGroupsErr := tx.Model(&group).Association("RelatedGroups").Append(&groups); createGroupsErr != nil {
			tx.Rollback()
			return nil, createGroupsErr
		}
	}

	if err := tx.Commit().Error; err != nil {
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

	tx := ctx.db.Begin()

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	groups := BuildAssociationSlicePtr(groupQuery.Groups, GroupPtrFromID)
	tags := BuildAssociationSlicePtr(groupQuery.Tags, TagPtrFromID)

	if groupQuery.Meta == "" {
		groupQuery.Meta = "{}"
	}

	if err := ValidateMeta(groupQuery.Meta); err != nil {
		tx.Rollback()
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
			tx.Rollback()
			return nil, err
		}

		groupUrl := (*types.URL)(parsedURL)
		group.URL = groupUrl
	} else {
		group.URL = nil
	}

	if groupQuery.OwnerId != 0 {
		if groupQuery.OwnerId == groupQuery.ID {
			tx.Rollback()
			return nil, errors.New("a group cannot be its own owner")
		}
		// Verify the proposed owner exists, then walk up its ancestry to detect cycles.
		currentAncestor := groupQuery.OwnerId
		for i := 0; i < 100; i++ { // depth limit to prevent infinite loops
			var ancestor models.Group
			if err := tx.Select("id", "owner_id").First(&ancestor, currentAncestor).Error; err != nil {
				if i == 0 {
					// First iteration: the proposed owner itself doesn't exist
					tx.Rollback()
					return nil, errors.New("owner group not found")
				}
				break // further ancestor not found, no cycle
			}
			if ancestor.OwnerId == nil {
				break // reached a root group, no cycle
			}
			if *ancestor.OwnerId == groupQuery.ID {
				tx.Rollback()
				return nil, errors.New("setting this owner would create an ownership cycle")
			}
			currentAncestor = *ancestor.OwnerId
		}
		group.OwnerId = &groupQuery.OwnerId
		group.Owner = &models.Group{ID: groupQuery.OwnerId}
	} else if err := tx.Model(group).Association("Owner").Clear(); err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Model(group).Select("Name", "Description", "Meta", "URL", "OwnerId", "Owner", "CategoryId").Updates(group).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	// Clean up GroupRelation records that become invalid after category change.
	// A relation is invalid if the group's new category doesn't match what
	// the relation type requires for that position (from or to).
	newCategoryId := groupQuery.CategoryId
	if newCategoryId == 0 {
		// Category cleared: delete all relations where this group occupies a position
		// that has a non-NULL category constraint on the relation type.
		if err := tx.Where(
			"from_group_id = ? AND relation_type_id IN (SELECT id FROM group_relation_types WHERE from_category_id IS NOT NULL)",
			group.ID,
		).Delete(&models.GroupRelation{}).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
		if err := tx.Where(
			"to_group_id = ? AND relation_type_id IN (SELECT id FROM group_relation_types WHERE to_category_id IS NOT NULL)",
			group.ID,
		).Delete(&models.GroupRelation{}).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
	} else {
		// Category changed to a specific value: delete relations where the relation
		// type's category constraint for this group's position doesn't match.
		if err := tx.Where(
			"from_group_id = ? AND relation_type_id IN (SELECT id FROM group_relation_types WHERE from_category_id IS NOT NULL AND from_category_id != ?)",
			group.ID, newCategoryId,
		).Delete(&models.GroupRelation{}).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
		if err := tx.Where(
			"to_group_id = ? AND relation_type_id IN (SELECT id FROM group_relation_types WHERE to_category_id IS NOT NULL AND to_category_id != ?)",
			group.ID, newCategoryId,
		).Delete(&models.GroupRelation{}).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if err := tx.Model(group).Association("Tags").Replace(tags); err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Model(group).Association("RelatedGroups").Replace(groups); err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Commit().Error; err != nil {
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
		Preload("Relationships").
		Preload("Relationships.ToGroup").
		Preload("Relationships.RelationType").
		Preload("BackRelations").
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

	return groups, ctx.db.Scopes(groupScope).Limit(maxResults).
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

	return count, ctx.db.Scopes(database_scopes.GroupQuery(query, true, ctx.db)).Model(&group).Count(&count).Error
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

	return res, db.Scan(&res).Error
}

func (ctx *MahresourcesContext) DeleteGroup(groupId uint) error {
	_, hookErr := ctx.RunBeforePluginHooks("before_group_delete", map[string]any{"id": float64(groupId)})
	if hookErr != nil {
		return hookErr
	}

	// Load group name before deletion for audit log
	var group models.Group
	if err := ctx.db.First(&group, groupId).Error; err != nil {
		return err
	}
	groupName := group.Name

	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		ctx.EnsureForeignKeysActive(tx)

		// Explicitly clear owned entities' owner_id (SET NULL) since SQLite
		// PRAGMA foreign_keys is a no-op inside transactions, so FK constraints don't fire.
		// This covers groups, notes, and resources that have this group as owner.
		if err := tx.Model(&models.Group{}).Where("owner_id = ?", groupId).Update("owner_id", nil).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.Note{}).Where("owner_id = ?", groupId).Update("owner_id", nil).Error; err != nil {
			return err
		}
		if err := tx.Model(&models.Resource{}).Where("owner_id = ?", groupId).Update("owner_id", nil).Error; err != nil {
			return err
		}

		// Explicitly clean reverse side of group_related_groups join table
		// since SQLite FK cascades don't fire in transactions and GORM's
		// Select("RelatedGroups").Delete() only handles the owning side (group_id).
		if err := tx.Exec("DELETE FROM group_related_groups WHERE related_group_id = ?", groupId).Error; err != nil {
			return err
		}

		return tx.
			Select("RelatedResources", "RelatedNotes", "RelatedGroups", "Relationships", "BackRelations", "Tags").
			Delete(&group).Error
	})
	if err == nil {
		ctx.Logger().Info(models.LogActionDelete, "group", &groupId, groupName, "Deleted group", nil)
		ctx.RunAfterPluginHooks("after_group_delete", map[string]any{"id": float64(groupId), "name": groupName})
		ctx.InvalidateSearchCacheByType(EntityTypeGroup)
	}
	return err
}
