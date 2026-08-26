package application_context

import (
	"errors"
	"fmt"
	"time"

	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
)

// Reasons a Cluster is refused at apply. Each names what changed, because the
// reviewer's next move depends on which one it was.
const (
	staleReasonMemberGone   = "a Resource in this Cluster no longer exists, or is no longer one you may see"
	staleReasonHashChanged  = "the bytes of a Resource in this Cluster changed since it was reviewed"
	staleReasonPairGone     = "the perceptual match between the Winner and a Loser no longer holds"
	staleReasonNothingToDo  = "every Loser in this Cluster has been ejected, so there is nothing to merge"
	staleReasonAlreadyMoved = "this Cluster changed while the batch was running"
	staleReasonMergeRefused = "the merge itself was refused"
)

// ApplyResourceReduction merges every checked Cluster.
//
// Repeatable and partial by design: it applies what is checked, marks those
// Clusters applied, and leaves everything else open for tomorrow. A Cluster that
// fails revalidation is skipped, marked stale, named in the result and kept in
// the Reduction — never a whole-batch refusal, because one stray edit must not
// waste a review of four hundred.
//
// Three structural decisions, each of which would be a defect the other way:
//
//   - There is no transaction around the loop. MergeResources opens its own and
//     runs its file cleanup *after* that commits, deciding whether to remove each
//     file from a reference count taken inside it. An outer transaction would make
//     "after commit" untrue and take that decision on stale numbers.
//   - Each Cluster's outcome is written the moment it happens, not at the end. A
//     crash mid-batch then leaves the Clusters that merged marked applied, rather
//     than leaving them checked and open over Losers that no longer exist.
//   - Revalidation happens per Cluster immediately before its own merge, not once
//     up front. Earlier merges in this same batch change the world, and so does
//     anything else running at the time.
func (ctx *MahresourcesContext) ApplyResourceReduction(request *query_models.ReductionApply, ownerUserID *uint, ownerRestricted bool) (*contracts.ReductionApplyResult, error) {
	if request == nil || request.ID == 0 {
		return nil, errors.New("no Resource Reduction given")
	}

	reduction, err := ctx.loadReductionForUpdate(request.ID, ownerUserID, ownerRestricted)
	if err != nil {
		return nil, err
	}
	if EffectiveReductionStatus(reduction) == models.ReductionStatusComputing {
		return nil, ErrReductionBusy
	}

	// The claim, taken before anything is destroyed and in the shape
	// ClaimDownloadHistoryRetry uses: a reviewer working from a stale page cannot
	// apply, and a concurrent apply or override loses here rather than
	// interleaving merges with this one.
	claimed, err := ctx.casReduction(reduction.ID, request.Version, map[string]any{})
	if err != nil {
		return nil, err
	}
	if !claimed {
		return nil, ErrReductionConflict
	}

	plan, err := DecodeReductionPlan(reduction.Plan)
	if err != nil {
		return nil, err
	}

	result := &contracts.ReductionApplyResult{}
	for _, planned := range plan.Clusters {
		if planned.State != models.ReductionClusterOpen || !planned.Checked {
			continue
		}
		outcome := ctx.applyOneCluster(reduction.ID, planned.ID, reduction)
		switch {
		case outcome.Applied:
			result.Applied = append(result.Applied, outcome.Report)
		default:
			result.Stale = append(result.Stale, outcome.Report)
		}
	}

	return result, nil
}

type clusterApplyOutcome struct {
	Applied bool
	Report  contracts.ReductionApplyOutcome
}

