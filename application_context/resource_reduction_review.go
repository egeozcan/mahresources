package application_context

import (
	"mahresources/contracts"
	"mahresources/models"
)

// ReductionClustersPerPage is how many Clusters one page of the review shows.
//
// Small, because a Cluster is several thumbnails, a justification and a row of
// controls, and the reviewer is meant to look at each one. A Reduction with
// thousands of Clusters still has to load.
const ReductionClustersPerPage = 20

// GetReductionReview loads a page of a Reduction's plan with the Resources behind
// it.
//
// Membership is re-checked against the *current* principal here rather than
// trusted from compute time. A scoped user's subtree can change after a Reduction
// is created, so a Cluster computed when they could see a Resource must not still
// show it to them once they cannot. The check is the db handle's own scope: the
// Resources are loaded through it, and anything outside the subtree simply does
// not come back.
func (ctx *MahresourcesContext) GetReductionReview(id uint, ownerUserID *uint, ownerRestricted bool, page int) (*contracts.ReductionReview, error) {
	reduction, err := ctx.loadReductionForUpdate(id, ownerUserID, ownerRestricted)
	if err != nil {
		return nil, err
	}
	plan, err := DecodeReductionPlan(reduction.Plan)
	if err != nil {
		return nil, err
	}
	storedExtent, err := DecodeReductionExtent(reduction.Extent)
	if err != nil {
		return nil, err
	}

	if page < 1 {
		page = 1
	}
	start := (page - 1) * ReductionClustersPerPage
	if start > len(plan.Clusters) {
		start = len(plan.Clusters)
	}
	end := start + ReductionClustersPerPage
	if end > len(plan.Clusters) {
		end = len(plan.Clusters)
	}
	pageClusters := plan.Clusters[start:end]

	// Counted over every Cluster, not the page: apply acts on the whole plan, and
	// a confirmation that counts only what is on screen understates the blast
	// radius by however many pages the reviewer has not opened.
	//
	// A Cluster reaching a Resource this caller may not see is left out of the
	// count entirely. It cannot be applied, and counting it would tell them "there
	// is one more Cluster, with two Resources in it" about Clusters the page
	// otherwise refuses to describe. Only the checked ones are resolved, so this
	// costs one lookup over what an apply would actually touch.
	checkedClusters := make([]*models.ReductionCluster, 0, len(plan.Clusters))
	var checkedMemberIDs []uint
	for _, cluster := range plan.Clusters {
		if cluster.State == models.ReductionClusterOpen && cluster.Checked {
			checkedClusters = append(checkedClusters, cluster)
			for _, member := range cluster.Members {
				if !member.Ejected {
					checkedMemberIDs = append(checkedMemberIDs, member.ResourceID)
				}
			}
		}
	}
	visible, err := ctx.loadResourcesByID(checkedMemberIDs)
	if err != nil {
		return nil, err
	}
	checked, checkedLosers := 0, 0
	for _, cluster := range checkedClusters {
		reachable := true
		for _, member := range cluster.Members {
			if !member.Ejected && visible[member.ResourceID] == nil {
				reachable = false
				break
			}
		}
		if !reachable {
			continue
		}
		checked++
		checkedLosers += len(cluster.LoserIDs())
	}

	// One load for the whole page rather than one per Cluster.
	var wanted []uint
	for _, cluster := range pageClusters {
		for _, member := range cluster.Members {
			wanted = append(wanted, member.ResourceID)
		}
	}
	resources, err := ctx.loadResourcesByID(wanted)
	if err != nil {
		return nil, err
	}

	views := make([]contracts.ReductionClusterView, 0, len(pageClusters))
	for _, cluster := range pageClusters {
		view := contracts.ReductionClusterView{
			ReductionCluster: cluster,
			Winner:           resources[cluster.WinnerID],
			DecidedByLabel:   models.WinnerCriterionLabel(cluster.DecidedBy),
			StateLabel:       reductionStateLabel(cluster),
		}
		for _, member := range cluster.Members {
			resource := resources[member.ResourceID]
			// A member that does not come back is one this principal may not see —
			// except a *Loser* of a Cluster whose merge is confirmed, which was
			// destroyed by this very row moments ago. Confirmed, not merely claimed:
			// apply claims before it merges, so a crash in between leaves Losers
			// alive, and reading those as gone would show a live Resource the
			// reviewer may not see. Counting those as withheld would hide the record
			// of what the Reduction just did, and showing them discloses nothing:
			// the reviewer performed the deletion.
			//
			// The Winner is not exempt. It survived the merge, so a Winner that
			// cannot be read is one the reviewer's access no longer covers, and
			// everything the Cluster says about it — the criterion, the margin, the
			// curation warning — is a statement about a Resource that is still there
			// and is not theirs to know about.
			// A Loser, specifically. An ejected member was never destroyed — that is
			// what ejection means — so one that later leaves the caller's scope is a
			// live Resource outside their access, not a merged-away one.
			mergedAway := cluster.Merged && member.IsLoser(cluster.WinnerID)
			if resource == nil && !mergedAway {
				view.Withheld++
			}
			view.Members = append(view.Members, contracts.ReductionMemberView{
				ReductionMember: member,
				Resource:        resource,
				IsWinner:        member.ResourceID == cluster.WinnerID,
				IsLoser:         member.IsLoser(cluster.WinnerID),
			})
		}
		view.Position = len(views) + 1
		if view.Withheld > 0 {
			view = redactWithheldCluster(view)
		}
		views = append(views, view)
	}

	// Deliberately no walk of the Extent here. Resolving it is one recursive CTE
	// per selected Group, which is cheap; *counting* what it reaches is not, and a
	// Reduction over a top-level Group in a library of millions would pay that on
	// every page view. The size the page shows is the one the last compute
	// recorded, which is the figure its coverage line is about anyway; the drift
	// query below is filtered on creation time first, so only what arrived since
	// then is ever materialised.
	extent, err := ctx.resolveReductionExtent(storedExtent)
	if err != nil {
		return nil, err
	}
	entered := 0
	if reduction.ComputedAt != nil {
		entered, err = ctx.extentArrivalsSince(extent, *reduction.ComputedAt)
		if err != nil {
			return nil, err
		}
	}

	return &contracts.ReductionReview{
		Reduction:           reduction,
		Status:              EffectiveReductionStatus(reduction),
		Coverage:            plan.Coverage,
		Clusters:            views,
		ClusterCount:        len(plan.Clusters),
		CheckedCount:        checked,
		CheckedLoserCount:   checkedLosers,
		Page:                page,
		PageSize:            ReductionClustersPerPage,
		ExtentSize:          plan.Coverage.ExtentSize,
		EnteredSinceCompute: entered,
	}, nil
}

