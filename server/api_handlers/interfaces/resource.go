package interfaces

import (
	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/query_models"
)

type ResourceWriter interface {
	AddResource(file application_context.File, fileName string, resourceQuery *query_models.ResourceCreator) (*models.Resource, error)
	EditResource(resourceQuery *query_models.ResourceEditor) (*models.Resource, error)
}

type ResourceReader interface {
	GetResource(id uint) (*models.Resource, error)
	GetResources(i int, page int, h *query_models.ResourceQuery) (*[]models.Resource, error)
}

type ResourceDeleter interface {
	DeleteResource(resourceId uint) error
}
