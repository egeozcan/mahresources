package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"mahresources/auth"
	"mahresources/download_queue"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/types"

	"gorm.io/gorm"
)

// This file is the Resource Reduction clustering Kind adapter.
//
// A clustering run is the one Kind here whose *domain row* owns most of the state:
// the Reduction holds the Extent, the plan, the version and the compute claim, and
// the queue job is only the process that walks it. So the two things worth saying
// are about who may take the row's claim:
//
//   - The claim is a compare-and-set with a generation nonce, and it stays the
//     domain's. Accepting a Job does not claim the row; the CAS does, exactly as it
//     did before this Kind had a durable identity. A Retry therefore does not replay
//     a stale claim — it reads the row as it stands now and takes it under a fresh
//     generation, which is what makes a superseded run discard its own plan rather
//     than overwrite a newer one.
//   - Retry is advertised only while the row says it is computable. Recomputing is
//     the one control this Kind offers that writes to the library's clustering
//     state, and a Retry whose row is already `ready` (somebody else recomputed) or
//     `computing` (a run is in flight) would be a second run of work that is done or
//     already running. What the row *is* is the durable answer, so the adapter reads
//     it rather than guessing from the Job's own state.

const (
	// JobKindReductionCompute is one Resource Reduction clustering run.
	JobKindReductionCompute = "resource-reduction-compute"
	// jobReductionKindVersion is the version of this Kind's input semantics.
	jobReductionKindVersion = 1
	// jobReductionOutput is the Reduction a succeeded run computed. Success depends
	// on it: a clustering run whose Reduction is gone computed nothing a reader can
	// open.
	jobReductionOutput = "reduction"
)

// reductionComputeJobInput is what a clustering Job is accepted with. The version is
// the plan version the request was made against — the row's own compare-and-set
// guard, recorded so a reader can tell which proposal a run was for.
type reductionComputeJobInput struct {
	ReductionID uint `json:"reductionId"`
	Version     uint `json:"version"`
}

func reductionComputeJobCodec() jobs.ReplayCodec {
	return jobs.ReplayCodec{
		Sanitize: sanitizeReductionComputeInput,
		Encode:   encodeReductionComputeInput,
		Decode:   decodeReductionComputeInput,
		Migrate: func(payload json.RawMessage, fromVersion, toVersion uint) (json.RawMessage, error) {
			if fromVersion != toVersion {
				return nil, fmt.Errorf("jobs: no reduction input migration from v%d to v%d", fromVersion, toVersion)
			}
			return payload, nil
		},
	}
}

// reductionComputeSummary is the bounded, searchable half of a clustering run's
// input: which Reduction, and which version of it. A Reduction's name is its
// owner's own text and stays out of a summary a different reader may search.
type reductionComputeSummary struct {
	ReductionID uint `json:"reductionId"`
	Version     uint `json:"version,omitempty"`
}

