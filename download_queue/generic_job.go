package download_queue

import (
	"context"
	"fmt"
	"time"
)

// ProgressSink is the manager-bound facade a generic JobRunFn uses to report
// live state. Every method mutates the underlying DownloadJob AND notifies
// SSE subscribers so the admin UI and CLI can render mid-flight updates.
type ProgressSink interface {
	SetPhase(phase string)
	SetPhaseProgress(current, total int64)
	UpdateProgress(done, total int64)
	AppendWarning(msg string)
	SetResultPath(path string)
}

// JobRunFn is the signature of a generic job worker.
type JobRunFn func(ctx context.Context, j *DownloadJob, p ProgressSink) error

// managedSink is the concrete ProgressSink. Holds a reference to the manager
// so every mutation triggers notifySubscribers.
type managedSink struct {
	m *DownloadManager
	j *DownloadJob
}

func (s *managedSink) SetPhase(phase string) {
	s.j.SetPhase(phase)
	s.m.notifyJob("updated", s.j)
}

func (s *managedSink) SetPhaseProgress(current, total int64) {
	s.j.SetPhaseProgress(current, total)
	s.m.notifyJob("updated", s.j)
}

func (s *managedSink) UpdateProgress(done, total int64) {
	s.j.UpdateProgress(done, total)
	s.m.notifyJob("updated", s.j)
}

func (s *managedSink) AppendWarning(msg string) {
	s.j.AppendWarning(msg)
	s.m.notifyJob("updated", s.j)
}

func (s *managedSink) SetResultPath(path string) {
	s.j.SetResultPath(path)
	s.m.notifyJob("updated", s.j)
}

// JobOptions describes a generic background job at construction.
//
// Everything a subscriber has to know about a job on the "added" event belongs
// here rather than in a setter called on the returned job. The setters ran after
// SubmitJob had already broadcast "added" *and* started the worker, and the owner is
// not cosmetic: under -auth the SSE stream drops any event whose job the principal
// may not see, and a job with no owner yet is one a non-admin may not see. Their own
// export or import therefore never appeared in their panel — the "added" was
// filtered out, the early "updated" events were dropped by the panel as unknown ids,
// and the row only turned up on the next reconnect.
type JobOptions struct {
	Source       string
	InitialPhase string
	// URL is optional. Generic jobs have no URL and the import handlers repurpose the
	// field as "source file path", which the delete handler reads back by job id.
	URL string
	// OwnerUserID is the submitting user, or nil for the auth-off super-user.
	OwnerUserID *uint
}

// SubmitJob enqueues an unowned generic background job.
func (m *DownloadManager) SubmitJob(source, initialPhase string, runFn JobRunFn) (*DownloadJob, error) {
	return m.SubmitJobWithOptions(JobOptions{Source: source, InitialPhase: initialPhase}, runFn)
}

// SubmitJobWithOptions enqueues a generic background job fully described at
// construction. See JobOptions.
func (m *DownloadManager) SubmitJobWithOptions(opts JobOptions, runFn JobRunFn) (*DownloadJob, error) {
	if runFn == nil {
		return nil, fmt.Errorf("download_queue: SubmitJob requires non-nil runFn")
	}

	m.mu.Lock()

	if !m.makeRoomForNewJob() {
		m.mu.Unlock()
		return nil, fmt.Errorf("download queue is full (max %d jobs) - all jobs are active or paused", MaxQueueSize)
	}

	ctx, cancel := context.WithCancel(context.Background())

	job := &DownloadJob{
		ID:              generateShortID(),
		URL:             opts.URL,
		Status:          JobStatusPending,
		Progress:        0,
		TotalSize:       -1,
		ProgressPercent: -1,
		CreatedAt:       time.Now(),
		Source:          opts.Source,
		Phase:           opts.InitialPhase,
		ctx:             ctx,
		cancel:          cancel,
		runFn:           runFn,
		ownerUserID:     opts.OwnerUserID,
	}

	m.jobs[job.ID] = job
	m.jobOrder = append(m.jobOrder, job.ID)

	// Announced under the registry lock and before the worker exists, for the reason
	// Submit gives: no event about this job may reach a subscriber ahead of the one
	// that says it exists.
	m.notifyJob("added", job)

	m.mu.Unlock()

	go m.processGenericJob(job)

	return job, nil
}

// processGenericJob runs runFn under the shared semaphore and broadcasts
// the terminal state to subscribers.
func (m *DownloadManager) processGenericJob(j *DownloadJob) {
	// The attempt this goroutine owns, and the context to judge its result by. Read
	// once, as in processJob: Retry installs a fresh context, and a stale attempt must
	// not classify itself against it.
	runID, ctx := j.attempt()

	// Acquire semaphore (blocks if MaxConcurrentDownloads jobs already running)
	select {
	case m.semaphore <- struct{}{}:
	case <-ctx.Done():
		// Worded, like the download path's, because the panel renders Error as the
		// row's reason: a cancelled job with an empty one just says "Cancelled" and
		// leaves the reader to guess whether anything ran.
		if j.finish(runID, JobStatusCancelled, "Cancelled before starting", 0, time.Now()) {
			m.notifyJob("updated", j)
		}
		return
	}
	defer func() { <-m.semaphore }()

	// Claimed for the same reason processJob claims its own start. A generic job can
	// never be paused, so the status half of the refusal is unreachable today; the
	// generation half is not — a Retry while this attempt was starting would bump it.
	if !j.claimStart(runID, JobStatusProcessing, time.Now()) {
		return
	}
	m.notifyJob("updated", j)

	sink := &managedSink{m: m, j: j}
	err := j.runFn(ctx, j, sink)

	// The context outranks the return value. A runFn may honour cancellation by
	// stopping and returning nil — "I gave up, and that is not an error" — and
	// reporting that as `completed` made Cancel's 200 a lie: the panel said an export
	// had finished when it had been abandoned, with a partial tar or none at all.
	// Unlike a download, an abandoned generic job has no created resource to orphan by
	// calling it what it is.
	status, errMsg := JobStatusCompleted, ""
	switch {
	case ctx.Err() != nil:
		status, errMsg = JobStatusCancelled, "Cancelled"
	case err != nil:
		status, errMsg = JobStatusFailed, err.Error()
	}

	// One atomic terminal write, as in processJob.
	if !j.finish(runID, status, errMsg, 0, time.Now()) {
		return
	}
	m.notifyJob("updated", j)
}
