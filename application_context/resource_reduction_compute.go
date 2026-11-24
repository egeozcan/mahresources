package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"mahresources/download_queue"
	"mahresources/jobs"
	"mahresources/models"
)

// ReductionComputeDeadline is how long a clustering job may hold a Reduction at
// `computing` before the row reads as failed and becomes recomputable.
//
// Generic queue jobs take the shared five-second shutdown drain. The deadline
// still recovers a Reduction left computing after a process crash or a worker
// that outlives that bounded drain, on a table that never expires.
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
	if err := reductionUpgrading(reduction); err != nil {
		return nil, err
	}
	if EffectiveReductionStatus(reduction) == models.ReductionStatusComputing {
		return nil, ErrReductionBusy
	}
	if err := ctx.requireSimilarityIndexes(); err != nil {
		return nil, err
	}

	// The caller's version, not the one just read. Recompute replaces the plan,
	// so a request made from a page that predates somebody else's decisions would
	// discard them without their author ever seeing a refusal — and "every write
	// is a compare-and-set" has to mean this write too. The same statement refuses
	// a running apply or settle, so queued and replayed starts are refused too.
	nonce, generation, err := ctx.beginReductionComputeStore(reduction.ID, version)
	if err != nil {
		return nil, err
	}

	dm := ctx.DownloadManager()
	if dm == nil {
		// No queue in this deployment (the CLI's and the tests' bare contexts).
		// Running it inline is better than leaving the row at `computing` with
		// nothing that will ever move it.
		if runErr := ctx.runReductionCompute(context.Background(), reduction.ID, "", generation, nil); runErr != nil {
			return nil, runErr
		}
		return ctx.loadReductionForUpdate(reduction.ID, ownerUserID, ownerRestricted)
	}

	// The durable Job is admitted before anything is dispatched, and the queue entry
	// that runs the clustering takes the id the Job's own handle records — the same
	// order and the same one-id rule every other queue-backed Kind follows. The
	// admission is also what decides whether this process runs the clustering at all:
	// a Job admitted while the deployment's budget is full waits for whichever runtime
	// has a free slot, and takes the Reduction's compute claim when it gets there.
	legacyID := ""
	admission := queueJobAdmission{}
	var clusterInput *reductionComputeJobInput
	if service := ctx.JobService(); service != nil {
		clusterInput = &reductionComputeJobInput{ReductionID: reduction.ID, Version: version}
		input, marshalErr := json.Marshal(clusterInput)
		if marshalErr != nil {
			ctx.undoRefusedReductionCompute(reduction.ID, nonce, generation, marshalErr)
			return nil, marshalErr
		}
		legacyID = download_queue.NewJobID()
		admitted, acceptErr := ctx.admitQueueJob(jobs.Acceptance{
			Kind:        JobKindReductionCompute,
			KindVersion: jobReductionKindVersion,
			State:       jobs.StateQueued,
			OwnerUserID: actorUserID,
			ActorUserID: actorUserID,
			Origin:      "api",
			Title:       reductionComputeJobTitle(reduction.Name),
			Replay:      jobs.ReplayInput{Input: input},
			LegacyRefs:  []jobs.LegacyRef{{Namespace: ReductionComputeHandleNamespace, Handle: legacyID}},
		})
		if acceptErr != nil {
			ctx.undoRefusedReductionCompute(reduction.ID, nonce, generation, acceptErr)
			return nil, acceptErr
		}
		admission = admitted
		if !admission.Owned() {
			// The deployment's budget is full: the Job is durable, answers the id the
			// caller was handed, and starts no executor here. The row's compute claim is
			// handed back with it — the runtime that eventually dispatches the Job takes
			// the claim itself, and a row left `computing` would refuse it as busy. See
			// admitQueueJob.
			ctx.releaseReductionComputeClaim(reduction, generation)
			return ctx.loadReductionForUpdate(reduction.ID, ownerUserID, ownerRestricted)
		}
	}

	// The owner is named at construction rather than set afterwards: under -auth
	// the SSE stream drops any event whose job the principal may not see, so a
	// job with no owner yet never reaches its own submitter's jobs panel — which
	// is the only place the progress of this run is visible.
	entry, err := ctx.startReductionComputeQueueJob(reduction.ID, nonce, generation, legacyID,
		jobs.ExecutionRef{JobID: admission.Execution.JobID, ExecutionToken: admission.Execution.ExecutionToken}, actorUserID)
	if err != nil {
		// The queue refused it, so nothing is going to compute this. Put the row
		// back rather than leaving it at `computing` until the deadline.
		ctx.undoRefusedReductionCompute(reduction.ID, nonce, generation, err)
		ctx.failUndispatchedQueueJob(admission, err)
		return nil, err
	}
	if admission.Owned() {
		ctx.ownQueueExecution(admission, entry, func(owner *MahresourcesContext, snap *download_queue.DownloadJob) error {
			return (&reductionComputeAdapter{ctx: owner, kind: JobKindReductionCompute}).publishOutcome(admission.Execution, clusterInput, snap)
		})
	}

	// The read-back can lose a shared-cache table lock to the claim write.
	// SubmitJobWithOptions starts the worker goroutine before it returns, and the
	// worker's first act is claimReductionComputeJob — an UPDATE on this same
	// table — so the request's own final read races the job it just started.
	// SQLITE_LOCKED is a shared-cache table lock that does not go through
	// busy_timeout; in the WAL configuration the app runs under a reader never
	// blocks on a writer, so this only fires under a shared-cache test DSN. The request has already landed by the time this read runs — the CAS
	// wrote `computing` and the job is in the queue — so a lock lost here must not
	// report the request as failed to the caller. Retried exactly like the claim
	// write's own loop above; anything that is not contention still fails
	// immediately.
	var fresh *models.ResourceReduction
	var lastErr error
	for attempt := 0; attempt < reductionCASRetries; attempt++ {
		fresh, lastErr = ctx.loadReductionForUpdate(reduction.ID, ownerUserID, ownerRestricted)
		if lastErr == nil {
			return fresh, nil
		}
		if !isLockContentionError(lastErr) {
			return nil, lastErr
		}
		waitOutContention(attempt)
	}
	return nil, lastErr
}