func sanitizeReductionComputeInput(input json.RawMessage) (json.RawMessage, error) {
	parsed, err := reductionComputeInputOf(input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(reductionComputeSummary{ReductionID: parsed.ReductionID, Version: parsed.Version})
}

func encodeReductionComputeInput(input json.RawMessage) (json.RawMessage, error) {
	if _, err := reductionComputeInputOf(input); err != nil {
		return nil, err
	}
	return input, nil
}

func decodeReductionComputeInput(payload json.RawMessage, version uint) (json.RawMessage, error) {
	if version != jobReductionKindVersion {
		return nil, fmt.Errorf("%w: reduction v%d input", jobs.ErrReplayCodecUnregistered, version)
	}
	if _, err := reductionComputeInputOf(payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func reductionComputeInputOf(input json.RawMessage) (*reductionComputeJobInput, error) {
	if len(input) == 0 || !json.Valid(input) {
		return nil, errors.New("a clustering Job's input is not valid JSON")
	}
	var decoded reductionComputeJobInput
	if err := json.Unmarshal(input, &decoded); err != nil {
		return nil, fmt.Errorf("a clustering Job's input is not readable: %w", err)
	}
	if decoded.ReductionID == 0 {
		return nil, errors.New("a clustering Job's input names no Resource Reduction")
	}
	return &decoded, nil
}

// reductionComputeAdapter runs one clustering run.
type reductionComputeAdapter struct {
	ctx  *MahresourcesContext
	kind string
}

func (a *reductionComputeAdapter) Definition() jobs.Definition {
	return jobs.Definition{
		Kind:        a.kind,
		KindVersion: jobReductionKindVersion,
		Restorable:  true,
		Visibility:  jobs.VisibilityOwner,
	}
}

func (a *reductionComputeAdapter) Dispatch(ctx context.Context, execution jobs.Execution) error {
	if a.ctx == nil || a.ctx.downloadManager == nil {
		return errors.New("the download queue is not available")
	}
	input, err := reductionComputeInputOf(execution.Input)
	if err != nil {
		return err
	}
	if execution.KindVersion != jobReductionKindVersion {
		return fmt.Errorf("%w: reduction v%d input", jobs.ErrReplayCodecUnregistered, execution.KindVersion)
	}

	// One binding for both halves: the refusal below and the clustering itself are
	// resolved against the same principal. A submission path runs on a
	// request-scoped context and inherits the requester's subtree; a capacity-queued
	// run, a Retry and a redispatched Job after a restart reach this adapter with a
	// Job whose only remaining principal is the recorded actor, and the process's
	// singleton context is unscoped. The Extent is resolved through the handle's own
	// scope filter, so an unbound dispatch clustered — and could destroy — Resources
	// outside the acting principal's subtree.
	a = a.forExecution(execution)
	if reason := a.refusalReason(execution, input); reason != "" {
		return a.ctx.blockQueueJob(execution.JobID, execution.ExecutionToken, reason)
	}

	entry, found := a.ctx.queueEntryFor(execution.JobID)
	if !found {
		entry, err = a.start(execution, input)
		if err != nil {
			if errors.Is(err, ErrReductionBusy) {
				return a.ctx.blockQueueJob(execution.JobID, execution.ExecutionToken, "reduction-busy")
			}
			if isReductionGone(err) {
				return a.ctx.blockQueueJob(execution.JobID, execution.ExecutionToken, "reduction-missing")
			}
			return err
		}
	} else if ref, ok := entry.CanonicalExecution(); !ok || ref.ExecutionToken != execution.ExecutionToken {
		if !entry.AttachCanonical(download_queue.CanonicalRef{
			JobID: execution.JobID, ExecutionToken: execution.ExecutionToken,
		}) {
			return fmt.Errorf("the queue entry %s publishes into another Job", entry.ID)
		}
	}

	if snap := entry.Snapshot(); queueJobTerminal(snap.Status) {
		return a.publishOutcome(execution, input, snap)
	}
	snap, err := a.ctx.waitForQueueExecution(ctx, execution, entry)
	if err != nil {
		return err
	}
	return a.publishOutcome(execution, input, snap)
}

// forExecution returns this adapter bound to the principal one execution acts as,
// so that what is *authorized* and what is *executed* are one view of the subtree.
//
// The binding is the whole confinement story for a redispatched run. The Extent is
// resolved against the database handle's scope filter (`resolveReductionExtent`'
// own comment says so), and the handle a dispatch reaches for is the process's
// singleton unless something binds it. A capacity-queued run, a Retry and a
// redispatched Job all reach here with no request behind them, so the recorded actor
// is the only principal left — and resolving it once, here, is what makes the check
// and the run answer the same question.
func (a *reductionComputeAdapter) forExecution(execution jobs.Execution) *reductionComputeAdapter {
	if a.ctx == nil || execution.Access.UserID == 0 {
		return a
	}
	principal := a.ctx.principalForPluginActor(execution.Access.UserID)
	if principal == nil {
		return a
	}
	return &reductionComputeAdapter{ctx: a.ctx.WithPrincipal(principal), kind: a.kind}
}

// refusalReason answers why this execution may not start, or an empty string.
//
// Everything here was checked when the request arrived, and none of it is a standing
// permission: a Retry runs as whoever asked for the retry, a queued Job may run after
// the deployment restarted, and a principal's role or subtree may have narrowed since.
// The Reduction's own visibility is the owner predicate — the row is not subtree-scoped
// itself, only everything it reaches is — so it is asked with the acting principal's
// owner filter, exactly as the HTTP surface asks it.
func (a *reductionComputeAdapter) refusalReason(execution jobs.Execution, input *reductionComputeJobInput) string {
	if execution.Access.UserID == 0 {
		return ""
	}
	if err := a.ctx.requireWriteRole("run a clustering run"); err != nil {
		return "role-refused"
	}
	owner, restricted := reductionOwnerFilter(a.ctx.Principal())
	if _, err := a.ctx.loadReductionForUpdate(input.ReductionID, owner, restricted); err != nil {
		return "reduction-refused"
	}
	return ""
}

// reductionOwnerFilter is the owner predicate a principal reads a Reduction under:
// administrators (and the auth-off super-user) see every row, and every other
// principal only its own. It is the application layer's copy of the HTTP surface's
// rule, expressed in the terms the predicate itself takes.
func reductionOwnerFilter(principal *auth.Principal) (*uint, bool) {
	if principal == nil || principal.IsAdmin() || principal.UserID == 0 {
		return nil, false
	}
	id := principal.UserID
	return &id, true
}

// start takes the Reduction's compute claim and submits the clustering run.
//
// It is the request path's own claim, not a replay of one: the row is read as it
// stands and taken under a fresh generation, so a Job whose original execution died
// does not resurrect a claim the deadline already retired, and a newer run that took
// the row in the meantime is what this one is superseded by rather than what it
// overwrites.
func (a *reductionComputeAdapter) start(execution jobs.Execution, input *reductionComputeJobInput) (*download_queue.DownloadJob, error) {
	reduction, err := a.ctx.loadReductionForUpdate(input.ReductionID, nil, false)
	if err != nil {
		return nil, err
	}
	if EffectiveReductionStatus(reduction) == models.ReductionStatusComputing {
		return nil, ErrReductionBusy
	}
	generation, err := a.ctx.beginReductionCompute(reduction.ID, reduction.Version)
	if err != nil {
		return nil, err
	}
	legacyID, err := a.ctx.jobHandleFor(execution.JobID, ReductionComputeHandleNamespace)
	if err != nil {
		return nil, err
	}
	if legacyID == "" {
		legacyID = download_queue.NewJobID()
	}
	return a.ctx.startReductionComputeQueueJob(
		reduction.ID, generation, legacyID,
		jobs.ExecutionRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken},
		a.ctx.jobOwnerFor(execution.JobID),
	)
}

func (a *reductionComputeAdapter) publishOutcome(execution jobs.Execution, input *reductionComputeJobInput, snap *download_queue.DownloadJob) error {
	switch snap.Status {
	case download_queue.JobStatusCompleted:
		reference, err := json.Marshal(map[string]any{"reductionId": input.ReductionID})
		if err != nil {
			return err
		}
		if _, err := execution.Output(jobs.OutputInput{
			Key:       jobReductionOutput,
			Type:      jobs.OutputTypeEntity,
			Label:     "Resource Reduction",
			Reference: reference,
			Required:  true,
		}); err != nil {
			return a.ctx.finishQueueJob(execution, jobs.StateFailed,
				&jobs.Failure{
					Code:    "reduction-output-unavailable",
					Class:   jobs.FailureClassInternal,
					Message: "the clustering run produced no Resource Reduction to open",
				},
				[]string{jobReductionOutput})
		}
		return a.ctx.finishQueueJob(execution, jobs.StateSucceeded, nil, []string{jobReductionOutput})
	case download_queue.JobStatusCancelled:
		return a.ctx.finishQueueJob(execution, jobs.StateCancelled, nil, nil)
	default:
		return a.ctx.finishQueueJob(execution, jobs.StateFailed,
			&jobs.Failure{
				Code:    "reduction-compute-failed",
				Class:   jobs.FailureClassInternal,
				Message: "the Resource Reduction could not be clustered",
			},
			nil)
	}
}

// Reconcile answers what should happen to one clustering run whose claim expired.
//
// The Reduction row is the evidence, and it is decisive in a way the other Kinds'
// files are not: a run that landed its plan set the row to `ready` and stamped the
// queue id that wrote it, so a Job whose entry is gone can be *succeeded* — with its
// output published under the expired claim's own token — rather than run again over
// a plan that is already there. Everything else is queued again, and that is safe
// for the reason the Kind's claim is: a re-run takes the row under a fresh
// generation, and the run it supersedes discards its own plan at the end rather
// than writing it.
func (a *reductionComputeAdapter) Reconcile(_ context.Context, request jobs.ReconcileRequest) (jobs.ReconcileDecision, error) {
	if a.ctx == nil || a.ctx.downloadManager == nil {
		return jobs.ReconcileExternalWorkUnproven, nil
	}
	if entry, found := a.ctx.queueEntryFor(request.Snapshot.ID); found {
		if !queueJobTerminal(entry.GetStatus()) {
			return jobs.ReconcileResume, nil
		}
		return jobs.ReconcileQueue, nil
	}
	input, err := reductionComputeInputOf(request.Input)
	if err != nil {
		return jobs.ReconcileBlock, nil
	}
	reduction, err := a.ctx.loadReductionForUpdate(input.ReductionID, nil, false)
	if err != nil {
		return jobs.ReconcileFail, nil
	}
	handle, err := a.ctx.jobHandleFor(request.Snapshot.ID, ReductionComputeHandleNamespace)
	if err != nil {
		return jobs.ReconcileExternalWorkUnproven, nil
	}
	if reduction.Status == models.ReductionStatusReady && handle != "" && reduction.ComputeJobID == handle {
		outputs, err := a.ctx.jobOutputsFor(request.Snapshot.ID)
		if err != nil {
			return jobs.ReconcileExternalWorkUnproven, nil
		}
		if _, published := findJobOutput(outputs, jobReductionOutput); !published {
			reference, err := json.Marshal(map[string]any{"reductionId": reduction.ID})
			if err != nil {
				return jobs.ReconcileExternalWorkUnproven, nil
			}
			if _, err := request.Execution.Output(jobs.OutputInput{
				Key:       jobReductionOutput,
				Type:      jobs.OutputTypeEntity,
				Label:     "Resource Reduction",
				Reference: reference,
				Required:  true,
			}); err != nil {
				return jobs.ReconcileExternalWorkUnproven, nil
			}
		}
		return jobs.ReconcileSucceed, nil
	}
	return jobs.ReconcileQueue, nil
}

// CleanupArtifacts accounts for one clustering run's outputs: its output is the
// Reduction row itself, a domain record the library owns and never a file this Kind
// staged.
func (a *reductionComputeAdapter) CleanupArtifacts(_ context.Context, _ jobs.ArtifactCleanupRequest) (jobs.ArtifactCleanupResult, error) {
	return jobs.ArtifactCleanupResult{}, nil
}

// Commands reports what one clustering run offers.
//
// Cancel is cooperative — the queue's cancellation reaches the runFn's context and
// the clustering loop asks it between phases. Retry is advertised only while the
// Reduction row itself says the work can be done again, because that row is the
// authority on whether a run is wanted.
func (a *reductionComputeAdapter) Commands(_ context.Context, commandContext jobs.CommandContext) ([]jobs.Command, error) {
	// §8: a principal demoted below "may write" keeps the history and loses the
	// controls over it.
	if a.ctx.commandActorRefusal(commandContext.Deps, commandContext.Access, "") != "" {
		return nil, nil
	}
	commands := []jobs.Command{{
		Key:          jobs.CommandCancel,
		Label:        "Cancel",
		Destructive:  true,
		Confirmation: "Stop clustering? The Reduction keeps the plan it had.",
	}}
	state := commandContext.Snapshot.State
	if state != jobs.StateFailed && state != jobs.StateCancelled && state != jobs.StateInterrupted {
		return commands, nil
	}
	input, err := a.inputOf(commandContext.Deps, commandContext.Snapshot.ID)
	if err != nil {
		return commands, nil
	}
	if a.ctx.reductionComputableOn(commandContext.Deps, input.ReductionID) {
		commands = append(commands, jobs.Command{Key: jobs.CommandRetry, Label: "Compute again"})
	}
	return commands, nil
}

// inputOf opens one Job's sealed input as this Kind reads it, on the caller's own
// handle: the command plane re-asks this question inside the transaction that would
// create the successor, and a second connection there deadlocks a pool of one.
func (a *reductionComputeAdapter) inputOf(deps jobs.Deps, jobID string) (*reductionComputeJobInput, error) {
	service := a.ctx.JobService()
	if service == nil {
		return nil, errors.New("this context has no job control plane installed")
	}
	opened, err := service.OpenReplay(deps, jobs.Access{Administrator: true}, jobID)
	if err != nil {
		return nil, err
	}
	return reductionComputeInputOf(opened.Input)
}

// reductionComputable answers whether the row one Job names can be clustered again:
// it is there, and it is not already computing or already computed. A row whose
// compute deadline has passed reads as failed, which is exactly the state this
// mechanism exists to make recomputable.
func (ctx *MahresourcesContext) reductionComputable(reductionID uint) bool {
	return ctx.reductionComputableOn(ctx.jobDeps(), reductionID)
}

// reductionComputableOn is reductionComputable on a caller's own handle, for the
// advertisement the command plane re-asks inside its transaction.
func (ctx *MahresourcesContext) reductionComputableOn(deps jobs.Deps, reductionID uint) bool {
	if ctx == nil || deps.DB == nil || reductionID == 0 {
		return false
	}
	var reduction models.ResourceReduction
	if err := deps.DB.First(&reduction, reductionID).Error; err != nil {
		return false
	}
	return EffectiveReductionStatus(&reduction) == models.ReductionStatusFailed
}

// isReductionGone reports whether a dispatch refusal means the Reduction itself
// cannot be read — deleted, or never there.
func isReductionGone(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

// ExecuteCommand runs one control the host decided this Kind owns.
func (a *reductionComputeAdapter) ExecuteCommand(_ context.Context, execution jobs.CommandExecution) (jobs.CommandOutcome, error) {
	if a.ctx == nil || a.ctx.downloadManager == nil {
		return jobs.CommandOutcome{}, errors.New("the download queue is not available")
	}
	if execution.Key != jobs.CommandCancel {
		return jobs.CommandOutcome{}, fmt.Errorf("%w: %s", jobs.ErrCommandNotAdvertised, execution.Key)
	}
	handle, err := a.ctx.jobHandleFor(execution.JobID, ReductionComputeHandleNamespace)
	if err != nil {
		return jobs.CommandOutcome{}, err
	}
	if handle == "" {
		return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded, Message: "the clustering run is no longer active"}, nil
	}
	if _, found := a.ctx.downloadManager.GetJob(handle); !found {
		return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded, Message: "the clustering run is no longer active"}, nil
	}
	if err := a.ctx.downloadManager.Cancel(handle); err != nil {
		var conflict *download_queue.StateConflictError
		if errors.As(err, &conflict) {
			return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded, Message: "the clustering run had already finished"}, nil
		}
		return jobs.CommandOutcome{}, err
	}
	return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded, Message: "cancelling"}, nil
}

