package application_context

import (
	"mahresources/contracts"
	"mahresources/models"
)

// GetReductionProgress counts each Reduction's Clusters by state, for the list
// page.
//
// Computed on read from the plan each row already carries, for the page of rows
// only. An unrestricted principal decodes and counts. A subtree-confined one also
// needs to know which Clusters it may see, and that is one id-only read over the
// union of every member of every Reduction on the page, not one per Cluster or per
// Reduction. A Cluster reaching a Resource they may not see is in no count.
//
// A plan that cannot be decoded counts as empty rather than failing the list: one
// damaged row must not take the page down, and the review page reports the error.
func (ctx *MahresourcesContext) GetReductionProgress(reductions []models.ResourceReduction) (map[uint]contracts.ReductionProgress, error) {
	var out map[uint]contracts.ReductionProgress
	// One snapshot for the plans and the visibility of their members. The rows the
	// caller holds were read earlier; read apart from the visibility query, a merge
	// committing in between would count a Cluster as withheld that was visible both
	// before and after it. The plans are re-read here, inside the snapshot.
	err := ctx.withReadSnapshot(func(snap *MahresourcesContext) error {
		var err error
		out, err = snap.reductionProgress(reductions)
		return err
	})
	return out, err
}

func (ctx *MahresourcesContext) reductionProgress(reductions []models.ResourceReduction) (map[uint]contracts.ReductionProgress, error) {
	if len(reductions) > 0 {
		ids := make([]uint, len(reductions))
		for i := range reductions {
			ids[i] = reductions[i].ID
		}
		var fresh []models.ResourceReduction
		if err := ctx.db.Select("id", "plan").Where("id IN ?", ids).Find(&fresh).Error; err != nil {
			return nil, err
		}
		plans := make(map[uint]models.ResourceReduction, len(fresh))
		for _, row := range fresh {
			plans[row.ID] = row
		}
		reductions = append([]models.ResourceReduction(nil), reductions...)
		for i := range reductions {
			if row, ok := plans[reductions[i].ID]; ok {
				reductions[i].Plan = row.Plan
			}
		}
	}
	out := make(map[uint]contracts.ReductionProgress, len(reductions))
	plans := make(map[uint]models.ResourceReductionPlan, len(reductions))
	for i := range reductions {
		plan, err := DecodeReductionPlan(reductions[i].Plan)
		if err != nil {
			continue
		}
		plans[reductions[i].ID] = plan
	}

	// The same reading the review's tabs use, so the list and the page agree.
	var ids []uint
	for _, plan := range plans {
		ids = append(ids, reductionMemberIDs(plan.Clusters)...)
	}
	visible, err := ctx.visibleMemberIDs(ids)
	if err != nil {
		return nil, err
	}

	for id, plan := range plans {
		plan := plan
		ctx.readDeletions(&plan, visible)
		planVisible, err := ctx.withMergedHistory(plan.Clusters, visible)
		if err != nil {
			return nil, err
		}
		var progress contracts.ReductionProgress
		for _, cluster := range plan.Clusters {
			if ctx.isScopedPrincipal() && clusterWithheld(cluster, planVisible) {
				continue
			}
			progress.Clusters++
			switch cluster.State {
			case models.ReductionClusterOpen:
				progress.Open++
				if clusterApproved(cluster) {
					progress.Approved++
				}
			case models.ReductionClusterSkipped:
				progress.Skipped++
			case models.ReductionClusterStale:
				progress.Stale++
			case models.ReductionClusterApplied:
				if cluster.Merged {
					progress.Applied++
				} else {
					progress.Merging++
				}
			}
		}
		out[id] = progress
	}
	return out, nil
}
