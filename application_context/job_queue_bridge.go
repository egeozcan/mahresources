package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"time"

	"mahresources/download_queue"
	"mahresources/jobs"
)

// This file is the shared half of every Kind whose work the in-memory download
// queue executes: the export, import, reduction and maintenance Kinds of Task 8.
//
// The queue is the executor for all of them — it owns their semaphore, their
// worker goroutines, their cancellation and their panel rows — and what the
// control plane adds is durable identity, an outcome, and typed outputs. The four
// things every one of those Kinds therefore needs are the same four, and they live
// here rather than four times over:
//
//   - accepting the durable Job before the queue entry that carries it exists;
//   - submitting that entry with the identity the acceptance already handed the
//     client, so one legacy id names one current execution;
//   - waiting for the entry and publishing progress while it runs, because these
//     executors publish no progress of their own into a Job;
//   - ending the Job, bounded and classed, from the queue entry's terminal status.
//
// It is deliberately not a Kind adapter: what an export publishes, what makes its
// Retry honest and what reconciles its leftovers are the Kind's own answers and
// live in its own file.

// Legacy handle namespaces for the queue-backed Kinds. A handle names the queue
// entry carrying one execution, in the namespace of the Kind whose id space it
// belongs to — separate per Kind because they are all short random strings and one
// must never resolve as another.
const (
	// GroupExportHandleNamespace is the export queue's legacy ids.
	GroupExportHandleNamespace = "group-export"
	// ImportParseHandleNamespace is the import-parse queue's legacy ids. The plan,
	// result and staged archive of one import are all named by it, which is why the
	// apply Job resolves its parent through this namespace rather than its own.
	ImportParseHandleNamespace = "group-import-parse"
	// ImportApplyHandleNamespace is the import-apply queue's legacy ids.
	ImportApplyHandleNamespace = "group-import-apply"
	// ReductionComputeHandleNamespace is one Resource Reduction clustering run's
	// queue id.
	ReductionComputeHandleNamespace = "resource-reduction-compute"
	// SimilarityRecomputeHandleNamespace is one similarity recompute's queue id.
	SimilarityRecomputeHandleNamespace = "similarity-recompute"
)

// queueJobPollInterval is how often a queue-backed Kind re-reads the entry it is
// running. The queue publishes no completion signal an adapter can select on — its
// cancellation is a context and its completion is a field — so waiting is a bounded
// poll, exactly as the download adapter's is and for the same reason.
const queueJobPollInterval = 100 * time.Millisecond

// QueueJobSubmission is one queue-backed submission's outcome: the queue entry
// carrying the work, the durable Job it publishes into, and the refusal when there
// is neither.
//
// It is declared here rather than in the queue package because both the
// application layer that accepts the Jobs and the HTTP layer that answers the
// request have to name it, and this is the package they already share. The shape is
// download_queue.RemoteDownloadSubmission's, deliberately: one submission answers
// the same question whatever Kind it was for.
type QueueJobSubmission struct {
	// QueueJobID is the id the queue entry took, which is also the legacy handle
	// the client is answered with.
	QueueJobID string
	// CanonicalJobID is the durable Job the work publishes into; empty when the
	// deployment has no control plane installed.
	CanonicalJobID string
	// Err is the refusal, nil when the work was accepted.
	Err error
}

// acceptQueueJob accepts the durable Job one queue-backed submission stands for.
//
// A context with no control plane answers a zero Snapshot and no error: the queue
// runs exactly as it always has, and the caller submits with no canonical identity
// because there is none to name. Every caller treats that as "no dual publication"
// rather than as a failure, which is what keeps the CLI's, the package tests' and a
// bare embedder's queue working.
func (ctx *MahresourcesContext) acceptQueueJob(acceptance jobs.Acceptance) (jobs.Snapshot, error) {
	service := ctx.JobService()
	if service == nil {
		return jobs.Snapshot{}, nil
	}
	return service.Accept(ctx.jobDeps(), acceptance)
}

