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
	JobSourceDownload        = "download"
	JobSourcePlugin          = "plugin"
	JobSourceGroupExport     = "group-export"
	JobSourceGroupImportParse = "group-import-parse"
	JobSourceGroupImportApply = "group-import-apply"
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
}

// UpdateProgress safely updates the job's progress fields
func (j *DownloadJob) UpdateProgress(downloaded, total int64) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Progress = downloaded
	j.TotalSize = total
	if total > 0 {
		j.ProgressPercent = float64(downloaded) / float64(total) * 100
	} else {
		j.ProgressPercent = -1
	}
}

// SetStatus safely updates the job's status
func (j *DownloadJob) SetStatus(status JobStatus) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Status = status
}

// SetError safely sets the job's error message
func (j *DownloadJob) SetError(err string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Error = err
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

// IsActive returns true if the job is still in progress
func (j *DownloadJob) IsActive() bool {
	status := j.GetStatus()
	return status == JobStatusPending || status == JobStatusDownloading || status == JobStatusProcessing
}

// CanPause returns true if the job can be paused.
// Generic jobs (runFn != nil, e.g. group-export) can never be paused because
// their runFn is a streaming operation that can't be suspended and resumed.
func (j *DownloadJob) CanPause() bool {
	j.mu.RLock()
	defer j.mu.RUnlock()
	if j.runFn != nil {
		return false
	}
	return j.Status == JobStatusPending || j.Status == JobStatusDownloading
}

// CanResume returns true if the job can be resumed
func (j *DownloadJob) CanResume() bool {
	return j.GetStatus() == JobStatusPaused
}

// CanRetry returns true if the job can be retried
func (j *DownloadJob) CanRetry() bool {
	status := j.GetStatus()
	return status == JobStatusFailed || status == JobStatusCancelled
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
