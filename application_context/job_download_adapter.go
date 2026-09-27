package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"mahresources/download_queue"
	"mahresources/hostfetch"
	"mahresources/jobs"
	"mahresources/models"
	"mahresources/models/query_models"

	"gorm.io/gorm"
)

// This file is the download Kind adapter: the one place that knows how a remote or
// deferred download joins the durable Job control plane.
//
// The work itself stays where it was. The queue is unchanged as an executor — it
// fetches, it assembles HLS, it writes the Resource — and what is added is that an
// accepted Job names the execution, that every lifecycle fact the transfer
// produces is mirrored into it under the execution's own fencing token, and that
// the controls the queue already has (cancel, resume) are reached through the
// canonical command surface the Job advertises.
//
// Two things are deliberately *not* here:
//
//   - A pause command. §1 defines `paused` as a checkpoint the executor confirmed,
//     and this queue's resume restarts a transfer from zero — it has none. A held
//     download is therefore reported as `blocked` (a person is holding it), and the
//     Job advertises `resume` rather than `pause` until the queue can honestly
//     checkpoint.
//   - A Repeat command. Re-fetching a URL that already produced a Resource would
//     create a second copy of content the library already holds, and the content
//     hash would refuse it at the end of the transfer. Retry is for unsuccessful
//     work, which is what this Kind offers.

const (
	// JobKindRemoteDownload is a download a person or plugin submitted for now.
	JobKindRemoteDownload = "remote-download"
	// JobKindDeferredDownload is one submitted for a concrete future time. It is a
	// scheduled Job from acceptance, and its due time materializes the same Job
	// rather than a second execution.
	JobKindDeferredDownload = "deferred-download"
	// jobDownloadKindVersion is the version of these Kinds' input semantics: the
	// shape of the payload below, and of the summary derived from it.
	jobDownloadKindVersion = 1

	// jobDownloadPollInterval is how often the adapter re-reads the queue entry it
	// is waiting for. The queue is in memory and its worker publishes no completion
	// signal this adapter can select on, so waiting is a bounded poll rather than a
	// channel: the alternative — a live pointer with a lock — is the race the
	// snapshots exist to avoid.
	jobDownloadPollInterval = 200 * time.Millisecond
)

// downloadJobInput is what a download Job is accepted with: the submission's own
// payload plus the origin that selects its egress policy.
//
// The plugin name is part of the sealed input rather than a Job column because it
// is execution-required: a Retry replays it on a worker in a process that may
// never have seen the original submission, and a retry that forgot the origin
// would silently run as a host fetch under the wider host policy.
type downloadJobInput struct {
	Creator *query_models.ResourceFromRemoteCreator `json:"creator"`
	Plugin  string                                  `json:"plugin,omitempty"`
}

// remoteDownloadInputJSON accepts one submission and returns the input to seal.
func remoteDownloadInputJSON(creator *query_models.ResourceFromRemoteCreator, pluginName string) (json.RawMessage, error) {
	if creator == nil {
		return nil, errors.New("a download Job needs the submission it was accepted for")
	}
	if strings.TrimSpace(creator.URL) == "" {
		return nil, errors.New("a download Job needs a URL")
	}
	return json.Marshal(downloadJobInput{Creator: creator, Plugin: pluginName})
}

// downloadJobCodec is the Kind's declaration of how its input becomes searchable
// text and how it becomes sealed bytes.
func downloadJobCodec() jobs.ReplayCodec {
	return jobs.ReplayCodec{
		Sanitize: sanitizeDownloadInput,
		Encode:   encodeDownloadInput,
		Decode:   decodeDownloadInput,
		Migrate: func(payload json.RawMessage, fromVersion, toVersion uint) (json.RawMessage, error) {
			if fromVersion != toVersion {
				return nil, fmt.Errorf("jobs: no download input migration from v%d to v%d", fromVersion, toVersion)
			}
			return payload, nil
		},
	}
}

// downloadSummary is the bounded, searchable half of a download's input: what a
// reader may see about it without ever seeing the input itself.
//
// The URL is reduced to its scheme and host — never its query, its userinfo or its
// fragment, which is where a signed link's token lives — and the headers are
// omitted entirely, because they are the one field that can carry a credential.
type downloadSummary struct {
	Scheme  string   `json:"scheme,omitempty"`
	Host    string   `json:"host,omitempty"`
	Name    string   `json:"name,omitempty"`
	Plugin  string   `json:"plugin,omitempty"`
	Targets []string `json:"targets,omitempty"`
}

