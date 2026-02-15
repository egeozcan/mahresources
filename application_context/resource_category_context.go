package application_context

import (
	"errors"
	"gorm.io/gorm/clause"
	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
	"strings"
)

func (ctx *MahresourcesContext) GetResourceCategory(id uint) (*models.ResourceCategory, error) {
	var resourceCategory models.ResourceCategory

	return &resourceCategory, ctx.db.Preload(clause.Associations, pageLimit).First(&resourceCategory, id).Error
}

func (ctx *MahresourcesContext) GetResourceCategories(offset, maxResults int, query *query_models.ResourceCategoryQuery) ([]models.ResourceCategory, error) {
	var resourceCategories []models.ResourceCategory
	scope := database_scopes.ResourceCategoryQuery(query)

	return resourceCategories, ctx.db.Scopes(scope).Limit(maxResults).Offset(offset).Find(&resourceCategories).Error
}

func (ctx *MahresourcesContext) GetResourceCategoriesCount(query *query_models.ResourceCategoryQuery) (int64, error) {
	var resourceCategory models.ResourceCategory
	var count int64

	return count, ctx.db.Scopes(database_scopes.ResourceCategoryQuery(query)).Model(&resourceCategory).Count(&count).Error
}

func (ctx *MahresourcesContext) GetResourceCategoriesWithIds(ids *[]uint, limit int) ([]models.ResourceCategory, error) {
	var resourceCategories []models.ResourceCategory

	if len(*ids) == 0 {
		return resourceCategories, nil
	}

	query := ctx.db

	if limit > 0 {
		query = query.Limit(limit)
	}

	return resourceCategories, query.Find(&resourceCategories, *ids).Error
}

func (ctx *MahresourcesContext) CreateResourceCategory(query *query_models.ResourceCategoryCreator) (*models.ResourceCategory, error) {
	if strings.TrimSpace(query.Name) == "" {
		return nil, errors.New("resource category name must be non-empty")
	}

	resourceCategory := models.ResourceCategory{
		Name:          query.Name,
		Description:   query.Description,
		CustomHeader:  query.CustomHeader,
		CustomSidebar: query.CustomSidebar,
		CustomSummary: query.CustomSummary,
		CustomAvatar:  query.CustomAvatar,
		MetaSchema:    query.MetaSchema,
	}

	if err := ctx.db.Create(&resourceCategory).Error; err != nil {
		return nil, err
	}

	ctx.Logger().Info(models.LogActionCreate, "resourceCategory", &resourceCategory.ID, resourceCategory.Name, "Created resource category", nil)

	ctx.InvalidateSearchCacheByType(EntityTypeResourceCategory)
	return &resourceCategory, nil
}

func (ctx *MahresourcesContext) UpdateResourceCategory(query *query_models.ResourceCategoryEditor) (*models.ResourceCategory, error) {
	var resourceCategory models.ResourceCategory
	if err := ctx.db.First(&resourceCategory, query.ID).Error; err != nil {
		return nil, err
	}

	if strings.TrimSpace(query.Name) != "" {
		resourceCategory.Name = query.Name
	}
	resourceCategory.Description = query.Description
	resourceCategory.CustomHeader = query.CustomHeader
	resourceCategory.CustomSidebar = query.CustomSidebar
	resourceCategory.CustomSummary = query.CustomSummary
	resourceCategory.CustomAvatar = query.CustomAvatar
	resourceCategory.MetaSchema = query.MetaSchema

	if err := ctx.db.Save(&resourceCategory).Error; err != nil {
		return nil, err
	}

	ctx.Logger().Info(models.LogActionUpdate, "resourceCategory", &resourceCategory.ID, resourceCategory.Name, "Updated resource category", nil)

	ctx.InvalidateSearchCacheByType(EntityTypeResourceCategory)
	return &resourceCategory, nil
}

func (ctx *MahresourcesContext) DeleteResourceCategory(resourceCategoryId uint) error {
	var resourceCategory models.ResourceCategory
	if err := ctx.db.First(&resourceCategory, resourceCategoryId).Error; err != nil {
		return err
	}
	resourceCategoryName := resourceCategory.Name

	err := ctx.db.Select(clause.Associations).Delete(&resourceCategory).Error
	if err == nil {
		ctx.Logger().Info(models.LogActionDelete, "resourceCategory", &resourceCategoryId, resourceCategoryName, "Deleted resource category", nil)
		ctx.InvalidateSearchCacheByType(EntityTypeResourceCategory)
	}
	return err
}
