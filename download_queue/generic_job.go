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
	s.m.notifySubscribers(JobEvent{Type: "updated", Job: s.j.Snapshot()})
}

func (s *managedSink) SetPhaseProgress(current, total int64) {
	s.j.SetPhaseProgress(current, total)
	s.m.notifySubscribers(JobEvent{Type: "updated", Job: s.j.Snapshot()})
}

func (s *managedSink) UpdateProgress(done, total int64) {
	s.j.UpdateProgress(done, total)
	s.m.notifySubscribers(JobEvent{Type: "updated", Job: s.j.Snapshot()})
}

func (s *managedSink) AppendWarning(msg string) {
	s.j.AppendWarning(msg)
	s.m.notifySubscribers(JobEvent{Type: "updated", Job: s.j.Snapshot()})
}

func (s *managedSink) SetResultPath(path string) {
	s.j.SetResultPath(path)
	s.m.notifySubscribers(JobEvent{Type: "updated", Job: s.j.Snapshot()})
}

// SubmitJob enqueues a generic background job.
func (m *DownloadManager) SubmitJob(source, initialPhase string, runFn JobRunFn) (*DownloadJob, error) {
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
		Status:          JobStatusPending,
		Progress:        0,
		TotalSize:       -1,
		ProgressPercent: -1,
		CreatedAt:       time.Now(),
		Source:          source,
		Phase:           initialPhase,
		ctx:             ctx,
		cancel:          cancel,
		runFn:           runFn,
	}

	m.jobs[job.ID] = job
	m.jobOrder = append(m.jobOrder, job.ID)

	m.mu.Unlock()

	m.notifySubscribers(JobEvent{Type: "added", Job: job.Snapshot()})

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
		if j.finish(runID, JobStatusCancelled, "", 0, time.Now()) {
			m.notifySubscribers(JobEvent{Type: "updated", Job: j.Snapshot()})
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
	m.notifySubscribers(JobEvent{Type: "updated", Job: j.Snapshot()})

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
		status = JobStatusCancelled
	case err != nil:
		status, errMsg = JobStatusFailed, err.Error()
	}

	// One atomic terminal write, as in processJob.
	if !j.finish(runID, status, errMsg, 0, time.Now()) {
		return
	}
	m.notifySubscribers(JobEvent{Type: "updated", Job: j.Snapshot()})
}