func sanitizeDownloadInput(input json.RawMessage) (json.RawMessage, error) {
	summary, err := downloadSummaryOf(input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(summary)
}

func encodeDownloadInput(input json.RawMessage) (json.RawMessage, error) {
	// The input is validated by being summarized, so a payload nothing can read is
	// refused at acceptance rather than stored as bytes no execution can use.
	if _, err := downloadSummaryOf(input); err != nil {
		return nil, err
	}
	return input, nil
}

func decodeDownloadInput(payload json.RawMessage, version uint) (json.RawMessage, error) {
	if version != jobDownloadKindVersion {
		return nil, fmt.Errorf("%w: download v%d input", jobs.ErrReplayCodecUnregistered, version)
	}
	if _, err := downloadSummaryOf(payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// downloadSummaryOf derives one submission's safe summary.
func downloadSummaryOf(input json.RawMessage) (downloadSummary, error) {
	var decoded downloadJobInput
	if len(input) == 0 || !json.Valid(input) {
		return downloadSummary{}, errors.New("a download Job's input is not valid JSON")
	}
	if err := json.Unmarshal(input, &decoded); err != nil {
		return downloadSummary{}, fmt.Errorf("a download Job's input is not readable: %w", err)
	}
	if decoded.Creator == nil || strings.TrimSpace(decoded.Creator.URL) == "" {
		return downloadSummary{}, errors.New("a download Job's input names no URL")
	}

	summary := downloadSummary{Name: decoded.Creator.FileName, Plugin: decoded.Plugin}
	if parsed, err := url.Parse(strings.TrimSpace(decoded.Creator.URL)); err == nil {
		summary.Scheme = parsed.Scheme
		summary.Host = parsed.Host
	}
	if decoded.Creator.OwnerId != 0 {
		summary.Targets = append(summary.Targets, fmt.Sprintf("owner:%d", decoded.Creator.OwnerId))
	}
	for _, groupID := range decoded.Creator.Groups {
		summary.Targets = append(summary.Targets, fmt.Sprintf("group:%d", groupID))
	}
	for _, noteID := range decoded.Creator.Notes {
		summary.Targets = append(summary.Targets, fmt.Sprintf("note:%d", noteID))
	}
	if decoded.Creator.GroupName != "" {
		summary.Targets = append(summary.Targets, "creates-group")
	}
	return summary, nil
}

// downloadJobTitle is the Job's own title: the file name the submission chose, or
// the URL's host when it chose none. Never the URL itself — a title is searchable
// text, and a URL can carry a token. The title is a bounded projection; the replay
// input keeps the complete file name for execution and retry.
func downloadJobTitle(input json.RawMessage) string {
	summary, err := downloadSummaryOf(input)
	if err != nil {
		return "Download"
	}
	title := "Download"
	if summary.Name != "" {
		title = summary.Name
	} else if summary.Host != "" {
		title = "Download from " + summary.Host
	}
	return truncateDownloadJobTitle(title)
}

func truncateDownloadJobTitle(title string) string {
	if len(title) <= jobs.MaxTitleBytes {
		return title
	}
	title = title[:jobs.MaxTitleBytes]
	for !utf8.ValidString(title) {
		title = title[:len(title)-1]
	}
	return title
}

// downloadJobAdapter runs one of the download Kinds.
type downloadJobAdapter struct {
	ctx  *MahresourcesContext
	kind string
	// waits is set on the registered adapter, the one the dispatch loop runs; an
	// adapter built to publish one outcome dispatches nothing and needs none.
	waits *downloadURLWaits
}

// Definition declares what the control plane must know before it claims any of
// this Kind's work.
//
// Restorable, because the payload that started a transfer is sealed with the Job:
// a Job whose process died is one this Kind can start again. It draws on no shared
// capacity group and declares no ceiling of its own, because the queue already
// owns both — its own semaphore and its per-domain gate — and a second budget here
// would be a second, invisible limit on the same work.
func (a *downloadJobAdapter) Definition() jobs.Definition {
	return jobs.Definition{
		Kind:        a.kind,
		KindVersion: jobDownloadKindVersion,
		Restorable:  true,
		Visibility:  jobs.VisibilityOwner,
	}
}

// Dispatch runs one claimed download.
//
// It does not perform the transfer itself: it makes sure this process's queue is
// running it, and waits. The waiting is the dispatch contract — an adapter that
// returned while its Job was still running would have its Job failed by the
// runtime as unfinished — and it is also where a Job whose queue entry already
// finished (a transfer that outlived a claim, or one that ran before this process
// adopted it) gets the outcome it is missing.
func (a *downloadJobAdapter) Dispatch(ctx context.Context, execution jobs.Execution) error {
	if a.ctx == nil || a.ctx.downloadManager == nil {
		return errors.New("the download queue is not available")
	}
	input, err := decodeDownloadInput(execution.Input, execution.KindVersion)
	if err != nil {
		return err
	}
	var decoded downloadJobInput
	if err := json.Unmarshal(input, &decoded); err != nil {
		return err
	}

	if reason := a.refusalReason(execution, &decoded); reason != "" {
		return a.block(execution, reason)
	}

	entry, found := a.ctx.downloadManager.GetJobByCanonicalJobID(execution.JobID)
	if !found {
		// Asked of the queue's memory first, and decided again under the queue's
		// lock by the start itself.
		if live := a.ctx.downloadManager.OtherActiveTransfer(decoded.Creator.URL, execution.JobID); live != "" {
			return a.waitForTheURL(execution, decoded.Creator.URL)
		}
		entry, err = a.start(execution, &decoded)
		var busy *download_queue.URLActiveError
		if errors.As(err, &busy) {
			return a.waitForTheURL(execution, decoded.Creator.URL)
		}
		if reason := download_queue.InvalidDownloadURLReason(err); reason != "" {
			// Accepted before submission refused such an address, and no later
			// attempt can fetch it: the Job fails as invalid input, which offers no
			// Retry, rather than as an executor error that would.
			return a.finishFailed(execution, download_queue.FailureInvalidURL,
				"the stored address is not a download: "+reason)
		}
		if err != nil {
			return err
		}
	} else if ref, ok := entry.CanonicalExecution(); !ok || ref.ExecutionToken != execution.ExecutionToken {
		if !entry.AttachCanonical(download_queue.CanonicalRef{
			JobID: execution.JobID, ExecutionToken: execution.ExecutionToken,
		}) {
			return fmt.Errorf("the queue entry %s publishes into another Job", entry.ID)
		}
	}

	// A held transfer is not adopted here. It is waiting for a person, and §4 makes
	// the executor's own confirmation the thing that ends a hold. The confirmation is
	// durable rather than a call into this process: a resume queues the Job, so a
	// paused entry reached by a *dispatch* is one whose hold the person released —
	// whereas a Job that is still running under a paused entry is a hold nobody
	// released, and restarting it would undo what its owner deliberately stopped.
	if entry.GetStatus() == download_queue.JobStatusPaused {
		if !a.queuedForDispatch(execution) {
			return a.block(execution, "paused")
		}
		if err := a.ctx.downloadManager.ResumeExclusive(entry.ID); err != nil {
			var busy *download_queue.URLActiveError
			if errors.As(err, &busy) {
				return a.waitForTheURL(execution, entry.GetURL())
			}
			var conflict *download_queue.StateConflictError
			if errors.As(err, &conflict) {
				// The entry moved while this dispatch held its claim: whatever it moved
				// to is the executor's answer, and the wait below publishes it.
				return a.block(execution, "paused")
			}
			return err
		}
	}

	snap, err := a.waitForTerminal(ctx, execution, entry)
	if err != nil {
		return err
	}
	return a.ctx.finishQueueExecution(execution, snap, func(finished *download_queue.DownloadJob) error {
		return a.publishOutcome(execution, finished)
	})
}

// downloadWaitingPhase is the phase a download's Job reports while it waits for
// another transfer of its URL.
const downloadWaitingPhase = "waiting"

// downloadURLWaits remembers, in this process, the Jobs that went back to the
// queue to wait for another transfer of their URL, so the dispatch loop passes
// over them until that transfer ends (ClaimExclusions).
//
// It is memory and meant to be: the transfer a Job waits for is this process's
// queue entry, which a restart takes away too, and a Job the next process claims
// finds out for itself whether its URL is busy there.
type downloadURLWaits struct {
	mu    sync.Mutex
	byJob map[string]string
}

func newDownloadURLWaits() *downloadURLWaits {
	return &downloadURLWaits{byJob: make(map[string]string)}
}

func (w *downloadURLWaits) add(jobID, url string) {
	if w == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	w.byJob[jobID] = url
}

// stillWaiting forgets every Job whose URL is free and lists the rest.
func (w *downloadURLWaits) stillWaiting(busy func(jobID, url string) bool) []string {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	waiting := make([]string, 0, len(w.byJob))
	for jobID, url := range w.byJob {
		if busy(jobID, url) {
			waiting = append(waiting, jobID)
			continue
		}
		delete(w.byJob, jobID)
	}
	sort.Strings(waiting)
	return waiting
}

// ClaimExclusions names the waiting Jobs of this Kind whose URL another transfer
// in this process is still fetching. The dispatch loop passes over them, so they
// wait in the queue holding no claim and no capacity, and are claimed on the first
// pass after the URL frees.
func (a *downloadJobAdapter) ClaimExclusions() []string {
	if a == nil || a.ctx == nil || a.ctx.downloadManager == nil {
		return nil
	}
	return a.waits.stillWaiting(func(jobID, url string) bool {
		return a.ctx.downloadManager.OtherActiveTransfer(url, jobID) != ""
	})
}

// downloadAdapterFor answers this process's registered adapter for a download
// Kind, whose record of waiting Jobs the dispatch loop consults. A process with no
// adapter registered gets one without that record: its waiting Job is then claimed
// on the next pass and, finding the URL still busy, goes back to wait there.
func (ctx *MahresourcesContext) downloadAdapterFor(kind string) *downloadJobAdapter {
	if service := ctx.JobService(); service != nil {
		if adapter, ok := service.AdapterFor(kind, jobDownloadKindVersion); ok {
			if download, ok := adapter.(*downloadJobAdapter); ok {
				return download
			}
		}
	}
	return &downloadJobAdapter{ctx: ctx, kind: kind}
}

// waitForTheURL hands this execution's Job back to the queue to wait for the
// transfer already fetching its URL.
//
// One URL is fetched once at a time here, and a Job that finds its URL downloading
// waits for that transfer rather than being blocked: nothing would release a block
// when the other transfer ended, and the person who pressed Retry would be left
// holding a Job that needed them for no reason. It waits in the queue rather than
// in this dispatch, because a claim held while waiting holds a slot of the
// deployment's budget, and a few duplicate retries would starve every other Kind.
// Its row says what it is waiting for, a cancel reaches it as queued work, and the
// dispatch loop passes over it until the URL frees (ClaimExclusions).
func (a *downloadJobAdapter) waitForTheURL(execution jobs.Execution, url string) error {
	a.waits.add(execution.JobID, url)
	return a.ctx.requeueDownloadExecution(execution, jobDownloadWaitingForURLReason,
		downloadWaitingPhase, "Waiting for another download of this URL to finish")
}

// jobDownloadWaitingForURLReason is the reason a download's Job records when it
// goes back to the queue to wait for another transfer of its URL.
const jobDownloadWaitingForURLReason = "waiting-for-url"

// requeueDownloadExecution hands a running download's Job back to the queue under
// its execution's token, saying why: in its progress, which every Jobs surface
// shows on the row, and in the queued event's reason.
//
// It is the answer for work that has not failed and will run again: a transfer the
// deployment's shutdown stopped, and one waiting for its URL. The transition takes
// the claim and the capacity with it. A cancellation already recorded against the
// Job owns the outcome instead, and ends it cancelled. A write the fence refuses
// is not this execution's to make and is dropped; any other failure is returned
// as errQueuePublicationUnfinished, which leaves the Job running under its lease
// for the reconciliation an expired claim gets, rather than ending it.
func (ctx *MahresourcesContext) requeueDownloadExecution(execution jobs.Execution, reason, phase, message string) error {
	service := ctx.JobService()
	if service == nil || execution.ExecutionToken == "" {
		return nil
	}
	deps := ctx.jobDeps()
	ref := jobs.ExecutionRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken}
	detail, err := json.Marshal(map[string]string{"reason": reason})
	if err != nil {
		return err
	}
	var lastErr error
	for attempt := 0; attempt < queuePublicationWriteAttempts; attempt++ {
		current, err := service.Get(deps, jobs.Access{Administrator: true}, execution.JobID)
		if err != nil {
			return settleRequeue(err)
		}
		if current.State != jobs.StateRunning {
			return nil
		}
		if current.ControlIntent == jobs.ControlIntentCancel {
			return settleRequeue(ctx.finishQueueJob(execution, jobs.StateCancelled, nil, nil))
		}
		if _, err := service.UpdateProgress(deps, ref, jobs.Progress{Phase: phase, Message: message}); err != nil {
			if mirrorRefusalIsSilent(err) && !errors.Is(err, jobs.ErrVersionConflict) {
				return nil
			}
			lastErr = err
			continue
		}
		_, err = service.Transition(deps, jobs.Transition{
			JobID:           execution.JobID,
			ExpectedVersion: current.Version,
			ExecutionToken:  execution.ExecutionToken,
			To:              jobs.StateQueued,
			Phase:           phase,
			Event:           jobs.EventInput{Type: jobs.EventQueued, Detail: detail},
		})
		if err == nil {
			return nil
		}
		if !errors.Is(err, jobs.ErrVersionConflict) {
			return settleRequeue(err)
		}
		lastErr = err
	}
	return settleRequeue(lastErr)
}

// settleRequeue sorts what a requeue's write answered: nothing, or the fence doing
// its job, is done; anything else leaves the Job to its lease.
func settleRequeue(err error) error {
	if err == nil || (mirrorRefusalIsSilent(err) && !errors.Is(err, jobs.ErrVersionConflict)) {
		return nil
	}
	log.Printf("warning: a download Job could not be returned to the queue (%v); its lease settles it", err)
	return errQueuePublicationUnfinished
}

// start submits the transfer this execution needs, taking the id from the Job's own
// download handle when it has one so the panel and the Job Center name one row.
// It is refused with a *download_queue.URLActiveError while another transfer is
// fetching the same URL (see waitForTheURL).
func (a *downloadJobAdapter) start(execution jobs.Execution, input *downloadJobInput) (*download_queue.DownloadJob, error) {
	if input.Creator == nil {
		return nil, errors.New("the download Job names no submission")
	}
	job, err := a.ctx.JobService().Get(a.ctx.jobDeps(), jobs.Access{Administrator: true}, execution.JobID)
	if err != nil {
		return nil, err
	}
	legacyID, err := a.ctx.JobHandleNamespaceFor(execution.JobID, DownloadHandleNamespace)
	if err != nil {
		return nil, err
	}
	// The handle is the entry's id, always. After a retry the handle names the
	// successor, and the finished entry it used to name is replaced rather than
	// duplicated: one legacy id means one current execution, and the legacy history
	// row for that id goes on describing the attempt that is running.
	//
	// A Job accepted without one (a deferred download is accepted under its row's
	// id) is given one here, before the entry exists. The entry's id is what the
	// download surfaces show and look it up by, and the legacy history row is
	// written under it, so an id that named no Job was a row nobody could resolve
	// and a history write that failed.
	if legacyID == "" {
		legacyID = download_queue.NewJobID()
		if err := a.ctx.JobService().AddLegacyHandle(a.ctx.jobDeps(), execution.JobID,
			jobs.LegacyRef{Namespace: DownloadHandleNamespace, Handle: legacyID}); err != nil {
			return nil, err
		}
	}
	return a.ctx.downloadManager.SubmitForPluginWithOptions(input.Creator, job.OwnerUserID, input.Plugin,
		download_queue.SubmissionOptions{
			JobID:        legacyID,
			Canonical:    &download_queue.CanonicalRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken},
			ExclusiveURL: true,
		})
}

// refusalReason answers why this execution may not start, or an empty string.
//
// Everything rechecked here was checked when the submission arrived. None of it is
// a standing permission: the plugin may have been disabled, the principal's role or
// scope may have narrowed, and a retry or a dispatch after a restart runs on a
// worker with no request behind it at all.
func (a *downloadJobAdapter) refusalReason(execution jobs.Execution, input *downloadJobInput) string {
	if input.Plugin != "" && !a.ctx.scheduledDownloadPluginAvailable(input.Plugin, nil) {
		return "plugin-unavailable"
	}
	if execution.Access.UserID != 0 {
		scoped := a.ctx.WithPrincipal(a.ctx.principalForPluginActor(execution.Access.UserID))
		if err := scoped.requireWriteRole("run a download"); err != nil {
			return "role-refused"
		}
		if err := scoped.validateDownloadTargetsInScope(input.Creator); err != nil {
			return "scope-refused"
		}
	}
	return ""
}

// block records that this Job cannot proceed and who has to decide. It is the
// adapter's answer for a policy refusal, and its reason is a bounded code rather
// than the refusal's text: only the Kind knows what in its own error is safe, and
// this lands in a Job's timeline where a reader searches.
func (a *downloadJobAdapter) block(execution jobs.Execution, reason string) error {
	current, err := a.ctx.JobService().Get(a.ctx.jobDeps(), jobs.Access{Administrator: true}, execution.JobID)
	if err != nil {
		return err
	}
	if current.State.Terminal() {
		return nil
	}
	if current.State == jobs.StateBlocked {
		return nil
	}
	detail, err := json.Marshal(map[string]string{"reason": reason})
	if err != nil {
		return err
	}
	_, err = a.ctx.JobService().Transition(a.ctx.jobDeps(), jobs.Transition{
		JobID:           execution.JobID,
		ExpectedVersion: current.Version,
		ExecutionToken:  execution.ExecutionToken,
		To:              jobs.StateBlocked,
		Event:           jobs.EventInput{Type: jobs.EventBlocked, Detail: detail},
	})
	return err
}

// waitForTerminal blocks until the queue entry reaches a terminal status, the
// context is cancelled, or the entry disappears from this process's queue.
func (a *downloadJobAdapter) waitForTerminal(ctx context.Context, execution jobs.Execution, entry *download_queue.DownloadJob) (*download_queue.DownloadJob, error) {
	// One read before the loop: a transfer that finished while the Job was being
	// claimed needs no wait at all.
	if snap := entry.Snapshot(); downloadTerminal(snap.Status) {
		return snap, nil
	}
	ticker := time.NewTicker(jobDownloadPollInterval)
	defer ticker.Stop()
	var nextIntentCheck time.Time
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			snap := entry.Snapshot()
			if downloadTerminal(snap.Status) {
				return snap, nil
			}
			// A cancellation recorded against this Job by anybody — another runtime's
			// command endpoint, another process's compatibility route — is delivered
			// here, because this is the execution that owns the transfer.
			a.ctx.deliverCancelIntent(execution, entry, &nextIntentCheck)
		}
	}
}

