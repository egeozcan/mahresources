package application_context

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"

	"gorm.io/gorm/clause"
)

// Reasons a Cluster is refused at apply. Each names what changed, because the
// reviewer's next move depends on which one it was.
const (
	staleReasonMemberGone   = "a Resource in this Cluster no longer exists, or is no longer one you may see"
	staleReasonDeleted      = "a Resource in this Cluster no longer exists"
	staleReasonHashChanged  = "the bytes of a Resource in this Cluster changed since it was reviewed"
	staleReasonPairGone     = "the perceptual match between the Winner and a Loser no longer holds"
	staleReasonNothingToDo  = "every Loser in this Cluster has been ejected, so there is nothing to merge"
	staleReasonAlreadyMoved = "this Cluster changed while the batch was running"
	staleReasonReplaced     = "this Cluster was recomputed while the batch was running, so it is no longer the one that was reviewed"
	staleReasonMergeRefused = "the merge itself was refused"
)

// ReductionApplyCeiling bounds one apply batch. The batch runs on a context
// detached from the request, so a closed tab cannot cancel it halfway; the ceiling
// is what stops a wedged one from running forever. Clusters it does not reach stay
// approved.
//
// A variable so a test can shorten it; nothing else assigns it.
var ReductionApplyCeiling = 30 * time.Minute

// reductionApplySettleTimeout bounds the write that settles a Cluster whose claim
// is in flight when the ceiling passes. Separate from the batch's own deadline,
// because that one is by then already spent.
const reductionApplySettleTimeout = time.Minute

// reductionRequestIDMax bounds the client's apply correlation id.
const reductionRequestIDMax = 64

// errMergeVisibilityLost is a Cluster's member leaving the caller's access between
// the claim and the merge. The merge is refused, the claim released, and nothing is
// reported: naming the Cluster would disclose what the access change hides.
var errMergeVisibilityLost = errors.New("a member of this Cluster left the caller's access")

// ErrReductionApplyFiguresChanged refuses an apply whose confirmed figures no
// longer describe what it would merge.
var ErrReductionApplyFiguresChanged = errors.New("what this apply would merge changed since you confirmed it; reload and review again")

// reductionPreconditionHook runs at the start of a merge's precondition, inside its
// transaction. Only a test sets it.
var reductionPreconditionHook func()

// SetReductionPreconditionHookForTest installs it and returns the remover.
func SetReductionPreconditionHookForTest(hook func()) (restore func()) {
	reductionPreconditionHook = hook
	return func() { reductionPreconditionHook = nil }
}

// reductionApplyBeforeStaleHook runs after a failed revalidation and before the
// stale outcome is recorded. Only a test sets it.
var reductionApplyBeforeStaleHook func()

// SetReductionApplyBeforeStaleHookForTest installs it and returns the remover.
func SetReductionApplyBeforeStaleHookForTest(hook func()) (restore func()) {
	reductionApplyBeforeStaleHook = hook
	return func() { reductionApplyBeforeStaleHook = nil }
}

// reductionApplyAfterClaimHook runs between a Cluster's claim and its merge. Only
// a test sets it.
var reductionApplyAfterClaimHook func()

// SetReductionApplyAfterClaimHookForTest installs a function that runs between each
// Cluster's claim and its merge, and returns the function that removes it.
func SetReductionApplyAfterClaimHookForTest(hook func()) (restore func()) {
	reductionApplyAfterClaimHook = hook
	return func() { reductionApplyAfterClaimHook = nil }
}

