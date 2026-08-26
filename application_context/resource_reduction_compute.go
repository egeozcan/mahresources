package application_context

import (
	"context"
	"errors"
	"fmt"
	"time"

	"mahresources/download_queue"
	"mahresources/models"
	"mahresources/models/types"
)

// ReductionComputeDeadline is how long a clustering job may hold a Reduction at
// `computing` before the row reads as failed and becomes recomputable.
//
// Generic queue jobs are not drained at shutdown — workers.Add exists only on the
// download path — so a restart mid-clustering leaves the row saying `computing`
// with nothing alive to move it off, on a table that never expires. The deadline
// is what prevents a Reduction being stranded there forever, and it is cheaper
// and safer than teaching the download queue to drain every generic job.
//
// A run that legitimately outlives it is not damaged: it still writes its plan,
// because the write is conditional on the row still naming *this* job. A second
// compute started in the meantime takes the row's job id with it, and the older
// run then discards its own result rather than overwriting a newer one.
const ReductionComputeDeadline = time.Hour

// reductionCASRetries bounds the compare-and-set loop that lands a finished plan.
// The only writers that can move the version during a compute are a widening of
// the Extent and a settings edit, neither of which touches the plan, so this loses
// at most to a burst of those.
const reductionCASRetries = 5

// ErrReductionComputeSuperseded is what a clustering run reports when the row it
// was computing for has since been handed to a different run.
var ErrReductionComputeSuperseded = errors.New("this Resource Reduction is being computed by a newer job")

// ErrReductionStaleCompute is what a finished clustering run reports when the
// Reduction changed under it — a widened Extent, an edited Winner Rule, or a
// review decision taken past the compute deadline.
var ErrReductionStaleCompute = errors.New("this Resource Reduction changed while it was being computed; compute it again")