// releaseReductionComputeClaim hands one clustering run's compute claim back when the
// admission could not take the capacity to honour it.
//
// A Job accepted to wait for a slot is not an execution: no worker has taken the
// Reduction, and the runtime that eventually dispatches the Job takes the claim
// itself, in `start`. A row left `computing` in the meantime would refuse that runtime
// as busy and leave the Job blocked for a person, so the state the request found is
// restored — and only while the row still carries this request's own generation,
// because a newer request that has taken the row since owns its outcome.
func (ctx *MahresourcesContext) releaseReductionComputeClaim(before *models.ResourceReduction, generation string) {
	if before == nil || generation == "" {
		return
	}
	err := retryOnLockContention(reductionCASRetries, func() error {
		// Only while the row still carries this request's own generation: a newer
		// request that has taken the row since owns its outcome, and restoring this
		// request's snapshot over it would take their claim away.
		return ctx.reductionStore().db.Exec(`UPDATE resource_reductions SET status = ?, computing_started_at = ?, compute_deadline = ?, compute_job_id = ?,
				compute_error = ?, compute_generation = NULL, compute_lease_until = 0, version = version + 1, updated_at = ?
			WHERE id = ? AND status = 'computing' AND compute_generation = ?`,
			before.Status, before.ComputingStartedAt, before.ComputeDeadline, before.ComputeJobID, before.ComputeError, time.Now(),
			before.ID, generation).Error
	})
	if err != nil {
		ctx.logReductionClaimRelease(before.ID, generation, err)
	}
}

// logReductionClaimRelease reports a compute claim that could not be handed back.
// The Job stays accepted either way: a person reading the log is what closes that, and
// the row's own deadline is what retires the claim it still carries.
func (ctx *MahresourcesContext) logReductionClaimRelease(reductionID uint, generation string, cause error) {
	ctx.Logger().Warning(models.LogActionUpdate, "resource_reduction", &reductionID, "",
		"Could not hand back the compute claim of a queued clustering job ("+generation+"): "+cause.Error(), nil)
}

// undoRefusedReductionCompute puts a Reduction back after a submission that never
// reached the queue.
//
// It re-reads rather than assuming the version is the claim's + 1: a widening or a
// settings edit can land between the claim and the refusal, and a compare-and-set
// against a guessed version quietly affects zero rows — leaving the Reduction at
// `computing` with nothing running until its deadline. The generation is what tells a
// refusal the row has since been handed to somebody else apart from one that is still
// this request's to undo.
func (ctx *MahresourcesContext) undoRefusedReductionCompute(reductionID uint, nonce, generation string, cause error) {
	var name string
	if reduction, err := ctx.loadReductionForUpdate(reductionID, nil, false); err == nil {
		name = reduction.Name
	}
	if undoErr := ctx.storeFailCompute(reductionID, nonce, generation, cause); undoErr != nil {
		ctx.Logger().Warning(models.LogActionUpdate, "resource_reduction", &reductionID, name,
			"Could not record a refused clustering job: "+undoErr.Error(), nil)
	}
}

