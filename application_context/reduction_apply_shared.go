package application_context

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"mahresources/contracts"
	"mahresources/models"
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
var reductionApplySettleTimeout = time.Minute

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

type clusterApplyOutcome struct {
	Applied bool
	// Omit leaves the Cluster out of the response: one the caller may not fully
	// see, or one the batch's deadline stopped it reaching. Nothing was written.
	Omit bool
	// Failed marks a Cluster the batch could not even read (not stale, not changed):
	// counted apart as an error.
	Failed bool
	// Changed marks a Cluster another decision moved before its turn (unchecked,
	// skipped, ejected, recomputed). That is not a finding about the Cluster, so it
	// is counted apart from the stale ones and the Stale tab does not gain it.
	Changed bool
	Report  contracts.ReductionApplyOutcome
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

// staleWrite is what recording a stale outcome came to.
type staleWrite int

const (
	staleRecorded   staleWrite = iota // the Cluster is now stale
	staleSuperseded                   // another decision got there first
	staleHidden                       // the Cluster left the caller's scope
	staleFailed                       // the write itself failed: nothing was recorded
)

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

// newClaimID is the token of one claim.
func newClaimID() string {
	var b [12]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}