// queuedForDispatch reports whether the Job this execution claimed was waiting work.
//
// A queued or scheduled Job reached this adapter through a claim, which is what a
// released hold produces; a Job claimed from running is a reconciliation handing the
// same execution back under a fresh token. The two need opposite answers about a
// paused queue entry, and the claim records which one this is.
func (a *downloadJobAdapter) queuedForDispatch(execution jobs.Execution) bool {
	switch execution.ClaimedFrom {
	case jobs.StateQueued, jobs.StateScheduled:
		return true
	default:
		return false
	}
}

func downloadTerminal(status download_queue.JobStatus) bool {
	switch status {
	case download_queue.JobStatusCompleted, download_queue.JobStatusFailed, download_queue.JobStatusCancelled:
		return true
	default:
		return false
	}
}

// publishOutcome records what the transfer reached, if nobody has recorded it yet.
//
// "Nobody yet" is the whole point: the queue mirrors its own terminal state through
// the sink while an execution is attached to it, and this is the path for the one
// case that mirror cannot cover — a transfer that finished before this process
// adopted the Job, so the mirror had nobody to publish under. Reading the Job first
// is what makes both writers safe: whoever gets there second finds it terminal and
// writes nothing.
func (a *downloadJobAdapter) publishOutcome(execution jobs.Execution, snap *download_queue.DownloadJob) error {
	service := a.ctx.JobService()
	deps := a.ctx.jobDeps()
	// The read goes through the shared completion seam, which is where a test can
	// refuse it: a read that fails is not a Job that finished, and the publication is
	// what is retried.
	current, err := a.ctx.jobCompletionRead(service, execution.JobID)
	if err != nil {
		return err
	}
	if current.State.Terminal() {
		return nil
	}
	ref := jobs.ExecutionRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken}

	switch snap.Status {
	case download_queue.JobStatusCompleted, download_queue.JobStatusProcessing:
		resourceID := uint(0)
		if snap.ResourceID != nil {
			resourceID = *snap.ResourceID
		}
		if resourceID == 0 {
			return a.finish(execution, jobs.StateFailed, "download-produced-nothing")
		}
		reference, err := json.Marshal(map[string]any{"resourceId": resourceID})
		if err != nil {
			return err
		}
		if _, err := service.PublishOutput(deps, ref, jobs.OutputInput{
			Key:       jobDownloadResourceOutput,
			Type:      jobs.OutputTypeEntity,
			Label:     "Created resource",
			Reference: reference,
			Required:  true,
		}); err != nil {
			// The Resource exists and the reference to it did not land. That is a
			// publication the owner retries, not a transfer that produced nothing, and
			// ending the Job here lost the successful download.
			return err
		}
		return a.finish(execution, jobs.StateSucceeded, "")
	case download_queue.JobStatusCancelled:
		return a.finish(execution, jobs.StateCancelled, "")
	default:
		if snap.ExistingResourceID != nil && *snap.ExistingResourceID != 0 {
			return a.finishExisting(execution, *snap.ExistingResourceID, snap.FailureReason)
		}
		code := snap.FailureCode
		if code == "" {
			code = download_queue.FailureDownloadFailed
		}
		return a.finishFailed(execution, code, snap.FailureReason)
	}
}

