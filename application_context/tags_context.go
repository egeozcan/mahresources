package application_context

import (
	"errors"
	"fmt"

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

// GetTagByID returns a tag without preloading associations.
// Use this for internal operations that only need the tag entity itself.
func (ctx *MahresourcesContext) GetTagByID(id uint) (*models.Tag, error) {
	var tag models.Tag
	return &tag, ctx.db.First(&tag, id).Error
}

// GetTagByName returns the tag with an exact-match name. Used to resolve an
// idempotent CreateTag on a unique-name conflict so callers get the existing
// tag back instead of an error.
func (ctx *MahresourcesContext) GetTagByName(name string) (*models.Tag, error) {
	var tag models.Tag
	return &tag, ctx.db.Where("name = ?", name).First(&tag).Error
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

	if err := ValidateEntityName(tagQuery.Name, "tag"); err != nil {
		return nil, err
	}

	hookData := map[string]any{
		"id":          float64(0),
		"name":        tagQuery.Name,
		"description": tagQuery.Description,
	}
	hookData, hookErr := ctx.RunBeforePluginHooks("before_tag_create", hookData)
	if hookErr != nil {
		return nil, hookErr
	}
	if name, ok := hookData["name"].(string); ok {
		tagQuery.Name = name
	}
	if desc, ok := hookData["description"].(string); ok {
		tagQuery.Description = desc
	}

	tag := models.Tag{
		Name:        tagQuery.Name,
		Description: tagQuery.Description,
	}

	if err := ctx.db.Create(&tag).Error; err != nil {
		if isUniqueConstraintError(err) {
			// Idempotent create: a tag with this name already exists, so return it
			// rather than erroring. This lets the autocompleter "Add" a name that
			// exists but sits beyond the suggestion window resolve to the real tag.
			// Nothing was created, so skip the create hooks, audit log, and search
			// cache invalidation. Fall back to the friendly error only if the
			// existing row cannot be read back (so the raw constraint never leaks).
			if existing, lookupErr := ctx.GetTagByName(tag.Name); lookupErr == nil {
				return existing, nil
			}
			return nil, fmt.Errorf("a tag named %q already exists", tagQuery.Name)
		}
		return nil, err
	}

	ctx.Logger().Info(models.LogActionCreate, "tag", &tag.ID, tag.Name, "Created tag", nil)

	ctx.RunAfterPluginHooks("after_tag_create", map[string]any{
		"id":          float64(tag.ID),
		"name":        tag.Name,
		"description": tag.Description,
	})

	ctx.InvalidateSearchCacheByType(EntityTypeTag)
	return &tag, nil
}

func (ctx *MahresourcesContext) UpdateTag(tagQuery *query_models.TagCreator) (*models.Tag, error) {
	if err := ValidateEntityName(tagQuery.Name, "tag"); err != nil {
		return nil, err
	}

	hookData := map[string]any{
		"id":          float64(tagQuery.ID),
		"name":        tagQuery.Name,
		"description": tagQuery.Description,
	}
	hookData, hookErr := ctx.RunBeforePluginHooks("before_tag_update", hookData)
	if hookErr != nil {
		return nil, hookErr
	}
	if name, ok := hookData["name"].(string); ok {
		tagQuery.Name = name
	}
	if desc, ok := hookData["description"].(string); ok {
		tagQuery.Description = desc
	}

	var tag models.Tag
	if err := ctx.db.First(&tag, tagQuery.ID).Error; err != nil {
		return nil, err
	}

	if strings.TrimSpace(tagQuery.Name) != "" {
		tag.Name = tagQuery.Name
	}
	tag.Description = tagQuery.Description

	if err := ctx.db.Save(&tag).Error; err != nil {
		if isUniqueConstraintError(err) {
			return nil, fmt.Errorf("a tag named %q already exists", tag.Name)
		}
		return nil, err
	}

	ctx.Logger().Info(models.LogActionUpdate, "tag", &tag.ID, tag.Name, "Updated tag", nil)

	ctx.RunAfterPluginHooks("after_tag_update", map[string]any{
		"id":          float64(tag.ID),
		"name":        tag.Name,
		"description": tag.Description,
	})

	ctx.InvalidateSearchCacheByType(EntityTypeTag)
	return &tag, nil
}

func (ctx *MahresourcesContext) DeleteTag(tagId uint) error {
	_, hookErr := ctx.RunBeforePluginHooks("before_tag_delete", map[string]any{"id": float64(tagId)})
	if hookErr != nil {
		return hookErr
	}

	// Load tag name before deletion for audit log
	var tag models.Tag
	if err := ctx.db.First(&tag, tagId).Error; err != nil {
		return err
	}
	tagName := tag.Name

	err := ctx.db.Select(clause.Associations).Delete(&tag).Error
	if err == nil {
		ctx.Logger().Info(models.LogActionDelete, "tag", &tagId, tagName, "Deleted tag", nil)
		ctx.RunAfterPluginHooks("after_tag_delete", map[string]any{"id": float64(tagId), "name": tagName})
		ctx.InvalidateSearchCacheByType(EntityTypeTag)
	}
	return err
}