// submitQueueJob enqueues one generic queue job as the projection of the Job that
// was already accepted for it.
//
// legacyID is the handle that Job answers to, which is also the id the client was
// handed — the entry has to take it, or the same work would be one row in the Job
// Center and another in the panel. An empty legacyID lets the queue generate its own
// id, which is what a deployment with no control plane gets.
func (ctx *MahresourcesContext) submitQueueJob(
	opts download_queue.JobOptions,
	legacyID string,
	ref jobs.ExecutionRef,
	runFn download_queue.JobRunFn,
) (*download_queue.DownloadJob, error) {
	if ctx == nil || ctx.downloadManager == nil {
		return nil, errors.New("the download queue is not available")
	}
	opts.JobID = legacyID
	if ref.JobID == "" {
		// No control plane: the entry publishes nowhere, and the queue names itself
		// when no id was handed over.
		opts.Canonical = nil
	} else {
		if legacyID == "" {
			return nil, errors.New("a canonical submission needs the legacy id its job answers to")
		}
		opts.Canonical = &download_queue.CanonicalRef{JobID: ref.JobID, ExecutionToken: ref.ExecutionToken}
	}
	return ctx.submitQueueEntry(opts, legacyID, ref, runFn)
}

// submitQueueEntry submits one entry and tolerates the other writer having got there
// first.
//
// Two writers can create the entry of one execution: the submission that admitted the
// work (the request path), and the execution that is asked to run it (a runtime's
// dispatch, which starts an entry for a Job whose own admission is still in flight or
// whose entry died with a previous process). They are both reaching for the same
// entry, so whichever loses is answered with what the winner created rather than with
// a refusal — one Job, one queue entry, whatever order the two arrive in.
func (ctx *MahresourcesContext) submitQueueEntry(
	opts download_queue.JobOptions,
	legacyID string,
	ref jobs.ExecutionRef,
	runFn download_queue.JobRunFn,
) (*download_queue.DownloadJob, error) {
	entry, err := ctx.downloadManager.SubmitJobWithOptions(opts, runFn)
	if err == nil {
		return entry, nil
	}
	if ref.JobID != "" && legacyID != "" {
		if existing, found := ctx.downloadManager.GetJob(legacyID); found && existing.CanonicalJobID == ref.JobID {
			return existing, nil
		}
	}
	return nil, err
}

// failUndispatchedQueueJob ends an accepted Job whose work the queue refused.
//
// The queue's own error text is deliberately not carried: a Job's failure record is
// a bounded taxonomy a reader groups on. A refusal here is not a failed export — it
// is an admission the deployment could not take — so it is classed as a policy
// refusal of the submission.
func (ctx *MahresourcesContext) failUndispatchedQueueJob(accepted jobs.Snapshot, cause error) {
	service := ctx.JobService()
	if service == nil || accepted.ID == "" {
		return
	}
	if cause != nil {
		log.Printf("warning: the download queue refused the submission for job %s: %v", accepted.ID, cause)
	}
	current, err := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, accepted.ID)
	if err != nil || current.State.Terminal() {
		return
	}
	if _, err := service.Finish(ctx.jobDeps(), jobs.FinishRequest{
		ExecutionRef:    jobs.ExecutionRef{JobID: accepted.ID},
		ExpectedVersion: current.Version,
		Outcome:         jobs.StateFailed,
		Failure: &jobs.Failure{
			Code:    "submission-refused",
			Class:   jobs.FailureClassPolicy,
			Message: "the download queue refused this submission",
		},
	}); err != nil {
		log.Printf("warning: could not record a refused submission as a failed job: %v", err)
	}
}

