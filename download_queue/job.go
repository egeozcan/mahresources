package download_queue

import (
	"context"
	"mahresources/models/query_models"
	"sync"
	"time"
)

// JobStatus represents the current state of a download job
type JobStatus string

const (
	JobStatusPending     JobStatus = "pending"
	JobStatusDownloading JobStatus = "downloading"
	JobStatusProcessing  JobStatus = "processing"
	JobStatusCompleted   JobStatus = "completed"
	JobStatusFailed      JobStatus = "failed"
	JobStatusCancelled   JobStatus = "cancelled"
	JobStatusPaused      JobStatus = "paused"
)

const (
	JobSourceDownload         = "download"
	JobSourcePlugin           = "plugin"
	JobSourceGroupExport      = "group-export"
	JobSourceGroupImportParse = "group-import-parse"
	JobSourceGroupImportApply = "group-import-apply"
	// JobSourceResourceReduction is a Resource Reduction's clustering run. Its
	// own source rather than a reuse of one above, so the jobs panel can label it
	// and so recordTerminal keeps excluding it from the download history — a
	// history row's Retry button has no URL to press.
	JobSourceResourceReduction = "resource-reduction"
)

// DownloadJob represents a single remote URL download task
type DownloadJob struct {
	ID              string     `json:"id"`
	URL             string     `json:"url"`
	Status          JobStatus  `json:"status"`
	Progress        int64      `json:"progress"`
	TotalSize       int64      `json:"totalSize"`
	ProgressPercent float64    `json:"progressPercent"`
	Error           string     `json:"error,omitempty"`
	ResourceID      *uint      `json:"resourceId,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
	StartedAt       *time.Time `json:"startedAt,omitempty"`
	CompletedAt     *time.Time `json:"completedAt,omitempty"`
	Source          string     `json:"source"` // "download", "plugin", or "group-export"

	Phase      string   `json:"phase,omitempty"`
	PhaseCount int64    `json:"phaseCount,omitempty"`
	PhaseTotal int64    `json:"phaseTotal,omitempty"`
	ResultPath string   `json:"resultPath,omitempty"`
	Warnings   []string `json:"warnings,omitempty"`

	// Internal fields (not serialized to JSON)
	creator *query_models.ResourceFromRemoteCreator
	runFn   func(ctx context.Context, j *DownloadJob, p ProgressSink) error
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.RWMutex
	// cancelRequested records that a cancel has been accepted for this job. It is
	// set under mu by claimCancel and makes claimPause and claimResume refuse, so a
	// cancellation the caller was told about cannot be overwritten by a later
	// control. Cleared by claimRetry, which is the user asking for the job again.
	cancelRequested bool
	// discarded records that the user deleted this job's history row, so a terminal
	// write still in flight does not re-insert it. See markDiscarded.
	discarded bool
	// runID identifies the current attempt. Resume and Retry bump it, so the previous
	// attempt — which is still unwinding while the new one starts — can tell that it
	// no longer speaks for this job. Without it a paused-then-resumed job took the old
	// attempt's terminal state: `cancelled` landed on a job that was downloading
	// again, and the new attempt's own terminal write later overwrote that.
	runID uint64
	// initialPhase is what the job was submitted with, kept so a retry can put the
	// label back rather than leaving the failed attempt's.
	initialPhase string
	ownerUserID  *uint // RBAC: user that created the job (export download ownership)
	// pluginName names the plugin that submitted this download, empty for one a
	// person submitted. It selects the egress policy the transfer runs under:
	// a plugin's fetch is confined to that plugin's own declared network list,
	// and falling back to the host policy would widen it to every public host —
	// the confused deputy the plugin egress work closed from the other end.
	//
	// It is also persisted on the history row, because a retry replays the
	// stored payload on a fresh worker, possibly in a process that never saw
	// this job. A retry that forgot the origin would silently become a host
	// fetch.
	pluginName string
}

// Status transitions
//
// Every control (Cancel, Pause, Resume, Retry) and the worker's terminal write go
// through one of the claim*/finish methods below, and each of those is a *single*
// critical section over j.mu. That is the point of them: a plain status setter and
// GetStatus are each safe on their own, but a control that reads the status, decides
// from that read, and then writes holds no lock across the two — and another control
// can land in between.
//
// There is deliberately no exported plain setter any more. `SetStatus` survived the
// ownership rewrite unused, and an unguarded exported write is how the rule gets
// broken next: it skips both j.mu-spanning decisions and the runID check, so any
// future caller would reintroduce finding 1 without touching any of the code that
// documents it. Attempt-owned writes go through setStatusForRun.
//
// UI bug hunt 2026-07-29, review remediation finding 1: Cancel read `downloading`
// and so skipped its paused branch; Pause then wrote `paused` and cancelled the
// context; Cancel's job.Cancel() was a no-op on the already-cancelled context; and
// processJob saw `paused` and returned without stamping a terminal state. The job
// sat at `paused` — offering Resume — while the caller had been told 200
// {"status":"cancelled"}. Re-reading the status once more would not have fixed it;
// only one atomic claim does.
//
// The lock is the job's own rather than the manager's because the state being
// guarded is per-job. The manager's mu guards the registry (jobs, jobOrder), and
// promoting status transitions to it would serialise every job's per-chunk progress
// write against every other job's control calls, and would invert the manager -> job
// lock order that Resume and Retry already take.

// claimCancel atomically decides whether the job may be abandoned and records the
// intent. It returns the status observed, so the caller can tell a paused job from
// an active one, and the two are handled differently:
//
//   - paused: the terminal transition happens here, because Pause already cancelled
//     the context and no goroutine is left to observe a second cancellation.
//   - active: the status is left to the worker, which stamps it when it unwinds.
//
// Once claimed, claimPause and claimResume refuse.
// The snapshot is taken here, under the same lock as the write, for the same
// reason finish returns one: the paused branch is a terminal write that the
// history recorder has to describe, and re-reading the job afterwards would
// describe whatever a Retry had done to it in between.
func (j *DownloadJob) claimCancel(completedAt time.Time) (JobStatus, *DownloadJob, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()

	prev := j.Status
	if !j.canCancelLocked() {
		return prev, nil, false
	}
	j.cancelRequested = true
	if prev == JobStatusPaused {
		j.Status = JobStatusCancelled
		j.Error = "Download cancelled"
		// CompletedAt for the same reason the ordinary path sets it: it is what
		// cleanupOldJobs uses to retire the row.
		j.CompletedAt = &completedAt
	}
	j.cancelLocked()
	return prev, j.snapshotLocked(), true
}

// cancelLocked cancels the context the claim just observed.
//
// The cancellation belongs inside the claim, not after it: Pause used to write
// `paused`, release this lock, and only then cancel — and a Resume landing in that
// gap swapped ctx/cancel out, so the pause cancelled the *new* attempt's context and
// left the old attempt running with a live one. Both controls returned success.
//
// Safe under j.mu: a context.CancelFunc closes a channel and cancels children. It
// does not call back into the job, and any goroutine it wakes runs elsewhere.
func (j *DownloadJob) cancelLocked() {
	if j.cancel != nil {
		j.cancel()
	}
}

// attempt returns the identity of the attempt a worker is starting: the run it owns
// and the context that run must be judged against. Both come from one lock
// acquisition, so a worker cannot end up classifying its result against a context a
// later Resume installed.
func (j *DownloadJob) attempt() (uint64, context.Context) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.runID, j.ctx
}

// Ownership
//
// One sentence decides every remaining question in this file:
//
//	a job in pending, downloading or processing belongs to the attempt running it;
//	a job that is paused or terminal belongs to whichever control put it there.
//
// runID says *which* attempt, because a paused-then-resumed job has two alive for a
// moment. activeLocked says whether an attempt owns the job at all, and the two
// together are ownedByRunLocked — the single predicate every attempt-owned write is
// gated on. Splitting the second half out is what the 2026-07-29 audit's finding 1
// needed: runID guarded only claimStart and finish, so a control could take a job
// and the attempt's *other* writes — progress, and the `processing` its EOF callback
// reports — would still land on it.

func (j *DownloadJob) activeLocked() bool {
	return j.Status == JobStatusPending || j.Status == JobStatusDownloading ||
		j.Status == JobStatusProcessing
}

// ownedByRunLocked reports whether the given attempt may still write to the job.
func (j *DownloadJob) ownedByRunLocked(runID uint64) bool {
	return j.runID == runID && j.activeLocked()
}

// updateProgressForRun records progress on an attempt's behalf, and reports whether
// the write took so the caller can skip a notification for state it did not change.
//
// Unconditional here was audit finding 1's second half: a stale attempt's in-flight
// read completing after a Resume overwrote the *new* attempt's progress, and a
// straggler landing on a paused job moved a readout the pause exists to freeze.
func (j *DownloadJob) updateProgressForRun(runID uint64, downloaded, total int64) bool {
	j.mu.Lock()
	defer j.mu.Unlock()

	if !j.ownedByRunLocked(runID) {
		return false
	}
	j.setProgressLocked(downloaded, total)
	return true
}

// setStatusForRun moves the job to a status its own attempt reports — today only
// `processing`, when the transfer hits EOF and the resource write begins.
//
// Unconditional here was audit finding 1's first half, and it silently undid a
// control that had already answered 200: EOF fires, the callback notifies
// subscribers, a Pause claims `paused` and cancels the context in that gap, and the
// callback then writes `processing` over it. finish accepted the same attempt
// afterwards — it *is* the same attempt — so the job was retired and the pause had
// simply never happened.
func (j *DownloadJob) setStatusForRun(runID uint64, status JobStatus) bool {
	j.mu.Lock()
	defer j.mu.Unlock()

	if !j.ownedByRunLocked(runID) {
		return false
	}
	j.Status = status
	return true
}

// claimStart atomically hands the job to its worker: pending -> the running status
// the worker is about to report, stamped with its start time.
//
// It refuses when the job is no longer pending, which means a control took it while
// the goroutine was starting: a Pause (the job is paused and waiting for Resume) or
// the cancel of a paused job (already terminal). The worker's *forward* writes used
// to be unconditional, and a job's goroutine starts while the job is `pending` —
// which is pausable — so a Pause landing between the semaphore acquisition and the
// first status write was silently overwritten. The caller had been told 200
// "paused"; the job then downloaded under an already-cancelled context and ended up
// cancelled with its progress discarded. That is claimCancel's defect in mirror
// image.
//
// It deliberately does *not* refuse a job whose cancel has been accepted. An active
// job's terminal state is its worker's to stamp, so refusing here would leave the
// job pending forever with nobody left to retire it. The worker starts, the
// cancelled context fails the download immediately, and finish stamps `cancelled`.
func (j *DownloadJob) claimStart(runID uint64, running JobStatus, startedAt time.Time) bool {
	j.mu.Lock()
	defer j.mu.Unlock()

	if j.runID != runID || j.Status != JobStatusPending {
		return false
	}
	j.Status = running
	j.StartedAt = &startedAt
	return true
}

// claimPause atomically transitions a pausable job to paused. It refuses a job
// whose cancel has already been accepted: the user asked for that download to be
// abandoned, and a pause winning here would leave a job its worker has given up on
// sitting in a state that offers Resume.
func (j *DownloadJob) claimPause() (JobStatus, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if j.cancelRequested {
		// The status the caller is told about is the one the job is heading for, not
		// the `downloading` it still reads: "cannot be paused (status: downloading)"
		// would be a puzzle, and the cancel is already guaranteed.
		return JobStatusCancelled, false
	}
	if !j.canPauseLocked() {
		return j.Status, false
	}
	j.Status = JobStatusPaused
	j.Error = "" // Clear any previous error
	j.cancelLocked()
	return JobStatusPaused, true
}

// claimResume atomically transitions a paused job back to pending and installs the
// context the new attempt will run under. Both halves have to be one step: a resume
// that set the status and then installed the context could be cancelled in between,
// and the cancellation would apply to a context nothing is using.
func (j *DownloadJob) claimResume(ctx context.Context, cancel context.CancelFunc) (JobStatus, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if j.cancelRequested {
		return JobStatusCancelled, false
	}
	if !j.canResumeLocked() {
		return j.Status, false
	}
	j.runID++
	j.ctx, j.cancel = ctx, cancel
	j.Status = JobStatusPending
	j.Progress, j.TotalSize, j.ProgressPercent = 0, -1, -1
	j.StartedAt = nil
	return JobStatusPending, true
}

// claimRetry atomically resets a terminal job for another attempt. It clears
// cancelRequested — the user has explicitly asked for this job to run again, so the
// cancel that ended it no longer stands, and without clearing it a retried download
// could never be paused.
func (j *DownloadJob) claimRetry(ctx context.Context, cancel context.CancelFunc) (JobStatus, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if !j.canRetryLocked() {
		return j.Status, false
	}
	j.cancelRequested = false
	j.runID++
	j.ctx, j.cancel = ctx, cancel
	j.Status = JobStatusPending
	j.Error = ""
	j.Progress, j.TotalSize, j.ProgressPercent = 0, -1, -1
	j.StartedAt, j.CompletedAt, j.ResourceID = nil, nil, nil
	// The previous attempt's *reported* leftovers go too, which the counters and the
	// error already did and these three did not. A retried import re-reports every
	// warning it hits, so keeping the failed run's list showed each one twice and
	// climbing; ResultPath still named a tar the retry has not written yet; and the
	// phase counters read as a run that was part-way through something.
	j.Warnings, j.ResultPath = nil, ""
	j.PhaseCount, j.PhaseTotal = 0, 0
	// Phase goes back to what the job was submitted with, not to "". Clearing the
	// counters and leaving the label was the worst of both: a parse that failed
	// half-way was re-listed as pending/"parsing" — a phase the new attempt has not
	// begun and, queued behind a busy semaphore, may not begin for a while.
	j.Phase = j.initialPhase
	return JobStatusPending, true
}

// finish stamps the job's terminal state in one step and reports whether it took.
//
// It is ownedByRunLocked and nothing else, which covers three refusals that used to
// be two separate reads or no read at all:
//
//   - a stale attempt speaks for nobody. Pause makes a job resumable while its worker
//     is still unwinding, so the previous attempt can reach this while the next one is
//     already downloading — and its terminal state would land on a live job.
//   - a paused download keeps its progress and waits for Resume, so the worker must
//     not retire it.
//   - a job a control has already retired stays retired. This is the third one, and
//     it was open: the check was `Status == paused` alone, so an attempt whose
//     AddResource succeeded after a Pause-then-Cancel wrote `completed` over the
//     `cancelled` the user had already been shown, resource id and all. Whether a
//     *live* download should report `completed` despite an accepted cancel is a
//     separate question (see processJob) — writing over a terminal state that is
//     already on screen is not.
func (j *DownloadJob) finish(runID uint64, status JobStatus, errMsg string, resourceID uint, completedAt time.Time) bool {
	_, ok := j.finishSnapshot(runID, status, errMsg, resourceID, completedAt)
	return ok
}

// finishSnapshot is finish, plus the point-in-time capture of what it wrote.
//
// The capture belongs under the same lock as the write. The durable history row
// is built from it, and the alternative — stamping the terminal state, notifying
// subscribers, then re-reading the job — hands a client the whole round trip in
// which to press Retry: the re-read would then describe a job that is `pending`
// again, and the row would record a download that never finished and that no
// retention window can expire.
func (j *DownloadJob) finishSnapshot(runID uint64, status JobStatus, errMsg string, resourceID uint, completedAt time.Time) (*DownloadJob, bool) {
	j.mu.Lock()
	defer j.mu.Unlock()

	if !j.ownedByRunLocked(runID) {
		return nil, false
	}

	// A cancel accepted while this attempt was running outranks its success. The
	// transfer finished, the resource row exists — and the user was answered 200
	// {"status":"cancelled"} before any of that landed. Reporting `completed` makes
	// that answer a lie, which is the defect this whole audit started from and the one
	// thing every other control here was fixed to stop doing.
	//
	// The resource id survives the conversion, and that is what makes honouring the
	// control safe: the file was saved either way, so dropping the id would hide it
	// rather than un-create it. The row says both things — cancelled, and here is what
	// had already been written. Nothing is deleted to make the status true; a control
	// pressed to stop work is not a request to destroy a file that already exists.
	//
	// Decided here rather than by the caller because reading cancelRequested and then
	// calling finish is check-then-act: a cancel landing in that gap would be answered
	// and then contradicted exactly as before, only in a narrower window.
	if status == JobStatusCompleted && j.cancelRequested {
		status = JobStatusCancelled
		if errMsg == "" {
			errMsg = "Cancelled after the file had been saved"
		}
	}

	j.Status = status
	if errMsg != "" {
		j.Error = errMsg
	}
	j.CompletedAt = &completedAt
	if resourceID != 0 {
		j.ResourceID = &resourceID
	}
	return j.snapshotLocked(), true
}

// UpdateProgress safely updates the job's progress fields. Used by generic jobs
// through ProgressSink, whose runFn is the job's only writer; a download attempt
// goes through updateProgressForRun instead, because it may not be the job's only
// writer any more.
func (j *DownloadJob) UpdateProgress(downloaded, total int64) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.setProgressLocked(downloaded, total)
}

func (j *DownloadJob) setProgressLocked(downloaded, total int64) {
	j.Progress = downloaded
	j.TotalSize = total
	if total > 0 {
		j.ProgressPercent = float64(downloaded) / float64(total) * 100
	} else {
		j.ProgressPercent = -1
	}
}

// SetError safely sets the job's error message
func (j *DownloadJob) SetError(err string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Error = err
}

// SetURL safely sets the job's URL field. For generic jobs this is unused
// by the core manager; callers can repurpose it to store a source file path.
func (j *DownloadJob) SetURL(url string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.URL = url
}

// GetURL safely returns the job's URL field.
func (j *DownloadJob) GetURL() string {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.URL
}

// SetResourceID safely sets the completed resource ID.
// A zero value clears the resource ID.
func (j *DownloadJob) SetResourceID(id uint) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if id == 0 {
		j.ResourceID = nil
	} else {
		j.ResourceID = &id
	}
}

// SetStartedAt safely sets the job's start time.
// A zero time value clears the start time.
func (j *DownloadJob) SetStartedAt(t time.Time) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if t.IsZero() {
		j.StartedAt = nil
	} else {
		j.StartedAt = &t
	}
}

// SetCompletedAt safely sets the job's completion time.
// A zero time value clears the completion time.
func (j *DownloadJob) SetCompletedAt(t time.Time) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if t.IsZero() {
		j.CompletedAt = nil
	} else {
		j.CompletedAt = &t
	}
}

// GetContext safely returns the job's context.
func (j *DownloadJob) GetContext() context.Context {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.ctx
}

// SetContext safely sets the job's context and cancel function.
func (j *DownloadJob) SetContext(ctx context.Context, cancel context.CancelFunc) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.ctx = ctx
	j.cancel = cancel
}

// Cancel safely calls the job's cancel function.
func (j *DownloadJob) Cancel() {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if j.cancel != nil {
		j.cancel()
	}
}

// GetCompletedAt safely returns the job's completion time.
func (j *DownloadJob) GetCompletedAt() *time.Time {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.CompletedAt
}

// GetStatus safely returns the job's current status
func (j *DownloadJob) GetStatus() JobStatus {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.Status
}

// IsActive returns true if the job is still in progress — which is the same thing
// as "owned by its attempt rather than by a control", so it shares that predicate.
func (j *DownloadJob) IsActive() bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.activeLocked()
}

// CanCancel returns true if the job can still be abandoned.
//
// This is deliberately *not* IsActive(): a paused job is not active — it holds no
// semaphore slot and has no goroutine — but it is very much cancellable, and
// treating it as finished is UI bug hunt 2026-07-29 finding 2. IsActive() is left
// alone because ActiveCount() and Shutdown() both mean "is running" by it.
func (j *DownloadJob) CanCancel() bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.canCancelLocked()
}

// CanPause returns true if the job can be paused.
// Generic jobs (runFn != nil, e.g. group-export) can never be paused because
// their runFn is a streaming operation that can't be suspended and resumed.
func (j *DownloadJob) CanPause() bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.canPauseLocked()
}

// CanResume returns true if the job can be resumed
func (j *DownloadJob) CanResume() bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.canResumeLocked()
}

// CanRetry returns true if the job can be retried
func (j *DownloadJob) CanRetry() bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.canRetryLocked()
}

// The four predicates, defined once, for callers that already hold j.mu. The
// exported Can* wrappers above and the claim* transitions below share them, so a
// control's check and its act cannot drift apart.

func (j *DownloadJob) canCancelLocked() bool {
	return j.Status == JobStatusPending || j.Status == JobStatusDownloading ||
		j.Status == JobStatusProcessing || j.Status == JobStatusPaused
}

func (j *DownloadJob) canPauseLocked() bool {
	if j.runFn != nil {
		return false
	}
	return j.Status == JobStatusPending || j.Status == JobStatusDownloading
}

func (j *DownloadJob) canResumeLocked() bool {
	return j.Status == JobStatusPaused
}

func (j *DownloadJob) canRetryLocked() bool {
	return j.Status == JobStatusFailed || j.Status == JobStatusCancelled
}

// PluginName reports the plugin that submitted this download, or "" for one a
// person submitted.
func (j *DownloadJob) PluginName() string {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.pluginName
}

// SetPhase safely sets the job's current phase name.
func (j *DownloadJob) SetPhase(phase string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Phase = phase
}

// SetPhaseProgress safely sets the per-phase progress counters.
func (j *DownloadJob) SetPhaseProgress(current, total int64) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.PhaseCount = current
	j.PhaseTotal = total
}

// AppendWarning safely appends a warning message to the job.
func (j *DownloadJob) AppendWarning(msg string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Warnings = append(j.Warnings, msg)
}

// SetResultPath safely sets the result file path for the job.
func (j *DownloadJob) SetResultPath(path string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.ResultPath = path
}

// GetError safely returns the job's error message.
func (j *DownloadJob) GetError() string {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.Error
}

// GetResultPath safely returns the job's result file path.
func (j *DownloadJob) GetResultPath() string {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.ResultPath
}

// The owner is named at construction (JobOptions.OwnerUserID) and never set
// afterwards. The setter that used to live here was how the export and import
// handlers did it, and setting it on the job SubmitJob had already returned meant
// the "added" event went out ownerless — which under -auth is an event the SSE
// stream drops for its own submitter, so a user's export never appeared in their own
// panel until the next reconnect. Removing the setter keeps that from being written
// again.

// markDiscarded records that the user has thrown this job's record away, so a
// terminal write still on its way to the store does not put it back.
//
// The flag lives on the job rather than in a set on the manager because the
// pending recorder is holding this very struct: no bookkeeping, nothing to
// expire. It narrows the window rather than closing it — a recorder that has
// already passed the check can still be descheduled past the row deletion — but
// that is a gap between two instructions rather than the length of a DB write.
func (j *DownloadJob) markDiscarded() {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.discarded = true
}

func (j *DownloadJob) isDiscarded() bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.discarded
}

// CreatorCopy is creatorCopy, exported for the queue's own control endpoints:
// restarting a job replays this payload on the unscoped worker, so the handler
// has to be able to re-check it against the principal pressing the button.
func (j *DownloadJob) CreatorCopy() *query_models.ResourceFromRemoteCreator {
	return j.creatorCopy()
}

// creatorCopy returns a value copy of the submitted creator, or nil for a job
// that has none (every generic job, and the bare jobs tests build directly).
//
// A copy because the caller marshals it, and `creator` is a pointer the job keeps
// for the lifetime of every retry — encoding the live one on a recording
// goroutine would read fields a resubmission could be writing. Nothing mutates it
// today; the copy is what keeps that from becoming a race the day something does.
func (j *DownloadJob) creatorCopy() *query_models.ResourceFromRemoteCreator {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if j.creator == nil {
		return nil
	}
	c := *j.creator
	return &c
}

// GetOwnerUserID returns the user that created the job, or nil if unset.
func (j *DownloadJob) GetOwnerUserID() *uint {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.ownerUserID
}

// Snapshot returns a shallow value-copy of the job's exported fields. The
// returned *DownloadJob is a fresh struct whose fields are safe to read
// without acquiring j.mu — it's a point-in-time capture. The copy does not
// share the original's mutex, context, or runFn; don't mutate it or pass
// it back to the manager.
//
// Used by notifySubscribers so JobEvent.Job can be read by subscribers
// without racing setters that may fire concurrently.
func (j *DownloadJob) Snapshot() *DownloadJob {
	j.mu.RLock()
	defer j.mu.RUnlock()
	return j.snapshotLocked()
}

// snapshotLocked is Snapshot's body, for callers that already hold j.mu — the
// transitions that stamp a terminal state and must describe exactly what they
// wrote.
func (j *DownloadJob) snapshotLocked() *DownloadJob {
	snap := &DownloadJob{
		ID:              j.ID,
		URL:             j.URL,
		Status:          j.Status,
		Progress:        j.Progress,
		TotalSize:       j.TotalSize,
		ProgressPercent: j.ProgressPercent,
		Error:           j.Error,
		ResourceID:      j.ResourceID,
		CreatedAt:       j.CreatedAt,
		StartedAt:       j.StartedAt,
		CompletedAt:     j.CompletedAt,
		Source:          j.Source,
		Phase:           j.Phase,
		PhaseCount:      j.PhaseCount,
		PhaseTotal:      j.PhaseTotal,
		ResultPath:      j.ResultPath,
		ownerUserID:     j.ownerUserID,
		pluginName:      j.pluginName,
	}
	// Deep-copy the Warnings slice so subscribers can't observe a torn append.
	if j.Warnings != nil {
		snap.Warnings = make([]string, len(j.Warnings))
		copy(snap.Warnings, j.Warnings)
	}
	return snap
}

// JobEvent represents a change in job state for SSE broadcasting
type JobEvent struct {
	Type string       `json:"type"` // "added", "updated", "removed"
	Job  *DownloadJob `json:"job"`
}
