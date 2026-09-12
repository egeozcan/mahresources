package application_context

import (
	"errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
	"mahresources/models/types"
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
	if err := ctx.requireTaxonomyRole("creating a resource category"); err != nil {
		return nil, err
	}
	if strings.TrimSpace(query.Name) == "" {
		return nil, errors.New("resource category name must be non-empty")
	}

	if err := ValidateEntityName(query.Name, "resource category"); err != nil {
		return nil, err
	}

	// Findings 17/93.
	if err := validateCategoryMetadataIndexes(query.MetadataIndexes, "resource"); err != nil {
		return nil, err
	}
	if err := ValidateMetaSchema(query.MetaSchema); err != nil {
		return nil, err
	}

	if err := ValidateAutoDetectRules(query.AutoDetectRules); err != nil {
		return nil, err
	}

	resourceCategory := models.ResourceCategory{
		Name:                        query.Name,
		Description:                 query.Description,
		CustomHeader:                query.CustomHeader,
		CustomHeaderCSS:             query.CustomHeaderCSS,
		CustomSidebar:               query.CustomSidebar,
		CustomSidebarCSS:            query.CustomSidebarCSS,
		CustomSummary:               query.CustomSummary,
		CustomSummaryCSS:            query.CustomSummaryCSS,
		CustomAvatar:                query.CustomAvatar,
		CustomAvatarCSS:             query.CustomAvatarCSS,
		CustomListHeader:            query.CustomListHeader,
		CustomListHeaderCSS:         query.CustomListHeaderCSS,
		CustomDetailFooter:          query.CustomDetailFooter,
		CustomDetailFooterCSS:       query.CustomDetailFooterCSS,
		CustomListFooter:            query.CustomListFooter,
		CustomListFooterCSS:         query.CustomListFooterCSS,
		CustomHoverCard:             query.CustomHoverCard,
		CustomHoverCardCSS:          query.CustomHoverCardCSS,
		CustomPreview:               query.CustomPreview,
		CustomPreviewCSS:            query.CustomPreviewCSS,
		CustomLightbox:              query.CustomLightbox,
		CustomLightboxCSS:           query.CustomLightboxCSS,
		CustomCell:                  query.CustomCell,
		CustomCellCSS:               query.CustomCellCSS,
		CustomMRQLResult:            query.CustomMRQLResult,
		CustomMRQLResultCSS:         query.CustomMRQLResultCSS,
		CustomEntityPickerResult:    query.CustomEntityPickerResult,
		CustomEntityPickerResultCSS: query.CustomEntityPickerResultCSS,
		CustomCSS:                   query.CustomCSS,
		MetaSchema:                  query.MetaSchema,
		MetadataIndexes:             metadataIndexesValue(query.MetadataIndexes),
		AutoDetectRules:             query.AutoDetectRules,
	}
	if query.SectionConfig != "" {
		resourceCategory.SectionConfig = types.JSON(query.SectionConfig)
	}

	if err := ctx.db.Create(&resourceCategory).Error; err != nil {
		return nil, friendlyUniqueError("resource category", err)
	}

	ctx.Logger().Info(models.LogActionCreate, "resourceCategory", &resourceCategory.ID, resourceCategory.Name, "Created resource category", nil)

	ctx.InvalidateSearchCacheByType(EntityTypeResourceCategory)
	return &resourceCategory, nil
}

