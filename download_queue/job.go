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

// DownloadJob represents a single remote URL download task
type DownloadJob struct {
	ID              string    `json:"id"`
	URL             string    `json:"url"`
	Status          JobStatus `json:"status"`
	Progress        int64     `json:"progress"`
	TotalSize       int64     `json:"totalSize"`
	ProgressPercent float64   `json:"progressPercent"`
	Error           string    `json:"error,omitempty"`
	ResourceID      *uint     `json:"resourceId,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	StartedAt       *time.Time `json:"startedAt,omitempty"`
	CompletedAt     *time.Time `json:"completedAt,omitempty"`

	// Internal fields (not serialized to JSON)
	creator *query_models.ResourceFromRemoteCreator
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

// SetResourceID safely sets the completed resource ID
func (j *DownloadJob) SetResourceID(id uint) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.ResourceID = &id
}

// SetStartedAt safely sets the job's start time
func (j *DownloadJob) SetStartedAt(t time.Time) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.StartedAt = &t
}

// SetCompletedAt safely sets the job's completion time
func (j *DownloadJob) SetCompletedAt(t time.Time) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.CompletedAt = &t
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

// CanPause returns true if the job can be paused
func (j *DownloadJob) CanPause() bool {
	status := j.GetStatus()
	return status == JobStatusPending || status == JobStatusDownloading
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

// JobEvent represents a change in job state for SSE broadcasting
type JobEvent struct {
	Type string       `json:"type"` // "added", "updated", "removed"
	Job  *DownloadJob `json:"job"`
}
