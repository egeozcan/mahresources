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
	"mahresources/plugin_system"
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

// queueJobIntentPollInterval is how often a claimed execution re-reads its Job's
// durable control intent.
//
// §4 splits a cancellation in two: the intent is recorded where it can outlive an
// executor that stops answering, and the executor that holds the work publishes the
// outcome. When the person asking and the process running are the same one, the command
// path cancels the entry directly — but the Job may equally be owned by another runtime
// of the deployment, and nothing carries the request across that gap. The execution
// reads it for itself instead, which is the delivery mechanism that needs nothing new
// invented: it is already the thing that owns the work and the only thing that may end
// it. The cadence is slower than the entry poll because a cancellation is not a progress
// tick and this is a second query.
const queueJobIntentPollInterval = time.Second

// deliverCancelIntent reads one owned execution's durable control intent and, when a
// cancellation is waiting, stops the queue entry carrying the work in this process.
//
// next is the waiting loop's own clock for this check, so the several loops that need it
// do not each grow a ticker: the zero value asks immediately, which is what a Job
// cancelled before its execution started waiting needs.
//
// A refusal to cancel is deliberately not reported. An entry that is already terminal,
// already being cancelled, or gone from this process's registry is exactly what a
// delivered cancellation looks like, and the terminal publish that follows is what
// records the outcome.
func (ctx *MahresourcesContext) deliverCancelIntent(execution jobs.Execution, entry *download_queue.DownloadJob, next *time.Time) {
	now := time.Now()
	if next != nil {
		if now.Before(*next) {
			return
		}
		*next = now.Add(queueJobIntentPollInterval)
	}
	service := ctx.JobService()
	if service == nil || ctx.downloadManager == nil || entry == nil || execution.JobID == "" || entry.ID == "" {
		return
	}
	snap, err := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, execution.JobID)
	if err != nil || snap.State.Terminal() {
		return
	}
	if snap.ControlIntent != jobs.ControlIntentCancel {
		return
	}
	if err := ctx.downloadManager.Cancel(entry.ID); err != nil {
		var conflict *download_queue.StateConflictError
		if errors.As(err, &conflict) {
			// The entry moved on its own; nothing was asked of it any more.
			return
		}
		log.Printf("warning: job %s was cancelled and its executor's entry could not be stopped: %v", execution.JobID, err)
	}
}

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

// queueJobAdmission is what one queue-backed submission holds once the durable Job
// is committed: the Job itself, and — when this process may run it — the claim and
// execution token that keep every other runtime out of it.
type queueJobAdmission struct {
	// Accepted is the durable Job as it was committed.
	Accepted jobs.Snapshot
	// Execution is the claim this process owns. A zero value means the Job was
	// accepted to wait instead: the deployment's concurrency budget was full, so
	// there is no executor here and whichever runtime has a free slot takes it.
	Execution jobs.Execution
	// Lease is the claim's lease — how long it survives without the heartbeat that
	// renews it — and therefore the interval the heartbeat is derived from.
	Lease time.Duration
}

// Owned reports whether this process holds the Job's claim, and so owes it an
// executor.
func (a queueJobAdmission) Owned() bool { return a.Execution.ExecutionToken != "" }

