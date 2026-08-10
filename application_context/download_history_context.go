package application_context

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"mahresources/auth"
	"mahresources/download_queue"
	"mahresources/models"
	"mahresources/models/database_scopes"
	"mahresources/models/query_models"
	"mahresources/models/types"
)

// Column widths, mirrored from the model. A value that exceeds one is truncated
// rather than rejected: Postgres errors on an over-long varchar and SQLite stores
// it anyway, and neither outcome should be able to lose the record of a failed
// download over a long error message.
const (
	maxHistoryURLLength   = 2048
	maxHistoryNameLength  = 1000
	maxHistoryErrorLength = 2000
)

// historySweepBatch bounds one DELETE so a large backlog cannot hold a write
// transaction (or, on SQLite, the writer lock) for the length of the whole sweep.
const historySweepBatch = 1000

// historySweepMaxBatches bounds one sweep *cycle*. A backlog bigger than this is
// carried into the next tick five minutes later rather than being finished at
// any cost — the sweep shares the cleanup goroutine with the export sweep.
const historySweepMaxBatches = 20

// defaultDownloadFailedRetention / defaultDownloadHistoryRetention are the
// fallbacks for a context built with a zero-value config (every test fixture
// that constructs MahresourcesConfig{} directly). Without them a zero retention
// would read as "expire immediately" and the sweep would empty the table.
const (
	defaultDownloadFailedRetention  = 7 * 24 * time.Hour
	defaultDownloadHistoryRetention = 24 * time.Hour
	defaultDownloadCockpitLimit     = 10
)

// RecordTerminalDownload persists one finished download. Implements
// download_queue.HistoryRecorder.
//
// Upserted on job_id rather than inserted: a job that fails, is retried in place
// and then succeeds is one download as far as the user is concerned, so it stays
// one row whose status is the latest outcome. Attempts counts the outcomes.
func (ctx *MahresourcesContext) RecordTerminalDownload(rec download_queue.HistoryRecord) error {
	if ctx == nil || ctx.db == nil {
		return nil
	}
	if rec.JobID == "" {
		return fmt.Errorf("download history: record has no job id")
	}

	entry := models.DownloadHistoryEntry{
		JobID:       rec.JobID,
		URL:         truncateRunes(rec.URL, maxHistoryURLLength),
		Name:        truncateRunes(rec.Name, maxHistoryNameLength),
		Status:      rec.Status,
		Error:       truncateRunes(rec.Error, maxHistoryErrorLength),
		ResourceID:  rec.ResourceID,
		TotalSize:   rec.TotalSize,
		Progress:    rec.Progress,
		Attempts:    1,
		CreatedAt:   rec.CreatedAt,
		StartedAt:   rec.StartedAt,
		CompletedAt: rec.CompletedAt,
		// A fresh pointer, not the record's. GORM's field.Set writes *through* an
		// existing non-nil pointer, and the stamp callback sets this field on every
		// create — so sharing the pointer lets the callback rewrite the value the
		// caller still holds. The caller here is the download manager, whose job
		// keeps that same *uint as its owner: the write would silently reassign a
		// live job's owner in memory, and with it every later visibility check.
		CreatedByUserId: copyUintPtr(rec.CreatedByUserId),
	}
	if len(rec.Payload) > 0 {
		entry.Payload = types.JSON(rec.Payload)
	}

	// The actor is bound explicitly because the stamp callback overwrites
	// CreatedByUserId from the db context, falling back to the default actor — and
	// this write happens on a worker goroutine that carries no principal. With auth
	// off that default is the root admin, so the submitter the manager captured at
	// enqueue would be replaced by root, and a non-admin (who sees only rows it
	// owns) would lose sight of their own failures. A Session with SkipHooks would
	// not help: it suppresses model hooks, not registered callbacks.
	//
	// With no owner (auth off, or a job with no submitter) the default actor
	// applies, which is the same root attribution the rest of the app uses.
	db := ctx.db
	if rec.CreatedByUserId != nil && *rec.CreatedByUserId != 0 {
		db = ctx.WithPrincipal(&auth.Principal{UserID: *rec.CreatedByUserId}).db
	}

	return db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "job_id"}},
		// An outcome never overwrites a newer one. The recording goroutine runs
		// after its attempt has already been published, so a slow write from a
		// failed attempt can land after the retry that followed it has completed —
		// and an unguarded upsert would then restore `failed` over `completed`, with
		// the older attempt's timestamps. Both dialects spell the proposed row
		// `excluded`; the NULL arms keep the historic behaviour for a record with no
		// completion time rather than silently dropping it.
		Where: clause.Where{Exprs: []clause.Expression{
			gorm.Expr("download_history_entries.completed_at IS NULL OR excluded.completed_at IS NULL OR excluded.completed_at >= download_history_entries.completed_at"),
		}},
		DoUpdates: clause.Assignments(map[string]any{
			"status":       entry.Status,
			"error":        entry.Error,
			"resource_id":  entry.ResourceID,
			"total_size":   entry.TotalSize,
			"progress":     entry.Progress,
			"started_at":   entry.StartedAt,
			"completed_at": entry.CompletedAt,
			"url":          entry.URL,
			"name":         entry.Name,
			"payload":      entry.Payload,
			// Qualified with the table name so both dialects read the stored value
			// rather than the excluded one. A write the guard above rejects bumps
			// nothing, so attempts counts outcomes recorded *in order* — one lost
			// increment in that race is preferable to a row that describes the wrong
			// attempt.
			"attempts": gorm.Expr("download_history_entries.attempts + 1"),
		}),
	}).Create(&entry).Error
}