// claimReductionComputeJob records which queue job owns the row's `computing`
// state.
//
// Bookkeeping rather than a decision, so it is a plain conditional UPDATE rather
// than a write through the version: nothing about the plan changes here, and the
// version belongs to the plan. The `compute_job_id = ”` predicate is what makes
// it idempotent under a retry, and the `status = 'computing'` predicate is what
// stops a run whose row has since been recomputed from re-claiming it.
func (ctx *MahresourcesContext) claimReductionComputeJob(reductionID uint, nonce, jobID string) error {
	for attempt := 0; attempt < reductionCASRetries; attempt++ {
		res := ctx.db.Model(&models.ResourceReduction{}).
			Where("id = ? AND status = ? AND compute_job_id = ?", reductionID, models.ReductionStatusComputing, nonce).
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

// runReductionCompute is the clustering job's body: it admits the run's lease, keeps
// it renewed while the Clusters are built in memory, and lands them as a new
// generation of rows (design D4, D11).
func (ctx *MahresourcesContext) runReductionCompute(jobCtx context.Context, reductionID uint, jobID, generation string, progress download_queue.ProgressSink) error {
	if err := ctx.storeAdmitComputeLease(reductionID, generation); err != nil {
		if errors.Is(err, ErrReductionLeaseLost) {
			// Another generation owns the row, or this reservation ran out: the newer
			// run reports its own outcome, so this one records nothing.
			return ErrReductionComputeSuperseded
		}
		return ctx.failReductionCompute(reductionID, jobID, generation, err)
	}
	started := time.Now()
	ticker, stopTicker := context.WithCancel(jobCtx)
	defer stopTicker()
	go runLeaseTicker(ticker, leaseRenewEvery, func(now time.Time) (bool, error) {
		return ctx.reductionStore().renewComputeLease(reductionID, generation, started, now)
	})

	var built *builtReductionPlan
	var err error
	for attempt := 0; attempt < reductionComputeAttempts; attempt++ {
		built, err = ctx.computeReductionPlan(jobCtx, reductionID, progress)
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
		return ctx.failReductionCompute(reductionID, jobID, generation, err)
	}
	if storeErr := ctx.storeLandComputedPlan(built.reduction, jobID, generation, built.version, built.fresh, built.coverage, built.settings); storeErr != nil {
		// A superseded run belongs to nobody: the newer job owns the row and will
		// report its own outcome, so this one must not overwrite it. Every other
		// refusal has to be recorded, or the row sits at `computing` until its
		// deadline with nothing alive to move it.
		if errors.Is(storeErr, ErrReductionComputeSuperseded) {
			return storeErr
		}
		return ctx.failReductionCompute(reductionID, jobID, generation, storeErr)
	}
	return nil
}

// failReductionCompute records a compute that ended without landing and returns the
// cause, saying so when the record itself could not be written.
func (ctx *MahresourcesContext) failReductionCompute(reductionID uint, jobID, generation string, cause error) error {
	if writeErr := ctx.storeFailCompute(reductionID, jobID, generation, cause); writeErr != nil {
		return fmt.Errorf("%w (and the failure could not be recorded: %v)", cause, writeErr)
	}
	return cause
}

// requireSimilarityIndexes refuses a compute until both effective-distance indexes
// the neighbour query reads exist and read valid (design D1).
func (ctx *MahresourcesContext) requireSimilarityIndexes() error {
	valid, err := models.SimilarityDistanceIndexesValid(ctx.reductionStore().db)
	if err != nil {
		return err
	}
	if !valid {
		return ErrReductionIndexesBuilding
	}
	return nil
}

// builtReductionPlan is what the in-memory clustering produced, ready to land: the
// fresh Clusters (carried ones are copied by the landing itself), the coverage and
// the settings the run used, and the version the plan is about.
type builtReductionPlan struct {
	reduction *models.ResourceReduction
	version   uint
	fresh     []*models.ReductionCluster
	coverage  models.ReductionCoverage
	settings  *models.ReductionSettings
}

// computeReductionPlan does the work: resolve the Extent, cluster it, holding out
// every Resource a carried judgement already claims.
func (ctx *MahresourcesContext) computeReductionPlan(jobCtx context.Context, reductionID uint, progress download_queue.ProgressSink) (*builtReductionPlan, error) {
	// Read unscoped by owner: this runs for the Reduction the request already
	// authorized, and the principal's *data* scope still applies through the db
	// handle. Re-applying the owner predicate here would fail under the auth-off
	// super-user, whose id the job does not carry.
	reduction, err := ctx.loadReductionForUpdate(reductionID, nil, false)
	if err != nil {
		return nil, err
	}
	// The version this plan is *about*. Everything below describes the Extent,
	// the Winner Rule and the decisions as they stand right now, so the landing
	// has to lose if any of them moved.
	built := &builtReductionPlan{reduction: reduction, version: reduction.Version}

	storedExtent, err := ctx.storedReductionExtent(reduction)
	if err != nil {
		return nil, err
	}
	rule := DecodeWinnerRule(reduction.WinnerRule)

	if progress != nil {
		progress.SetPhase("resolving the Extent")
	}
	extent, err := ctx.resolveReductionExtent(storedExtent)
	if err != nil {
		return nil, err
	}
	extent.ExcludeExternalResources = reduction.ExcludeExternalResources
	if err := jobCtx.Err(); err != nil {
		return nil, err
	}

	if progress != nil {
		progress.SetPhase("measuring coverage")
	}
	if built.coverage, err = ctx.reductionCoverage(extent); err != nil {
		return nil, err
	}

	// A judgement already made is never rearranged by a later compute. Frozen
	// Clusters carry over whole and their members are held out of the pool, so
	// growing the Extent can add Clusters but cannot move a Resource out of one
	// the reviewer has acted on.
	//
	// An *applied* Cluster is different, and deliberately: its Losers no longer
	// exist and its Winner goes back into the pool as an ordinary candidate. A
	// claim that is not confirmed is a keep decision in flight (or stranded by a
	// crash): its Resources stay out of the pool until it settles.
	excluded, err := ctx.reductionStore().carriedResourceIDs(reduction)
	if err != nil {
		return nil, err
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
		return nil, err
	}
	if err := jobCtx.Err(); err != nil {
		return nil, err
	}

	// Every Resource an Identical Cluster claims — Winner and Losers alike —
	// leaves the pool before the perceptual tier runs. Byte-identical images
	// decode to identical pixels and are therefore also stored as distance-zero
	// pairs, so without this the same Resources would appear in two Clusters with
	// different defaults and possibly different Winners. When both apply, take the
	// fact.
	built.fresh = identical
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
			return nil, nearErr
		}
		built.fresh = append(built.fresh, near...)
	}

	ensureDistinctClusterIDs(built.fresh)
	// What this plan was computed with, so the page can say when the settings have
	// since changed. The matching mode and rule are the normalised ones actually
	// used, not whatever the row's column happened to hold.
	built.settings = &models.ReductionSettings{
		MatchingMode:    normalizeMatchingMode(reduction.MatchingMode),
		WinnerRule:      rule,
		ExcludeExternal: reduction.ExcludeExternalResources,
	}
	return built, nil
}

