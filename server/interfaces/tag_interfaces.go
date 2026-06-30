package interfaces

import (
	"mahresources/models"
	"mahresources/models/query_models"
)

type TagsWriter interface {
	UpdateTag(t *query_models.TagCreator) (*models.Tag, error)
	CreateTag(t *query_models.TagCreator) (*models.Tag, error)
	GetTagByID(id uint) (*models.Tag, error)
	GetTagByName(name string) (*models.Tag, error)
	// PreviewTagCreateName resolves the name a CreateTag call would actually attempt
	// to persist, after validation and the before_tag_create hook. See
	// MahresourcesContext.PreviewTagCreateName for why this exists separately from
	// CreateTag itself.
	PreviewTagCreateName(name, description string) (string, error)
}

type TagsReader interface {
	GetTags(i int, results int, h *query_models.TagQuery) ([]models.Tag, error)
}

type TagDeleter interface {
	DeleteTag(tagId uint) error
}

// TagMerger handles tag merging operations
type TagMerger interface {
	MergeTags(winnerId uint, loserIds []uint) error
}

// BulkTagDeleter handles bulk tag deletion
type BulkTagDeleter interface {
	BulkDeleteTags(query *query_models.BulkQuery) error
}
