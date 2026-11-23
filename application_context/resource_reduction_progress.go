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
	out := make(map[uint]contracts.ReductionProgress, len(reductions))
	plans := make(map[uint]models.ResourceReductionPlan, len(reductions))
	for i := range reductions {
		plan, err := DecodeReductionPlan(reductions[i].Plan)
		if err != nil {
			continue
		}
		plans[reductions[i].ID] = plan
	}

	var visible map[uint]bool
	if ctx.isScopedPrincipal() {
		var ids []uint
		for _, plan := range plans {
			ids = append(ids, reductionMemberIDs(plan.Clusters)...)
		}
		var err error
		if visible, err = ctx.visibleMemberIDs(ids); err != nil {
			return nil, err
		}
	}

	for id, plan := range plans {
		var progress contracts.ReductionProgress
		for _, cluster := range plan.Clusters {
			if visible != nil && clusterWithheld(cluster, visible) {
				continue
			}
			progress.Clusters++
			switch cluster.State {
			case models.ReductionClusterOpen:
				progress.Open++
				if cluster.Checked {
					progress.Approved++
				}
			case models.ReductionClusterSkipped:
				progress.Skipped++
			case models.ReductionClusterStale:
				progress.Stale++
			case models.ReductionClusterApplied:
				progress.Applied++
			}
		}
		out[id] = progress
	}
	return out, nil
}
