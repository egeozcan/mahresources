package interfaces

import (
	"context"
	"io"
	"mahresources/models"
	"mahresources/models/query_models"
)

type File interface {
	io.Reader
	io.Closer
}

// --- Granular Resource Writer Interfaces ---

// ResourceCreator handles resource creation operations
type ResourceCreator interface {
	AddResource(file File, fileName string, resourceQuery *query_models.ResourceCreator) (*models.Resource, error)
	AddLocalResource(fileName string, resourceQuery *query_models.ResourceFromLocalCreator) (*models.Resource, error)
	AddRemoteResource(resourceQuery *query_models.ResourceFromRemoteCreator) (*models.Resource, error)
}

// ResourceEditor handles resource editing operations
type ResourceEditor interface {
	EditResource(resourceQuery *query_models.ResourceEditor) (*models.Resource, error)
}

// BulkResourceTagEditor handles bulk tag operations on resources
type BulkResourceTagEditor interface {
	BulkAddTagsToResources(query *query_models.BulkEditQuery) error
	BulkRemoveTagsFromResources(query *query_models.BulkEditQuery) error
	BulkReplaceTagsFromResources(query *query_models.BulkEditQuery) error
}

// BulkResourceGroupEditor handles bulk group operations on resources
type BulkResourceGroupEditor interface {
	BulkAddGroupsToResources(query *query_models.BulkEditQuery) error
}

// BulkResourceMetaEditor handles bulk meta operations on resources
type BulkResourceMetaEditor interface {
	BulkAddMetaToResources(query *query_models.BulkEditMetaQuery) error
}

// BulkResourceDeleter handles bulk resource deletion
type BulkResourceDeleter interface {
	BulkDeleteResources(query *query_models.BulkQuery) error
}

// ResourceMerger handles resource merging operations
type ResourceMerger interface {
	MergeResources(winnerId uint, loserIds []uint) error
}

// ResourceMediaProcessor handles media operations on resources
type ResourceMediaProcessor interface {
	RotateResource(resourceId uint, degrees int) error
	RecalculateResourceDimensions(query *query_models.EntityIdQuery) error
	SetResourceDimensions(resourceId uint, width, height uint) error
}

// --- Composite Interface (backward compatibility) ---

// ResourceWriter combines all resource write operations
type ResourceWriter interface {
	ResourceCreator
	ResourceEditor
	BulkResourceTagEditor
	BulkResourceGroupEditor
	BulkResourceMetaEditor
	BulkResourceDeleter
	ResourceMerger
	ResourceMediaProcessor
}

type ResourceReader interface {
	GetResource(id uint) (*models.Resource, error)
	GetResources(offset int, maxResults int, h *query_models.ResourceSearchQuery) ([]models.Resource, error)
}

type ResourceDeleter interface {
	DeleteResource(resourceId uint) error
}

// ResourceMetaReader provides access to resource metadata keys
type ResourceMetaReader interface {
	ResourceMetaKeys() ([]MetaKey, error)
}

// ResourceThumbnailLoader handles thumbnail retrieval for resources
type ResourceThumbnailLoader interface {
	ResourceReader
	LoadOrCreateThumbnailForResource(resourceId, width, height uint, ctx context.Context) (*models.Preview, error)
}
