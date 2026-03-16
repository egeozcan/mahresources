package application_context

import (
	"errors"
	"gorm.io/gorm/clause"
	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
	"strings"
)

func (ctx *MahresourcesContext) GetCategory(id uint) (*models.Category, error) {
	var category models.Category

	return &category, ctx.db.Preload(clause.Associations, pageLimit).First(&category, id).Error
}

func (ctx *MahresourcesContext) GetCategories(offset, maxResults int, query *query_models.CategoryQuery) ([]models.Category, error) {
	var categories []models.Category
	scope := database_scopes.CategoryQuery(query)

	return categories, ctx.db.Scopes(scope).Limit(maxResults).Offset(offset).Find(&categories).Error
}

func (ctx *MahresourcesContext) GetCategoriesCount(query *query_models.CategoryQuery) (int64, error) {
	var category models.Category
	var count int64

	return count, ctx.db.Scopes(database_scopes.CategoryQuery(query)).Model(&category).Count(&count).Error
}

func (ctx *MahresourcesContext) GetCategoriesWithIds(ids *[]uint, limit int) ([]models.Category, error) {
	var categories []models.Category

	if len(*ids) == 0 {
		return categories, nil
	}

	query := ctx.db

	if limit > 0 {
		query = query.Limit(limit)
	}

	return categories, query.Find(&categories, *ids).Error
}

func (ctx *MahresourcesContext) CreateCategory(categoryQuery *query_models.CategoryCreator) (*models.Category, error) {
	if strings.TrimSpace(categoryQuery.Name) == "" {
		return nil, errors.New("category name must be non-empty")
	}

	hookData := map[string]any{
		"id":          float64(0),
		"name":        categoryQuery.Name,
		"description": categoryQuery.Description,
	}
	hookData, hookErr := ctx.RunBeforePluginHooks("before_category_create", hookData)
	if hookErr != nil {
		return nil, hookErr
	}
	if name, ok := hookData["name"].(string); ok {
		categoryQuery.Name = name
	}
	if desc, ok := hookData["description"].(string); ok {
		categoryQuery.Description = desc
	}

	category := models.Category{
		Name:          categoryQuery.Name,
		Description:   categoryQuery.Description,
		CustomHeader:  categoryQuery.CustomHeader,
		CustomSidebar: categoryQuery.CustomSidebar,
		CustomSummary: categoryQuery.CustomSummary,
		CustomAvatar:  categoryQuery.CustomAvatar,
		MetaSchema:    categoryQuery.MetaSchema,
	}

	if err := ctx.db.Create(&category).Error; err != nil {
		return nil, err
	}

	ctx.Logger().Info(models.LogActionCreate, "category", &category.ID, category.Name, "Created category", nil)

	ctx.RunAfterPluginHooks("after_category_create", map[string]any{
		"id":          float64(category.ID),
		"name":        category.Name,
		"description": category.Description,
	})

	ctx.InvalidateSearchCacheByType(EntityTypeCategory)
	return &category, nil
}

func (ctx *MahresourcesContext) UpdateCategory(categoryQuery *query_models.CategoryEditor) (*models.Category, error) {
	hookData := map[string]any{
		"id":          float64(categoryQuery.ID),
		"name":        categoryQuery.Name,
		"description": categoryQuery.Description,
	}
	hookData, hookErr := ctx.RunBeforePluginHooks("before_category_update", hookData)
	if hookErr != nil {
		return nil, hookErr
	}
	if name, ok := hookData["name"].(string); ok {
		categoryQuery.Name = name
	}
	if desc, ok := hookData["description"].(string); ok {
		categoryQuery.Description = desc
	}

	var category models.Category
	if err := ctx.db.First(&category, categoryQuery.ID).Error; err != nil {
		return nil, err
	}

	if strings.TrimSpace(categoryQuery.Name) != "" {
		category.Name = categoryQuery.Name
	}
	if categoryQuery.Description != "" {
		category.Description = categoryQuery.Description
	}
	if categoryQuery.CustomHeader != "" {
		category.CustomHeader = categoryQuery.CustomHeader
	}
	if categoryQuery.CustomSidebar != "" {
		category.CustomSidebar = categoryQuery.CustomSidebar
	}
	if categoryQuery.CustomSummary != "" {
		category.CustomSummary = categoryQuery.CustomSummary
	}
	if categoryQuery.CustomAvatar != "" {
		category.CustomAvatar = categoryQuery.CustomAvatar
	}
	if categoryQuery.MetaSchema != "" {
		category.MetaSchema = categoryQuery.MetaSchema
	}

	if err := ctx.db.Save(&category).Error; err != nil {
		return nil, err
	}

	ctx.Logger().Info(models.LogActionUpdate, "category", &category.ID, category.Name, "Updated category", nil)

	ctx.RunAfterPluginHooks("after_category_update", map[string]any{
		"id":          float64(category.ID),
		"name":        category.Name,
		"description": category.Description,
	})

	ctx.InvalidateSearchCacheByType(EntityTypeCategory)
	return &category, nil
}

func (ctx *MahresourcesContext) DeleteCategory(categoryId uint) error {
	_, hookErr := ctx.RunBeforePluginHooks("before_category_delete", map[string]any{"id": float64(categoryId)})
	if hookErr != nil {
		return hookErr
	}

	// Load category name before deletion for audit log
	var category models.Category
	if err := ctx.db.First(&category, categoryId).Error; err != nil {
		return err
	}
	categoryName := category.Name

	// Do NOT use Select(clause.Associations) — Category's only association is
	// Groups, and deleting a category must SET NULL on groups (not cascade-delete them).
	// Explicitly clear CategoryId since SQLite FK constraints don't fire reliably.
	if err := ctx.db.Model(&models.Group{}).Where("category_id = ?", categoryId).Update("category_id", nil).Error; err != nil {
		return err
	}

	err := ctx.db.Delete(&category).Error
	if err == nil {
		ctx.Logger().Info(models.LogActionDelete, "category", &categoryId, categoryName, "Deleted category", nil)
		ctx.RunAfterPluginHooks("after_category_delete", map[string]any{"id": float64(categoryId), "name": categoryName})
		ctx.InvalidateSearchCacheByType(EntityTypeCategory)
	}
	return err
}
