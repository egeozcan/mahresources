package interfaces

import (
	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/query_models"
)

type ResourceWriter interface {
	AddResource(file application_context.File, fileName string, resourceQuery *query_models.ResourceCreator) (*models.Resource, error)
	AddLocalResource(fileName string, resourceQuery *query_models.ResourceFromLocalCreator) (*models.Resource, error)
	AddRemoteResource(resourceQuery *query_models.ResourceFromRemoteCreator) (*models.Resource, error)
	EditResource(resourceQuery *query_models.ResourceEditor) (*models.Resource, error)
	BulkRemoveTagsFromResources(query *query_models.BulkEditQuery) error
	BulkReplaceTagsFromResources(query *query_models.BulkEditQuery) error
	BulkAddMetaToResources(query *query_models.BulkEditMetaQuery) error
	BulkAddTagsToResources(query *query_models.BulkEditQuery) error
	BulkAddGroupsToResources(query *query_models.BulkEditQuery) error
	BulkDeleteResources(query *query_models.BulkQuery) error
	MergeResources(winnerId uint, loserIds []uint) error
}

type ResourceReader interface {
	GetResource(id uint) (*models.Resource, error)
	GetResources(offset int, maxResults int, h *query_models.ResourceSearchQuery) (*[]models.Resource, error)
}

type ResourceDeleter interface {
	DeleteResource(resourceId uint) error
}