// loadResourcesByID reads the named Resources through the caller's scoped handle.
// An id that is missing from the result is one that was deleted or that this
// principal may not see; the two are deliberately not distinguished.
func (ctx *MahresourcesContext) loadResourcesByID(ids []uint) (map[uint]*models.Resource, error) {
	out := map[uint]*models.Resource{}
	for _, chunk := range chunkUints(dedupeUints(ids), idChunk) {
		var batch []*models.Resource
		if err := ctx.db.
			Preload("Series").
			Preload("ResourceCategory").
			Where("resources.id IN ?", chunk).
			Find(&batch).Error; err != nil {
			return nil, err
		}
		for _, r := range batch {
			out[r.ID] = r
		}
	}
	return out, nil
}

// reductionStateLabel is a Cluster's state in words. "Reviewed" is not a state of
// its own — it is a judgement recorded on an open Cluster — but it is what the
// reviewer needs to see, because it is the difference between a Cluster a
// recompute may rearrange and one it may not.
func reductionStateLabel(cluster *models.ReductionCluster) string {
	switch cluster.State {
	case models.ReductionClusterSkipped:
		return "Skipped"
	case models.ReductionClusterApplied:
		return "Applied"
	case models.ReductionClusterStale:
		return "Stale — refused at apply"
	}
	if cluster.Reviewed {
		return "Reviewed"
	}
	return "Open"
}

// redactWithheldCluster strips a Cluster the caller may not fully see down to the
// fact that it exists.
//
// Done here rather than in the template, because the template is not the only
// reader. Every server-rendered page also answers `.json` and an
// `Accept: application/json` request by serialising its whole context
// (RenderTemplate), so a placeholder in the HTML leaves the Cluster's id, tier,
// Winner, member ids, reviewed hashes and justification available one URL suffix
// away. Redacting the view is what makes the rule hold on both.
//
// The id goes with the rest: it is a hash of the tier and the member ids, and ids
// here are small integers, so it recovers them.
func redactWithheldCluster(view contracts.ReductionClusterView) contracts.ReductionClusterView {
	return contracts.ReductionClusterView{
		ReductionCluster: &models.ReductionCluster{},
		Withheld:         view.Withheld,
		Position:         view.Position,
	}
}
