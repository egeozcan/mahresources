package contracts

import (
	"time"

	"mahresources/models"
	"mahresources/models/query_models"
)

// DownloadHistoryReader provides read access to the durable record of finished
// background downloads.
//
// ownerUserID/ownerRestricted are the caller's visibility, resolved from the
// authenticated principal rather than from the request: admins see every row,
// every other principal sees only the downloads it submitted.
type DownloadHistoryReader interface {
	GetDownloadHistory(offset, maxResults int, query *query_models.DownloadHistoryQuery) ([]models.DownloadHistoryEntry, error)
	GetDownloadHistoryCount(query *query_models.DownloadHistoryQuery) (int64, error)
	GetDownloadHistoryEntries(ids []uint, ownerUserID *uint, ownerRestricted bool) ([]models.DownloadHistoryEntry, error)
	// DownloadHistoryPayload decodes the submission a row was created from, so it
	// can be run again. The caller must re-validate it against the acting
	// principal's scope before resubmitting.
	DownloadHistoryPayload(entry *models.DownloadHistoryEntry) (*query_models.ResourceFromRemoteCreator, error)
	// DownloadHistoryStatusForJob reports how the row recording that job ended, or
	// "" when no row does. Outlives the job's eviction from the queue.
	DownloadHistoryStatusForJob(jobID string) (string, error)
}

// DownloadHistoryWriter provides the mutations the /downloads page performs.
type DownloadHistoryWriter interface {
	DeleteDownloadHistoryEntries(ids []uint, ownerUserID *uint, ownerRestricted bool, notRetriedSince time.Time) (int64, error)
	MarkDownloadHistoryRetried(id uint, jobID string, at time.Time) error
	ClaimDownloadHistoryRetry(id uint, expectedJobID, expectedStatus, claimToken string, at time.Time) (bool, error)
}