// finishExisting ends a download the library refused because it already holds the
// bytes. That is a conflict, not an internal fault, and the useful answer is the
// resource holding them: it is published as an optional entity output, whose link
// re-checks on every open that the viewer may see that resource.
//
// A publication that fails is returned rather than ended past, as the success
// path's is: the outcome publication is retried, and ending the Job first would
// leave the link unpublishable, since a terminal Job takes no more outputs.
func (a *downloadJobAdapter) finishExisting(execution jobs.Execution, resourceID uint, reason string) error {
	reference, err := json.Marshal(map[string]any{"resourceId": resourceID})
	if err != nil {
		return err
	}
	if err := a.ctx.jobFaults.outputPublication(); err != nil {
		return err
	}
	ref := jobs.ExecutionRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken}
	if _, err := a.ctx.JobService().PublishOutput(a.ctx.jobDeps(), ref, jobs.OutputInput{
		Key:       JobDownloadExistingResourceOutput,
		Type:      jobs.OutputTypeEntity,
		Label:     "Existing resource",
		Reference: reference,
	}); err != nil {
		return err
	}
	failure := &jobs.Failure{
		Code:    JobDownloadResourceExistsCode,
		Class:   jobs.FailureClassConflict,
		Message: downloadFailureMessage(reason),
	}
	return a.ctx.finishQueueJob(execution, jobs.StateFailed, failure, []string{jobDownloadResourceOutput})
}

// jobDownloadResourceOutput is the output key a succeeded download publishes. §7
// makes a success depend on it: a download that produced no Resource did not
// succeed, whatever the queue's own status says.
const jobDownloadResourceOutput = "resource"

// JobDownloadExistingResourceOutput is the output a download that collided with
// content the library already holds publishes: the resource holding those bytes.
// It is never required and never a result. A Job whose claim expired after this
// was published is replayed with the output still on it, since an output cannot
// be withdrawn, so it may sit beside jobDownloadResourceOutput or under a later,
// different failure: readers show it only while the Job's failure is
// JobDownloadResourceExistsCode, and never offer it as what the Job made
// (server/jobview ResultLinkFor, and FAILURE_OUTPUT_KEY in
// src/components/jobCenter.js, which change with these two).
const JobDownloadExistingResourceOutput = "existing-resource"