// GetDownloadHistory lists history rows for the given filters.
func (ctx *MahresourcesContext) GetDownloadHistory(offset, maxResults int, query *query_models.DownloadHistoryQuery) ([]models.DownloadHistoryEntry, error) {
	var entries []models.DownloadHistoryEntry
	return entries, ctx.db.Scopes(database_scopes.DownloadHistoryQuery(query, false)).
		Limit(maxResults).
		Offset(offset).
		Find(&entries).Error
}

// GetDownloadHistoryCount returns how many rows match the filters.
func (ctx *MahresourcesContext) GetDownloadHistoryCount(query *query_models.DownloadHistoryQuery) (int64, error) {
	var count int64
	return count, ctx.db.Model(&models.DownloadHistoryEntry{}).
		Scopes(database_scopes.DownloadHistoryQuery(query, true)).
		Count(&count).Error
}

// GetDownloadHistoryEntries loads the named rows, filtered by the same owner
// predicate the listing uses.
//
// Ids the caller may not see are simply absent from the result rather than
// reported as forbidden, so the row ids of other users cannot be probed — the
// same reasoning as GetDownloadJobHandler's 404.
func (ctx *MahresourcesContext) GetDownloadHistoryEntries(ids []uint, ownerUserID *uint, ownerRestricted bool) ([]models.DownloadHistoryEntry, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	var entries []models.DownloadHistoryEntry
	q := &query_models.DownloadHistoryQuery{OwnerUserID: ownerUserID, OwnerRestricted: ownerRestricted}
	return entries, ctx.db.
		Scopes(database_scopes.DownloadHistoryQuery(q, true)).
		Where("id IN ?", ids).
		Find(&entries).Error
}

// DeleteDownloadHistoryEntries removes the named rows the caller may see and
// returns how many went.
// notRetriedSince is the instant the caller read the rows. A row whose retry slot
// was claimed after that is left alone: the handler decided it was deletable from
// a copy that predates the claim, and deleting it would discard the record of a
// download that is starting right now.
func (ctx *MahresourcesContext) DeleteDownloadHistoryEntries(ids []uint, ownerUserID *uint, ownerRestricted bool, notRetriedSince time.Time) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	q := &query_models.DownloadHistoryQuery{OwnerUserID: ownerUserID, OwnerRestricted: ownerRestricted}
	res := ctx.db.
		Scopes(database_scopes.DownloadHistoryQuery(q, true)).
		Where("id IN ?", ids).
		Where("last_retry_at IS NULL OR last_retry_at < ?", notRetriedSince).
		Delete(&models.DownloadHistoryEntry{})
	return res.RowsAffected, res.Error
}

// MarkDownloadHistoryRetried records the job a resubmission produced, so the page
// can point at the attempt that is running now.
func (ctx *MahresourcesContext) MarkDownloadHistoryRetried(id uint, jobID string, at time.Time) error {
	return ctx.db.Model(&models.DownloadHistoryEntry{}).
		Where("id = ?", id).
		Updates(map[string]any{"last_retry_job_id": jobID, "last_retry_at": at}).Error
}

