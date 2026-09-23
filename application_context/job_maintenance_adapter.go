package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"mahresources/download_queue"
	"mahresources/hash_worker"
	"mahresources/jobs"
)

// This file is the maintenance Kind adapter: the similarity recompute — the one
// user-facing maintenance operation the admin surface offers as a background job.
//
// Two properties separate it from the other queue-backed Kinds:
//
//   - It is admin-only, and durably so. The Kind's registered Definition fixes its
//     visibility class, which is what makes the answer independent of who submitted
//     it, and dispatch rechecks the acting principal's role because a Retry runs as
//     whoever asked for the retry rather than as whoever asked for the original.
//     §2's rule is that a Kind a caller could choose the class of would be a Kind
//     that did not fix one.
//   - Its executor is a whole-library rebuild that is *already* guarded
//     process-wide (`hash_worker.RecomputeInProgress`). That guard is the only
//     evidence this process has about whether the work is in flight, and
//     reconciliation uses it as such: while a recompute is running here the Job
//     keeps its claim rather than being dispatched a second time over the same
//     tables.
//
// It advertises no Repeat. Re-running a rebuild that just succeeded is the same
// work twice over the same rows, which is what the recompute's own process-wide
// guard refuses — advertising a control the executor would refuse is exactly the
// button-that-lies the command surface exists to prevent.

const (
	// JobKindSimilarityRecompute is one similarity recompute accepted from the admin
	// surface.
	JobKindSimilarityRecompute = "similarity-recompute"
	// jobMaintenanceKindVersion is the version of this Kind's input semantics.
	jobMaintenanceKindVersion = 1
)

// maintenanceJobInput is what a maintenance Job is accepted with. A recompute takes
// no parameters — the deployment's own settings are its input — so the sealed input
// says which maintenance operation this is, which is what keeps the Kind's shape
// extensible without moving to a second version the moment one gains an argument.
type maintenanceJobInput struct {
	Operation string `json:"operation"`
}

const maintenanceOperationSimilarityRecompute = "similarity-recompute"

