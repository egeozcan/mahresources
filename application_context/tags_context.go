package application_context

import (
	"errors"

	"gorm.io/gorm/clause"
	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
	"strings"
)

func (ctx *MahresourcesContext) GetTags(offset, maxResults int, query *query_models.TagQuery) ([]models.Tag, error) {
	var tags []models.Tag

	return tags, ctx.db.Scopes(database_scopes.TagQuery(query, false)).Limit(maxResults).Offset(offset).Find(&tags).Error
}

func (ctx *MahresourcesContext) GetTagsCount(query *query_models.TagQuery) (int64, error) {
	var tag models.Tag
	var count int64

	return count, ctx.db.Scopes(database_scopes.TagQuery(query, true)).Model(&tag).Count(&count).Error
}

func (ctx *MahresourcesContext) GetTag(id uint) (*models.Tag, error) {
	var tag models.Tag

	return &tag, ctx.db.Preload(clause.Associations, pageLimit).First(&tag, id).Error
}

func (ctx *MahresourcesContext) GetTagsWithIds(ids *[]uint, limit int) ([]models.Tag, error) {
	var tags []models.Tag

	if len(*ids) == 0 {
		return tags, nil
	}

	query := ctx.db

	if limit > 0 {
		query = query.Limit(limit)
	}

	return tags, query.Find(&tags, *ids).Error
}

func (ctx *MahresourcesContext) CreateTag(tagQuery *query_models.TagCreator) (*models.Tag, error) {
	if strings.TrimSpace(tagQuery.Name) == "" {
		return nil, errors.New("tag name must be non-empty")
	}

	tag := models.Tag{
		Name:        tagQuery.Name,
		Description: tagQuery.Description,
	}

	if err := ctx.db.Create(&tag).Error; err != nil {
		return nil, err
	}

	ctx.Logger().Info(models.LogActionCreate, "tag", &tag.ID, tag.Name, "Created tag", nil)

	ctx.InvalidateSearchCacheByType(EntityTypeTag)
	return &tag, nil
}

func (ctx *MahresourcesContext) UpdateTag(tagQuery *query_models.TagCreator) (*models.Tag, error) {
	var tag models.Tag
	if err := ctx.db.First(&tag, tagQuery.ID).Error; err != nil {
		return nil, err
	}

	if strings.TrimSpace(tagQuery.Name) != "" {
		tag.Name = tagQuery.Name
	}
	tag.Description = tagQuery.Description

	if err := ctx.db.Save(&tag).Error; err != nil {
		return nil, err
	}

	ctx.Logger().Info(models.LogActionUpdate, "tag", &tag.ID, tag.Name, "Updated tag", nil)

	ctx.InvalidateSearchCacheByType(EntityTypeTag)
	return &tag, nil
}

func (ctx *MahresourcesContext) DeleteTag(tagId uint) error {
	// Load tag name before deletion for audit log
	var tag models.Tag
	if err := ctx.db.First(&tag, tagId).Error; err != nil {
		return err
	}
	tagName := tag.Name

	err := ctx.db.Select(clause.Associations).Delete(&tag).Error
	if err == nil {
		ctx.Logger().Info(models.LogActionDelete, "tag", &tagId, tagName, "Deleted tag", nil)
		ctx.InvalidateSearchCacheByType(EntityTypeTag)
	}
	return err
}