// JobDownloadResourceExistsCode is the failure code of that collision.
const JobDownloadResourceExistsCode = download_queue.FailureResourceExists

// AuthorizeJobOutput hides the collision link from a Job that no longer reports
// the collision. Listing and opening both ask this, so a reconciled replay that
// failed differently or succeeded offers the stale link nowhere: not on the Job
// page's Outputs list, not as a result, and not from a bookmarked output URL.
// Every other output keeps the standard policy.
func (a *downloadJobAdapter) AuthorizeJobOutput(_ context.Context, request JobOutputOpenRequest) error {
	if request.Output.Key != JobDownloadExistingResourceOutput {
		return nil
	}
	snap := request.Snapshot
	if snap.State == jobs.StateFailed && snap.Failure != nil && snap.Failure.Code == JobDownloadResourceExistsCode {
		return nil
	}
	return ErrJobOutputForbidden
}

// finish ends the Job with a bounded classification, through the same completion path
// every queue-backed Kind uses: the read, the versioned retry and the "somebody else
// already ended it" tolerance are one implementation rather than one per Kind.
func (a *downloadJobAdapter) finish(execution jobs.Execution, outcome jobs.State, code string) error {
	if outcome == jobs.StateFailed {
		return a.finishFailed(execution, code, "")
	}
	return a.ctx.finishQueueJob(execution, outcome, nil, []string{jobDownloadResourceOutput})
}

// finishFailed ends the Job as failed, saying why.
//
// The reason is the queue's FailureReason, never its Error: Error is the error's
// own text, which can name the URL the transfer failed on, query and all, and a
// Job's failure message is stored in the clear and searched. FailureReason is the
// same reason with every URL cut to its origin, rendered by the queue while it
// still held the error value. The code is the queue's too, and the class follows
// from it (downloadFailureKinds).
func (a *downloadJobAdapter) finishFailed(execution jobs.Execution, code, reason string) error {
	failure := &jobs.Failure{
		Code:    code,
		Class:   downloadFailureKindOf(code).class,
		Message: downloadFailureMessage(reason),
	}
	return a.ctx.finishQueueJob(execution, jobs.StateFailed, failure, []string{jobDownloadResourceOutput})
}

// downloadFailureKind is what one failure code means to a download's Job: the
// class the failure breakdown groups it under, and whether asking again with the
// same input could answer differently.
type downloadFailureKind struct {
	class string
	// alike is true when the stored input can never be fetched by itself, so a
	// Retry, which replays it, would be refused the same way: an address that is
	// not an http or https URL. Nothing else qualifies. A remote's answer can
	// change (a 404 becomes a 200 once something is published, a live stream
	// ends), the library can change (the resource already holding the bytes can
	// be deleted), and so can this deployment's policy and limits; the Retry that
	// follows any of those is how the same download is asked for again.
	alike bool
}

// downloadFailureKinds classes every code the queue records. A code missing here
// is classed internal and keeps Retry, which is the answer for a failure nobody
// has explained yet.
var downloadFailureKinds = map[string]downloadFailureKind{
	download_queue.FailureInvalidURL:        {class: jobs.FailureClassValidation, alike: true},
	download_queue.FailureRemoteClientError: {class: jobs.FailureClassDependency},
	download_queue.FailureRemoteForbidden:   {class: jobs.FailureClassDependency},
	download_queue.FailureRemoteBusy:        {class: jobs.FailureClassDependency},
	download_queue.FailureRemoteServerError: {class: jobs.FailureClassDependency},
	download_queue.FailureRemoteConnection:  {class: jobs.FailureClassDependency},
	download_queue.FailureRemoteTimeout:     {class: jobs.FailureClassTimeout},
	download_queue.FailureIdleTimeout:       {class: jobs.FailureClassTimeout},
	download_queue.FailureOverallTimeout:    {class: jobs.FailureClassTimeout},
	download_queue.FailureAddressRefused:    {class: jobs.FailureClassPolicy},
	download_queue.FailurePluginUnavailable: {class: jobs.FailureClassPolicy},
	download_queue.FailureSubmitterRefused:  {class: jobs.FailureClassPolicy},
	download_queue.FailureUnsupportedStream: {class: jobs.FailureClassValidation},
	download_queue.FailureStreamOverLimit:   {class: jobs.FailureClassPolicy},
	download_queue.FailureFfmpegUnavailable: {class: jobs.FailureClassDependency},
	download_queue.FailureResourceExists:    {class: jobs.FailureClassConflict},
	download_queue.FailureDownloadFailed:    {class: jobs.FailureClassInternal},
}

func downloadFailureKindOf(code string) downloadFailureKind {
	if kind, ok := downloadFailureKinds[code]; ok {
		return kind
	}
	return downloadFailureKind{class: jobs.FailureClassInternal}
}

// downloadRetryWouldFailAlike reports whether a failed download's Retry would be
// refused the same way (downloadFailureKind.alike).
func downloadRetryWouldFailAlike(failure *jobs.Failure) bool {
	return failure != nil && downloadFailureKindOf(failure.Code).alike
}

// downloadFailureCodesThatRepeat lists every code whose Retry would fail alike, in
// a stable order, for the command selector's predicate.
func downloadFailureCodesThatRepeat() []string {
	codes := make([]string, 0, len(downloadFailureKinds))
	for code, kind := range downloadFailureKinds {
		if kind.alike {
			codes = append(codes, code)
		}
	}
	sort.Strings(codes)
	return codes
}

// downloadFailureFallback is the message of a failure the queue gave no reason for.
const downloadFailureFallback = "the download did not complete"

// downloadFailureMessage makes a rendered reason storable: valid text with no NUL,
// which PostgreSQL refuses in a text column, and within the Service's ceiling,
// cut on a rune boundary. A Finish the Service refused would leave the Job
// running with nothing left to end it.
func downloadFailureMessage(reason string) string {
	message := strings.ToValidUTF8(reason, "")
	message = strings.ReplaceAll(message, "\x00", "")
	message = strings.TrimSpace(message)
	if message == "" {
		return downloadFailureFallback
	}
	if len(message) > jobs.MaxFailureMessageBytes {
		cut := jobs.MaxFailureMessageBytes - len("…")
		for cut > 0 && !utf8.RuneStart(message[cut]) {
			cut--
		}
		message = message[:cut] + "…"
	}
	return message
}