func maintenanceJobInputJSON() json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"operation":%q}`, maintenanceOperationSimilarityRecompute))
}

func similarityRecomputeJobCodec() jobs.ReplayCodec {
	return jobs.ReplayCodec{
		Sanitize: func(input json.RawMessage) (json.RawMessage, error) {
			if _, err := maintenanceInputOf(input); err != nil {
				return nil, err
			}
			return json.Marshal(map[string]string{"operation": maintenanceOperationSimilarityRecompute})
		},
		Encode: func(input json.RawMessage) (json.RawMessage, error) {
			if _, err := maintenanceInputOf(input); err != nil {
				return nil, err
			}
			return input, nil
		},
		Decode: func(payload json.RawMessage, version uint) (json.RawMessage, error) {
			if version != jobMaintenanceKindVersion {
				return nil, fmt.Errorf("%w: maintenance v%d input", jobs.ErrReplayCodecUnregistered, version)
			}
			if _, err := maintenanceInputOf(payload); err != nil {
				return nil, err
			}
			return payload, nil
		},
		Migrate: func(payload json.RawMessage, fromVersion, toVersion uint) (json.RawMessage, error) {
			if fromVersion != toVersion {
				return nil, fmt.Errorf("jobs: no maintenance input migration from v%d to v%d", fromVersion, toVersion)
			}
			return payload, nil
		},
	}
}

func maintenanceInputOf(input json.RawMessage) (*maintenanceJobInput, error) {
	if len(input) == 0 || !json.Valid(input) {
		return nil, errors.New("a maintenance Job's input is not valid JSON")
	}
	var decoded maintenanceJobInput
	if err := json.Unmarshal(input, &decoded); err != nil {
		return nil, fmt.Errorf("a maintenance Job's input is not readable: %w", err)
	}
	if decoded.Operation != maintenanceOperationSimilarityRecompute {
		return nil, fmt.Errorf("this release runs no maintenance operation named %q", decoded.Operation)
	}
	return &decoded, nil
}

// similarityRecomputeAdapter runs one similarity recompute.
type similarityRecomputeAdapter struct {
	ctx  *MahresourcesContext
	kind string
}

// Definition declares the Kind admin-only. It is the *Kind* that decides this
// rather than the caller, which is what makes an ownerless or migrated
// maintenance Job unreadable by a plain user as well.
func (a *similarityRecomputeAdapter) Definition() jobs.Definition {
	return jobs.Definition{
		Kind:        a.kind,
		KindVersion: jobMaintenanceKindVersion,
		Restorable:  true,
		Visibility:  jobs.VisibilityAdmin,
	}
}

func (a *similarityRecomputeAdapter) Dispatch(ctx context.Context, execution jobs.Execution) error {
	if a.ctx == nil || a.ctx.downloadManager == nil {
		return errors.New("the download queue is not available")
	}
	if _, err := maintenanceInputOf(execution.Input); err != nil {
		return err
	}
	if execution.KindVersion != jobMaintenanceKindVersion {
		return fmt.Errorf("%w: maintenance v%d input", jobs.ErrReplayCodecUnregistered, execution.KindVersion)
	}
	if reason := a.refusalReason(execution); reason != "" {
		return a.ctx.blockQueueJob(execution.JobID, execution.ExecutionToken, reason)
	}

	entry, found := a.ctx.queueEntryFor(execution.JobID)
	if !found {
		started, startErr := a.start(execution)
		if startErr != nil {
			return startErr
		}
		entry = started
	} else if ref, ok := entry.CanonicalExecution(); !ok || ref.ExecutionToken != execution.ExecutionToken {
		if !entry.AttachCanonical(download_queue.CanonicalRef{
			JobID: execution.JobID, ExecutionToken: execution.ExecutionToken,
		}) {
			return fmt.Errorf("the queue entry %s publishes into another Job", entry.ID)
		}
	}

	if snap := entry.Snapshot(); queueJobTerminal(snap.Status) {
		return a.publishOutcome(execution, snap)
	}
	snap, err := a.ctx.waitForQueueExecution(ctx, execution, entry)
	if err != nil {
		return err
	}
	return a.publishOutcome(execution, snap)
}

// refusalReason answers why this execution may not start, or an empty string.
//
// It rechecks the acting principal's role because the acting principal of a Retry is
// whoever asked for the retry: the original submission was an administrator's, and
// that says nothing about the account asking for the second run. A principal the
// resolver cannot find — a deleted or disabled account — is deny-all rather than
// unscoped, which is what `principalForPluginActor` answers.
func (a *similarityRecomputeAdapter) refusalReason(execution jobs.Execution) string {
	if execution.Access.UserID == 0 {
		return ""
	}
	scoped := a.ctx.WithPrincipal(a.ctx.principalForPluginActor(execution.Access.UserID))
	if err := scoped.requireAdminRole("recompute similarities"); err != nil {
		return "role-refused"
	}
	return ""
}

// start submits the recompute this execution needs.
func (a *similarityRecomputeAdapter) start(execution jobs.Execution) (*download_queue.DownloadJob, error) {
	legacyID, err := a.ctx.jobHandleFor(execution.JobID, SimilarityRecomputeHandleNamespace)
	if err != nil {
		return nil, err
	}
	if legacyID == "" {
		legacyID = download_queue.NewJobID()
	}
	return a.ctx.submitQueueJob(
		download_queue.JobOptions{
			Source:       maintenanceJobSource,
			InitialPhase: "recomputing",
			OwnerUserID:  a.ctx.jobOwnerFor(execution.JobID),
		},
		legacyID,
		jobs.ExecutionRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken},
		a.ctx.buildSimilarityRecomputeRunFn(),
	)
}

func (a *similarityRecomputeAdapter) publishOutcome(execution jobs.Execution, snap *download_queue.DownloadJob) error {
	switch snap.Status {
	case download_queue.JobStatusCompleted:
		return a.ctx.finishQueueJob(execution, jobs.StateSucceeded, nil, nil)
	case download_queue.JobStatusCancelled:
		return a.ctx.finishQueueJob(execution, jobs.StateCancelled, nil, nil)
	default:
		return a.ctx.finishQueueJob(execution, jobs.StateFailed,
			&jobs.Failure{
				Code:    "similarity-recompute-failed",
				Class:   jobs.FailureClassInternal,
				Message: "the similarity pairs could not be rebuilt",
			},
			nil)
	}
}

// Reconcile answers what should happen to one maintenance Job whose claim expired.
//
// The process-wide recompute guard is the evidence: while a rebuild is running in
// this process, the Job keeps its claim and its capacity rather than being
// dispatched a second time over the same tables. With the guard free there is no
// run here, and the work is queued again — it is a full rebuild of one derived
// table, so a re-run converges on the same answer rather than compounding one.
func (a *similarityRecomputeAdapter) Reconcile(_ context.Context, request jobs.ReconcileRequest) (jobs.ReconcileDecision, error) {
	if a.ctx == nil || a.ctx.downloadManager == nil {
		return jobs.ReconcileExternalWorkUnproven, nil
	}
	if entry, found := a.ctx.queueEntryFor(request.Snapshot.ID); found {
		if !queueJobTerminal(entry.GetStatus()) {
			return jobs.ReconcileResume, nil
		}
		return jobs.ReconcileQueue, nil
	}
	if hash_worker.RecomputeInProgress() {
		return jobs.ReconcileExternalWorkUnproven, nil
	}
	// The guard is this process's, so its freedom says nothing about a rebuild
	// another process is running: the claim's own runtime identity is what decides
	// whether a replacement may be dispatched.
	return a.ctx.queueOnlyIfTheRuntimeIsProvedGone(request), nil
}

// CleanupArtifacts accounts for one maintenance Job's outputs: a rebuild stages no
// file and publishes no artifact, so there is nothing to establish the absence of.
func (a *similarityRecomputeAdapter) CleanupArtifacts(_ context.Context, _ jobs.ArtifactCleanupRequest) (jobs.ArtifactCleanupResult, error) {
	return jobs.ArtifactCleanupResult{}, nil
}

// Commands reports what one maintenance Job offers: a cooperative cancel, and a
// Retry of an unsuccessful outcome. No Repeat — see the file comment.
func (a *similarityRecomputeAdapter) Commands(_ context.Context, commandContext jobs.CommandContext) ([]jobs.Command, error) {
	// §8: a principal demoted below "may write" keeps the history and loses the
	// controls over it.
	if a.ctx.commandActorRefusal(commandContext.Deps, commandContext.Access, "") != "" {
		return nil, nil
	}
	commands := []jobs.Command{{
		Key:          jobs.CommandCancel,
		Label:        "Cancel",
		Destructive:  true,
		Confirmation: "Stop rebuilding the similarity index? The pairs already rebuilt stay.",
	}}
	switch commandContext.Snapshot.State {
	case jobs.StateFailed, jobs.StateCancelled, jobs.StateInterrupted:
		commands = append(commands, jobs.Command{Key: jobs.CommandRetry, Label: "Retry"})
	}
	return commands, nil
}

// ExecuteCommand runs one control the host decided this Kind owns. Only cancel
// reaches here: Retry is the control plane's own lineage work.
func (a *similarityRecomputeAdapter) ExecuteCommand(_ context.Context, execution jobs.CommandExecution) (jobs.CommandOutcome, error) {
	if a.ctx == nil || a.ctx.downloadManager == nil {
		return jobs.CommandOutcome{}, errors.New("the download queue is not available")
	}
	if execution.Key != jobs.CommandCancel {
		return jobs.CommandOutcome{}, fmt.Errorf("%w: %s", jobs.ErrCommandNotAdvertised, execution.Key)
	}
	handle, err := a.ctx.jobHandleFor(execution.JobID, SimilarityRecomputeHandleNamespace)
	if err != nil {
		return jobs.CommandOutcome{}, err
	}
	if handle == "" {
		return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded, Message: "the rebuild is no longer running"}, nil
	}
	if _, found := a.ctx.downloadManager.GetJob(handle); !found {
		return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded, Message: "the rebuild is no longer running"}, nil
	}
	if err := a.ctx.downloadManager.Cancel(handle); err != nil {
		var conflict *download_queue.StateConflictError
		if errors.As(err, &conflict) {
			return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded, Message: "the rebuild had already finished"}, nil
		}
		return jobs.CommandOutcome{}, err
	}
	return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded, Message: "cancelling"}, nil
}

// maintenanceJobSource labels one maintenance queue entry in the jobs panel.
const maintenanceJobSource = "similarity-recompute"

// buildSimilarityRecomputeRunFn is the maintenance executor's body: the rebuild the
// admin surface has always run, with the queue's own cancellation and progress
// plumbing around it.
func (ctx *MahresourcesContext) buildSimilarityRecomputeRunFn() download_queue.JobRunFn {
	return func(jobCtx context.Context, _ *download_queue.DownloadJob, progress download_queue.ProgressSink) error {
		batchSize := ctx.Config.HashBatchSize
		if batchSize <= 0 {
			batchSize = 500
		}
		return hash_worker.RecomputeV2Pairs(ctx.db, batchSize,
			func() bool { return jobCtx.Err() != nil },
			func(done, total int64) { progress.UpdateProgress(done, total) },
		)
	}
}

// SubmitSimilarityRecompute is the one door a similarity recompute is submitted
// through. It answers the id the admin surface has always returned: the legacy queue
// id, which the Job's own handle records.
//
// The process-wide guard is asked first, so the conflict the admin endpoint answers
// 409 for is still synchronous — a second acceptance would be a second Job for work
// that cannot run.
func (ctx *MahresourcesContext) SubmitSimilarityRecompute() (string, error) {
	if ctx == nil || ctx.downloadManager == nil {
		return "", errors.New("the download queue is not available")
	}
	if hash_worker.RecomputeInProgress() {
		return "", hash_worker.ErrRecomputeInProgress
	}
	owner := ctx.queueSubmitterOwner()
	service := ctx.JobService()
	if service == nil {
		job, err := ctx.downloadManager.SubmitJobWithOptions(download_queue.JobOptions{
			Source:       maintenanceJobSource,
			InitialPhase: "recomputing",
			OwnerUserID:  owner,
		}, ctx.buildSimilarityRecomputeRunFn())
		if err != nil {
			return "", err
		}
		return job.ID, nil
	}

	legacyID := download_queue.NewJobID()
	admission, err := ctx.admitQueueJob(jobs.Acceptance{
		Kind:        JobKindSimilarityRecompute,
		KindVersion: jobMaintenanceKindVersion,
		State:       jobs.StateQueued,
		OwnerUserID: owner,
		ActorUserID: owner,
		Origin:      "admin",
		Title:       "Rebuild similarity pairs",
		Replay:      jobs.ReplayInput{Input: maintenanceJobInputJSON()},
		LegacyRefs:  []jobs.LegacyRef{{Namespace: SimilarityRecomputeHandleNamespace, Handle: legacyID}},
	})
	if err != nil {
		return "", err
	}
	if !admission.Owned() {
		// The deployment's budget is full: the Job is durable, answers the id the
		// admin surface was handed, and starts no executor here. A runtime with a free
		// slot takes it — see admitQueueJob — and the run is a full rebuild of one
		// derived table, so it converges on the same answer whenever it happens.
		return legacyID, nil
	}

	entry, err := ctx.submitQueueJob(
		download_queue.JobOptions{
			Source:       maintenanceJobSource,
			InitialPhase: "recomputing",
			OwnerUserID:  owner,
		},
		legacyID,
		jobs.ExecutionRef{JobID: admission.Execution.JobID, ExecutionToken: admission.Execution.ExecutionToken},
		ctx.buildSimilarityRecomputeRunFn(),
	)
	if err != nil {
		ctx.failUndispatchedQueueJob(admission, err)
		return "", err
	}
	ctx.ownQueueExecution(admission, entry, func(snap *download_queue.DownloadJob) error {
		return (&similarityRecomputeAdapter{ctx: ctx, kind: JobKindSimilarityRecompute}).publishOutcome(admission.Execution, snap)
	})
	return entry.ID, nil
}