// admitQueueJob accepts one queue-backed Job and, when the deployment has room,
// claims it in the same transaction.
//
// The order is not negotiable and it is why this lives here rather than in each of
// the submission paths. A submission starts the specialized executor — a queue entry
// — and until the Job is *owned* it is ordinary queued work of a registered Kind. A
// runtime claims queued work; a runtime in another process that claims this one
// finds no entry in its own queue, starts a second executor for it, and two processes
// run one transfer, one export or one import. Committing the claim and the capacity
// that admits it together with the acceptance leaves no such interval: the Job is
// owned from the moment it exists, or it is not running at all.
//
// A full deployment budget is therefore not a refusal. Capacity is a deployment-wide
// invariant — work that runs must have been admitted against it — but it says nothing
// about work that may be *queued* for later, so the Job is accepted in `queued` with
// its legacy handle and its sealed input, and no executor is started. It waits,
// visible and controllable in the Job Center, until a runtime with a free slot claims
// it and starts the executor there. Refusing a submission a busy deployment could run
// a minute later would be refusing to queue work, which is the one thing a queue is
// for.
func (ctx *MahresourcesContext) admitQueueJob(acceptance jobs.Acceptance) (queueJobAdmission, error) {
	service := ctx.JobService()
	if service == nil {
		return queueJobAdmission{}, nil
	}
	lease := ctx.queueJobClaimLease(service, acceptance.Kind, acceptance.KindVersion)
	execution, accepted, err := service.AcceptClaimed(context.Background(), ctx.jobDeps(), acceptance, jobs.ClaimRequest{
		Kind:        acceptance.Kind,
		KindVersion: acceptance.KindVersion,
		Claimant:    defaultJobRuntimeClaimant(),
		Capacity:    ctx.hostClaimCapacityBudget(),
		Lease:       lease,
	})
	switch {
	case err == nil:
		return queueJobAdmission{Accepted: accepted, Execution: execution, Lease: lease}, nil
	case errors.Is(err, jobs.ErrCapacityExhausted):
		// Accepted to wait rather than refused: nothing was written by the claim
		// attempt, so this is one acceptance on its own.
		waiting, acceptErr := service.Accept(ctx.jobDeps(), acceptance)
		if acceptErr != nil {
			return queueJobAdmission{}, acceptErr
		}
		return queueJobAdmission{Accepted: waiting}, nil
	default:
		return queueJobAdmission{}, err
	}
}

// queueJobClaimLease answers the lease a queue-backed admission claims with: the
// deployment's override when a test set one, otherwise the Kind's own.
func (ctx *MahresourcesContext) queueJobClaimLease(service *jobs.Service, kind string, version uint) time.Duration {
	if ctx != nil && ctx.queueClaimLease > 0 {
		return ctx.queueClaimLease
	}
	if adapter, ok := service.AdapterFor(kind, version); ok {
		return adapter.Definition().EffectiveLease()
	}
	return jobs.DefaultClaimLease
}

// ownQueueExecution keeps one queue-backed execution's claim alive for as long as the
// queue entry that runs it, and publishes the Kind's own outcome when it ends.
//
// It is the executor's half of the admission above: the queue is the specialized
// executor and this is its owner, for the whole of the entry's lifetime. Three things
// have to hold and none of them is a queue concern:
//
//   - the claim is renewed, or work that outlives its lease is reconciled out from
//     under itself — and an execution whose Job was blocked by that reconciliation has
//     its outcome refused outright, because `blocked -> succeeded` is not a
//     transition;
//   - the Kind's own outcome is published when the entry ends, because no polling
//     runtime will ever see this Job: it is owned from the moment it exists;
//   - the claim and the capacity that admitted it are handed back when the entry ends,
//     rather than held until a lease runs out.
//
// publish is the Kind's terminal publication — its adapter's outcome path, with the
// input the submission already holds — because what a Job publishes when its executor
// ends is the Kind's business and not this seam's. It is idempotent by construction:
// a download's own mirror may have got there first, and whoever arrives second finds
// the Job terminal and writes nothing.
//
// Nothing is written when the deployment is shutting down. A graceful stop cancels the
// queue's active entries, and the Job those entries were running is left exactly as it
// is — running, claimed, with its lease — for the next process to reconcile from the
// evidence the executor left behind. Recording `cancelled` there would end a Job the
// process taking over could still settle from its archive, its plan or the row it
// wrote.
func (ctx *MahresourcesContext) ownQueueExecution(admission queueJobAdmission, entry *download_queue.DownloadJob, publish func(*download_queue.DownloadJob) error) {
	service := ctx.JobService()
	if service == nil || !admission.Owned() || entry == nil {
		return
	}
	execution := admission.Execution
	done := make(chan struct{})
	go ctx.renewQueueExecutionClaim(execution, admission.Lease, done)
	go func() {
		defer close(done)
		snap, stopped := ctx.followQueueExecution(execution, entry)
		if stopped {
			return
		}
		// Asked again at the edge, because the entry's terminal status may *be* the
		// shutdown: the flag is set before any entry is cancelled, so a publish that
		// reached here after the queue stopped is one the stop caused, and the Job
		// belongs to the next process rather than to this record.
		if ctx.queueIsShuttingDown() {
			return
		}
		// The terminal snapshot is what the executor ended with, and it is retained here
		// for as long as the publication takes: the queue evicts the entry and the process
		// may be asked for the outcome long after its worker returned, so "the executor
		// ended" and "the Job says so" are two facts and only the second is durable.
		if ctx.publishTerminalOutcome(execution, snap, publish) == publicationAcknowledged {
			ctx.finishOwnedExecution(service, execution, nil)
		}
	}()
}

