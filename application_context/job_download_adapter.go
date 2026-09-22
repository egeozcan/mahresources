package application_context

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"mahresources/download_queue"
	"mahresources/hostfetch"
	"mahresources/jobs"
	"mahresources/models/query_models"
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
// text, and a URL can carry a token.
func downloadJobTitle(input json.RawMessage) string {
	summary, err := downloadSummaryOf(input)
	if err != nil {
		return "Download"
	}
	if summary.Name != "" {
		return summary.Name
	}
	if summary.Host != "" {
		return "Download from " + summary.Host
	}
	return "Download"
}

// downloadJobAdapter runs one of the download Kinds.
type downloadJobAdapter struct {
	ctx  *MahresourcesContext
	kind string
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
		entry, err = a.start(execution, &decoded)
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

	// A held transfer is not resumed here. It is waiting for a person, and §4 makes
	// the executor's own confirmation the thing that ends a hold: adopting it would
	// restart a download its owner deliberately stopped.
	if entry.GetStatus() == download_queue.JobStatusPaused {
		return a.block(execution, "paused")
	}

	snap, err := a.waitForTerminal(ctx, entry)
	if err != nil {
		return err
	}
	return a.publishOutcome(execution, snap)
}

// start submits the transfer this execution needs, taking the id from the Job's own
// download handle when it has one so the panel and the Job Center name one row.
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
	if legacyID == "" {
		legacyID = download_queue.NewJobID()
	}
	return a.ctx.downloadManager.SubmitForPluginWithOptions(input.Creator, job.OwnerUserID, input.Plugin,
		download_queue.SubmissionOptions{
			JobID:     legacyID,
			Canonical: &download_queue.CanonicalRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken},
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
	if input.Creator != nil {
		if live, running := download_queue.ActiveDownloadForURL(a.ctx.downloadManager, input.Creator.URL); running && live != "" {
			if entry, ok := a.ctx.downloadManager.GetJob(live); !ok || entry.CanonicalJobID != execution.JobID {
				return "url-already-downloading"
			}
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
func (a *downloadJobAdapter) waitForTerminal(ctx context.Context, entry *download_queue.DownloadJob) (*download_queue.DownloadJob, error) {
	// One read before the loop: a transfer that finished while the Job was being
	// claimed needs no wait at all.
	if snap := entry.Snapshot(); downloadTerminal(snap.Status) {
		return snap, nil
	}
	ticker := time.NewTicker(jobDownloadPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			snap := entry.Snapshot()
			if downloadTerminal(snap.Status) {
				return snap, nil
			}
		}
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
	current, err := service.Get(deps, jobs.Access{Administrator: true}, execution.JobID)
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
			return a.finish(execution, jobs.StateFailed, "download-output-unavailable")
		}
		return a.finish(execution, jobs.StateSucceeded, "")
	case download_queue.JobStatusCancelled:
		return a.finish(execution, jobs.StateCancelled, "")
	default:
		return a.finish(execution, jobs.StateFailed, "download-failed")
	}
}

// jobDownloadResourceOutput is the output key a succeeded download publishes. §7
// makes a success depend on it: a download that produced no Resource did not
// succeed, whatever the queue's own status says.
const jobDownloadResourceOutput = "resource"

// finish ends the Job with a bounded classification.
//
// The queue's own error text is deliberately not carried. The legacy surfaces show
// it (that is where a person debugs one transfer), but a Job's failure message is
// searchable text, and the queue's errors can name the URL including its query.
func (a *downloadJobAdapter) finish(execution jobs.Execution, outcome jobs.State, code string) error {
	current, err := a.ctx.JobService().Get(a.ctx.jobDeps(), jobs.Access{Administrator: true}, execution.JobID)
	if err != nil {
		return err
	}
	if current.State.Terminal() {
		return nil
	}
	request := jobs.FinishRequest{
		ExecutionRef:    jobs.ExecutionRef{JobID: execution.JobID, ExecutionToken: execution.ExecutionToken},
		ExpectedVersion: current.Version,
		Outcome:         outcome,
		RequiredOutputs: []string{jobDownloadResourceOutput},
	}
	if outcome == jobs.StateFailed {
		request.Failure = &jobs.Failure{
			Code:    code,
			Class:   jobs.FailureClassInternal,
			Message: "the download did not complete",
		}
	}
	_, err = a.ctx.JobService().Finish(a.ctx.jobDeps(), request)
	if err != nil && errors.Is(err, jobs.ErrStaleExecution) {
		// Somebody else's publish won: the Job is not this execution's to end.
		return nil
	}
	return err
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
		// Nothing here is running this transfer, and whether another process is is
		// the question the claim's own identity answers.
		return a.ctx.queueOnlyIfTheRuntimeIsProvedGone(request), nil
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
// unsuccessful leaf of a lineage, a cancel never on finished work. Pause is
// deliberately absent (see the file comment); resume is offered for held work,
// which is the state a pause leaves this Kind in.
func (a *downloadJobAdapter) Commands(_ context.Context, commandContext jobs.CommandContext) ([]jobs.Command, error) {
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
	if state == jobs.StateFailed || state == jobs.StateCancelled || state == jobs.StateInterrupted {
		commands = append(commands, jobs.Command{
			Key:   jobs.CommandRetry,
			Label: "Retry",
		})
	}
	return commands, nil
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
		if !found {
			// The held transfer is gone from the queue — this process restarted, or
			// the queue evicted it. The Job still holds everything a transfer needs,
			// so resuming it means starting again, and the host returning it to the
			// queue is what does that.
			return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded, Message: "the held transfer was gone; the download will start again"}, nil
		}
		if err := a.ctx.downloadManager.Resume(entry.ID); err != nil {
			var conflict *download_queue.StateConflictError
			if errors.As(err, &conflict) {
				return jobs.CommandOutcome{}, fmt.Errorf("the transfer is %s, not paused", conflict.Status)
			}
			if !found {
				return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded, Message: "the download will start again"}, nil
			}
			return jobs.CommandOutcome{}, err
		}
		return jobs.CommandOutcome{Status: jobs.CommandStatusSucceeded, Message: "resuming"}, nil
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
	progress := jobs.Progress{
		Phase:   downloadPhase(snap),
		Message: snap.Phase,
	}
	if snap.TotalSize > 0 {
		completed, total := snap.Progress, snap.TotalSize
		progress.Completed, progress.Total, progress.Unit = &completed, &total, "bytes"
	}
	if _, err := service.UpdateProgress(s.ctx.jobDeps(), executionRefOf(ref), progress); err != nil {
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
		if err := service.RegisterAdapter(&downloadJobAdapter{ctx: ctx, kind: kind}); err != nil {
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
		single := *creator
		single.URL = url
		single.Headers = hostfetch.CopyHeaders(creator.Headers)
		submissions = append(submissions, ctx.submitRemoteDownload(&single, ownerUserID, pluginName, origin))
	}
	return submissions
}

// submitRemoteDownload accepts and dispatches one URL.
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
		return result
	}

	input, err := remoteDownloadInputJSON(creator, pluginName)
	if err != nil {
		result.Err = err
		return result
	}
	legacyID := download_queue.NewJobID()
	accepted, err := service.Accept(ctx.jobDeps(), jobs.Acceptance{
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
	result.CanonicalJobID = accepted.ID

	job, err := ctx.downloadManager.SubmitForPluginWithOptions(creator, ownerUserID, pluginName,
		download_queue.SubmissionOptions{
			JobID:     legacyID,
			Canonical: &download_queue.CanonicalRef{JobID: accepted.ID},
		})
	if err != nil {
		// The Job was accepted and the queue refused the transfer. It is ended here
		// rather than left queued: a Job nothing will ever dispatch would sit in the
		// Job Center claiming work that was never admitted, and the refusal is what
		// its outcome should say.
		ctx.failUndispatchedDownload(accepted, err)
		result.Err = err
		return result
	}
	result.Job = job
	return result
}

// failUndispatchedDownload ends an accepted Job whose transfer the queue refused.
//
// It is bounded and classed, and the queue's own error text is deliberately not
// carried: it can name the URL. A failure here is not a failed download — nothing
// was ever fetched — so it is recorded as a policy refusal of the admission.
func (ctx *MahresourcesContext) failUndispatchedDownload(accepted jobs.Snapshot, cause error) {
	service := ctx.JobService()
	if service == nil {
		return
	}
	current, err := service.Get(ctx.jobDeps(), jobs.Access{Administrator: true}, accepted.ID)
	if err != nil || current.State.Terminal() {
		return
	}
	_, err = service.Finish(ctx.jobDeps(), jobs.FinishRequest{
		ExecutionRef:    jobs.ExecutionRef{JobID: accepted.ID},
		ExpectedVersion: current.Version,
		Outcome:         jobs.StateFailed,
		Failure: &jobs.Failure{
			Code:    "submission-refused",
			Class:   jobs.FailureClassPolicy,
			Message: "the download queue refused this submission",
		},
	})
	if err != nil {
		log.Printf("warning: could not record a refused download submission as a failed job: %v", err)
	}
}
