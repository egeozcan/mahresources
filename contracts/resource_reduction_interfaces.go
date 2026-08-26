package contracts

import (
	"mahresources/models"
	"mahresources/models/query_models"
)

// ResourceReductionReader provides read access to Resource Reductions.
//
// ownerUserID/ownerRestricted are the caller's visibility, resolved from the
// authenticated principal rather than from the request: administrators see every
// Reduction, every other principal sees only the ones it created, and a row with
// no owner belongs to nobody.
type ResourceReductionReader interface {
	GetResourceReductions(offset, maxResults int, query *query_models.ResourceReductionQuery) ([]models.ResourceReduction, error)
	GetResourceReductionCount(query *query_models.ResourceReductionQuery) (int64, error)
	GetResourceReduction(id uint, ownerUserID *uint, ownerRestricted bool) (*models.ResourceReduction, error)
}

// ResourceReductionWriter provides the mutations the Reduction pages perform.
type ResourceReductionWriter interface {
	CreateOrExtendResourceReduction(creator *query_models.ResourceReductionCreator, ownerUserID *uint, ownerRestricted bool) (*models.ResourceReduction, error)
	UpdateResourceReductionSettings(editor *query_models.ResourceReductionEditor, ownerUserID *uint, ownerRestricted bool) (*models.ResourceReduction, error)
	DeleteResourceReduction(id uint, ownerUserID *uint, ownerRestricted bool) error
}