// settlementContext is a handle for the write that settles a claimed Cluster: it
// keeps the caller's scope and identity but not the batch's deadline.
func (ctx *MahresourcesContext) settlementContext() (*MahresourcesContext, context.CancelFunc) {
	settle, cancel := context.WithTimeout(context.WithoutCancel(ctx.db.Statement.Context), reductionApplySettleTimeout)
	return ctx.WithParentContext(settle), cancel
}

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

	// Detached from the request, here and not in the handler (whose context has no
	// way to rebase the handle). A closed tab or a dropped connection cancels the
	// request, and a cancellation landing between a Cluster's claim and its merge
	// would strand it marked applied over Losers that still exist, with the failed
	// merge never demoted because that write is cancelled too. WithoutCancel keeps
	// the values on the handle and WithParentContext carries the scope filter and
	// acting user across, so the batch still runs as the caller and inside their
	// subtree. The ceiling bounds a wedged batch; Clusters it does not reach stay
	// approved. Every read and write below goes through this handle.
	detached, cancel := context.WithTimeout(context.WithoutCancel(ctx.db.Statement.Context), ReductionApplyCeiling)
	defer cancel()
	ctx = ctx.WithParentContext(detached)

	if len(request.RequestID) > reductionRequestIDMax {
		return nil, fmt.Errorf("requestId is longer than %d characters", reductionRequestIDMax)
	}
	reduction, err := ctx.loadReductionForUpdate(request.ID, ownerUserID, ownerRestricted)
	if err != nil {
		return nil, err
	}
	if EffectiveReductionStatus(reduction) == models.ReductionStatusComputing {
		return nil, ErrReductionBusy
	}
	// Everything below is derived from the plan just loaded, so the claim is taken
	// against that very version, and a request that names another one is refused
	// outright: comparing the submitted number only at the write would let a
	// version that became current after the load commit a plan read at an earlier
	// one.
	if request.Version != reduction.Version {
		return nil, ErrReductionConflict
	}
	if request.SettleOnly {
		return ctx.settleReductionClaims(reduction)
	}
	// The claim, taken before anything is destroyed and in the shape
	// ClaimDownloadHistoryRetry uses: a reviewer working from a stale page cannot
	// apply, and a concurrent apply or override loses here rather than
	// interleaving merges with this one. It also writes the batch's own record,
	// running, so a lost response is answered on reload.
	//
	// What this apply may touch is decided once, before the claim, as the approved
	// Clusters the caller can see right now. A Cluster outside that set is never
	// reached, so a scope change landing during the batch cannot widen it, and when
	// the caller confirmed figures they are checked against this very set.
	eligible, err := ctx.eligibleApplyClusters(reduction, request)
	if err != nil {
		return nil, err
	}
	plan, startedAt, settled, err := ctx.claimReductionApplyBatch(reduction, request.RequestID)
	if err != nil {
		return nil, err
	}

	// Claims settled on the way in that turned out stale are part of this apply's
	// result and of lastApply.stale, since the Stale tab gained them.
	result := &contracts.ReductionApplyResult{Stale: settled}
	for _, planned := range plan.Clusters {
		if planned.State != models.ReductionClusterOpen || !planned.Checked || !eligible[planned.ID] {
			continue
		}
		// Past the ceiling nothing new starts. What was not reached stays approved
		// and open, to be applied by the next request.
		if detached.Err() != nil {
			break
		}
		// The reviewed content, as it stood when this request was authorized. Every
		// Cluster is re-read before it is merged — an ejection landing mid-batch has
		// to be honoured — but the re-read may not introduce anything: a recompute
		// can replace a Cluster with one of the same id and different bytes, and
		// merging that would destroy content this reviewer never saw.
		outcome := ctx.applyOneCluster(reduction.ID, planned.ID, reduction, approvedContent(planned))
		switch {
		case outcome.Omit:
		case outcome.Changed:
			result.Changed++
		case outcome.Applied:
			result.Applied = append(result.Applied, outcome.Report)
		default:
			result.Stale = append(result.Stale, outcome.Report)
		}
	}

	// On its own bounded context: the ceiling may be what ended the loop, and the
	// record must still say the batch finished.
	settle, settleCancel := ctx.settlementContext()
	defer settleCancel()
	settle.finishReductionApplyBatch(reduction.ID, startedAt, result)
	return result, nil
}

// eligibleApplyClusters is the set of Clusters an apply may act on: open, approved
// and fully visible to the caller, ejected members included. The same rule the
// review's figures use, so what the dialog counted is what is applied. When the
// request carries confirmed figures, a mismatch refuses the apply before anything
// is written.
func (ctx *MahresourcesContext) eligibleApplyClusters(reduction *models.ResourceReduction, request *query_models.ReductionApply) (map[string]bool, error) {
	plan, err := DecodeReductionPlan(reduction.Plan)
	if err != nil {
		return nil, err
	}
	visible, err := ctx.reviewReach(&plan, reductionMemberIDs(plan.Clusters))
	if err != nil {
		return nil, err
	}
	eligible := map[string]bool{}
	var approved []*models.ReductionCluster
	var clusters, losers uint
	for _, cluster := range plan.Clusters {
		if cluster.State != models.ReductionClusterOpen || !clusterApproved(cluster) || clusterWithheld(cluster, visible) {
			continue
		}
		eligible[cluster.ID] = true
		approved = append(approved, cluster)
		clusters++
		losers += uint(len(cluster.LoserIDs()))
	}
	if request.ApplyToken != "" && request.ApplyToken != applyToken(approved) {
		return nil, ErrReductionApplyFiguresChanged
	}
	if (request.ExpectedClusters != nil && *request.ExpectedClusters != clusters) ||
		(request.ExpectedLosers != nil && *request.ExpectedLosers != losers) {
		return nil, ErrReductionApplyFiguresChanged
	}
	return eligible, nil
}

