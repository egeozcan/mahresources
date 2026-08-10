package models

import (
	"mahresources/models/types"
	"time"
)

// DownloadHistoryEntry is the durable record of a background download that has
// reached a terminal state.
//
// The download queue itself is in-memory: jobs are evicted once the map reaches
// its cap and swept an hour after they finish, so a failed download used to stop
// being retryable within the hour — sooner if a hundred jobs passed through — and
// to be gone entirely on restart. This table is what survives all three, and what
// /downloads reads.
//
// One row per job, keyed on JobID — the manager's short id — so an in-manager
// retry updates the row it already has rather than adding a second one. Attempts
// counts how many terminal outcomes that job has produced.
//
// Payload is the submitted query_models.ResourceFromRemoteCreator, kept so a
// retry is possible after the job itself has been evicted from memory. It is
// re-validated against the acting principal's scope before it is ever resubmitted
// (see the retry handler) — a stored payload is not a permission.
type DownloadHistoryEntry struct {
	ID uint `gorm:"primarykey" json:"id"`

	// JobID is the download manager's id for the job that produced this row.
	JobID string `gorm:"uniqueIndex:idx_dl_history_job_id;size:64;not null" json:"jobId"`

	URL  string `gorm:"size:2048" json:"url"`
	Name string `gorm:"size:1000" json:"name"`

	// Status is terminal only: completed, failed or cancelled.
	Status string `gorm:"index:idx_dl_history_status;size:20;not null" json:"status"`
	Error  string `gorm:"size:2000" json:"error,omitempty"`

	ResourceID *uint `gorm:"index:idx_dl_history_resource_id" json:"resourceId,omitempty"`

	TotalSize int64 `json:"totalSize"`
	Progress  int64 `json:"progress"`
	Attempts  int   `gorm:"not null;default:1" json:"attempts"`

	// CreatedAt is the job's submission time, not the row's insertion time, so the
	// history sorts the way the queue does.
	CreatedAt   time.Time  `gorm:"index:idx_dl_history_created_at" json:"createdAt"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `gorm:"index:idx_dl_history_completed_at" json:"completedAt,omitempty"`

	// CreatedByUserId is the submitting user. Named to match the other 15 stamped
	// models so the user-deletion sweep (nullCreatorReferences) covers this table
	// for free. A job with no submitter — every download under no-auth — is
	// attributed by the stamp callback's default actor, which is the root admin,
	// exactly as the rest of the app's no-auth creates are. It is left NULL only
	// when a user is deleted.
	CreatedByUserId *uint `gorm:"index:idx_dl_history_created_by" json:"createdByUserId,omitempty"`

	// Payload is the original ResourceFromRemoteCreator as JSON. Never rendered to
	// the page — it is only read back by the retry path.
	Payload types.JSON `gorm:"type:json" json:"-"`

	// LastRetryJobID / LastRetryAt record the most recent resubmission made from
	// this row, so the page can point at the job that is running now.
	LastRetryJobID string     `gorm:"size:64" json:"lastRetryJobId,omitempty"`
	LastRetryAt    *time.Time `json:"lastRetryAt,omitempty"`
}

func (d DownloadHistoryEntry) GetId() uint {
	return d.ID
}

// Terminal statuses a history row may hold. They mirror download_queue's job
// statuses; the constants are duplicated rather than imported because models/
// sits below download_queue in the dependency direction.
const (
	DownloadHistoryStatusCompleted = "completed"
	DownloadHistoryStatusFailed    = "failed"
	DownloadHistoryStatusCancelled = "cancelled"
)

// DownloadHistoryRetryable reports whether a row's status permits a resubmission.
// Completed downloads are deliberately excluded: the resource already exists, and
// "retry" on one would silently create a duplicate.
func DownloadHistoryRetryable(status string) bool {
	return status == DownloadHistoryStatusFailed || status == DownloadHistoryStatusCancelled
}