// beginReductionCompute takes one Reduction's `computing` claim under a fresh
// generation, and returns the generation the queue job must claim.
//
// It is the one place the claim is taken, so the request path and a Retry cannot
// disagree about what taking it means. The version is the caller's compare-and-set
// guard: a request made from a page that predates somebody else's decisions is
// refused rather than discarding them.
func (ctx *MahresourcesContext) beginReductionCompute(reductionID uint, version uint) (string, error) {
	now := time.Now()
	deadline := now.Add(ReductionComputeDeadline)
	// A nonce written before the job exists, and swapped for the job's own id when
	// the worker starts. Without it the worker's claim asks only "is the slot
	// empty", which a run delayed past its deadline answers yes to — taking the slot
	// of the newer run that replaced it, and then computing under the subtree scope
	// it captured an hour ago while the accepted recompute is turned away as
	// superseded.
	generation := "pending:" + string(types.NewUUIDv7())
	// The caller's version, not the one just read. Recompute replaces the plan, so a
	// request made from a page that predates somebody else's decisions would discard
	// them without their author ever seeing a refusal — and "every write is a
	// compare-and-set" has to mean this write too.
	ok, err := ctx.casReduction(reductionID, version, map[string]any{
		"status":               models.ReductionStatusComputing,
		"computing_started_at": now,
		"compute_deadline":     deadline,
		"compute_job_id":       generation,
		"compute_error":        "",
	})
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrReductionConflict
	}
	return generation, nil
}