// terminalPublication says what became of one queue-backed terminal publication.
type terminalPublication int

const (
	// publicationAcknowledged is the durable plane holding the outcome — either because
	// this publication recorded it, or because somebody else had already ended the Job.
	publicationAcknowledged terminalPublication = iota
	// publicationFenced is the Job no longer being this execution's to end: it moved on,
	// somebody else settled it, or it is gone. There is nothing left to publish and a
	// second outcome is the one thing worse than a late one.
	publicationFenced
	// publicationUnfinished is this process being unable to make the outcome durable
	// before it stopped. The Job keeps its state, its claim and its lease, and the next
	// process reconciles it from the evidence the executor left behind.
	publicationUnfinished
)

// queuePublicationRetryInterval is how long a refused terminal publication waits before
// it is offered to the durable plane again. Short, because what it is waiting for is a
// transient write refusal and the Job stays visibly running until it lands.
const queuePublicationRetryInterval = 250 * time.Millisecond

// publishTerminalOutcome offers one queue execution's terminal outcome — the outputs it
// publishes and the outcome itself — to the durable plane until it is acknowledged.
//
// §3's acceptance boundary is the reason this cannot be a single attempt. A write
// refused by a locked database, a pool briefly exhausted, or a version that moved under
// a concurrent command is the write being refused, not the outcome being wrong; reading
// it as the executor failing turned a finished export into an immutable
// `dispatch-failed` Job whose archive was on disk and whose artifact was never
// published. The snapshot is immutable here, so every retry publishes the same outcome
// rather than recomputing one.
//
// Only a refusal by the fence, by the state machine, or by the Job's absence is final:
// there is nothing left to publish in any of those, and retrying would be a second
// outcome. Every other error is retried until the deployment stops. There is no time
// limit: the queue may evict its own terminal entry before a prolonged database outage
// ends, but this goroutine retains the snapshot the executor returned and continues to
// offer it. The Job stays claimed and its heartbeat stays live for the entire wait.
func (ctx *MahresourcesContext) publishTerminalOutcome(execution jobs.Execution, snap *download_queue.DownloadJob, publish func(*download_queue.DownloadJob) error) terminalPublication {
	if publish == nil {
		return publicationAcknowledged
	}
	logged := false
	for {
		err := publish(snap)
		if err == nil {
			return publicationAcknowledged
		}
		if settleRefused(err) {
			// The fence working, the state machine refusing, or the Job gone: the
			// outcome cannot be recorded and offering it again would be a second one.
			return publicationFenced
		}
		if ctx.queueIsShuttingDown() {
			log.Printf("warning: the outcome of queue job %s was not durable before shutdown; the next process reconciles it",
				execution.JobID)
			return publicationUnfinished
		}
		// One line per stuck execution rather than one per attempt: the retries are every
		// quarter second, and what an operator needs from this log is that an outcome is
		// waiting, not a count of how many times it was offered.
		if !logged {
			logged = true
			log.Printf("warning: could not publish the outcome of queue job %s (%v); retaining it to report again",
				execution.JobID, err)
		}
		time.Sleep(queuePublicationRetryInterval)
	}
}