// jobHandleFor answers the legacy handle in one namespace a Job carries, without a
// viewer: an adapter has no principal, and a handle carries no authority — it is a
// name the executor's own id space answers to.
func (ctx *MahresourcesContext) jobHandleFor(jobID, namespace string) (string, error) {
	service := ctx.JobService()
	if service == nil {
		return "", nil
	}
	refs, err := service.LegacyHandlesFor(ctx.jobDeps(), jobID)
	if err != nil {
		return "", err
	}
	for _, ref := range refs {
		if ref.Namespace == namespace {
			return ref.Handle, nil
		}
	}
	return "", nil
}

// jobOwnerFor answers the owner one Job records, for the queue entry that will run
// it: the legacy panel and the legacy history filter on the submitter, and a
// successor Job accepted by a Retry is owned by whoever asked for the retry.
func (ctx *MahresourcesContext) jobOwnerFor(jobID string) *uint {
	service := ctx.JobService()
	if service == nil {
		return nil
	}
	snap, err := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, jobID)
	if err != nil {
		return nil
	}
	return snap.OwnerUserID
}

// queueEntryFor answers the queue entry carrying one execution, in this process.
//
// It looks the entry up by the canonical Job rather than by the handle, because the
// handle is resolved through the database and the queue is not: an entry this
// process submitted publishes into its Job from the moment it exists, whether or
// not the handle row can be read right now.
func (ctx *MahresourcesContext) queueEntryFor(jobID string) (*download_queue.DownloadJob, bool) {
	if ctx == nil || ctx.downloadManager == nil || jobID == "" {
		return nil, false
	}
	return ctx.downloadManager.GetJobByCanonicalJobID(jobID)
}

// waitForQueueExecution blocks until one queue entry reaches a terminal status, the
// context is cancelled, or the entry's terminal status was already there. Progress
// is published while it waits, because nothing else does: these executors report to
// the queue's own sink, and the Job is what a Job Center reader watches.
func (ctx *MahresourcesContext) waitForQueueExecution(
	ctxDone context.Context,
	execution jobs.Execution,
	entry *download_queue.DownloadJob,
) (*download_queue.DownloadJob, error) {
	if snap := entry.Snapshot(); queueJobTerminal(snap.Status) {
		return snap, nil
	}
	ticker := time.NewTicker(queueJobPollInterval)
	defer ticker.Stop()

	var published jobs.Progress
	for {
		select {
		case <-ctxDone.Done():
			return nil, ctxDone.Err()
		case <-ticker.C:
			snap := entry.Snapshot()
			if progress := queueJobProgress(snap); !sameProgress(progress, published) {
				published = progress
				if _, err := execution.Progress(progress); err != nil && !mirrorRefusalIsSilent(err) {
					return nil, err
				}
			}
			if queueJobTerminal(snap.Status) {
				return snap, nil
			}
		}
	}
}

// queueJobProgress is the bounded progress one queue snapshot describes.
//
// The queue's phase counters are items and its byte counters are the download
// path's; a job that reports neither is left indeterminate rather than reported as
// zero of an unknown total, which is a bar that never moves.
func queueJobProgress(snap *download_queue.DownloadJob) jobs.Progress {
	if snap == nil {
		return jobs.Progress{}
	}
	progress := jobs.Progress{Phase: snap.Phase, Message: snap.Phase}
	switch {
	case snap.PhaseTotal > 0:
		done, total := snap.PhaseCount, snap.PhaseTotal
		progress.Completed, progress.Total, progress.Unit = &done, &total, "items"
	case snap.TotalSize > 0:
		done, total := snap.Progress, snap.TotalSize
		progress.Completed, progress.Total, progress.Unit = &done, &total, "bytes"
	}
	return progress
}

// sameProgress reports whether two snapshots describe the same thing, so a poll
// that learned nothing new writes nothing. Every tick is 100ms and a Job's
// timeline is not the place to record that nothing happened.
func sameProgress(left, right jobs.Progress) bool {
	return left.Phase == right.Phase &&
		left.Message == right.Message &&
		left.Unit == right.Unit &&
		sameCount(left.Completed, right.Completed) &&
		sameCount(left.Total, right.Total)
}