// Reconcile answers what should happen to one download whose claim expired.
//
// This queue is memory, so a transfer it no longer holds is not running *here*.
// Three answers follow, and none of them guesses that external work stopped: an
// entry that is still active keeps the Job running under a fresh token (the runtime
// re-attaches and continues waiting), an entry that reached a terminal status is
// handed back so the fresh dispatch can publish the outcome the mirror missed, and
// no entry at all means the work has to start again from the payload.
func (a *downloadJobAdapter) Reconcile(_ context.Context, request jobs.ReconcileRequest) (jobs.ReconcileDecision, error) {
	if a.ctx == nil || a.ctx.downloadManager == nil {
		return jobs.ReconcileExternalWorkUnproven, nil
	}
	entry, found := a.ctx.downloadManager.GetJobByCanonicalJobID(request.Snapshot.ID)
	if !found {
		// A committed receipt is durable evidence that the transfer ended even if
		// its source URL has since expired. Require positive runtime quiescence
		// before publishing that side effect under the expired execution token.
		if !runtimeIsProvedGone(request) {
			return jobs.ReconcileExternalWorkUnproven, nil
		}
		var receipt models.JobResourceReceipt
		err := a.ctx.db.Where("job_id = ?", request.Snapshot.ID).First(&receipt).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return "", err
		}
		if err == nil {
			if !sameOptionalUint(receipt.ActorUserID, request.Snapshot.ActorUserID) {
				return "", fmt.Errorf("download receipt actor does not match Job %s", request.Snapshot.ID)
			}
			var resource models.Resource
			if err := a.ctx.db.First(&resource, receipt.ResourceID).Error; err != nil {
				return "", fmt.Errorf("load resource for download receipt: %w", err)
			}
			reference, err := json.Marshal(map[string]any{"resourceId": resource.ID})
			if err != nil {
				return "", err
			}
			if _, err := request.Execution.Output(jobs.OutputInput{
				Key: jobDownloadResourceOutput, Type: jobs.OutputTypeEntity,
				Label: "Created resource", Reference: reference, Required: true,
			}); err != nil {
				return "", err
			}
			return jobs.ReconcileSucceed, nil
		}
		return jobs.ReconcileQueue, nil
	}
	if downloadTerminal(entry.GetStatus()) {
		return jobs.ReconcileQueue, nil
	}
	return jobs.ReconcileResume, nil
}

// CleanupArtifacts accounts for one expired Job's artifact outputs.
//
// A download publishes none: its output is the Resource it created, which is a
// domain row the library owns and never a file this Kind staged. An empty answer is
// therefore a complete one, and claiming to have removed a key this Kind never
// wrote would be the opposite of the accounting §9 asks for.
func (a *downloadJobAdapter) CleanupArtifacts(_ context.Context, _ jobs.ArtifactCleanupRequest) (jobs.ArtifactCleanupResult, error) {
	return jobs.ArtifactCleanupResult{}, nil
}

// Commands reports the controls one download offers right now.
//
// Retry and cancel are advertised from the Kind — whether its work may be re-run at
// all is its own policy — and the host then narrows both: a Retry only on the
// unsuccessful leaf of a lineage, a cancel never on finished work. The Kind's own
// policy leaves Retry out for a failure a Retry would repeat
// (downloadRetryWouldFailAlike): a stored address that is not a download. Pause is
// deliberately absent (see the file comment); resume is offered for held work,
// which is the state a pause leaves this Kind in.
func (a *downloadJobAdapter) Commands(_ context.Context, commandContext jobs.CommandContext) ([]jobs.Command, error) {
	// §8: ownership grants visibility, not permanent control. A principal demoted below
	// "may write", or one whose access to the plugin this download belongs to has been
	// revoked, keeps the sanitized history and loses every control over it.
	if a.ctx.commandActorRefusal(commandContext.Deps, commandContext.Access,
		jobCommandSummaryPlugin(commandContext.Snapshot.Summary)) != "" {
		return nil, nil
	}
	state := commandContext.Snapshot.State
	commands := make([]jobs.Command, 0, 3)

	commands = append(commands, jobs.Command{
		Key:          jobs.CommandCancel,
		Label:        "Cancel",
		Destructive:  true,
		Confirmation: "Stop this download? A file already saved stays in the library.",
	})
	if state == jobs.StateBlocked || state == jobs.StatePaused {
		commands = append(commands, jobs.Command{
			Key:   jobs.CommandResume,
			Label: "Resume",
		})
	}
	if (state == jobs.StateFailed || state == jobs.StateCancelled || state == jobs.StateInterrupted) &&
		!downloadRetryWouldFailAlike(commandContext.Snapshot.Failure) {
		commands = append(commands, downloadRetryCommand(commandContext.Snapshot))
	}
	return commands, nil
}

// downloadRetryCommand is the Retry one download offers. A Retry's successor is
// an ordinary download that starts now. For a deferred download that never
// started, that drops the time it was scheduled for, so the control says so
// before it is used instead of reading "Retry"; the plugin's deferred row stays
// cancelled and does not follow the successor.
func downloadRetryCommand(snapshot jobs.Snapshot) jobs.Command {
	if snapshot.ScheduledFor != nil && snapshot.StartedAt == nil {
		return jobs.Command{
			Key:          jobs.CommandRetry,
			Label:        "Download now",
			Confirmation: "This download was scheduled for later and never started. Download now starts it immediately; it does not wait for the scheduled time.",
		}
	}
	return jobs.Command{Key: jobs.CommandRetry, Label: "Retry"}
}

// ExecuteCommand runs one control the host decided this Kind owns.
//
// Only cancel and resume reach here: retry is the control plane's own lineage work,
// and pause is never advertised.
func (a *downloadJobAdapter) ExecuteCommand(_ context.Context, execution jobs.CommandExecution) (jobs.CommandOutcome, error) {
	if a.ctx == nil || a.ctx.downloadManager == nil {
		return jobs.CommandOutcome{}, errors.New("the download queue is not available")
	}
	entry, found := a.ctx.downloadManager.GetJobByCanonicalJobID(execution.JobID)

	switch execution.Key {
	case jobs.CommandCancel:
		if !found {
			// Nothing in this process's queue is running it, so there is nothing to
			// stop: the host has already ended an unowned Job, and a Job whose
			// transfer is gone has nothing left to cancel.
			return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded, Message: "the transfer is no longer running"}, nil
		}
		if err := a.ctx.downloadManager.Cancel(entry.ID); err != nil {
			var conflict *download_queue.StateConflictError
			if errors.As(err, &conflict) {
				return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded, Message: "the transfer had already finished"}, nil
			}
			return jobs.CommandOutcome{}, err
		}
		return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded, Message: "cancelling"}, nil

	case jobs.CommandResume:
		// A hold is released by queueing the Job, never by starting a worker from
		// here. The Job's claim and the capacity that admitted it were handed back
		// when it was held, so a worker started by this command would run unowned
		// and unbudgeted — and another runtime could claim the queued Job in the
		// same instant, giving one transfer two executors. The executor starts
		// inside a fresh claim, under that claim's own token and against the
		// deployment's budget: see Dispatch, which is where the paused queue entry
		// is resumed.
		return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded, Message: "queued to start again"}, nil
	}
	return jobs.CommandOutcome{}, fmt.Errorf("%w: %s", jobs.ErrCommandNotAdvertised, execution.Key)
}

// jobDownloadSink mirrors the queue's own lifecycle into the durable Jobs.
//
// It is the queue's seam (download_queue.CanonicalSink) and lives here because this
// is the layer that owns the Job tables. Every write carries the execution token
// the transfer was adopted with, so a mirror from a claim that has since been
// replaced is refused rather than published — and every refusal is the fence doing
// its job, which is why they are not logged as failures.
type jobDownloadSink struct {
	ctx *MahresourcesContext
}

func (s *jobDownloadSink) DownloadProgress(ref download_queue.CanonicalRef, snap *download_queue.DownloadJob) error {
	service := s.service()
	if service == nil || snap == nil {
		return nil
	}
	if _, err := service.UpdateProgress(s.ctx.jobDeps(), executionRefOf(ref), downloadJobProgress(snap)); err != nil {
		return s.mirrorRefusal(err)
	}
	return nil
}

