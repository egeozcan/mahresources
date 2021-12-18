package application_context

import (
	"errors"
	"gorm.io/gorm"
	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
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

	group := models.Group{
		Name:        groupQuery.Name,
		Description: groupQuery.Description,
		CategoryId:  &groupQuery.CategoryId,
		Meta:        []byte(groupQuery.Meta),
	}

	if groupQuery.OwnerId != 0 {
		group.OwnerId = &groupQuery.OwnerId
	}

	if err := tx.Create(&group).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	if len(groupQuery.Tags) > 0 {
		tags := make([]models.Tag, len(groupQuery.Tags))
		for i, v := range groupQuery.Tags {
			tags[i] = models.Tag{
				ID: v,
			}
		}

		if createTagsErr := tx.Model(&group).Association("Tags").Append(&tags); createTagsErr != nil {
			tx.Rollback()
			return nil, createTagsErr
		}
	}

	if len(groupQuery.Groups) > 0 {
		groups := make([]models.Group, len(groupQuery.Groups))
		for i, v := range groupQuery.Groups {
			groups[i] = models.Group{
				ID: v,
			}
		}

		if createGroupsErr := tx.Model(&group).Association("RelatedGroups").Append(&groups); createGroupsErr != nil {
			tx.Rollback()
			return nil, createGroupsErr
		}
	}

	return &group, tx.Commit().Error
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

	groups := make([]*models.Group, len(groupQuery.Groups))

	for i, group := range groupQuery.Groups {
		groups[i] = &models.Group{
			ID: group,
		}
	}

	tags := make([]*models.Tag, len(groupQuery.Tags))

	for i, tag := range groupQuery.Tags {
		tags[i] = &models.Tag{
			ID: tag,
		}
	}

	if groupQuery.Meta == "" {
		groupQuery.Meta = "{}"
	}

	group := &models.Group{
		ID:          groupQuery.ID,
		Name:        groupQuery.Name,
		Description: groupQuery.Description,
		Meta:        []byte(groupQuery.Meta),
	}

	if groupQuery.OwnerId != 0 {
		group.OwnerId = &groupQuery.OwnerId
	} else if err := tx.Model(group).Association("Owner").Clear(); err != nil {
		tx.Rollback()
		return nil, err
	}

	if err := tx.Model(group).Updates(group).Error; err != nil {
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

	return group, tx.Commit().Error
}

func (ctx *MahresourcesContext) GetGroup(id uint) (*models.Group, error) {
	var group models.Group

	err := ctx.db.
		Preload("OwnGroups", pageLimit).
		Preload("OwnGroups.Category", pageLimit).
		Preload("OwnResources", pageLimit).
		Preload("OwnNotes", pageLimit).
		Preload("RelatedResources", pageLimit).
		Preload("RelatedNotes", pageLimit).
		Preload("RelatedGroups", pageLimit).
		Preload("Tags").
		Preload("Owner").
		Preload("Owner.Category").
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

func (ctx *MahresourcesContext) GetGroups(offset, maxResults int, query *query_models.GroupQuery) (*[]models.Group, error) {
	var groups []models.Group
	groupScope := database_scopes.GroupQuery(query)

	return &groups, ctx.db.Scopes(groupScope).Limit(maxResults).
		Offset(offset).Preload("Tags").Preload("Category").Find(&groups).Error
}

func (ctx *MahresourcesContext) GetGroupsWithIds(ids *[]uint) (*[]*models.Group, error) {
	var groups []*models.Group

	if len(*ids) == 0 {
		return &groups, nil
	}

	return &groups, ctx.db.Preload("Category").Find(&groups, ids).Error
}

func (ctx *MahresourcesContext) GetGroupsCount(query *query_models.GroupQuery) (int64, error) {
	var group models.Group
	var count int64

	return count, ctx.db.Scopes(database_scopes.GroupQuery(query)).Model(&group).Count(&count).Error
}

func (ctx *MahresourcesContext) DeleteGroup(groupId uint) error {
	group := models.Group{ID: groupId}

	return ctx.db.Transaction(func(tx *gorm.DB) error {
		ctx.EnsureForeignKeysActive(tx)

		return tx.
			Select("OwnGroups").
			Select("OwnNotes").
			Select("RelatedResources").
			Select("RelatedNotes").
			Select("RelatedGroups").
			Select("Relationships").
			Select("BackRelations").
			Select("Tags").
			Delete(&group).Error
	})
}

func (ctx *MahresourcesContext) GroupMetaKeys() (*[]fieldResult, error) {
	return metaKeys(ctx, "groups")
}