func sameCount(left, right *int64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

// queueJobTerminal reports whether a queue status ends an execution. Processing is
// deliberately not terminal here: a generic job's only nonterminal statuses are
// pending and processing.
func queueJobTerminal(status download_queue.JobStatus) bool {
	switch status {
	case download_queue.JobStatusCompleted, download_queue.JobStatusFailed, download_queue.JobStatusCancelled:
		return true
	default:
		return false
	}
}

// finishQueueJob ends one Job from its queue entry's terminal status, if nobody has
// ended it yet.
//
// Reading first is what makes two writers safe: the queue's own mirror and this
// path can both reach the same conclusion, and whoever gets there second finds the
// Job terminal and writes nothing.
func (ctx *MahresourcesContext) finishQueueJob(
	execution jobs.Execution,
	outcome jobs.State,
	failure *jobs.Failure,
	requiredOutputs []string,
) error {
	service := ctx.JobService()
	if service == nil {
		return nil
	}
	current, err := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, execution.JobID)
	if err != nil {
		return err
	}
	if current.State.Terminal() {
		return nil
	}
	_, err = service.Finish(ctx.jobDeps(), jobs.FinishRequest{
		ExecutionRef:    jobs.ExecutionRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken},
		ExpectedVersion: current.Version,
		Outcome:         outcome,
		Failure:         failure,
		RequiredOutputs: requiredOutputs,
	})
	if err != nil && errors.Is(err, jobs.ErrStaleExecution) {
		// Somebody else's publish won: the Job is not this execution's to end.
		return nil
	}
	return err
}

// blockQueueJob records that one queue-backed Job cannot proceed and who has to
// decide. The reason is a bounded code rather than an error's text: only the Kind
// knows what in its own error is safe, and this lands in a timeline a reader
// searches.
func (ctx *MahresourcesContext) blockQueueJob(jobID, executionToken, reason string) error {
	service := ctx.JobService()
	if service == nil {
		return nil
	}
	current, err := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, jobID)
	if err != nil {
		return err
	}
	if current.State.Terminal() || current.State == jobs.StateBlocked {
		return nil
	}
	detail, err := json.Marshal(map[string]string{"reason": reason})
	if err != nil {
		return err
	}
	_, err = service.Transition(ctx.jobDeps(), jobs.Transition{
		JobID:           jobID,
		ExpectedVersion: current.Version,
		ExecutionToken:  executionToken,
		To:              jobs.StateBlocked,
		Event:           jobs.EventInput{Type: jobs.EventBlocked, Detail: detail},
	})
	return err
}

// publishQueueArtifact publishes one staged file as this Kind's artifact output,
// after confirming it is there.
//
// The existence check is the publisher's rather than the caller's because "the
// export succeeded" and "there is a tar to fetch" are two different facts, and the
// §7 rule is that a success whose artifact cannot be handed over is not one. A file
// that is not there is refused here rather than published as a reference to
// nothing.
func (ctx *MahresourcesContext) publishQueueArtifact(
	execution jobs.Execution,
	key, label, path string,
	expiresAt time.Time,
) error {
	info, err := ctx.GetDefaultFs().Stat(path)
	if err != nil {
		return fmt.Errorf("the artifact %s is not there: %w", path, err)
	}
	reference, err := json.Marshal(queueArtifactReference{Path: path, Size: info.Size()})
	if err != nil {
		return err
	}
	_, err = execution.Output(jobs.OutputInput{
		Key:       key,
		Type:      jobs.OutputTypeArtifact,
		Label:     label,
		Reference: reference,
		Required:  true,
		ExpiresAt: &expiresAt,
	})
	return err
}