func (s *jobDownloadSink) DownloadHeld(ref download_queue.CanonicalRef, snap *download_queue.DownloadJob) error {
	service := s.service()
	if service == nil || snap == nil {
		return nil
	}
	current, err := service.Get(s.ctx.jobDeps(), jobs.Access{Administrator: true}, ref.JobID)
	if err != nil {
		return s.mirrorRefusal(err)
	}
	if current.State.Terminal() || current.State == jobs.StateBlocked {
		return nil
	}
	detail, err := json.Marshal(map[string]string{"reason": "paused", "resume": "restarts-from-the-beginning"})
	if err != nil {
		return err
	}
	_, err = service.Transition(s.ctx.jobDeps(), jobs.Transition{
		JobID:           ref.JobID,
		ExpectedVersion: current.Version,
		ExecutionToken:  ref.ExecutionToken,
		To:              jobs.StateBlocked,
		Phase:           "paused",
		Event:           jobs.EventInput{Type: jobs.EventBlocked, Detail: detail},
	})
	return s.mirrorRefusal(err)
}

func (s *jobDownloadSink) DownloadFinished(ref download_queue.CanonicalRef, snap *download_queue.DownloadJob) error {
	service := s.service()
	if service == nil || snap == nil {
		return nil
	}
	adapter := &downloadJobAdapter{ctx: s.ctx, kind: JobKindRemoteDownload}
	// The queue's snapshot carries no execution identity of its own, so the mirror
	// publishes through the adapter's own outcome path with the ref the queue
	// reported: identity and token, exactly the two things it needs.
	execution := jobs.Execution{
		JobID:          ref.JobID,
		KindVersion:    jobDownloadKindVersion,
		ExecutionToken: ref.ExecutionToken,
	}
	if snap.CanonicalJobID != "" {
		execution.JobID = snap.CanonicalJobID
	}
	return s.mirrorRefusal(adapter.publishOutcome(execution, snap))
}

// DownloadInterrupted hands a transfer the deployment's shutdown stopped back to
// the queue its Job came from.
//
// A graceful stop is not an outcome of the work, so the Job is neither cancelled
// nor failed. It goes back to the queue under the execution's own token, which also
// hands back the claim and the capacity it held, and its row and its event say why
// (requeueDownloadExecution). Another process's runtime claims it at once; with none
// running, the next one to start does, and starts the transfer again from the
// sealed input. That is also where a crash ends up once the claim's lease expires;
// this only saves the wait. A write that is refused here leaves the Job running
// with its lease, and that reconciliation is what settles it instead.
func (s *jobDownloadSink) DownloadInterrupted(ref download_queue.CanonicalRef, snap *download_queue.DownloadJob) error {
	if s.service() == nil {
		return nil
	}
	return s.ctx.requeueDownloadExecution(jobs.Execution{JobID: ref.JobID, ExecutionToken: ref.ExecutionToken},
		JobDownloadServerShutdownReason, downloadPhaseQueued,
		"Stopped by a server shutdown; it starts again from the beginning")
}

// downloadPhaseQueued is the phase of a download's Job waiting for a runtime.
const downloadPhaseQueued = "queued"

// JobDownloadServerShutdownReason is the reason a download's Job records when the
// deployment's shutdown returned it to the queue.
const JobDownloadServerShutdownReason = "server-shutdown"

// executionRefOf is the one translation from the queue's own reference to the
// control plane's: same two facts, and the queue does not import the Job module's
// types for them.
func executionRefOf(ref download_queue.CanonicalRef) jobs.ExecutionRef {
	return jobs.ExecutionRef{JobID: ref.JobID, ExecutionToken: ref.ExecutionToken}
}

// service is the installed control plane, or nil when this deployment has none —
// the CLI's queue, the package's own tests, a programmatic embed.
func (s *jobDownloadSink) service() *jobs.Service {
	if s == nil || s.ctx == nil {
		return nil
	}
	return s.ctx.JobService()
}

// mirrorRefusal classifies a mirror's error. A fenced-out or already-finished
// publish is the fence working, not a failure: the execution that lost its claim
// may not publish, and a Job that already ended may not be written to again. Both
// are silent. Anything else is logged — a mirror that cannot be written is worth
// knowing about, and it must never change what the download does.
func (s *jobDownloadSink) mirrorRefusal(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, jobs.ErrStaleExecution),
		errors.Is(err, jobs.ErrVersionConflict),
		errors.Is(err, jobs.ErrIllegalTransition),
		errors.Is(err, jobs.ErrNotFound):
		return nil
	default:
		log.Printf("warning: mirroring a download into the job control plane failed: %v", err)
		return nil
	}
}

// downloadPhase is the canonical phase label for one queue status. The queue's own
// statuses are finer than a normalized state and never redefine one.
// downloadJobProgress is the durable progress one transfer snapshot describes.
//
// Bytes are the primary measure whenever the size is known, and also when it is
// not but bytes are arriving — a chunked response still has a speed worth
// showing. An HLS stream's size is unknown until its last segment lands, so its
// segment counter is the measure there: it has a total, which is what gives the
// bar a percentage and the Job an ETA. Whichever measure is not primary is kept
// as a metric rather than dropped.
func downloadJobProgress(snap *download_queue.DownloadJob) jobs.Progress {
	progress := jobs.Progress{Phase: downloadPhase(snap), Message: snap.Phase}
	bytesMetric := func() {
		if snap.Progress > 0 {
			progress.Metrics = append(progress.Metrics, jobs.Metric{
				Key: "downloaded", Label: "Downloaded", Value: float64(snap.Progress), Unit: "bytes",
			})
		}
	}
	segmentsMetric := func() {
		if snap.PhaseTotal > 0 {
			total := float64(snap.PhaseTotal)
			progress.Metrics = append(progress.Metrics, jobs.Metric{
				Key: "segments", Label: "Segments", Value: float64(snap.PhaseCount), Total: &total, Unit: "items",
			})
		}
	}
	switch {
	case snap.TotalSize > 0:
		completed, total := snap.Progress, snap.TotalSize
		progress.Completed, progress.Total, progress.Unit = &completed, &total, "bytes"
		segmentsMetric()
	case snap.PhaseTotal > 0:
		completed, total := snap.PhaseCount, snap.PhaseTotal
		progress.Completed, progress.Total, progress.Unit = &completed, &total, "items"
		bytesMetric()
	case snap.Progress > 0:
		completed := snap.Progress
		progress.Completed, progress.Unit = &completed, "bytes"
	}
	return progress
}

func downloadPhase(snap *download_queue.DownloadJob) string {
	switch snap.Status {
	case download_queue.JobStatusPending:
		return "queued"
	case download_queue.JobStatusProcessing:
		return "saving"
	case download_queue.JobStatusPaused:
		return "paused"
	default:
		return string(snap.Status)
	}
}