// claimReductionApplyBatch takes the batch-level claim: one compare-and-set on the
// caller's version that also records the apply as running. Written once here and
// completed once by finishReductionApplyBatch, not by each Cluster's claim.
func (ctx *MahresourcesContext) claimReductionApplyBatch(reduction *models.ResourceReduction, requestID string) (models.ResourceReductionPlan, time.Time, []contracts.ReductionApplyOutcome, error) {
	plan, err := DecodeReductionPlan(reduction.Plan)
	if err != nil {
		return plan, time.Time{}, nil, err
	}
	// A batch still running, and not yet past its ceiling, owns the Reduction. A
	// second request that read a version after the first one's claim would win the
	// compare-and-set below and overwrite the record while both loops ran. Past the
	// ceiling the record is an interrupted one, and a new apply may proceed.
	if last := plan.LastApply; last != nil && last.FinishedAt == nil && time.Since(last.StartedAt) <= ReductionApplyCeiling {
		return plan, time.Time{}, nil, ErrReductionConflict
	}
	settled := ctx.reconcileClaims(&plan)
	startedAt := time.Now()
	plan.LastApply = &models.ReductionLastApply{StartedAt: startedAt, RequestID: requestID}
	encoded, err := encodeJSON(plan)
	if err != nil {
		return plan, startedAt, settled, err
	}
	claimed, err := ctx.casReduction(reduction.ID, reduction.Version, map[string]any{"plan": encoded})
	if err != nil {
		return plan, startedAt, settled, err
	}
	if !claimed {
		return plan, startedAt, settled, ErrReductionConflict
	}
	return plan, startedAt, settled, nil
}

// finishReductionApplyBatch completes the record the claim wrote. A withheld
// Cluster is not in the result at all, so it is in none of these counts.
func (ctx *MahresourcesContext) finishReductionApplyBatch(reductionID uint, startedAt time.Time, result *contracts.ReductionApplyResult) {
	finished := time.Now()
	_ = ctx.mutatePlan(reductionID, "apply", func(plan *models.ResourceReductionPlan) bool {
		// Only the record this batch wrote: a later apply has replaced it.
		if plan.LastApply == nil || !plan.LastApply.StartedAt.Equal(startedAt) {
			return false
		}
		plan.LastApply.FinishedAt = &finished
		plan.LastApply.Applied = len(result.Applied)
		plan.LastApply.Removed = result.DestroyedCount()
		plan.LastApply.Stale = len(result.Stale)
		plan.LastApply.ClusterIDs = nil
		for _, outcome := range result.Applied {
			plan.LastApply.ClusterIDs = append(plan.LastApply.ClusterIDs, outcome.ClusterID)
		}
		for _, outcome := range result.Stale {
			plan.LastApply.ClusterIDs = append(plan.LastApply.ClusterIDs, outcome.ClusterID)
		}
		return true
	})
}

type clusterApplyOutcome struct {
	Applied bool
	// Omit leaves the Cluster out of the response: one the caller may not fully
	// see, or one the batch's deadline stopped it reaching. Nothing was written.
	Omit bool
	// Changed marks a Cluster another decision moved before its turn (unchecked,
	// skipped, ejected, recomputed). That is not a finding about the Cluster, so it
	// is counted apart from the stale ones and the Stale tab does not gain it.
	Changed bool
	Report  contracts.ReductionApplyOutcome
}

// applyOneCluster revalidates and merges a single Cluster, then records what
// happened to it.
// approvedProposal is what an apply request was authorized to perform: which
// Resource survives, which may be destroyed, and the content each was reviewed as
// holding.
type approvedProposal struct {
	WinnerID uint
	Hashes   map[uint]string
}

func approvedContent(cluster *models.ReductionCluster) approvedProposal {
	approved := approvedProposal{WinnerID: cluster.WinnerID, Hashes: make(map[uint]string, len(cluster.Members))}
	for _, member := range cluster.Members {
		if !member.Ejected {
			approved.Hashes[member.ResourceID] = member.Hash
		}
	}
	return approved
}

// withinApproved reports whether a Cluster is still a subset of what was
// approved.
//
// Members may leave — that is an ejection, which the reviewer is entitled to make
// while the batch runs and which the batch must honour. Nothing may arrive, and
// no member's reviewed content may change: both of those mean a recompute
// substituted a different proposal under the same id, and its bytes were never
// reviewed by whoever pressed Apply.
func withinApproved(cluster *models.ReductionCluster, approved approvedProposal) bool {
	// The Winner is not merely "one of the approved members". A promotion swaps it
	// without changing the membership or a single hash, so a subset check alone
	// accepts it — and then the merge destroys the Resource this request was told
	// to keep. Ejections are honoured because they only ever spare something; a
	// promotion changes which file dies, and that is not a change an apply already
	// in flight may pick up.
	if cluster.WinnerID != approved.WinnerID {
		return false
	}
	for _, member := range cluster.Members {
		if member.Ejected {
			continue
		}
		hash, wasApproved := approved.Hashes[member.ResourceID]
		if !wasApproved || hash != member.Hash {
			return false
		}
	}
	return true
}

