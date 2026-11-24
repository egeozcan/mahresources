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
	// GetReductionProgress counts each of the given Reductions' Clusters by state,
	// over the Clusters the caller may fully see. The rows are the page the caller
	// already loaded, so nothing outside it is read. A Reduction whose plan is still
	// the legacy document has Upgrading set and no counts.
	GetReductionProgress(reductions []models.ResourceReduction) (map[uint]ReductionProgress, error)
	// ListResourceReductions is one list page: its rows, the count under the same
	// predicate and each row's progress, read in one snapshot so the three agree.
	ListResourceReductions(offset, maxResults int, query *query_models.ResourceReductionQuery) ([]models.ResourceReduction, int64, map[uint]ReductionProgress, error)
	// GetReductionClusterMembers is one page of a Cluster's members after a cursor
	// (a ReductionClusterView's MembersAfter), for a Cluster the review page cut
	// short. A Cluster the caller may not fully see answers as an unknown one.
	GetReductionClusterMembers(id uint, ownerUserID *uint, ownerRestricted bool, clusterKey, after string) (*ReductionMembersPage, error)
	// GetReductionReview is one page of a Reduction's plan with the Resources
	// behind it, filtered by the caller's Cluster query. Filtering happens before
	// pagination, so the page count and the ClusterCount in the review describe
	// what is shown.
	GetReductionReview(id uint, ownerUserID *uint, ownerRestricted bool, page int, query *query_models.ResourceReductionReviewQuery) (*ReductionReview, error)
}

// ResourceReductionWriter provides the mutations the Reduction pages perform.
type ResourceReductionWriter interface {
	CreateOrExtendResourceReduction(creator *query_models.ResourceReductionCreator, ownerUserID *uint, ownerRestricted bool) (*models.ResourceReduction, error)
	UpdateResourceReductionSettings(editor *query_models.ResourceReductionEditor, ownerUserID *uint, ownerRestricted bool) (*models.ResourceReduction, error)
	DeleteResourceReduction(id uint, ownerUserID *uint, ownerRestricted bool) error
	// RequestReductionCompute starts the clustering job. actorUserID is the
	// submitting user, named on the job at construction so its progress reaches
	// that user's own jobs panel — an ownerless job is invisible to every
	// non-admin, including the one who asked for it.
	RequestReductionCompute(id uint, version uint, ownerUserID *uint, ownerRestricted bool, actorUserID *uint) (*models.ResourceReduction, error)
	// OverrideReductionCluster records one review decision — promote, eject,
	// restore, skip, reopen, check or uncheck.
	OverrideReductionCluster(override *query_models.ReductionOverride, ownerUserID *uint, ownerRestricted bool) (*models.ResourceReduction, error)
	// BulkOverrideReductionClusters checks or unchecks many Clusters in one
	// compare-and-set write. Clusters that need attention are never checked in bulk;
	// they are counted in the result instead.
	BulkOverrideReductionClusters(request *query_models.ReductionBulkOverride, ownerUserID *uint, ownerRestricted bool) (*ReductionBulkResult, error)
	// ApplyResourceReduction merges every checked Cluster, reporting per Cluster
	// what happened. Partial and repeatable: what is not checked stays open, and a
	// Cluster refused at revalidation is named in the result and kept.
	ApplyResourceReduction(request *query_models.ReductionApply, ownerUserID *uint, ownerRestricted bool) (*ReductionApplyResult, error)
}