// applyOneCluster revalidates and merges a single Cluster, then records what
// happened to it.
func (ctx *MahresourcesContext) applyOneCluster(reductionID uint, clusterID string, reduction *models.ResourceReduction) clusterApplyOutcome {
	// Re-read rather than trusting the copy the batch started with. An earlier
	// Cluster in this batch may have merged one of these Resources away, and a
	// concurrent apply may have taken this Cluster already.
	current, err := ctx.loadReductionForUpdate(reductionID, nil, false)
	if err != nil {
		return clusterApplyOutcome{Report: contracts.ReductionApplyOutcome{ClusterID: clusterID, Reason: err.Error()}}
	}
	plan, err := DecodeReductionPlan(current.Plan)
	if err != nil {
		return clusterApplyOutcome{Report: contracts.ReductionApplyOutcome{ClusterID: clusterID, Reason: err.Error()}}
	}
	cluster := findCluster(&plan, clusterID)
	if cluster == nil || cluster.State != models.ReductionClusterOpen || !cluster.Checked {
		return clusterApplyOutcome{Report: contracts.ReductionApplyOutcome{ClusterID: clusterID, Reason: staleReasonAlreadyMoved}}
	}

	report := contracts.ReductionApplyOutcome{
		ClusterID: cluster.ID,
		Tier:      cluster.Tier,
		WinnerID:  cluster.WinnerID,
		LoserIDs:  cluster.LoserIDs(),
	}

	if reason := ctx.revalidateCluster(cluster); reason != "" {
		report.Reason = reason
		ctx.markClusterStale(reductionID, clusterID, reason)
		return clusterApplyOutcome{Report: report}
	}

	// The Cluster is claimed *before* it is merged, not marked afterwards. A
	// reviewer on a freshly loaded page can eject a Loser at any moment, and the
	// gap between reading a Cluster and destroying it is the one place an
	// ejection could be missed — which is exactly the thing ejection promises
	// cannot happen. Marking it applied first makes every override on it refuse
	// (ErrReductionClusterSettled), so the ids about to be merged are frozen.
	//
	// What is merged is the snapshot the claim itself committed, never the one
	// read above. An ejection landing between the two wins the version
	// compare-and-set, and the claim then runs against the plan it produced —
	// reading the ids off the earlier copy would delete the member that ejection
	// had just removed, which is the whole defect the claim exists to close.
	//
	// The mirror failure is a crash between this write and the merge, which
	// leaves a Cluster marked applied whose Losers still exist. That is the right
	// way round: it costs a merge that has to be redone, where the other order
	// costs a file the reviewer had un-approved. It also self-heals — an applied
	// Cluster's members go back into the pool, so the next recompute re-proposes
	// them.
	claimed := ctx.claimClusterForApply(reductionID, clusterID)
	if claimed == nil {
		report.Reason = staleReasonAlreadyMoved
		return clusterApplyOutcome{Report: report}
	}
	report.WinnerID = claimed.WinnerID
	report.LoserIDs = claimed.LoserIDs()

	// One flag per tier, because the design's defaults are opposite: a
	// byte-identical Loser has nothing to preserve, while a Near-Identical one
	// holds pixels the reviewer decided against. MergeResources takes a single
	// bool, which is exactly why this is easy to get wrong.
	keepAsVersion := reduction.KeepAsVersionIdentical
	if claimed.Tier == models.ReductionTierNear {
		keepAsVersion = reduction.KeepAsVersionNear
	}

	if err := ctx.MergeResourcesExpecting(claimed.WinnerID, report.LoserIDs, keepAsVersion, ctx.reductionMergePrecondition(claimed)); err != nil {
		reason := staleReasonMergeRefused + ": " + err.Error()
		if errors.Is(err, ErrMergeContentChanged) {
			reason = staleReasonHashChanged
		}
		report.Reason = reason
		ctx.demoteClusterToStale(reductionID, clusterID, reason)
		return clusterApplyOutcome{Report: report}
	}

	return clusterApplyOutcome{Applied: true, Report: report}
}

