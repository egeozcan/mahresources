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

func (ctx *MahresourcesContext) GetCategories(offset, maxResults int, query *query_models.CategoryQuery) (*[]models.Category, error) {
	var categories []models.Category
	scope := database_scopes.CategoryQuery(query)

	return &categories, ctx.db.Scopes(scope).Limit(maxResults).Offset(offset).Find(&categories).Error
}

func (ctx *MahresourcesContext) GetCategoriesCount(query *query_models.CategoryQuery) (int64, error) {
	var category models.Category
	var count int64

	return count, ctx.db.Scopes(database_scopes.CategoryQuery(query)).Model(&category).Count(&count).Error
}

func (ctx *MahresourcesContext) GetCategoriesWithIds(ids *[]uint, limit int) (*[]models.Category, error) {
	var categories []models.Category

	if len(*ids) == 0 {
		return &categories, nil
	}

	query := ctx.db

	if limit > 0 {
		query = query.Limit(limit)
	}

	return &categories, query.Find(&categories, *ids).Error
}

func (ctx *MahresourcesContext) CreateCategory(categoryQuery *query_models.CategoryCreator) (*models.Category, error) {
	if strings.TrimSpace(categoryQuery.Name) == "" {
		return nil, errors.New("category name must be non-empty")
	}

	category := models.Category{
		Name:        categoryQuery.Name,
		Description: categoryQuery.Description,
	}

	return &category, ctx.db.Create(&category).Error
}

func (ctx *MahresourcesContext) UpdateCategory(categoryQuery *query_models.CategoryEditor) (*models.Category, error) {
	if strings.TrimSpace(categoryQuery.Name) == "" {
		return nil, errors.New("category name must be non-empty")
	}

	category := models.Category{
		ID:          categoryQuery.ID,
		Name:        categoryQuery.Name,
		Description: categoryQuery.Description,
	}

	return &category, ctx.db.Save(&category).Error
}

func (ctx *MahresourcesContext) DeleteCategory(categoryId uint) error {
	category := models.Category{ID: categoryId}

	return ctx.db.Select(clause.Associations).Delete(&category).Error
}