// ClaimDownloadHistoryRetry takes the row's retry slot, and reports whether it
// got it.
//
// Compare-and-set on last_retry_job_id, the column that names the attempt a row
// has already spawned: the caller passes the value it read and a token nobody
// else can produce, and exactly one of two simultaneous requests can win. Without
// it, two retries of the same evicted row both find no live attempt, both
// resubmit, and the download runs twice — two jobs, two resources, one row whose
// marker records whichever finished writing last.
//
// The claim is taken before the submit rather than recorded after it because the
// duplicate is created by the submit; a marker written afterwards is a note about
// a race that has already happened.
func (ctx *MahresourcesContext) ClaimDownloadHistoryRetry(id uint, expectedJobID, expectedStatus, claimToken string, at time.Time) (bool, error) {
	q := ctx.db.Model(&models.DownloadHistoryEntry{}).
		Where("id = ?", id).
		// COALESCE so a NULL and an empty marker are the same "never retried" — the
		// column is nullable and GORM writes "" for the zero value.
		Where("COALESCE(last_retry_job_id, '') = ?", expectedJobID)
	if expectedStatus != "" {
		// The status the caller decided from, too. An in-place retry that finished
		// while this request was in flight has already turned the row into
		// `completed`, and resubmitting it then downloads a file that now exists.
		q = q.Where("status = ?", expectedStatus)
	}
	res := q.Updates(map[string]any{"last_retry_job_id": claimToken, "last_retry_at": at})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected == 1, nil
}

// DownloadHistoryStatusForJob returns the stored status of the row recording the
// given job, or "" when there is none.
//
// The queue forgets a finished job within the hour; the row does not. It is what
// lets a retry refuse a row whose resubmission has *already succeeded* long after
// the job itself has been evicted — otherwise pressing Retry on the old failure
// downloads a second copy of a file that exists.
func (ctx *MahresourcesContext) DownloadHistoryStatusForJob(jobID string) (string, error) {
	if jobID == "" {
		return "", nil
	}
	var entries []models.DownloadHistoryEntry
	if err := ctx.db.Model(&models.DownloadHistoryEntry{}).
		Where("job_id = ?", jobID).
		Limit(1).
		Find(&entries).Error; err != nil {
		return "", err
	}
	if len(entries) == 0 {
		return "", nil
	}
	return entries[0].Status, nil
}

// DownloadFailedRetention / DownloadHistoryRetention read the live settings, with
// a fallback for a context whose config never set them (test fixtures). A zero
// retention must never be taken literally: it would mean "expire on write".
func (ctx *MahresourcesContext) DownloadFailedRetention() time.Duration {
	// ctx.settings directly, not Settings(): that accessor panics before wiring,
	// and the sweep runs on the manager's cleanup ticker, which starts with the
	// context — before main.go installs the settings service.
	if s := ctx.settings; s != nil {
		if d := s.DownloadFailedRetention(); d > 0 {
			return d
		}
	}
	if d := ctx.Config.DownloadFailedRetention; d > 0 {
		return d
	}
	return defaultDownloadFailedRetention
}

// DownloadCockpitLimit is how many of the newest jobs the jobs panel renders.
//
// The same zero-guard the retentions need, and for a sharper reason: a context
// built from a raw MahresourcesConfig{} (every api_test, any programmatic embed)
// carries 0, spec bounds validate persisted overrides rather than boot defaults,
// and a limit of 0 renders an empty panel.
func (ctx *MahresourcesContext) DownloadCockpitLimit() int {
	if s := ctx.settings; s != nil {
		if n := s.DownloadCockpitLimit(); n > 0 {
			return n
		}
	}
	if n := ctx.Config.DownloadCockpitLimit; n > 0 {
		return n
	}
	return defaultDownloadCockpitLimit
}

func (ctx *MahresourcesContext) DownloadHistoryRetention() time.Duration {
	// ctx.settings directly, not Settings(): that accessor panics before wiring,
	// and the sweep runs on the manager's cleanup ticker, which starts with the
	// context — before main.go installs the settings service.
	if s := ctx.settings; s != nil {
		if d := s.DownloadHistoryRetention(); d > 0 {
			return d
		}
	}
	if d := ctx.Config.DownloadHistoryRetention; d > 0 {
		return d
	}
	return defaultDownloadHistoryRetention
}