// RequestReductionCompute puts a Reduction into `computing` and submits the
// clustering job.
//
// Clustering is never synchronous. Even the Identical tier is a GROUP BY over
// however much of the library the Extent reaches, and the page that asked for it
// is the page the reviewer is about to work on.
//
// The context this is called on is the *request-scoped* one, and the job closure
// captures it deliberately — the same handoff the group-export job makes. Scope
// rides inside the db handle, so a confined reviewer's clustering sees their
// subtree and nothing else without this code having to re-derive that. A db
// handle's statement context is always Background, so the job does not die when
// the request that started it returns.
func (ctx *MahresourcesContext) RequestReductionCompute(id uint, version uint, ownerUserID *uint, ownerRestricted bool, actorUserID *uint) (*models.ResourceReduction, error) {
	reduction, err := ctx.loadReductionForUpdate(id, ownerUserID, ownerRestricted)
	if err != nil {
		return nil, err
	}
	if EffectiveReductionStatus(reduction) == models.ReductionStatusComputing {
		return nil, ErrReductionBusy
	}

	now := time.Now()
	deadline := now.Add(ReductionComputeDeadline)
	// A nonce written before the job exists, and swapped for the job's own id when
	// the worker starts. Without it the worker's claim asks only "is the slot
	// empty", which a run delayed past its deadline answers yes to — taking the
	// slot of the newer run that replaced it, and then computing under the subtree
	// scope it captured an hour ago while the accepted recompute is turned away as
	// superseded.
	generation := "pending:" + string(types.NewUUIDv7())
	// The caller's version, not the one just read. Recompute replaces the plan,
	// so a request made from a page that predates somebody else's decisions would
	// discard them without their author ever seeing a refusal — and "every write
	// is a compare-and-set" has to mean this write too.
	ok, err := ctx.casReduction(reduction.ID, version, map[string]any{
		"status":               models.ReductionStatusComputing,
		"computing_started_at": now,
		"compute_deadline":     deadline,
		"compute_job_id":       generation,
		"compute_error":        "",
	})
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrReductionConflict
	}

	dm := ctx.DownloadManager()
	if dm == nil {
		// No queue in this deployment (the CLI's and the tests' bare contexts).
		// Running it inline is better than leaving the row at `computing` with
		// nothing that will ever move it.
		if runErr := ctx.runReductionCompute(context.Background(), reduction.ID, "", nil); runErr != nil {
			return nil, runErr
		}
		return ctx.loadReductionForUpdate(reduction.ID, ownerUserID, ownerRestricted)
	}

	// The owner is named at construction rather than set afterwards: under -auth
	// the SSE stream drops any event whose job the principal may not see, so a
	// job with no owner yet never reaches its own submitter's jobs panel — which
	// is the only place the progress of this run is visible.
	_, err = dm.SubmitJobWithOptions(download_queue.JobOptions{
		Source:       download_queue.JobSourceResourceReduction,
		InitialPhase: "clustering",
		OwnerUserID:  actorUserID,
	}, func(jobCtx context.Context, j *download_queue.DownloadJob, p download_queue.ProgressSink) error {
		// The row is told which job owns it here, from inside the worker, and not
		// by the caller after SubmitJobWithOptions returns. SubmitJobWithOptions
		// starts the goroutine before it returns, so a fast run could finish and
		// find compute_job_id still empty — read that as "a newer job owns this
		// row", discard its own finished plan, and leave the Reduction at
		// `computing` with nothing alive to move it off. Measured: one run in three
		// of the whole api_tests package.
		if claimErr := ctx.claimReductionComputeJob(reduction.ID, generation, j.ID); claimErr != nil {
			// A superseded run leaves the row alone: the newer request owns it and
			// will report its own outcome. Every other failure has to be recorded
			// here, because nothing downstream will — this run never reached
			// runReductionCompute, and an unrecorded refusal leaves the Reduction at
			// `computing` with nothing alive to move it until its deadline.
			if !errors.Is(claimErr, ErrReductionComputeSuperseded) {
				if writeErr := ctx.recordReductionComputeFailure(reduction.ID, generation, claimErr); writeErr != nil {
					ctx.Logger().Warning(models.LogActionUpdate, "resource_reduction", &reduction.ID, reduction.Name,
						"Could not record a clustering job that failed to claim its slot: "+writeErr.Error(), nil)
				}
			}
			return claimErr
		}
		return ctx.runReductionCompute(jobCtx, reduction.ID, j.ID, p)
	})
	if err != nil {
		// The queue refused it, so nothing is going to compute this. Put the row
		// back rather than leaving it at `computing` until the deadline.
		// Re-read rather than assuming the version is the claim's + 1: a widening
		// or a settings edit can land between the claim and the queue's refusal, and
		// a compare-and-set against a guessed version quietly affects zero rows —
		// leaving the Reduction at `computing` with nothing running, until its
		// deadline. Retried, and reported when it cannot be done.
		if undoErr := ctx.recordReductionComputeFailure(reduction.ID, "", err); undoErr != nil {
			ctx.Logger().Warning(models.LogActionUpdate, "resource_reduction", &reduction.ID, reduction.Name, "Could not record a refused clustering job: "+undoErr.Error(), nil)
		}
		return nil, err
	}

	return ctx.loadReductionForUpdate(reduction.ID, ownerUserID, ownerRestricted)
}

// claimReductionComputeJob records which queue job owns the row's `computing`
// state.
//
// Bookkeeping rather than a decision, so it is a plain conditional UPDATE rather
// than a write through the version: nothing about the plan changes here, and the
// version belongs to the plan. The `compute_job_id = ”` predicate is what makes
// it idempotent under a retry, and the `status = 'computing'` predicate is what
// stops a run whose row has since been recomputed from re-claiming it.
func (ctx *MahresourcesContext) claimReductionComputeJob(reductionID uint, generation, jobID string) error {
	for attempt := 0; attempt < reductionCASRetries; attempt++ {
		res := ctx.db.Model(&models.ResourceReduction{}).
			Where("id = ? AND status = ? AND compute_job_id = ?", reductionID, models.ReductionStatusComputing, generation).
			Update("compute_job_id", jobID)
		if res.Error != nil {
			if isLockContentionError(res.Error) {
				waitOutContention(attempt)
				continue
			}
			return res.Error
		}
		if res.RowsAffected == 1 {
			return nil
		}
		// Zero rows means this run's generation is no longer the one the row is
		// waiting for, and that has two causes. The claim already happened — a
		// retry of this same claim — which is fine. Or a newer request replaced the
		// generation, in which case this run must stop now rather than compute a
		// plan it would only be told to discard at the end. Reporting success on
		// both, which an unchecked Update does, lets a delayed run keep working
		// under somebody else's request.
		var current models.ResourceReduction
		if err := ctx.db.Select("compute_job_id").First(&current, reductionID).Error; err != nil {
			if isLockContentionError(err) {
				waitOutContention(attempt)
				continue
			}
			return err
		}
		if current.ComputeJobID == jobID {
			return nil
		}
		return ErrReductionComputeSuperseded
	}
	return fmt.Errorf("could not record the clustering job id for Resource Reduction %d", reductionID)
}

