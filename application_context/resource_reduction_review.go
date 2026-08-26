package application_context

import (
	"time"

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
			if resource == nil {
				view.Withheld++
			}
			view.Members = append(view.Members, contracts.ReductionMemberView{
				ReductionMember: member,
				Resource:        resource,
				IsWinner:        member.ResourceID == cluster.WinnerID,
				IsLoser:         member.IsLoser(cluster.WinnerID),
			})
		}
		views = append(views, view)
	}

	extent, err := ctx.resolveReductionExtent(storedExtent)
	if err != nil {
		return nil, err
	}
	entered := 0
	if reduction.ComputedAt != nil {
		entered, err = ctx.countExtentResourcesSince(extent, *reduction.ComputedAt)
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
		Page:                page,
		PageSize:            ReductionClustersPerPage,
		ExtentSize:          extent.Size,
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

// countExtentResourcesSince is the drift figure: how much has entered the Extent
// since the plan was computed.
func (ctx *MahresourcesContext) countExtentResourcesSince(extent *ReductionExtent, since time.Time) (int, error) {
	total := 0
	err := ctx.extentResourceIDs(extent, func(ids []uint) error {
		var count int64
		if err := ctx.db.Model(&models.Resource{}).
			Where("resources.id IN ?", ids).
			Where("resources.created_at > ?", since).
			Count(&count).Error; err != nil {
			return err
		}
		total += int(count)
		return nil
	})
	return total, err
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