// startReductionComputeQueueJob submits one clustering run's queue entry, with the
// generation its worker must claim and the durable identity it publishes into.
func (ctx *MahresourcesContext) startReductionComputeQueueJob(
	reductionID uint,
	generation string,
	legacyID string,
	ref jobs.ExecutionRef,
	ownerUserID *uint,
) (*download_queue.DownloadJob, error) {
	if ctx == nil || ctx.downloadManager == nil {
		return nil, errors.New("the download queue is not available")
	}
	// The owner is named at construction rather than set afterwards: under -auth the
	// SSE stream drops any event whose job the principal may not see, so a job with no
	// owner yet never reaches its own submitter's panel — which is the only place the
	// progress of this run is visible.
	opts := download_queue.JobOptions{
		Source:       download_queue.JobSourceResourceReduction,
		InitialPhase: "clustering",
		OwnerUserID:  ownerUserID,
	}
	runFn := func(jobCtx context.Context, j *download_queue.DownloadJob, p download_queue.ProgressSink) error {
		// The row is told which job owns it here, from inside the worker, and not by
		// the caller after the submission returns. The submission starts the goroutine
		// before it returns, so a fast run could finish and find compute_job_id still
		// empty — read that as "a newer job owns this row", discard its own finished
		// plan, and leave the Reduction at `computing` with nothing alive to move it
		// off.
		if claimErr := ctx.claimReductionComputeJob(reductionID, generation, j.ID); claimErr != nil {
			// A superseded run leaves the row alone: the newer request owns it and will
			// report its own outcome. Every other failure has to be recorded here,
			// because nothing downstream will — this run never reached
			// runReductionCompute, and an unrecorded refusal leaves the Reduction at
			// `computing` with nothing alive to move it until its deadline.
			if !errors.Is(claimErr, ErrReductionComputeSuperseded) {
				if writeErr := ctx.recordReductionComputeFailure(reductionID, generation, claimErr); writeErr != nil {
					ctx.Logger().Warning(models.LogActionUpdate, "resource_reduction", &reductionID, "",
						"Could not record a clustering job that failed to claim its slot: "+writeErr.Error(), nil)
				}
			}
			return claimErr
		}
		return ctx.runReductionCompute(jobCtx, reductionID, j.ID, p)
	}
	if ref.JobID == "" {
		return ctx.downloadManager.SubmitJobWithOptions(opts, runFn)
	}
	return ctx.submitQueueJob(opts, legacyID, ref, runFn)
}