// reductionComputeAttempts bounds the retry on lock contention.
//
// Clustering is a long read over tables the rest of the deployment is writing —
// uploads, the hash worker, another Reduction — so losing a lock partway through
// is transient contention rather than a failure on the run's own merits, and the
// run has written nothing at that point. Recording it as a failed compute would
// leave the reviewer looking at a Reduction that needs recomputing for no reason
// they could act on. The same reasoning AddResource's phase-3 retry gives.
const reductionComputeAttempts = 3

// runReductionCompute is the clustering job's body.
func (ctx *MahresourcesContext) runReductionCompute(jobCtx context.Context, reductionID uint, jobID string, progress download_queue.ProgressSink) error {
	var plan models.ResourceReductionPlan
	var version uint
	var err error
	for attempt := 0; attempt < reductionComputeAttempts; attempt++ {
		plan, version, err = ctx.computeReductionPlan(jobCtx, reductionID, progress)
		if err == nil || !isLockContentionError(err) || jobCtx.Err() != nil {
			break
		}
		// Backing off, like the bookkeeping loops. Three immediate attempts are
		// three attempts inside a few microseconds, so a lock held across them
		// consumes the whole budget and records a compute that was only unlucky as
		// one that failed.
		waitOutContention(attempt)
	}
	if err != nil {
		if writeErr := ctx.recordReductionComputeFailure(reductionID, jobID, err); writeErr != nil {
			return fmt.Errorf("%w (and the failure could not be recorded: %v)", err, writeErr)
		}
		return err
	}
	if storeErr := ctx.storeReductionPlan(reductionID, jobID, version, plan); storeErr != nil {
		// A superseded run belongs to nobody: the newer job owns the row and will
		// report its own outcome, so this one must not overwrite it. Every other
		// refusal has to be recorded, or the row sits at `computing` until its
		// deadline with nothing alive to move it — which is the exact stranding
		// the deadline exists to bound rather than to cause.
		if errors.Is(storeErr, ErrReductionComputeSuperseded) {
			return storeErr
		}
		if writeErr := ctx.recordReductionComputeFailure(reductionID, jobID, storeErr); writeErr != nil {
			return fmt.Errorf("%w (and the failure could not be recorded: %v)", storeErr, writeErr)
		}
		return storeErr
	}
	return nil
}