// registerDownloadJobKinds teaches one control plane to run this context's
// download Kinds, and how their input is summarized and sealed.
//
// It is idempotent because a context may install a service it was handed: the
// second registration of one (Kind, version) is refused by the control plane, so
// the caller asks first — the refusal exists to stop two executors for one Kind,
// not to make wiring order-dependent.
func (ctx *MahresourcesContext) registerDownloadJobKinds(service *jobs.Service) error {
	if ctx == nil || service == nil {
		return nil
	}
	for _, kind := range []string{JobKindRemoteDownload, JobKindDeferredDownload} {
		if !jobs.HasReplayCodec(service, kind, jobDownloadKindVersion) {
			if err := service.RegisterReplayCodec(kind, jobDownloadKindVersion, downloadJobCodec()); err != nil {
				return err
			}
		}
		if _, registered := service.AdapterFor(kind, jobDownloadKindVersion); registered {
			continue
		}
		if err := service.RegisterAdapter(&downloadJobAdapter{ctx: ctx, kind: kind, waits: newDownloadURLWaits()}); err != nil {
			return err
		}
	}
	return nil
}

// SubmitRemoteDownloads is the one door a remote download is submitted through.
//
// The order is the design's and it is not negotiable: the durable Job is accepted
// first, and only then is the transfer dispatched. An accepted Job that existed
// only in memory would disappear with the process, and a dispatch that happened
// first would leave a transfer running that nothing durable had agreed to.
//
// The legacy queue entry and the Job are then one thing seen two ways: the entry
// takes the id the Job's own handle records, and the entry names the Job it
// publishes into.
func (ctx *MahresourcesContext) SubmitRemoteDownloads(creator *query_models.ResourceFromRemoteCreator, ownerUserID *uint, pluginName, origin string) []download_queue.RemoteDownloadSubmission {
	if creator == nil {
		return nil
	}
	submissions := make([]download_queue.RemoteDownloadSubmission, 0, 1)
	for _, raw := range strings.Split(creator.URL, "\n") {
		url := strings.TrimSpace(raw)
		if url == "" {
			continue
		}
		// Refused here, before a Job is accepted, and per line: the lines around it
		// are still submitted, and the caller is told which one was not a download.
		if err := download_queue.ValidateDownloadURL(url); err != nil {
			submissions = append(submissions, download_queue.RemoteDownloadSubmission{URL: url, Err: err})
			continue
		}
		single := *creator
		single.URL = url
		single.Headers = hostfetch.CopyHeaders(creator.Headers)
		submissions = append(submissions, ctx.submitRemoteDownload(&single, ownerUserID, pluginName, origin))
	}
	return submissions
}

// submitRemoteDownload admits and dispatches one URL.
//
// The order is the design's and it is not negotiable: the durable Job is admitted —
// and, when the deployment has room, claimed — before anything runs, and only then is
// the transfer dispatched. An accepted Job that existed only in memory would disappear
// with the process; a dispatch that happened first would leave a transfer running that
// nothing durable had agreed to; and a Job left unclaimed for as long as it takes to
// enqueue the transfer is one another process's runtime starts a second transfer for.
//
// The legacy queue entry and the Job are then one thing seen two ways: the entry takes
// the id the Job's own handle records, and the entry names the Job it publishes into
// with the very token that owns it.
func (ctx *MahresourcesContext) submitRemoteDownload(creator *query_models.ResourceFromRemoteCreator, ownerUserID *uint, pluginName, origin string) download_queue.RemoteDownloadSubmission {
	result := download_queue.RemoteDownloadSubmission{URL: creator.URL}
	if ctx == nil || ctx.downloadManager == nil {
		result.Err = errors.New("the download queue is not available")
		return result
	}
	service := ctx.JobService()
	if service == nil {
		// No control plane in this process: the queue runs as it always has, and the
		// entry names no Job because there is none to name.
		job, err := ctx.downloadManager.SubmitForPlugin(creator, ownerUserID, pluginName)
		result.Job, result.Err = job, err
		if job != nil {
			result.Row = job.Snapshot()
		}
		return result
	}

	input, err := remoteDownloadInputJSON(creator, pluginName)
	if err != nil {
		result.Err = err
		return result
	}
	legacyID := download_queue.NewJobID()
	admission, err := ctx.admitQueueJob(jobs.Acceptance{
		Kind:        JobKindRemoteDownload,
		KindVersion: jobDownloadKindVersion,
		State:       jobs.StateQueued,
		OwnerUserID: ownerUserID,
		ActorUserID: ownerUserID,
		Origin:      origin,
		Title:       downloadJobTitle(input),
		Replay:      jobs.ReplayInput{Input: input},
		LegacyRefs:  []jobs.LegacyRef{{Namespace: DownloadHandleNamespace, Handle: legacyID}},
	})
	if err != nil {
		result.Err = err
		return result
	}
	result.CanonicalJobID = admission.Accepted.ID
	if !admission.Owned() {
		// The deployment's budget is full: the Job is durable, answers the id the client
		// was handed, and starts no executor here. A runtime with a free slot takes it,
		// from the sealed payload — see admitQueueJob. The reported row is the projection
		// of that Job, which is what a client polling the id would get from the
		// compatibility route anyway.
		result.Row = downloadRowFromJob(admission.Accepted, legacyID, download_queue.JobSourceDownload)
		return result
	}

	job, err := ctx.downloadManager.SubmitForPluginWithOptions(creator, ownerUserID, pluginName,
		download_queue.SubmissionOptions{
			JobID: legacyID,
			Canonical: &download_queue.CanonicalRef{
				JobID:          admission.Execution.JobID,
				ExecutionToken: admission.Execution.ExecutionToken,
			},
			ExclusiveURL: true,
		})
	var busy *download_queue.URLActiveError
	if errors.As(err, &busy) {
		// The URL is downloading here already. The Job waits for that transfer in the
		// queue, exactly as a dispatch that found the URL busy would, so one URL is
		// fetched once at a time whether or not the deployment had room to start this
		// submission at once.
		_ = ctx.downloadAdapterFor(JobKindRemoteDownload).waitForTheURL(admission.Execution, creator.URL)
		projected := admission.Accepted
		if current, getErr := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, admission.Accepted.ID); getErr == nil {
			projected = current
		}
		result.Row = downloadRowFromJob(projected, legacyID, download_queue.JobSourceDownload)
		return result
	}
	if err != nil {
		// The Job was admitted and the queue refused the transfer. It is ended here
		// rather than left running: a Job nothing will ever dispatch would sit in the
		// Job Center claiming work that was never admitted, holding the capacity that
		// admitted it, and the refusal is what its outcome should say.
		ctx.failUndispatchedQueueJob(admission, err)
		result.Err = err
		return result
	}
	result.Job = job
	result.Row = downloadRowFromEntry(job, legacyID, admission.Accepted.ID)
	ctx.ownQueueExecution(admission, job, func(snap *download_queue.DownloadJob) error {
		return (&downloadJobAdapter{ctx: ctx, kind: JobKindRemoteDownload}).publishOutcome(admission.Execution, snap)
	})
	return result
}

// ApplyHostTransition keeps a deferred download's row in step when the host
// cancels its Job while nothing runs it. The row is the plugin's record of the
// same deferral: left pending, the scheduler would reach it at the due time and
// record a submission of a download that was cancelled. It runs in the command's
// own transaction, so the Job and its row cannot disagree.
func (a *downloadJobAdapter) ApplyHostTransition(_ context.Context, deps jobs.Deps, snapshot jobs.Snapshot, key string, to jobs.State) error {
	if a.kind != JobKindDeferredDownload || key != jobs.CommandCancel || to != jobs.StateCancelled {
		return nil
	}
	return cancelDeferredDownloadRowTx(deps.DB, snapshot, time.Now())
}