// claimClusterForApply takes a Cluster out of play for the merge that is about to
// happen, and hands back the exact snapshot it committed — or nil, meaning
// somebody else applied it first or the reviewer moved it while this batch was
// running.
//
// The returned Cluster is the authority on what gets merged. It is captured
// inside the successful compare-and-set rather than before it, so a decision that
// landed in the meantime is part of what this apply acts on rather than something
// it overwrites.
func (ctx *MahresourcesContext) claimClusterForApply(reductionID uint, clusterID string) *models.ReductionCluster {
	var claimed *models.ReductionCluster
	committed := ctx.mutateCluster(reductionID, clusterID, func(cluster *models.ReductionCluster) bool {
		if cluster.State != models.ReductionClusterOpen || !cluster.Checked {
			claimed = nil
			return false
		}
		cluster.State = models.ReductionClusterApplied
		cluster.Checked = false
		cluster.Reviewed = true
		now := time.Now()
		cluster.AppliedAt = &now
		snapshot := *cluster
		snapshot.Members = append([]*models.ReductionMember(nil), cluster.Members...)
		claimed = &snapshot
		return true
	})
	if !committed {
		return nil
	}
	return claimed
}

// reductionMergePrecondition is the Cluster's own validity, re-asserted inside
// the merge's transaction and behind its row locks.
//
// revalidateCluster runs before the merge and cannot hold: a version upload
// changes a Loser's bytes, and a similarity recompute deletes the very pair that
// justified a Near-Identical deletion. Both leave the merge destroying something
// nobody reviewed, and both are ordinary background work in this deployment
// rather than exotic interleavings.
func (ctx *MahresourcesContext) reductionMergePrecondition(cluster *models.ReductionCluster) MergePrecondition {
	return func(txCtx *MahresourcesContext) error {
		ids := make([]uint, 0, len(cluster.Members))
		expected := map[uint]string{}
		for _, member := range cluster.Members {
			if member.Ejected {
				continue
			}
			ids = append(ids, member.ResourceID)
			expected[member.ResourceID] = member.Hash
		}

		resources, err := txCtx.loadResourcesByID(ids)
		if err != nil {
			return err
		}
		for id, hash := range expected {
			resource := resources[id]
			if resource == nil {
				return fmt.Errorf("%w: resource %d is gone", ErrMergeContentChanged, id)
			}
			if resource.Hash != hash {
				return fmt.Errorf("%w: resource %d", ErrMergeContentChanged, id)
			}
		}

		if cluster.Tier != models.ReductionTierNear {
			return nil
		}
		pairs, err := txCtx.similarWithin(cluster.WinnerID, txCtx.similarityThreshold())
		if err != nil {
			return err
		}
		paired := map[uint]bool{}
		for _, pair := range pairs {
			paired[pair.ResourceID] = true
		}
		for _, id := range cluster.LoserIDs() {
			if !paired[id] {
				return fmt.Errorf("%w: resource %d no longer matches the Winner", ErrMergeContentChanged, id)
			}
		}
		return nil
	}
}

// demoteClusterToStale takes back a claim whose merge was refused. Unlike
// markClusterStale it acts on a Cluster this batch has already marked applied,
// which is the only writer that may move one off that state.
func (ctx *MahresourcesContext) demoteClusterToStale(reductionID uint, clusterID, reason string) {
	_ = ctx.mutateCluster(reductionID, clusterID, func(cluster *models.ReductionCluster) bool {
		if cluster.State != models.ReductionClusterApplied {
			return false
		}
		cluster.State = models.ReductionClusterStale
		cluster.StaleReason = reason
		cluster.AppliedAt = nil
		cluster.Checked = false
		return true
	})
}