func (ctx *MahresourcesContext) applyOneCluster(reductionID uint, clusterID string, reduction *models.ResourceReduction, approved approvedProposal) clusterApplyOutcome {
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
		return clusterApplyOutcome{Changed: true}
	}
	if !withinApproved(cluster, approved) {
		return clusterApplyOutcome{Changed: true}
	}

	// Visibility before anything else, and over every member including the ejected
	// ones: a Cluster reaching a Resource this caller may not see is not theirs to
	// act on, report or change. A member that does not come back is either deleted
	// or outside their subtree, and the two are deliberately one answer, so the
	// Cluster is neither merged, nor marked stale, nor named. Checked before the
	// hash comparison because that comparison would otherwise answer first.
	if visible, err := ctx.clusterFullyVisible(cluster); err != nil || !visible {
		return clusterApplyOutcome{Omit: true}
	}

	report := contracts.ReductionApplyOutcome{
		ClusterID: cluster.ID,
		Tier:      cluster.Tier,
		WinnerID:  cluster.WinnerID,
		LoserIDs:  cluster.LoserIDs(),
	}

	if reason, failed := ctx.revalidateCluster(cluster); reason != "" {
		// A deadline is not a finding about the Cluster, and the refusal below would
		// persist it as one.
		if ctx.db.Statement.Context.Err() != nil {
			return clusterApplyOutcome{Omit: true}
		}
		// A member that vanished between the visibility read above and this one is
		// the same case as that read, caught late: left alone and unreported.
		if reason == staleReasonMemberGone {
			if ctx.isScopedPrincipal() {
				return clusterApplyOutcome{Omit: true}
			}
			reason = staleReasonDeleted
		}
		report.Reason = reason
		if reductionApplyBeforeStaleHook != nil {
			reductionApplyBeforeStaleHook()
		}
		// Marked stale only if the Cluster is still the one that failed. The
		// refusal was derived from a snapshot, and a reviewer who ejected the
		// offending member while this ran has already removed the reason for it —
		// recording it stale anyway would discard their fix and uncheck a Cluster
		// they had approved. Counted only once it has committed.
		switch ctx.recordStaleLocked(reductionID, cluster, reason, failed) {
		case staleSuperseded:
			return clusterApplyOutcome{Changed: true}
		case staleHidden:
			return clusterApplyOutcome{Omit: true}
		}
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
	if ctx.db.Statement.Context.Err() != nil {
		return clusterApplyOutcome{Omit: true}
	}
	claimed := ctx.claimClusterForApply(reductionID, clusterID, approved)
	if claimed == nil {
		return clusterApplyOutcome{Changed: true}
	}
	if reductionApplyAfterClaimHook != nil {
		reductionApplyAfterClaimHook()
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

	if err := ctx.MergeResourcesLocking(claimed.WinnerID, report.LoserIDs, keepAsVersion, heldMemberIDs(claimed), !ctx.isScopedPrincipal(), ctx.reductionMergeHooks(reductionID, claimed)); err != nil {
		switch {
		case errors.Is(err, errClaimLost):
			// Another writer settled or replaced this claim; nothing was deleted.
			return clusterApplyOutcome{Changed: true}
		case errors.Is(err, errMergeVisibilityLost),
			ctx.isScopedPrincipal() && errors.Is(err, ErrMergeParticipantUnavailable):
			// A member left this caller's access, judged on the subtree as it is now:
			// the claim is handed back and nothing is reported, because any name
			// would disclose what the change hides.
			settle, cancel := ctx.settlementContext()
			defer cancel()
			settle.releaseClaim(reductionID, clusterID, claimed.ClaimID)
			return clusterApplyOutcome{Omit: true}
		}
		reason := staleReasonMergeRefused + ": " + err.Error()
		// Named by what actually failed. Both refusals are safe, but they call for
		// different things from the reviewer: changed bytes mean look at the
		// Resource, a vanished pair means the perceptual match is gone.
		switch {
		case errors.Is(err, ErrMergePairGone):
			reason = staleReasonPairGone
		case errors.Is(err, ErrMergeContentChanged):
			reason = staleReasonHashChanged
		}
		report.Reason = reason
		// Settled on a context of its own: if the batch's deadline is what failed
		// the merge, writing the demotion on that same context would fail too and
		// strand the Cluster marked applied over Losers that still exist.
		var refusal *mergeRefusal
		var changedIDs []uint
		if errors.As(err, &refusal) {
			changedIDs = refusal.ids
		}
		settle, cancel := ctx.settlementContext()
		defer cancel()
		// Counted as stale only once the transition has committed; a claim settled by
		// someone else in the meantime is a change, not a finding.
		if !settle.demoteClusterToStale(reductionID, clusterID, claimed.ClaimID, reason, changedIDs) {
			return clusterApplyOutcome{Changed: true}
		}
		return clusterApplyOutcome{Report: report}
	}

	return clusterApplyOutcome{Applied: true, Report: report}
}

// OverwritePlanForTest and ClaimClusterForApplyForTest expose the two halves of
// the claim so a test can put a Cluster into the one state that has no other
// seam: the same id over a different proposal, which only a recompute landing
// inside an apply produces.
func (ctx *MahresourcesContext) OverwritePlanForTest(reductionID, version uint, plan models.ResourceReductionPlan) error {
	encoded, err := encodeJSON(plan)
	if err != nil {
		return err
	}
	ok, err := ctx.casReduction(reductionID, version, map[string]any{"plan": encoded})
	if err != nil {
		return err
	}
	if !ok {
		return ErrReductionConflict
	}
	return nil
}

func (ctx *MahresourcesContext) ClaimClusterForApplyForTest(reductionID uint, clusterID string, winnerID uint, approved map[uint]string) *models.ReductionCluster {
	return ctx.claimClusterForApply(reductionID, clusterID, approvedProposal{WinnerID: winnerID, Hashes: approved})
}

// ReleaseClaimForTest settles a claim as a delayed writer would, with the claim id
// it holds.
func (ctx *MahresourcesContext) ReleaseClaimForTest(reductionID uint, clusterID, claimID string) {
	ctx.releaseClaim(reductionID, clusterID, claimID)
}

// mergeRefusal is a merge refused because of particular members, carried so the
// stale Cluster can say which.
type mergeRefusal struct {
	cause error
	ids   []uint
}

func (e *mergeRefusal) Error() string { return e.cause.Error() }
func (e *mergeRefusal) Unwrap() error { return e.cause }

// errClaimLost is a merge finding, inside its transaction, that the claim it was
// started for is no longer the Cluster's current one.
var errClaimLost = errors.New("this Cluster's claim was settled or replaced")

// reductionMergeHooks ties the Reduction's plan to the merge's own transaction.
//
// The Reduction row is locked first, before the Series and Resource locks, which
// is the order every decision follows (the plan's compare-and-set, then the
// members). Inside that lock the claim is checked to still be this attempt's, and
// the Cluster is marked merged in the same transaction that deletes its Losers:
// the plan says merged if and only if the merge committed, so no recovery has to
// infer it.
func (ctx *MahresourcesContext) reductionMergeHooks(reductionID uint, claimed *models.ReductionCluster) MergeHooks {
	var row *models.ResourceReduction
	var plan models.ResourceReductionPlan
	hooks := ctx.reductionMergePrecondition(claimed)
	return MergeHooks{
		BeforeLocks: func(txCtx *MahresourcesContext) error {
			if txCtx.db.Dialector.Name() == "postgres" {
				var locked models.ResourceReduction
				if err := txCtx.db.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(&locked, reductionID).Error; err != nil {
					return err
				}
			}
			var err error
			if row, err = txCtx.loadReductionForUpdate(reductionID, nil, false); err != nil {
				return err
			}
			if plan, err = DecodeReductionPlan(row.Plan); err != nil {
				return err
			}
			cluster := findCluster(&plan, claimed.ID)
			if cluster == nil || cluster.State != models.ReductionClusterApplied || cluster.Merged || cluster.ClaimID != claimed.ClaimID {
				return errClaimLost
			}
			return nil
		},
		Precondition: hooks,
		InTx: func(txCtx *MahresourcesContext) error {
			findCluster(&plan, claimed.ID).Merged = true
			encoded, err := encodeJSON(plan)
			if err != nil {
				return err
			}
			ok, err := txCtx.casReduction(row.ID, row.Version, map[string]any{"plan": encoded})
			if err != nil {
				return err
			}
			if !ok {
				return ErrReductionConflict
			}
			return nil
		},
	}
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
func (ctx *MahresourcesContext) claimClusterForApply(reductionID uint, clusterID string, approved approvedProposal) *models.ReductionCluster {
	var claimed *models.ReductionCluster
	committed := ctx.mutateCluster(reductionID, clusterID, func(cluster *models.ReductionCluster) bool {
		if cluster.State != models.ReductionClusterOpen || !cluster.Checked {
			claimed = nil
			return false
		}
		// The same id no longer means the same proposal: a recompute can replace a
		// Cluster with one of identical membership and different reviewed hashes.
		// What this apply may destroy is a subset of what the request approved, so
		// an ejection landing here is accepted and a substitution is not.
		if !withinApproved(cluster, approved) {
			claimed = nil
			return false
		}
		cluster.State = models.ReductionClusterApplied
		cluster.Checked = false
		cluster.Reviewed = true
		cluster.Merged = false
		cluster.ClaimID = newClaimID()
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
		if reductionPreconditionHook != nil {
			reductionPreconditionHook()
		}
		// Every member, ejected ones included, through the scoped handle and locked
		// in id order: a Cluster that reaches anything the caller may not see is not
		// theirs to merge, whichever member it is.
		if err := txCtx.lockClusterMembersVisible([]*models.ReductionCluster{cluster}); err != nil {
			if errors.Is(err, ErrReductionClusterNotFound) {
				return errMergeVisibilityLost
			}
			return err
		}
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
		var changed []uint
		for id, hash := range expected {
			if resource := resources[id]; resource == nil || resource.Hash != hash {
				changed = append(changed, id)
			}
		}
		if len(changed) > 0 {
			sort.Slice(changed, func(i, j int) bool { return changed[i] < changed[j] })
			return &mergeRefusal{cause: fmt.Errorf("%w: resources %v", ErrMergeContentChanged, changed), ids: changed}
		}

		if cluster.Tier != models.ReductionTierNear {
			return nil
		}
		pairs, err := txCtx.similarWithinLocking(cluster.WinnerID, txCtx.similarityThreshold(), true)
		if err != nil {
			return err
		}
		paired := map[uint]bool{}
		for _, pair := range pairs {
			paired[pair.ResourceID] = true
		}
		var unpaired []uint
		for _, id := range cluster.LoserIDs() {
			if !paired[id] {
				unpaired = append(unpaired, id)
			}
		}
		if len(unpaired) > 0 {
			return &mergeRefusal{cause: fmt.Errorf("%w: resources %v", ErrMergePairGone, unpaired), ids: unpaired}
		}
		return nil
	}
}

// demoteClusterToStale takes back a claim whose merge was refused. Unlike
// markClusterStale it acts on a Cluster this batch has already marked applied,
// which is the only writer that may move one off that state.
func (ctx *MahresourcesContext) demoteClusterToStale(reductionID uint, clusterID, claimID, reason string, changed []uint) bool {
	return ctx.mutateCluster(reductionID, clusterID, func(cluster *models.ReductionCluster) bool {
		if cluster.State != models.ReductionClusterApplied || cluster.ClaimID != claimID {
			return false
		}
		cluster.State = models.ReductionClusterStale
		cluster.StaleReason = reason
		cluster.ChangedMembers = changed
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
func (ctx *MahresourcesContext) revalidateCluster(cluster *models.ReductionCluster) (string, []uint) {
	loserIDs := cluster.LoserIDs()
	if len(loserIDs) == 0 {
		return staleReasonNothingToDo, nil
	}

	// Loaded through the scoped handle, so "no longer exists" and "no longer one
	// this principal may see" are the same answer — which is the apply-side half
	// of re-checking membership against the *current* principal. A reviewer whose
	// subtree shrank must not be able to destroy what they can no longer be shown.
	wanted := append([]uint{cluster.WinnerID}, loserIDs...)
	resources, err := ctx.loadResourcesByID(wanted)
	if err != nil {
		return err.Error(), nil
	}
	// Every member whose bytes changed is recorded, not just the first: the card
	// has to say which copy to leave out, and whether the kept one is among them.
	var changed []uint
	for _, member := range cluster.Members {
		if member.Ejected {
			continue
		}
		resource := resources[member.ResourceID]
		if resource == nil {
			return staleReasonMemberGone, nil
		}
		if resource.Hash != member.Hash {
			changed = append(changed, member.ResourceID)
		}
	}
	if len(changed) > 0 {
		return staleReasonHashChanged, changed
	}

	if cluster.Tier != models.ReductionTierNear {
		return "", nil
	}
	pairs, err := ctx.similarWithin(cluster.WinnerID, ctx.similarityThreshold())
	if err != nil {
		return err.Error(), nil
	}
	paired := map[uint]bool{}
	for _, pair := range pairs {
		paired[pair.ResourceID] = true
	}
	var unpaired []uint
	for _, id := range loserIDs {
		if !paired[id] {
			unpaired = append(unpaired, id)
		}
	}
	if len(unpaired) > 0 {
		return staleReasonPairGone, unpaired
	}
	return "", nil
}

// clusterFingerprint is everything an apply decides about: the Winner, the exact
// set of Losers, and the content each of them was reviewed as holding.
//
// The hashes are in it because the Cluster id is not enough. That id is derived
// from the tier and the member ids, so a recompute landing mid-batch can replace
// a Cluster with one of the same membership and *different* reviewed hashes —
// same id, different bytes. Claiming by id alone would then merge a proposal this
// reviewer never saw; recording an outcome by id alone would attach a refusal to
// a Cluster that was never the one that failed.
func clusterFingerprint(cluster *models.ReductionCluster) string {
	parts := make([]string, 0, len(cluster.Members))
	for _, member := range cluster.Members {
		if member.Ejected {
			continue
		}
		parts = append(parts, strconv.FormatUint(uint64(member.ResourceID), 10)+"@"+member.Hash)
	}
	sort.Strings(parts)
	return strconv.FormatUint(uint64(cluster.WinnerID), 10) + "->" + strings.Join(parts, ",")
}

// staleWrite is what recording a stale outcome came to.
type staleWrite int

const (
	staleRecorded   staleWrite = iota // the Cluster is now stale
	staleSuperseded                   // another decision got there first
	staleHidden                       // the Cluster left the caller's scope
)

// recordStaleLocked records a Cluster as stale the way the merge does its work:
// the Reduction row first, then every member (ejected ones included) through the
// caller's scope as it is now, locked in canonical order, and only then the write.
// It refuses to write unless the Cluster is still open and approved with the very
// proposal that failed revalidation, so a decision made in between stands; and a
// Cluster that left the caller's scope is neither recorded nor reported.
func (ctx *MahresourcesContext) recordStaleLocked(reductionID uint, failed *models.ReductionCluster, reason string, changed []uint) staleWrite {
	fingerprint := clusterFingerprint(failed)
	outcome := staleSuperseded
	err := ctx.WithTransaction(func(txCtx *MahresourcesContext) error {
		outcome = staleSuperseded
		if txCtx.db.Dialector.Name() == "postgres" {
			var locked models.ResourceReduction
			if err := txCtx.db.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").First(&locked, reductionID).Error; err != nil {
				return err
			}
		}
		rescoped, err := txCtx.rescopeTx(txCtx.db)
		if err != nil {
			return err
		}
		txCtx.db = rescoped
		row, err := txCtx.loadReductionForUpdate(reductionID, nil, false)
		if err != nil {
			return err
		}
		plan, err := DecodeReductionPlan(row.Plan)
		if err != nil {
			return err
		}
		current := findCluster(&plan, failed.ID)
		if current == nil || current.State != models.ReductionClusterOpen || !clusterApproved(current) || clusterFingerprint(current) != fingerprint {
			return nil
		}
		if err := txCtx.lockClusterMembersVisible([]*models.ReductionCluster{current}); err != nil {
			if errors.Is(err, ErrReductionClusterNotFound) {
				outcome = staleHidden
				return nil
			}
			return err
		}
		current.State = models.ReductionClusterStale
		current.StaleReason = reason
		current.ChangedMembers = changed
		// Unchecked, so the next apply does not walk into the same refusal, and
		// frozen, so a recompute leaves it where the reviewer can look at it.
		current.Checked = false
		current.Reviewed = true
		encoded, err := encodeJSON(plan)
		if err != nil {
			return err
		}
		ok, err := txCtx.casReduction(row.ID, row.Version, map[string]any{"plan": encoded})
		if err != nil {
			return err
		}
		if !ok {
			return ErrReductionConflict
		}
		outcome = staleRecorded
		return nil
	})
	if err != nil {
		ctx.logReductionWriteFailure(reductionID, failed.ID, err)
		return staleHidden
	}
	return outcome
}

// mutateCluster applies one change to one Cluster under the version
// compare-and-set, re-reading on each attempt.
//
// The mutation reports whether it still applies. That is what stops an outcome
// being written over a newer one: a concurrent apply that reached this Cluster
// first has already moved it off `open`, and recording "stale" over its "applied"
// would describe a merge that did happen as one that did not.
func (ctx *MahresourcesContext) mutateCluster(reductionID uint, clusterID string, mutate func(*models.ReductionCluster) bool) bool {
	return ctx.mutatePlan(reductionID, clusterID, func(plan *models.ResourceReductionPlan) bool {
		cluster := findCluster(plan, clusterID)
		return cluster != nil && mutate(cluster)
	})
}

// mutatePlan is the read-modify-write every bookkeeping write of the plan goes
// through: re-read, change, compare-and-set, again while it loses. The whole
// decoded plan is what is written back, so the parts a caller does not touch (the
// settings and the last apply record included) are carried over by construction.
// label names what is being recorded, for the log when it cannot land.
func (ctx *MahresourcesContext) mutatePlan(reductionID uint, label string, mutate func(*models.ResourceReductionPlan) bool) bool {
	for attempt := 0; attempt < reductionCASRetries; attempt++ {
		current, err := ctx.loadReductionForUpdate(reductionID, nil, false)
		if err != nil {
			ctx.logReductionWriteFailure(reductionID, label, err)
			return false
		}
		plan, err := DecodeReductionPlan(current.Plan)
		if err != nil {
			ctx.logReductionWriteFailure(reductionID, label, err)
			return false
		}
		if !mutate(&plan) {
			return false
		}
		encoded, err := encodeJSON(plan)
		if err != nil {
			ctx.logReductionWriteFailure(reductionID, label, err)
			return false
		}
		ok, err := ctx.casReduction(current.ID, current.Version, map[string]any{"plan": encoded})
		if err != nil {
			ctx.logReductionWriteFailure(reductionID, label, err)
			return false
		}
		if ok {
			return true
		}
	}
	ctx.logReductionWriteFailure(reductionID, label, ErrReductionConflict)
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

// releaseClaim gives back this attempt's claim: the Cluster returns to open and
// approved, exactly as it was approved, with nothing merged.
func (ctx *MahresourcesContext) releaseClaim(reductionID uint, clusterID, claimID string) {
	_ = ctx.mutateCluster(reductionID, clusterID, func(cluster *models.ReductionCluster) bool {
		if cluster.State != models.ReductionClusterApplied || cluster.Merged || cluster.ClaimID != claimID {
			return false
		}
		cluster.State = models.ReductionClusterOpen
		cluster.Checked = true
		cluster.AppliedAt = nil
		cluster.ClaimID = ""
		return true
	})
}

// heldMemberIDs are the Cluster's members the merge does not touch (the ejected
// ones), which it must still hold still: they are locked in the merge's own
// canonical pass. For a scoped caller each must be there and visible, or the merge
// fails and the claim is released; for anyone else one that is gone is a deleted
// copy and nothing to wait for.
func heldMemberIDs(cluster *models.ReductionCluster) []uint {
	var ids []uint
	for _, member := range cluster.Members {
		if member.ResourceID != cluster.WinnerID && !member.IsLoser(cluster.WinnerID) {
			ids = append(ids, member.ResourceID)
		}
	}
	return ids
}

// newClaimID is the token of one claim.
func newClaimID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// reconcileClaims settles Clusters an earlier batch claimed and never finished. A
// merge and its confirmation commit in one transaction, so a claimed Cluster that is
// not confirmed was not merged: it goes back to open and approved when its members
// all still exist, and is stale (a Resource in it no longer exists) otherwise.
// Existence is read unscoped, because a Resource outside the caller's subtree is not
// a missing one, and a viewer who cannot see it keeps it withheld on every read.
//
// It runs only inside a new apply's batch claim (or settle-only), on the plan that
// claim is about to write under the version compare-and-set, after the
// running-apply refusal: no live batch is on these Clusters, and a late settlement
// from an older claim finds a different claim id and does nothing.
func (ctx *MahresourcesContext) reconcileClaims(plan *models.ResourceReductionPlan) []contracts.ReductionApplyOutcome {
	var stale []contracts.ReductionApplyOutcome
	// A scoped caller settles only what they can fully see. A claim they cannot see
	// is left for an unrestricted caller, because settling it would change the row
	// (and its version) on the strength of a Cluster they have no business knowing.
	var visible map[uint]bool
	if ctx.isScopedPrincipal() {
		var claimed []*models.ReductionCluster
		for _, cluster := range plan.Clusters {
			if cluster.State == models.ReductionClusterApplied && !cluster.Merged {
				claimed = append(claimed, cluster)
			}
		}
		var err error
		if visible, err = ctx.visibleMemberIDs(reductionMemberIDs(claimed)); err != nil {
			return nil
		}
	}
	for _, cluster := range plan.Clusters {
		if cluster.State != models.ReductionClusterApplied || cluster.Merged {
			continue
		}
		if visible != nil && clusterWithheld(cluster, visible) {
			continue
		}
		ids := make([]uint, 0, len(cluster.Members))
		for _, member := range cluster.Members {
			if !member.Ejected || member.ResourceID == cluster.WinnerID {
				ids = append(ids, member.ResourceID)
			}
		}
		var existing []uint
		if err := ctx.unscopedDB().Model(&models.Resource{}).Where("id IN ?", ids).Pluck("id", &existing).Error; err != nil {
			continue
		}
		if len(existing) == len(ids) {
			cluster.State, cluster.Checked, cluster.AppliedAt, cluster.ClaimID = models.ReductionClusterOpen, true, nil, ""
			continue
		}
		cluster.State, cluster.Checked, cluster.AppliedAt = models.ReductionClusterStale, false, nil
		cluster.StaleReason = staleReasonDeleted
		cluster.ClaimID = ""
		stale = append(stale, contracts.ReductionApplyOutcome{
			ClusterID: cluster.ID, Tier: cluster.Tier, WinnerID: cluster.WinnerID, LoserIDs: cluster.LoserIDs(), Reason: staleReasonDeleted,
		})
	}
	return stale
}

// settleReductionClaims is an apply that only settles: the same refusals and the
// same reconciliation as a batch claim, and nothing is merged, claimed or recorded
// as an apply. It is how a Cluster stranded by a crash is released without sending
// everything else that is approved along with it.
func (ctx *MahresourcesContext) settleReductionClaims(reduction *models.ResourceReduction) (*contracts.ReductionApplyResult, error) {
	plan, err := DecodeReductionPlan(reduction.Plan)
	if err != nil {
		return nil, err
	}
	if last := plan.LastApply; last != nil && last.FinishedAt == nil && time.Since(last.StartedAt) <= ReductionApplyCeiling {
		return nil, ErrReductionConflict
	}
	before, err := encodeJSON(plan)
	if err != nil {
		return nil, err
	}
	_ = ctx.reconcileClaims(&plan)
	after, err := encodeJSON(plan)
	if err != nil {
		return nil, err
	}
	if string(before) != string(after) {
		ok, err := ctx.casReduction(reduction.ID, reduction.Version, map[string]any{"plan": after})
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrReductionConflict
		}
	}
	return &contracts.ReductionApplyResult{}, nil
}
