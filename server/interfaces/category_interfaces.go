package interfaces

import (
	"mahresources/models"
	"mahresources/models/query_models"
)

type CategoryReader interface {
	GetCategories(offset, maxResults int, query *query_models.CategoryQuery) (*[]models.Category, error)
}

type CategoryWriter interface {
	UpdateCategory(categoryEditor *query_models.CategoryEditor) (*models.Category, error)
	CreateCategory(categoryCreator *query_models.CategoryCreator) (*models.Category, error)
}

type CategoryDeleter interface {
	DeleteCategory(categoryId uint) error
}

type ResourceCategoryReader interface {
	GetResourceCategories(offset, maxResults int, query *query_models.ResourceCategoryQuery) (*[]models.ResourceCategory, error)
}

type ResourceCategoryWriter interface {
	UpdateResourceCategory(query *query_models.ResourceCategoryEditor) (*models.ResourceCategory, error)
	CreateResourceCategory(query *query_models.ResourceCategoryCreator) (*models.ResourceCategory, error)
}

type ResourceCategoryDeleter interface {
	DeleteResourceCategory(resourceCategoryId uint) error
}