// queueArtifactReference is what an artifact output names: where the bytes are, and
// how many of them the publisher verified. The Kind's own reader understands it and
// the control plane never interprets it.
type queueArtifactReference struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// publishQueueReport publishes one staged JSON document as a report output.
//
// Required is the caller's answer to §7's question — does this Job's success depend
// on the document? A plan does: a parse whose plan cannot be handed over parsed
// nothing. An apply's partial-result report does not: it is published *because* the
// apply failed, and a failure that produced one is still a failure.
func (ctx *MahresourcesContext) publishQueueReport(
	execution jobs.Execution,
	key, label, path string,
	required bool,
) error {
	if _, err := ctx.GetDefaultFs().Stat(path); err != nil {
		return fmt.Errorf("the report %s is not there: %w", path, err)
	}
	reference, err := json.Marshal(queueArtifactReference{Path: path})
	if err != nil {
		return err
	}
	_, err = execution.Output(jobs.OutputInput{
		Key:       key,
		Type:      jobs.OutputTypeReport,
		Label:     label,
		Reference: reference,
		Required:  required,
	})
	return err
}

// jobOutputsFor reads one Job's outputs without a viewer: an adapter has no
// principal, and what it asks is what its own execution published.
func (ctx *MahresourcesContext) jobOutputsFor(jobID string) ([]jobs.Output, error) {
	service := ctx.JobService()
	if service == nil {
		return nil, errors.New("this context has no job control plane installed")
	}
	return service.Outputs(ctx.jobDeps(), jobs.Access{Administrator: true}, jobID)
}

// findJobOutput answers one output by key.
func findJobOutput(outputs []jobs.Output, key string) (jobs.Output, bool) {
	for _, output := range outputs {
		if output.Key == key {
			return output, true
		}
	}
	return jobs.Output{}, false
}

// artifactPathOr prefers the path an output's own reference names, falling back to
// the one the Kind derives when the reference cannot be read.
func artifactPathOr(output jobs.Output, fallback string) string {
	path, err := artifactPathOf(output.Reference)
	if err != nil {
		return fallback
	}
	return path
}

// artifactPathOf decodes one artifact output's own reference. A reference this Kind
// did not write is refused rather than guessed at, because the only thing a caller
// can do with it is remove the file it names.
func artifactPathOf(reference json.RawMessage) (string, error) {
	var decoded queueArtifactReference
	if len(reference) == 0 {
		return "", errors.New("the artifact output names no file")
	}
	if err := json.Unmarshal(reference, &decoded); err != nil {
		return "", fmt.Errorf("the artifact output is not readable: %w", err)
	}
	if decoded.Path == "" {
		return "", errors.New("the artifact output names no file")
	}
	return decoded.Path, nil
}

// removeArtifact removes one staged artifact, reporting it removed when it is gone
// and when it was already gone — §7 treats a missing artifact as removed rather
// than as an error, and the second pass over one Job must not fail because the
// first one did its job.
func (ctx *MahresourcesContext) removeArtifact(reference json.RawMessage) (bool, error) {
	path, err := artifactPathOf(reference)
	if err != nil {
		return false, err
	}
	if err := ctx.GetDefaultFs().Remove(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Already gone is gone: §7 treats a missing artifact as removed rather than
			// as an error, so a second pass is not refused because the first one worked.
			return true, nil
		}
		return false, fmt.Errorf("remove artifact %s: %w", path, err)
	}
	return true, nil
}

// mirrorRefusalIsSilent reports whether a publish refusal is the fence working
// rather than a failure to report. A fenced-out or already-finished publish is
// exactly what a stale execution should get, and an abandoned queue entry's
// progress write is worth neither a log line nor a failed export.
func mirrorRefusalIsSilent(err error) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, jobs.ErrStaleExecution),
		errors.Is(err, jobs.ErrVersionConflict),
		errors.Is(err, jobs.ErrIllegalTransition),
		errors.Is(err, jobs.ErrNotFound):
		return true
	default:
		return false
	}
}
