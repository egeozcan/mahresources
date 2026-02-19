package interfaces

import (
	"mahresources/models"
	"mahresources/models/query_models"
)

type TagsWriter interface {
	UpdateTag(t *query_models.TagCreator) (*models.Tag, error)
	CreateTag(t *query_models.TagCreator) (*models.Tag, error)
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
