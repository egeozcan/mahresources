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
	// Acquire semaphore (blocks if MaxConcurrentDownloads jobs already running)
	select {
	case m.semaphore <- struct{}{}:
	case <-j.ctx.Done():
		j.SetStatus(JobStatusCancelled)
		m.notifySubscribers(JobEvent{Type: "updated", Job: j.Snapshot()})
		return
	}
	defer func() { <-m.semaphore }()

	now := time.Now()
	j.SetStartedAt(now)
	j.SetStatus(JobStatusProcessing)
	m.notifySubscribers(JobEvent{Type: "updated", Job: j.Snapshot()})

	sink := &managedSink{m: m, j: j}
	err := j.runFn(j.ctx, j, sink)
	completedAt := time.Now()
	j.SetCompletedAt(completedAt)

	if j.GetStatus() == JobStatusPaused {
		return
	}

	if err != nil {
		if j.ctx.Err() != nil {
			j.SetStatus(JobStatusCancelled)
		} else {
			j.SetStatus(JobStatusFailed)
			j.SetError(err.Error())
		}
	} else {
		j.SetStatus(JobStatusCompleted)
	}
	m.notifySubscribers(JobEvent{Type: "updated", Job: j.Snapshot()})
}
