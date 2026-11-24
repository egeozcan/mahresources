package application_context

import (
	"errors"

	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
)

// The public entry points of a Reduction's review, decisions and apply. Each loads
// the Reduction under the caller's owner predicate (reductionColumns, never the
// legacy plan and extent documents), which is the authorization the store methods
// take as given, and hands the loaded row to the table-backed implementation.

// reductionUpgrading refuses a Reduction whose plan is still the legacy JSON
// document: the background migration has not moved it to tables yet (HTTP 503).
func reductionUpgrading(r *models.ResourceReduction) error {
	if r.PlanSchema == 0 {
		return ErrReductionUpgrading
	}
	return nil
}

// GetReductionReview is one page of a Reduction's plan with the Resources behind
// it, read from one snapshot at one plan generation (design D8).
func (ctx *MahresourcesContext) GetReductionReview(id uint, ownerUserID *uint, ownerRestricted bool, page int, query *query_models.ResourceReductionReviewQuery) (*contracts.ReductionReview, error) {
	review, extras, err := ctx.storeGetReductionReview(id, ownerUserID, ownerRestricted, page, query)
	if err != nil {
		return nil, err
	}
	for i := range review.Clusters {
		view := &review.Clusters[i]
		extra, ok := extras.Clusters[view.ID]
		if !ok {
			continue
		}
		view.MemberCount = extra.MemberCount
		view.MembersShown = extra.MembersShown
		view.MoreMembers = extra.MoreMembers
		view.MembersAfter = extra.MembersAfter
		view.Truncated = extra.Truncated
	}
	return review, nil
}

// GetReductionClusterMembers is one page of a Cluster's members after a cursor,
// hydrated like the review card's own. A Cluster the caller may not fully see is
// an unknown one.
func (ctx *MahresourcesContext) GetReductionClusterMembers(id uint, ownerUserID *uint, ownerRestricted bool, clusterKey, after string) (*contracts.ReductionMembersPage, error) {
	cursor, err := parseReductionMemberCursor(after)
	if err != nil {
		return nil, err
	}
	var page *contracts.ReductionMembersPage
	err = ctx.withReadSnapshot(func(snap *MahresourcesContext) error {
		reduction, err := snap.loadReductionForUpdate(id, ownerUserID, ownerRestricted)
		if err != nil {
			return err
		}
		if reduction.PlanSchema == 0 {
			return ErrReductionUpgrading
		}
		page, err = snap.storeClusterMembers(reduction, clusterKey, cursor)
		return err
	})
	return page, err
}

// OverrideReductionCluster records one review decision (design D3). It is refused
// while a clustering job is in flight (that job replaces the plan when it lands)
// and while the plan is still the legacy document.
func (ctx *MahresourcesContext) OverrideReductionCluster(override *query_models.ReductionOverride, ownerUserID *uint, ownerRestricted bool) (*models.ResourceReduction, error) {
	if override == nil || override.ID == 0 {
		return nil, errors.New("no Resource Reduction given")
	}
	reduction, err := ctx.loadReductionForUpdate(override.ID, ownerUserID, ownerRestricted)
	if err != nil {
		return nil, err
	}
	if err := reductionUpgrading(reduction); err != nil {
		return nil, err
	}
	return ctx.storeOverrideCluster(reduction, override)
}

// BulkOverrideReductionClusters checks or unchecks many Clusters in one
// compare-and-set write. Clusters that need attention are never checked in bulk;
// they are counted in the result instead.
func (ctx *MahresourcesContext) BulkOverrideReductionClusters(request *query_models.ReductionBulkOverride, ownerUserID *uint, ownerRestricted bool) (*contracts.ReductionBulkResult, error) {
	if request == nil || request.ID == 0 {
		return nil, errors.New("no Resource Reduction given")
	}
	reduction, err := ctx.loadReductionForUpdate(request.ID, ownerUserID, ownerRestricted)
	if err != nil {
		return nil, err
	}
	if err := reductionUpgrading(reduction); err != nil {
		return nil, err
	}
	return ctx.storeBulkOverride(reduction, request)
}

// ApplyResourceReduction merges every checked Cluster, reporting per Cluster what
// happened. Partial and repeatable: what is not checked stays open, and a Cluster
// refused at revalidation is named in the result and kept.
func (ctx *MahresourcesContext) ApplyResourceReduction(request *query_models.ReductionApply, ownerUserID *uint, ownerRestricted bool) (*contracts.ReductionApplyResult, error) {
	if request == nil || request.ID == 0 {
		return nil, errors.New("no Resource Reduction given")
	}
	reduction, err := ctx.loadReductionForUpdate(request.ID, ownerUserID, ownerRestricted)
	if err != nil {
		return nil, err
	}
	if err := reductionUpgrading(reduction); err != nil {
		return nil, err
	}
	return ctx.storeApplyReduction(reduction, request)
}