// computeReductionPlan does the work: resolve the Extent, cluster it, and carry
// forward every judgement already made.
func (ctx *MahresourcesContext) computeReductionPlan(jobCtx context.Context, reductionID uint, progress download_queue.ProgressSink) (models.ResourceReductionPlan, uint, error) {
	var plan models.ResourceReductionPlan

	// Read unscoped by owner: this runs for the Reduction the request already
	// authorized, and the principal's *data* scope still applies through the db
	// handle. Re-applying the owner predicate here would fail under the auth-off
	// super-user, whose id the job does not carry.
	reduction, err := ctx.loadReductionForUpdate(reductionID, nil, false)
	if err != nil {
		return plan, 0, err
	}
	// The version this plan is *about*. Everything below describes the Extent,
	// the Winner Rule and the decisions as they stand right now, so the write at
	// the end has to lose if any of them moved.
	version := reduction.Version

	storedExtent, err := DecodeReductionExtent(reduction.Extent)
	if err != nil {
		return plan, version, err
	}
	previous, err := DecodeReductionPlan(reduction.Plan)
	if err != nil {
		return plan, version, err
	}
	rule := DecodeWinnerRule(reduction.WinnerRule)

	if progress != nil {
		progress.SetPhase("resolving the Extent")
	}
	extent, err := ctx.resolveReductionExtent(storedExtent)
	if err != nil {
		return plan, version, err
	}
	if err := jobCtx.Err(); err != nil {
		return plan, version, err
	}

	if progress != nil {
		progress.SetPhase("measuring coverage")
	}
	coverage, err := ctx.reductionCoverage(extent)
	if err != nil {
		return plan, version, err
	}
	plan.Coverage = coverage

	// A judgement already made is never rearranged by a later compute. Frozen
	// Clusters carry over whole and their members are held out of the pool, so
	// growing the Extent can add Clusters but cannot move a Resource out of one
	// the reviewer has acted on.
	//
	// An *applied* Cluster is different, and deliberately: its Losers no longer
	// exist and its Winner goes back into the pool as an ordinary candidate. What
	// freezing protects is the judgement, not the Winner's future eligibility —
	// a duplicate arriving next month has to be catchable.
	carried := make([]*models.ReductionCluster, 0, len(previous.Clusters))
	excluded := map[uint]bool{}
	for _, cluster := range previous.Clusters {
		if cluster.State == models.ReductionClusterApplied {
			carried = append(carried, cluster)
			continue
		}
		if !cluster.Frozen() {
			continue
		}
		carried = append(carried, cluster)
		for _, member := range cluster.Members {
			excluded[member.ResourceID] = true
		}
	}

	if progress != nil {
		progress.SetPhase("Identical Resources")
	}
	identical, err := ctx.clusterIdentical(extent, rule, excluded, func(done, total int64) {
		if progress != nil {
			progress.SetPhaseProgress(done, total)
		}
	})
	if err != nil {
		return plan, version, err
	}
	if err := jobCtx.Err(); err != nil {
		return plan, version, err
	}

	// Every Resource an Identical Cluster claims — Winner and Losers alike —
	// leaves the pool before the perceptual tier runs. Byte-identical images
	// decode to identical pixels and are therefore also stored as distance-zero
	// pairs, so without this the same Resources would appear in two Clusters with
	// different defaults and possibly different Winners. When both apply, take the
	// fact.
	fresh := identical
	for _, cluster := range identical {
		for _, member := range cluster.Members {
			excluded[member.ResourceID] = true
		}
	}

	if reduction.MatchingMode != models.MatchingModeIdenticalOnly {
		if progress != nil {
			progress.SetPhase("Near-Identical Resources")
		}
		near, nearErr := ctx.clusterNearIdentical(jobCtx, extent, rule, excluded, func(done, total int64) {
			if progress != nil {
				progress.SetPhaseProgress(done, total)
			}
		})
		if nearErr != nil {
			return plan, version, nearErr
		}
		fresh = append(fresh, near...)
	}

	plan.Clusters = append(carried, fresh...)
	ensureDistinctClusterIDs(plan.Clusters)
	return plan, version, nil
}

// StoreReductionPlanForTest exposes the plan write so a test can reproduce the
// one interleaving that has no other seam: a run that finishes after the row it
// was computing for has moved on. The clustering job is the only real caller, and
// forcing it to lose a race it wins in microseconds is not something a test can do
// from outside.
func (ctx *MahresourcesContext) StoreReductionPlanForTest(reductionID uint, jobID string, version uint, plan models.ResourceReductionPlan) error {
	return ctx.storeReductionPlan(reductionID, jobID, version, plan)
}