// SweepDownloadHistory deletes expired history rows and returns how many went.
//
// Both windows are read here, per call, which is what makes them runtime config:
// an operator who shortens the failed-download retention on /admin/settings sees
// the next sweep honour it without a restart.
//
// Expiry is measured from completed_at — when the download finished — falling
// back to created_at for a row whose completion time is somehow absent, so a row
// can never become immortal by lacking a timestamp.
func (ctx *MahresourcesContext) SweepDownloadHistory() (int64, error) {
	if ctx == nil || ctx.db == nil {
		return 0, nil
	}

	now := time.Now()
	windows := []struct {
		statuses []string
		cutoff   time.Time
	}{
		{
			statuses: []string{models.DownloadHistoryStatusFailed, models.DownloadHistoryStatusCancelled},
			cutoff:   now.Add(-ctx.DownloadFailedRetention()),
		},
		{
			statuses: []string{models.DownloadHistoryStatusCompleted},
			cutoff:   now.Add(-ctx.DownloadHistoryRetention()),
		},
	}

	var total int64
	for _, w := range windows {
		for range historySweepMaxBatches {
			// Two statements rather than a single DELETE ... LIMIT, which Postgres
			// does not accept. The subselect is the portable form of the same bound.
			var ids []uint
			if err := ctx.db.Model(&models.DownloadHistoryEntry{}).
				Where("status IN ?", w.statuses).
				Where("COALESCE(completed_at, created_at) < ?", w.cutoff).
				// A row someone has just retried is not idle history: its own status
				// still describes the attempt that failed, so the two predicates above
				// match it even while the attempt launched from it is downloading, and
				// the row would vanish from under the user who pressed the button.
				Where("last_retry_at IS NULL OR last_retry_at < ?", w.cutoff).
				Limit(historySweepBatch).
				Pluck("id", &ids).Error; err != nil {
				return total, err
			}
			if len(ids) == 0 {
				break
			}
			// Both predicates are repeated on the DELETE, not just the id list.
			// Selecting ids and then deleting them is check-then-act across two
			// statements: a retry landing in between upserts the row to `completed`
			// with a fresh completion time, and an id-only DELETE would remove a row
			// that had stopped being expired — silently, on the sweep's own goroutine.
			res := ctx.db.
				Where("id IN ?", ids).
				Where("status IN ?", w.statuses).
				Where("COALESCE(completed_at, created_at) < ?", w.cutoff).
				Where("last_retry_at IS NULL OR last_retry_at < ?", w.cutoff).
				Delete(&models.DownloadHistoryEntry{})
			if res.Error != nil {
				return total, res.Error
			}
			total += res.RowsAffected
			if len(ids) < historySweepBatch {
				break
			}
		}
	}

	return total, nil
}

// ResubmitDownloadHistoryEntry retries a stored download.
//
// The stored payload is decoded and handed back to the caller rather than
// enqueued here, because what may be enqueued depends on the *acting* principal's
// scope — and this context does not know it. The handler validates the decoded
// creator exactly as the submit endpoint validates a fresh one, then submits.
// A stored payload is a record of what was asked for once, not a standing
// permission to ask for it again.
func (ctx *MahresourcesContext) DownloadHistoryPayload(entry *models.DownloadHistoryEntry) (*query_models.ResourceFromRemoteCreator, error) {
	if entry == nil {
		return nil, errors.New("download history: no entry")
	}
	creator := &query_models.ResourceFromRemoteCreator{}
	if len(entry.Payload) > 0 {
		if err := json.Unmarshal(entry.Payload, creator); err != nil {
			return nil, fmt.Errorf("download history: decode stored payload: %w", err)
		}
	}
	// A row written before the payload existed, or one whose payload failed to
	// encode, still has its URL — enough to try the download again.
	if creator.URL == "" {
		creator.URL = entry.URL
	}
	if creator.URL == "" {
		return nil, errors.New("download history: entry has no URL to retry")
	}
	return creator, nil
}

// historyLogger reports failed history writes. A download's outcome does not
// depend on the record of it, so these are logged and swallowed — the alternative
// is failing a download that actually succeeded because a bookkeeping row could
// not be written.
type historyLogger struct{}

func (historyLogger) Warn(format string, args ...any) { log.Printf("WARN: "+format, args...) }

// copyUintPtr returns a pointer to a copy of the value, so a callee that writes
// through the pointer cannot reach the caller's variable.
func copyUintPtr(v *uint) *uint {
	if v == nil {
		return nil
	}
	c := *v
	return &c
}

// truncateRunes cuts a string to at most limit runes, never splitting one.
func truncateRunes(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}
