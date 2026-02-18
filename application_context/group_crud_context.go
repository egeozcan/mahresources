package application_context

import (
	"errors"
	"net/url"

	"gorm.io/gorm"
	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
	"mahresources/models/types"
)

func (ctx *MahresourcesContext) CreateGroup(groupQuery *query_models.GroupCreator) (*models.Group, error) {
	if groupQuery.Name == "" {
		return nil, errors.New("group name needed")
	}

	if groupQuery.Meta == "" {
		groupQuery.Meta = "{}"
	}

	tx := ctx.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	parsedURL, err := url.Parse(groupQuery.URL)

	if groupQuery.URL != "" && err != nil {
		return nil, err
	}

	groupUrl := (*types.URL)(parsedURL)

	group := models.Group{
		Name:        groupQuery.Name,
		Description: groupQuery.Description,
		CategoryId:  &groupQuery.CategoryId,
		Meta:        []byte(groupQuery.Meta),
		URL:         groupUrl,
	}

	if groupQuery.OwnerId != 0 {
		group.OwnerId = &groupQuery.OwnerId
	}

	if err := tx.Create(&group).Error; err != nil {
		tx.Rollback()
		return nil, err
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

	ctx.Logger().Info(models.LogActionCreate, "group", &group.ID, group.Name, "Created group", nil)

	ctx.InvalidateSearchCacheByType(EntityTypeGroup)
	return &group, nil
}

func (ctx *MahresourcesContext) UpdateGroup(groupQuery *query_models.GroupEditor) (*models.Group, error) {
	if groupQuery.Name == "" {
		return nil, errors.New("group name needed")
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

	group := &models.Group{
		ID:          groupQuery.ID,
		Name:        groupQuery.Name,
		Description: groupQuery.Description,
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
		group.OwnerId = &groupQuery.OwnerId
		group.Owner = &models.Group{ID: groupQuery.OwnerId}
	} else if err := tx.Model(group).Association("Owner").Clear(); err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Model(group).Select("Name", "Description", "Meta", "URL", "OwnerId", "Owner").Updates(group).Error; err != nil {
		tx.Rollback()
		return nil, err
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

	ctx.Logger().Info(models.LogActionUpdate, "group", &group.ID, group.Name, "Updated group", nil)

	ctx.InvalidateSearchCacheByType(EntityTypeGroup)
	return group, nil
}

func (ctx *MahresourcesContext) GetGroup(id uint) (*models.Group, error) {
	var group models.Group

	err := ctx.db.
		Preload("OwnGroups", pageLimit).
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
	// Load group name before deletion for audit log
	var group models.Group
	if err := ctx.db.First(&group, groupId).Error; err != nil {
		return err
	}
	groupName := group.Name

	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		ctx.EnsureForeignKeysActive(tx)

		return tx.
			Select("OwnGroups", "OwnNotes", "RelatedResources", "RelatedNotes", "RelatedGroups", "Relationships", "BackRelations", "Tags").
			Delete(&group).Error
	})
	if err == nil {
		ctx.Logger().Info(models.LogActionDelete, "group", &groupId, groupName, "Deleted group", nil)
		ctx.InvalidateSearchCacheByType(EntityTypeGroup)
	}
	return err
}