// storedReductionExtent reads the Reduction's selection rows back as the Extent
// document the resolver takes, in id-keyset chunks.
func (ctx *MahresourcesContext) storedReductionExtent(r *models.ResourceReduction) (models.ResourceReductionExtent, error) {
	extent := models.ResourceReductionExtent{All: r.ExtentAll}
	s := ctx.reductionStore()
	for _, spec := range []struct {
		table, column string
		into          *[]uint
	}{
		{"reduction_extent_resources", "resource_id", &extent.ResourceIDs},
		{"reduction_extent_groups", "group_id", &extent.GroupIDs},
	} {
		var after uint
		for {
			var ids []uint
			if err := s.db.Raw(`SELECT `+spec.column+` FROM `+spec.table+` WHERE reduction_id = ? AND `+spec.column+` > ? ORDER BY `+spec.column+` LIMIT ?`,
				r.ID, after, reductionRowChunk*10).Scan(&ids).Error; err != nil {
				return extent, err
			}
			if len(ids) == 0 {
				break
			}
			*spec.into = append(*spec.into, ids...)
			after = ids[len(ids)-1]
		}
	}
	return extent, nil
}

// carriedResourceIDs is the pool a compute holds out: the members of every Cluster
// the landing will carry over that still claims them. An applied Cluster claims its
// members only until its merge is confirmed; read-only history claims only the
// copies a reviewer ejected (an explicit "don't remove" must not return in an
// auto-checked Cluster); any other frozen Cluster claims all of them.
func (s *reductionStore) carriedResourceIDs(r *models.ResourceReduction) (map[uint]bool, error) {
	out := map[uint]bool{}
	if r.PlanGeneration == nil {
		return out, nil
	}
	var after uint
	for {
		var rows []struct{ ID, ResourceID uint }
		if err := s.db.Raw(`SELECT m.id AS id, m.resource_id AS resource_id FROM reduction_members m JOIN reduction_clusters c ON c.id = m.cluster_id
			WHERE c.reduction_id = ? AND c.generation = ? AND m.id > ? AND (
				(c.state = 'applied' AND NOT c.merged)
				OR (c.state <> 'applied' AND c.read_only AND m.ejected)
				OR (c.state <> 'applied' AND NOT c.read_only AND (c.reviewed OR c.state IN ('skipped', 'stale'))))
			ORDER BY m.id LIMIT ?`, r.ID, *r.PlanGeneration, after, reductionRowChunk*10).Scan(&rows).Error; err != nil {
			return nil, err
		}
		if len(rows) == 0 {
			return out, nil
		}
		for _, row := range rows {
			out[row.ResourceID] = true
		}
		after = rows[len(rows)-1].ID
	}
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