// storeReductionPlan lands a finished plan.
//
// Two guards, doing different jobs. The version compare-and-set is the ordinary
// one every writer on this row goes through. The job-id check is what makes a run
// that outlived its own deadline harmless: if a second compute has since claimed
// the row, this result describes an Extent nobody is waiting for, and writing it
// would overwrite a newer plan with an older one.
func (ctx *MahresourcesContext) storeReductionPlan(reductionID uint, jobID string, version uint, plan models.ResourceReductionPlan) error {
	encoded, err := encodeJSON(plan)
	if err != nil {
		return err
	}
	now := time.Now()

	for attempt := 0; attempt < reductionCASRetries; attempt++ {
		current, err := ctx.loadReductionForUpdate(reductionID, nil, false)
		if err != nil {
			if isLockContentionError(err) {
				waitOutContention(attempt)
				continue
			}
			return err
		}
		if jobID != "" && current.ComputeJobID != jobID {
			return ErrReductionComputeSuperseded
		}
		if current.Version != version {
			// Refused, not rebased. The plan describes the Extent, the Winner Rule
			// and the decisions as they were when the run began; writing it over a
			// row that has since been widened, re-ruled or overridden would present
			// a stale proposal as current — with a fresh computed_at, so the page's
			// own drift line would report zero and the reviewer would have no way to
			// tell. R8 says a stale write is refused; this is one.
			return ErrReductionStaleCompute
		}
		ok, err := ctx.casReduction(current.ID, version, map[string]any{
			"plan":                 encoded,
			"status":               models.ReductionStatusReady,
			"computed_at":          now,
			"computing_started_at": nil,
			"compute_deadline":     nil,
			"compute_error":        "",
		})
		if err != nil {
			// Contention is retried like a lost compare-and-set: this write is the
			// only record that the run succeeded, and losing it would leave the
			// Reduction reading as failed with a finished plan nobody can see.
			if isLockContentionError(err) {
				waitOutContention(attempt)
				continue
			}
			return err
		}
		if ok {
			return nil
		}
	}
	return ErrReductionConflict
}

func (ctx *MahresourcesContext) recordReductionComputeFailure(reductionID uint, jobID string, cause error) error {
	for attempt := 0; attempt < reductionCASRetries; attempt++ {
		current, err := ctx.loadReductionForUpdate(reductionID, nil, false)
		if err != nil {
			// Contention is retried rather than returned. This is the only writer
			// that can move a Reduction off `computing` once its worker has given
			// up, so losing the read to a lock leaves the row stranded for an hour
			// — which is the deadline doing the job of a bug rather than its own.
			if isLockContentionError(err) {
				waitOutContention(attempt)
				continue
			}
			return err
		}
		if jobID != "" && current.ComputeJobID != jobID {
			// A newer run owns the row — the token here is whatever this run last
			// held, its generation nonce before the claim and its job id after, and
			// either way a mismatch means somebody else's outcome is the one that
			// counts.
			return nil
		}
		ok, err := ctx.casReduction(current.ID, current.Version, map[string]any{
			"status":               models.ReductionStatusFailed,
			"computing_started_at": nil,
			"compute_deadline":     nil,
			"compute_error":        truncateRunes(cause.Error(), 2000),
		})
		if err != nil {
			if isLockContentionError(err) {
				waitOutContention(attempt)
				continue
			}
			return err
		}
		if ok {
			return nil
		}
	}
	return ErrReductionConflict
}

// ensureDistinctClusterIDs guarantees no two Clusters in one plan answer to the
// same id.
//
// The id is derived from the tier and the member ids, which is what keeps a
// frozen Cluster addressable across recomputes — and it means a *carried* Cluster
// and a *fresh* one over the same members collide exactly. That is not
// hypothetical: an applied Cluster is carried forward while its members return to
// the pool, so a crash between an apply's claim and its merge leaves the Losers
// alive to re-cluster into the same set. findCluster takes the first match, so
// every override and every apply on the fresh Cluster would be refused as already
// settled, permanently.
//
// Numbering the collision rather than changing the derivation keeps the ordinary
// case stable, which is the property the derivation exists for.
func ensureDistinctClusterIDs(clusters []*models.ReductionCluster) {
	seen := make(map[string]bool, len(clusters))
	for _, cluster := range clusters {
		if !seen[cluster.ID] {
			seen[cluster.ID] = true
			continue
		}
		for suffix := 2; ; suffix++ {
			candidate := fmt.Sprintf("%s-%d", cluster.ID, suffix)
			if !seen[candidate] {
				cluster.ID = candidate
				seen[candidate] = true
				break
			}
		}
	}
}

// waitOutContention pauses between compare-and-set attempts that lost to a lock.
//
// Five immediate retries are five attempts inside a few microseconds, which is no
// wait at all: a lock held across them is held across all of them, and the writer
// gives up on a Reduction that is merely busy. Backing off turns the retry budget
// into an actual window. Deliberately short — this runs on a queue worker, and the
// alternative to waiting is a row stranded until its deadline.
func waitOutContention(attempt int) {
	time.Sleep(time.Duration(attempt+1) * 20 * time.Millisecond)
}