func (ctx *MahresourcesContext) UpdateResourceCategory(query *query_models.ResourceCategoryEditor) (*models.ResourceCategory, error) {
	if err := ctx.requireTaxonomyRole("editing a resource category"); err != nil {
		return nil, err
	}
	var resourceCategory models.ResourceCategory
	if err := ctx.db.First(&resourceCategory, query.ID).Error; err != nil {
		return nil, err
	}

	// Findings 17/93.
	if err := validateCategoryMetadataIndexes(query.MetadataIndexes, "resource"); err != nil {
		return nil, err
	}
	if err := ValidateMetaSchema(query.MetaSchema); err != nil {
		return nil, err
	}

	if err := ValidateAutoDetectRules(query.AutoDetectRules); err != nil {
		return nil, err
	}

	if strings.TrimSpace(query.Name) != "" {
		resourceCategory.Name = query.Name
	}
	resourceCategory.Description = query.Description
	resourceCategory.CustomHeader = query.CustomHeader
	resourceCategory.CustomHeaderCSS = query.CustomHeaderCSS
	resourceCategory.CustomSidebar = query.CustomSidebar
	resourceCategory.CustomSidebarCSS = query.CustomSidebarCSS
	resourceCategory.CustomSummary = query.CustomSummary
	resourceCategory.CustomSummaryCSS = query.CustomSummaryCSS
	resourceCategory.CustomAvatar = query.CustomAvatar
	resourceCategory.CustomAvatarCSS = query.CustomAvatarCSS
	resourceCategory.CustomListHeader = query.CustomListHeader
	resourceCategory.CustomListHeaderCSS = query.CustomListHeaderCSS
	resourceCategory.CustomDetailFooter = query.CustomDetailFooter
	resourceCategory.CustomDetailFooterCSS = query.CustomDetailFooterCSS
	resourceCategory.CustomListFooter = query.CustomListFooter
	resourceCategory.CustomListFooterCSS = query.CustomListFooterCSS
	resourceCategory.CustomHoverCard = query.CustomHoverCard
	resourceCategory.CustomHoverCardCSS = query.CustomHoverCardCSS
	resourceCategory.CustomPreview = query.CustomPreview
	resourceCategory.CustomPreviewCSS = query.CustomPreviewCSS
	resourceCategory.CustomLightbox = query.CustomLightbox
	resourceCategory.CustomLightboxCSS = query.CustomLightboxCSS
	resourceCategory.CustomCell = query.CustomCell
	resourceCategory.CustomCellCSS = query.CustomCellCSS
	resourceCategory.CustomMRQLResult = query.CustomMRQLResult
	resourceCategory.CustomMRQLResultCSS = query.CustomMRQLResultCSS
	resourceCategory.CustomEntityPickerResult = query.CustomEntityPickerResult
	resourceCategory.CustomEntityPickerResultCSS = query.CustomEntityPickerResultCSS
	resourceCategory.CustomCSS = query.CustomCSS
	resourceCategory.MetaSchema = query.MetaSchema
	if query.MetadataIndexes != nil {
		resourceCategory.MetadataIndexes = *query.MetadataIndexes
	}
	resourceCategory.AutoDetectRules = query.AutoDetectRules
	if query.SectionConfig != "" {
		resourceCategory.SectionConfig = types.JSON(query.SectionConfig)
	}

	if err := ctx.db.Save(&resourceCategory).Error; err != nil {
		return nil, friendlyUniqueError("resource category", err)
	}

	ctx.Logger().Info(models.LogActionUpdate, "resourceCategory", &resourceCategory.ID, resourceCategory.Name, "Updated resource category", nil)

	ctx.InvalidateSearchCacheByType(EntityTypeResourceCategory)
	return &resourceCategory, nil
}

func (ctx *MahresourcesContext) DeleteResourceCategory(resourceCategoryId uint) error {
	if err := ctx.requireTaxonomyRole("deleting a resource category"); err != nil {
		return err
	}
	if resourceCategoryId == ctx.DefaultResourceCategoryID {
		return errors.New("cannot delete the default resource category")
	}

	var resourceCategory models.ResourceCategory
	if err := ctx.db.First(&resourceCategory, resourceCategoryId).Error; err != nil {
		return err
	}
	resourceCategoryName := resourceCategory.Name

	// Wrap in a transaction so that if the reassignment fails, nothing changes.
	err := ctx.db.Transaction(func(tx *gorm.DB) error {
		// Reassign resources to the default category instead of setting NULL.
		// Done explicitly since SQLite FK constraints don't fire reliably.
		if err := tx.Model(&models.Resource{}).Where("resource_category_id = ?", resourceCategoryId).Update("resource_category_id", ctx.DefaultResourceCategoryID).Error; err != nil {
			return err
		}

		return tx.Delete(&resourceCategory).Error
	})

	if err == nil {
		ctx.Logger().Info(models.LogActionDelete, "resourceCategory", &resourceCategoryId, resourceCategoryName, "Deleted resource category", nil)
		ctx.InvalidateSearchCacheByType(EntityTypeResourceCategory)
	}
	return err
}