// finishQueueExecution is what a queue-backed Kind's Dispatch returns: the terminal
// snapshot its executor reached, published to the durable plane.
//
// The publication is retained and offered again while it is refused, exactly as the
// submission path retains it, because a dispatch that is running this Kind's work is the
// same executor by another route — a Job admitted to wait for a capacity slot, or one a
// reconciliation queued again — and a refused write there turned the same finished export
// into the same immutable failure. The only error it carries out is the sentinel that says
// the outcome could not be made durable here, which leaves the Job running, claimed and
// leasable for the next process rather than recording an outcome the work did not reach.
func (ctx *MahresourcesContext) finishQueueExecution(execution jobs.Execution, snap *download_queue.DownloadJob, publish func(*download_queue.DownloadJob) error) error {
	switch ctx.publishTerminalOutcome(execution, snap, publish) {
	case publicationUnfinished:
		return errQueuePublicationUnfinished
	case publicationFenced:
		return errQueuePublicationFenced
	}
	return nil
}

// errQueuePublicationUnfinished reports that one execution's terminal outcome could not be
// made durable before this process stopped.
//
// It is not a failure of the work and it is not read as one: the Job keeps its state, its
// claim and its lease, and the next process reconciles it from the evidence the executor
// left behind (an archive, a plan, a row). Recording `dispatch-failed` instead is how a
// finished export became an immutable failure whose bytes were on disk.
var errQueuePublicationUnfinished = errors.New("the execution's outcome could not be made durable")

// errQueuePublicationFenced says the durable plane refused this publication because
// this execution no longer owns the Job, it already ended, or the state machine refused
// the outcome. The runtime must not turn that refusal into a dispatch failure.
var errQueuePublicationFenced = errors.New("the execution's outcome was refused by its fence")

// renewQueueExecutionClaim heartbeats one owned execution's claim for as long as its
// executor runs.
//
// A heartbeat refused because the token no longer owns the Job is not a failure to
// log: it is the fence telling this process that its execution was replaced — a
// reconciliation after a lease expiry handed the Job to another runtime — so there is
// nothing left here to renew.
func (ctx *MahresourcesContext) renewQueueExecutionClaim(execution jobs.Execution, lease time.Duration, done <-chan struct{}) {
	service := ctx.JobService()
	if service == nil || execution.ExecutionToken == "" {
		return
	}
	if lease <= 0 {
		lease = jobs.DefaultClaimLease
	}
	interval := lease / jobRuntimeHeartbeatDivisor
	if interval < minJobRuntimeHeartbeatInterval {
		interval = minJobRuntimeHeartbeatInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	ref := jobs.ExecutionRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken}
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			if err := service.Heartbeat(ctx.jobDeps(), ref, lease); err != nil {
				if errors.Is(err, jobs.ErrStaleExecution) {
					return
				}
				log.Printf("warning: heartbeat for queue job %s failed: %v", execution.JobID, err)
			}
		}
	}
}

// followQueueExecution waits for one queue entry to reach a terminal status, mirroring
// the progress it observes while it waits.
//
// It is a bounded poll for the same reason the adapters' own wait is: the queue
// publishes no completion signal to select on — its cancellation is a context and its
// completion is a field. It answers stopped=true when the deployment began shutting
// down underneath it, which is the one case its caller must write nothing about.
func (ctx *MahresourcesContext) followQueueExecution(execution jobs.Execution, entry *download_queue.DownloadJob) (*download_queue.DownloadJob, bool) {
	ticker := time.NewTicker(queueJobPollInterval)
	defer ticker.Stop()

	var published jobs.Progress
	var nextIntentCheck time.Time
	for {
		if ctx.queueIsShuttingDown() {
			return nil, true
		}
		snap := entry.Snapshot()
		if queueJobTerminal(snap.Status) {
			return snap, false
		}
		if progress := queueJobProgress(snap); !sameProgress(progress, published) {
			published = progress
			if _, err := execution.Progress(progress); err != nil && !mirrorRefusalIsSilent(err) {
				log.Printf("warning: mirroring the progress of queue job %s failed: %v", execution.JobID, err)
			}
		}
		ctx.deliverCancelIntent(execution, entry, &nextIntentCheck)
		<-ticker.C
	}
}

