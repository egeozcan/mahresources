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

// ResourceEditReader combines editing with reading for partial-update support
type ResourceEditReader interface {
	ResourceEditor
	GetResource(id uint) (*models.Resource, error)
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
	MergeResources(winnerId uint, loserIds []uint, keepAsVersion bool) error
}

// ResourceMediaProcessor handles media operations on resources
type ResourceMediaProcessor interface {
	RotateResource(resourceId uint, degrees int) error
	CropResource(httpContext context.Context, resourceId uint, x, y, width, height int, comment string) error
	TrimVideo(httpContext context.Context, resourceId uint, start, end, comment string) error
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

// SuggestedTag is a single context-aware tag suggestion for a resource. Score
// and Sources are advisory (tooltips/telemetry); the frontend applies a chip
// using only ID and Name. The DTO lives here (not in application_context) so
// the interface below can reference it without an import cycle.
type SuggestedTag struct {
	ID      uint     `json:"ID"`
	Name    string   `json:"Name"`
	Score   float64  `json:"score"`
	Sources []string `json:"sources"`
}

// ResourceSuggestionReader exposes the context-aware tag suggestion ranking for
// a single resource. Implementations must fail closed under scoping: an
// out-of-subtree resource id yields a not-found error.
type ResourceSuggestionReader interface {
	GetSuggestedTags(resourceId uint, limit int) ([]SuggestedTag, error)
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
	LatestPreviewVersion(ctx context.Context, resourceId uint) uint
}

// ResourceThumbnailWriter handles custom-thumbnail uploads and reset.
type ResourceThumbnailWriter interface {
	SetCustomThumbnail(ctx context.Context, resourceId uint, reader io.Reader) error
	ClearThumbnails(ctx context.Context, resourceId uint) error
}