// revalidateCluster answers "is this still the thing that was reviewed", and
// returns why not when it is not.
//
// The content-hash snapshot is what makes this possible at all. A version upload
// rewrites resources.hash and leaves the similarity pairs untouched, so
// re-checking the pair table alone could not detect that the reviewed bytes are
// gone. The pair re-check stays for the Near-Identical tier, but it is no longer
// the thing being relied on.
//
// Ejected members are exempt from every check here. Ejection leaves a Resource
// completely untouched — that is what makes it a safe action — so a version
// upload on one must not stale a Cluster it is no longer part of.
func (ctx *MahresourcesContext) revalidateCluster(cluster *models.ReductionCluster) string {
	loserIDs := cluster.LoserIDs()
	if len(loserIDs) == 0 {
		return staleReasonNothingToDo
	}

	// Loaded through the scoped handle, so "no longer exists" and "no longer one
	// this principal may see" are the same answer — which is the apply-side half
	// of re-checking membership against the *current* principal. A reviewer whose
	// subtree shrank must not be able to destroy what they can no longer be shown.
	wanted := append([]uint{cluster.WinnerID}, loserIDs...)
	resources, err := ctx.loadResourcesByID(wanted)
	if err != nil {
		return err.Error()
	}
	for _, member := range cluster.Members {
		if member.Ejected {
			continue
		}
		resource := resources[member.ResourceID]
		if resource == nil {
			return staleReasonMemberGone
		}
		if resource.Hash != member.Hash {
			return staleReasonHashChanged
		}
	}

	if cluster.Tier != models.ReductionTierNear {
		return ""
	}
	pairs, err := ctx.similarWithin(cluster.WinnerID, ctx.similarityThreshold())
	if err != nil {
		return err.Error()
	}
	paired := map[uint]bool{}
	for _, pair := range pairs {
		paired[pair.ResourceID] = true
	}
	for _, id := range loserIDs {
		if !paired[id] {
			return staleReasonPairGone
		}
	}
	return ""
}

func (ctx *MahresourcesContext) markClusterStale(reductionID uint, clusterID, reason string) {
	_ = ctx.mutateCluster(reductionID, clusterID, func(cluster *models.ReductionCluster) bool {
		if cluster.State != models.ReductionClusterOpen {
			return false
		}
		cluster.State = models.ReductionClusterStale
		cluster.StaleReason = reason
		// Unchecked, so the next apply does not walk into the same refusal, and
		// frozen, so a recompute leaves it where the reviewer can look at it.
		cluster.Checked = false
		cluster.Reviewed = true
		return true
	})
}

// mutateCluster applies one change to one Cluster under the version
// compare-and-set, re-reading on each attempt.
//
// The mutation reports whether it still applies. That is what stops an outcome
// being written over a newer one: a concurrent apply that reached this Cluster
// first has already moved it off `open`, and recording "stale" over its "applied"
// would describe a merge that did happen as one that did not.
func (ctx *MahresourcesContext) mutateCluster(reductionID uint, clusterID string, mutate func(*models.ReductionCluster) bool) bool {
	for attempt := 0; attempt < reductionCASRetries; attempt++ {
		current, err := ctx.loadReductionForUpdate(reductionID, nil, false)
		if err != nil {
			ctx.logReductionWriteFailure(reductionID, clusterID, err)
			return false
		}
		plan, err := DecodeReductionPlan(current.Plan)
		if err != nil {
			ctx.logReductionWriteFailure(reductionID, clusterID, err)
			return false
		}
		cluster := findCluster(&plan, clusterID)
		if cluster == nil || !mutate(cluster) {
			return false
		}
		encoded, err := encodeJSON(plan)
		if err != nil {
			ctx.logReductionWriteFailure(reductionID, clusterID, err)
			return false
		}
		ok, err := ctx.casReduction(current.ID, current.Version, map[string]any{"plan": encoded})
		if err != nil {
			ctx.logReductionWriteFailure(reductionID, clusterID, err)
			return false
		}
		if ok {
			return true
		}
	}
	ctx.logReductionWriteFailure(reductionID, clusterID, ErrReductionConflict)
	return false
}

// logReductionWriteFailure records a bookkeeping write that could not land.
//
// It is logged rather than returned, because by the time it happens the merge has
// already run: the Resources are gone either way, and failing the request would
// tell the reviewer nothing happened when in fact it did. The row is then behind
// what the database holds, which the next revalidation catches as a stale Cluster
// rather than as a second deletion — the members no longer exist.
func (ctx *MahresourcesContext) logReductionWriteFailure(reductionID uint, clusterID string, cause error) {
	ctx.Logger().Warning(models.LogActionUpdate, "resource_reduction", &reductionID, clusterID,
		fmt.Sprintf("Could not record the outcome of Cluster %s: %s", clusterID, cause.Error()), nil)
}