// queueIsShuttingDown reports whether this deployment's queue has begun stopping.
func (ctx *MahresourcesContext) queueIsShuttingDown() bool {
	if ctx == nil || ctx.downloadManager == nil {
		return false
	}
	return ctx.downloadManager.ShuttingDown()
}

// finishOwnedExecution is what every owner of a claimed execution does when its
// executor returns.
//
// The control plane is passed rather than read off the context, because the owner is
// not always the context's own control plane: a dispatch runtime is built with the
// service it registers its Kind adapters on, and a deployment that handed it one
// different from the context's would otherwise have this write to a plane that never
// saw the Job.
//
// An execution that ended its Job — success, failure, a return to the queue, a pause,
// a block — is done, and this only hands the claim back. One that returned while its
// Job was still running has left a Job owned by nobody, and a Job in that state is
// resolved by nothing at all: it is not queued, no claim of it expires into a
// reconciliation, and no executor owns it. So it is ended here, bounded and classed,
// with the claim and the capacity.
//
// The one Job this does *not* hand anything back for is a quarantined one: blocked
// work whose claim nobody could resolve, still carrying this execution's token. Its
// claim is not the runtime's to return — the executor returning proves the observer
// let go, not that the worker stopped — so it is left for the execution's own
// outcome, which is the only proof §3 accepts.
//
// The executor's own error text is deliberately not recorded or logged. Only the Kind
// knows what in it is safe — a URL, a header, a plugin value — and the durable record
// is a bounded taxonomy instead. An executor that wants to explain itself appends a
// bounded event before it returns.
func (ctx *MahresourcesContext) finishOwnedExecution(service *jobs.Service, execution jobs.Execution, execErr error) {
	if service == nil || ctx == nil || execution.JobID == "" {
		return
	}
	deps := ctx.jobDeps()
	ref := jobs.ExecutionRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken}
	snap, err := service.Get(deps, jobs.Access{Administrator: true}, execution.JobID)
	if err != nil {
		log.Printf("job execution: reading job %s after its execution ended failed: %v", execution.JobID, err)
		return
	}

	if snap.State == jobs.StateRunning && errors.Is(execErr, errQueuePublicationUnfinished) {
		// The executor reached its outcome and could not make it durable here. Ending the
		// Job now — as unfinished or as dispatch-failed — would record an outcome the work
		// did not reach; leaving it running with its claim and its lease is what lets the
		// next process settle it from the evidence the executor left behind.
		return
	}
	if snap.State == jobs.StateRunning && errors.Is(execErr, errQueuePublicationFenced) {
		// The publication was refused as stale, already ended, or illegal. That is not
		// evidence that the executor failed; only its owner may publish its outcome.
		return
	}

	if snap.State != jobs.StateRunning {
		quarantined, ownErr := service.OwnsQuarantinedJob(deps, ref)
		if ownErr != nil {
			log.Printf("job execution: reading the claim state of job %s failed: %v", execution.JobID, ownErr)
			return
		}
		if quarantined {
			// The Job is blocked with this execution's token still recorded, which
			// is what a quarantine is: nobody could prove the external work the
			// claim started had stopped (§3). An executor returning is not that
			// proof — it says the *observer* let go, while the worker behind it (a
			// queue entry, a lua.LFunction) may still be running. Releasing here did
			// exactly that: a capacity-queued transfer was handed its claim and its
			// slot back mid-flight, and a Resume — which is then permitted, because
			// no unresolved claim is left — started a second transfer of one URL.
			//
			// The proof the design asks for is the execution's own outcome, and
			// that is what settles this Job: a terminal transition under the token
			// takes the claim and the capacity with it in one transaction. Until
			// then the quarantine is left exactly where it is.
			return
		}
		if _, err := service.ReleaseClaim(deps, jobs.ReleaseRequest{
			ExecutionRef: ref, Reason: jobs.ReleaseReasonExecutionEnded,
		}); err != nil {
			log.Printf("job execution: releasing job %s failed: %v", execution.JobID, err)
		}
		return
	}

	code := jobRuntimeUnfinishedCode
	if execErr != nil {
		code = jobRuntimeDispatchFailedCode
	}
	_, err = service.Finish(deps, jobs.FinishRequest{
		ExecutionRef:    ref,
		ExpectedVersion: snap.Version,
		Outcome:         jobs.StateFailed,
		Failure:         &jobs.Failure{Code: code, Class: jobs.FailureClassInternal},
	})
	switch {
	case err == nil:
		log.Printf("job execution: job %s was ended as %s by its runtime", execution.JobID, code)
	case errors.Is(err, jobs.ErrStaleExecution), errors.Is(err, jobs.ErrVersionConflict):
		// A reconciliation or another runtime owns the Job now, which is exactly what
		// the fence is for: nothing to do.
	default:
		log.Printf("job execution: ending job %s failed: %v", execution.JobID, err)
	}
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

