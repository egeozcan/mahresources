package models

import (
	"time"

	"mahresources/models/types"
)

// ScheduledDownload is a deferred plugin download that has not yet become a
// queue job.
//
// The download queue is intentionally in-memory, and pending queue jobs are not
// evicted to make room. A download that should start later therefore lives here
// until a scheduler tick claims it and submits exactly one queue job. PluginName
// is stored with the row so a restart still submits under the plugin's own
// egress policy rather than the host policy.
type ScheduledDownload struct {
	ID uint `gorm:"primarykey" json:"id"`

	PluginName string     `gorm:"size:128;index:idx_sched_dl_plugin;not null" json:"pluginName"`
	URL        string     `gorm:"size:2048;index:idx_sched_dl_url" json:"url"`
	Payload    types.JSON `gorm:"type:json" json:"-"`

	DueAt time.Time `gorm:"index:idx_sched_dl_due;not null" json:"dueAt"`

	// ClaimToken and ClaimedAt are a short-lived compare-and-set slot held only
	// while a process is submitting the queue job. A stale claim is abandoned
	// after the scheduled-download claim TTL in application_context; unlike a
	// plugin schedule, no long-running Lua handler is behind this claim.
	ClaimToken string     `gorm:"size:80" json:"-"`
	ClaimedAt  *time.Time `json:"claimedAt,omitempty"`

	Status    string `gorm:"size:20;index:idx_sched_dl_status;not null" json:"status"`
	JobID     string `gorm:"size:64;index:idx_sched_dl_job" json:"jobId,omitempty"`
	LastError string `gorm:"size:2000" json:"lastError,omitempty"`
	Attempts  int    `gorm:"not null;default:0" json:"attempts"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// CreatedByUserId is the submitting actor, and every fire re-validates the
	// stored payload against that actor before submitting. It deliberately has no
	// FK association, matching DownloadHistoryEntry and PluginSchedule. Deleting a
	// user NULLs this via stampedModels(), and the claim predicate then makes the
	// row inert rather than falling back to root.
	CreatedByUserId *uint `gorm:"index:idx_sched_dl_created_by" json:"createdByUserId,omitempty"`
}

func (s ScheduledDownload) GetId() uint {
	return s.ID
}

const (
	ScheduledDownloadStatusPending   = "pending"
	ScheduledDownloadStatusSubmitted = "submitted"
	ScheduledDownloadStatusFailed    = "failed"
	ScheduledDownloadStatusCancelled = "cancelled"
)
