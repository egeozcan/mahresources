package application_context

import (
	"errors"

	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
)

var (
	// ErrReductionBulkTarget refuses a bulk request that names its Clusters in none
	// of the two ways, or in both.
	ErrReductionBulkTarget = errors.New("name the Clusters with exactly one of clusterIds and allIdentical, and use action check or uncheck")
	// ErrReductionBulkNeedsCount refuses allIdentical without the count the caller
	// saw. The count is what keeps a stale button from approving more than it said.
	ErrReductionBulkNeedsCount = errors.New("allIdentical needs the expectedCount shown on the button")
	// ErrReductionCountMismatch is the 409 for an expectedCount that no longer
	// matches what allIdentical would act on.
	ErrReductionCountMismatch = errors.New("the number of Identical Clusters changed since you loaded this page; reload and try again")
	// ErrReductionClusterNotOpen refuses a bulk act that names a Cluster which is
	// not open. Skipped, stale and applied Clusters are reopened or settled one at
	// a time.
	ErrReductionClusterNotOpen = errors.New("a Cluster named here is not open; reload and try again")
)

// BulkOverrideReductionClusters checks or unchecks many Clusters in one write.
//
// The rules, each enforced here rather than by the page:
//
//   - A Cluster that needs attention (lossy, oversized, keeping a lower-resolution
//     copy) is never checked in bulk. It is left exactly as it was and counted in
//     NeedsReview, so the reviewer is told rather than surprised. Unchecking is
//     always safe.
//   - Unknown ids and Clusters the caller may not fully see refuse the whole
//     request with the one error an unknown Cluster gets, before anything is
//     written: any difference would be an oracle for hidden Clusters.
//   - allIdentical never reaches the Near-Identical tier.
//   - Every Cluster acted on is marked Reviewed, so it freezes exactly as a single
//     decision does.
//
// It goes through the same compare-and-set and retry path as one override.
func (ctx *MahresourcesContext) BulkOverrideReductionClusters(request *query_models.ReductionBulkOverride, ownerUserID *uint, ownerRestricted bool) (*contracts.ReductionBulkResult, error) {
	if request == nil || request.ID == 0 {
		return nil, errors.New("no Resource Reduction given")
	}
	if request.Action != ReductionActionCheck && request.Action != ReductionActionUncheck {
		return nil, ErrReductionBulkTarget
	}
	if request.AllIdentical == (len(request.ClusterIDs) > 0) {
		return nil, ErrReductionBulkTarget
	}
	if request.AllIdentical && request.ExpectedCount == nil {
		return nil, ErrReductionBulkNeedsCount
	}

	var result *contracts.ReductionBulkResult
	_, err := retryReductionWrite(func() (*models.ResourceReduction, error) {
		reduction, outcome, err := ctx.bulkOverrideReductionClustersOnce(request, ownerUserID, ownerRestricted)
		result = outcome
		return reduction, err
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// bulkOverrideReductionClustersOnce is one attempt: every read first, then the
// compare-and-set as the transaction's first statement.
func (ctx *MahresourcesContext) bulkOverrideReductionClustersOnce(request *query_models.ReductionBulkOverride, ownerUserID *uint, ownerRestricted bool) (*models.ResourceReduction, *contracts.ReductionBulkResult, error) {
	reduction, err := ctx.loadReductionForUpdate(request.ID, ownerUserID, ownerRestricted)
	if err != nil {
		return nil, nil, err
	}
	if EffectiveReductionStatus(reduction) == models.ReductionStatusComputing {
		return nil, nil, ErrReductionBusy
	}
	plan, err := DecodeReductionPlan(reduction.Plan)
	if err != nil {
		return nil, nil, err
	}

	targets, err := ctx.bulkTargets(&plan, request)
	if err != nil {
		return nil, nil, err
	}

	checking := request.Action == ReductionActionCheck
	result := &contracts.ReductionBulkResult{}
	for _, cluster := range targets {
		if checking && clusterNeedsAttention(cluster) {
			result.NeedsReview++
			continue
		}
		if cluster.Checked != checking {
			result.Changed++
		}
		cluster.Checked = checking
		cluster.Reviewed = true
	}
	if request.AllIdentical && uint(len(targets)-result.NeedsReview) != *request.ExpectedCount {
		return nil, nil, ErrReductionCountMismatch
	}

	encoded, err := encodeJSON(plan)
	if err != nil {
		return nil, nil, err
	}
	updated, err := ctx.casReductionAndReloadHolding(reduction.ID, request.Version, map[string]any{"plan": encoded}, ownerUserID, ownerRestricted, targets)
	if err != nil {
		return nil, nil, err
	}
	result.Version = updated.Version
	return updated, result, nil
}

// bulkTargets resolves the Clusters a request names. A Cluster the caller may not
// fully see answers exactly as an unknown id does.
func (ctx *MahresourcesContext) bulkTargets(plan *models.ResourceReductionPlan, request *query_models.ReductionBulkOverride) ([]*models.ReductionCluster, error) {
	if request.AllIdentical {
		var identical []*models.ReductionCluster
		for _, cluster := range plan.Clusters {
			if cluster.Tier == models.ReductionTierIdentical && cluster.State == models.ReductionClusterOpen {
				identical = append(identical, cluster)
			}
		}
		visible, err := ctx.visibleMemberIDs(reductionMemberIDs(identical))
		if err != nil {
			return nil, err
		}
		targets := identical[:0]
		for _, cluster := range identical {
			if !clusterWithheld(cluster, visible) {
				targets = append(targets, cluster)
			}
		}
		return targets, nil
	}

	seen := map[string]bool{}
	named := make([]*models.ReductionCluster, 0, len(request.ClusterIDs))
	for _, id := range request.ClusterIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		cluster := findCluster(plan, id)
		if cluster == nil {
			return nil, ErrReductionClusterNotFound
		}
		named = append(named, cluster)
	}
	visible, err := ctx.visibleMemberIDs(reductionMemberIDs(named))
	if err != nil {
		return nil, err
	}
	// Reachability for every id first, then state: a state answer about a Cluster
	// the caller may not see would tell them it exists.
	for _, cluster := range named {
		if clusterWithheld(cluster, visible) {
			return nil, ErrReductionClusterNotFound
		}
	}
	for _, cluster := range named {
		if cluster.State != models.ReductionClusterOpen {
			return nil, ErrReductionClusterNotOpen
		}
	}
	return named, nil
}