// failUndispatchedQueueJob ends a Job whose work the queue refused.
//
// The queue's own error text is deliberately not carried: a Job's failure record is
// a bounded taxonomy a reader groups on. A refusal here is not a failed export — it
// is an admission the deployment could not take — so it is classed as a policy
// refusal of the submission.
//
// It carries the execution token rather than the Job id alone, and that is the whole
// of "a submission whose executor could not start leaves nothing behind": the finish
// is one transaction that ends the Job, clears the token and frees the capacity the
// admission occupied. A Job left running with no executor would be work nothing can
// claim and nothing reconciles.
func (ctx *MahresourcesContext) failUndispatchedQueueJob(admission queueJobAdmission, cause error) {
	service := ctx.JobService()
	if service == nil || admission.Accepted.ID == "" {
		return
	}
	if cause != nil {
		log.Printf("warning: the download queue refused the submission for job %s: %v", admission.Accepted.ID, cause)
	}
	current, err := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, admission.Accepted.ID)
	if err != nil || current.State.Terminal() {
		return
	}
	if _, err := service.Finish(ctx.jobDeps(), jobs.FinishRequest{
		ExecutionRef: jobs.ExecutionRef{
			JobID:          admission.Execution.JobID,
			ExecutionToken: admission.Execution.ExecutionToken,
		},
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
	return ctx.jobHandleForDeps(ctx.jobDeps(), jobID, namespace)
}

// jobHandleForDeps is jobHandleFor on a caller's own handle.
//
// It exists because a handle read is one of the answers a Kind's *advertisement*
// gives, and an advertisement computed inside a command's transaction must read on
// that transaction's handle. Reaching for the process's own would take a second
// connection while the first is held.
func (ctx *MahresourcesContext) jobHandleForDeps(deps jobs.Deps, jobID, namespace string) (string, error) {
	service := ctx.JobService()
	if service == nil {
		return "", nil
	}
	refs, err := service.LegacyHandlesFor(deps, jobID)
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

// runtimeIsProvedGone reports whether the runtime a claim named cannot still be
// running its work: this host has booted since, or the process no longer exists.
//
// It is the only positive evidence of quiescence available to a reconciler that
// reaches nothing but the database, and it is deliberately conservative: another
// host's process table is not ours to read, a pid that exists may be a reused one,
// and with no boot session recorded a pid says nothing across a reboot. Every one of
// those answers "not proved", which is what keeps a Job nonterminal rather than
// terminating work that may still be running.
func runtimeIsProvedGone(request jobs.ReconcileRequest) bool {
	return runtimeClaimantIsProvedGone(request.Claimant)
}

func runtimeClaimantIsProvedGone(claimant string) bool {
	identity, ok := plugin_system.ParseRuntimeIdentity(claimant)
	if !ok {
		return false
	}
	return identity.Liveness() == plugin_system.RuntimeGone
}

// queueOnlyIfTheRuntimeIsProvedGone is the honest form of "run it again" for a Kind
// whose executor lives in one process's memory.
//
// The queue is not a database: an entry this process does not hold may be running
// perfectly well in another one, and "I cannot see it" is not "nobody is running
// it". Acting on that absence re-runs work a live process is doing — a second
// transfer of one URL, a second export of one tree — which is why §3 permits a
// replacement dispatch only once the owning runtime is *proved* quiescent, and why
// the claim's claimant is what is asked: it names the process that took the Job when
// it was dispatched, and it is the runtime's own identity rather than a second
// spelling of "this process".
//
// Proved gone — this host has rebooted since, or the pid no longer exists — the Job
// goes back to the queue and the next process starts the work. Anything else (alive,
// another host, an identity no reconciler can read) leaves the Job nonterminal and
// blocked with its claim and its capacity held, for an operator to resolve. That is
// the fail-safe direction the design takes everywhere: a Job waiting for a person is
// recoverable, a duplicated side effect is not.
//
// It is deliberately not used where a Kind has *positive* durable evidence that its
// execution ended — an archive at the published path, a plan on disk, a row the run
// wrote. That is evidence about the work rather than about a process's memory, and
// acting on it is what §3 means by reconciling durable side effects.
func (ctx *MahresourcesContext) queueOnlyIfTheRuntimeIsProvedGone(request jobs.ReconcileRequest) jobs.ReconcileDecision {
	if runtimeIsProvedGone(request) {
		return jobs.ReconcileQueue
	}
	return jobs.ReconcileExternalWorkUnproven
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
	var nextIntentCheck time.Time
	for {
		select {
		case <-ctxDone.Done():
			return nil, ctxDone.Err()
		case <-ticker.C:
			snap := entry.Snapshot()
			// A progress snapshot is telemetry, and a failure to record it is not the
			// executor ending. Returning here — which this did — let one refused write
			// end the Job as `dispatch-failed` while the queue's own worker was still
			// exporting, importing or reducing: the Job Center then offered a Retry of
			// work that had not stopped, and the claim and the capacity that owned it went
			// with the wrong outcome. Ownership lasts as long as the worker does, so a
			// refused write is logged and attempted again on the next tick; the terminal
			// status below is what ends the wait.
			if progress := queueJobProgress(snap); !sameProgress(progress, published) {
				if err := ctx.jobFaults.progressWrite(); err != nil {
					log.Printf("warning: mirroring the progress of queue job %s failed: %v", execution.JobID, err)
				} else if _, err := execution.Progress(progress); err != nil {
					if !mirrorRefusalIsSilent(err) {
						log.Printf("warning: mirroring the progress of queue job %s failed: %v", execution.JobID, err)
					} else {
						// A fenced-out publish is not retried: the execution that lost its
						// claim may not write to the Job at all, and the wait ends when the
						// entry does.
						published = progress
					}
				} else {
					published = progress
				}
			}
			ctx.deliverCancelIntent(execution, entry, &nextIntentCheck)
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
// Job terminal and writes nothing. The read is repeated when the version moved
// underneath it — a progress update, a control request, another writer's publish — and
// only a bounded number of times, because each attempt reads the row again and either
// finds the Job already terminal or ends it at the version it now carries. A write that
// fails for any other reason is *returned* rather than read as an outcome: the caller
// owns the terminal snapshot and offers it again until the durable plane has it.
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
	var contended error
	for attempt := 0; attempt < queuePublicationWriteAttempts; attempt++ {
		current, err := ctx.jobCompletionRead(service, execution.JobID)
		if err != nil {
			return err
		}
		if current.State.Terminal() {
			ctx.notifyQueueJobCanonicalUpdate(execution.JobID)
			return nil
		}
		if err := ctx.jobFaults.completionWrite(); err != nil {
			return err
		}
		_, err = service.Finish(ctx.jobDeps(), jobs.FinishRequest{
			ExecutionRef:    jobs.ExecutionRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken},
			ExpectedVersion: current.Version,
			Outcome:         outcome,
			Failure:         failure,
			RequiredOutputs: requiredOutputs,
		})
		switch {
		case err == nil:
			ctx.notifyQueueJobCanonicalUpdate(execution.JobID)
			return nil
		case errors.Is(err, jobs.ErrStaleExecution):
			// Somebody else's publish won: the Job is not this execution's to end.
			return nil
		case errors.Is(err, jobs.ErrVersionConflict):
			contended = err
		default:
			return err
		}
	}
	return contended
}

// notifyQueueJobCanonicalUpdate wakes legacy stream clients after the durable
// terminal state is committed. The stream resolves the queue event's legacy id
// again through the current handle and visibility projection before sending it.
func (ctx *MahresourcesContext) notifyQueueJobCanonicalUpdate(canonicalJobID string) {
	if ctx == nil || ctx.downloadManager == nil || canonicalJobID == "" {
		return
	}
	ctx.downloadManager.NotifyJobUpdatedByCanonicalJobID(canonicalJobID)
}

// queuePublicationWriteAttempts bounds the versioned retries of one terminal write. The
// loop re-reads the Job each time, so an attempt only fails again when a concurrent
// writer moved the version inside the window between the read and the write.
const queuePublicationWriteAttempts = 5

// jobCompletionRead reads one Job on the way to publishing a queue-backed outcome.
//
// It is a named step rather than an inline call because the failure paths either side of
// it are the contract this file exists for — a read that fails is not a Job that
// finished — and a test needs to reach the retry without inducing a real outage.
func (ctx *MahresourcesContext) jobCompletionRead(service *jobs.Service, jobID string) (jobs.Snapshot, error) {
	if err := ctx.jobFaults.completionRead(); err != nil {
		return jobs.Snapshot{}, err
	}
	return service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, jobID)
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
		return fmt.Errorf("%w: the artifact %s is not there: %w", errQueueStagedOutputMissing, path, err)
	}
	artifactReference := queueArtifactReference{Path: path, Size: info.Size()}
	if execution.Kind == JobKindGroupExport {
		request, requestErr := exportRequestOf(execution.Input)
		if requestErr != nil || path != exportArchivePath(execution.JobID, request.Gzip) {
			return errors.New("group export artifact path does not match its canonical Job")
		}
		manifest, manifestErr := readGroupExportScopeManifestFromPath(ctx, path)
		if manifestErr != nil {
			return fmt.Errorf("verify group export scope manifest: %w", manifestErr)
		}
		if err := validateGroupExportScopeManifest(manifest, execution.Input); err != nil {
			return fmt.Errorf("verify group export scope manifest: %w", err)
		}
		artifactReference.ScopeManifestVersion = jobExportScopeManifestVersion
	}
	reference, err := json.Marshal(artifactReference)
	if err != nil {
		return err
	}
	if err := ctx.jobFaults.outputPublication(); err != nil {
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

// errQueueStagedOutputMissing reports that the file a queue-backed output names is not
// there, which is the Kind's own evidence that its work produced nothing. It is
// deliberately distinct from the *publication* failing: a write refused by a locked
// database is not a missing archive, and reading the two as one ended a finished export
// as `export-artifact-missing` while its bytes sat on disk.
var errQueueStagedOutputMissing = errors.New("the staged output is not there")

// queueArtifactReference is what an artifact output names: where the bytes are, and
// how many of them the publisher verified. The Kind's own reader understands it and
// the control plane never interprets it.
type queueArtifactReference struct {
	Path                 string `json:"path"`
	Size                 int64  `json:"size"`
	ScopeManifestVersion uint   `json:"scopeManifestVersion,omitempty"`
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
		return fmt.Errorf("%w: the report %s is not there: %w", errQueueStagedOutputMissing, path, err)
	}
	reference, err := json.Marshal(queueArtifactReference{Path: path})
	if err != nil {
		return err
	}
	if err := ctx.jobFaults.outputPublication(); err != nil {
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
