package hash_worker

import (
	"errors"
	"sync/atomic"

	"gorm.io/gorm"
	"mahresources/models"
)

// ErrRecomputeInProgress is returned when a recompute is already running.
var ErrRecomputeInProgress = errors.New("similarity recompute already in progress")

// recomputeInFlight guards against concurrent RecomputeV2Pairs runs (process-wide).
var recomputeInFlight atomic.Bool

// TryBeginRecompute atomically acquires the process-wide recompute guard,
// returning false when a recompute is already running. A caller that gets true
// owns the guard and must release it with EndRecompute. Exposed for tests that
// need to simulate an in-flight recompute.
func TryBeginRecompute() bool {
	return recomputeInFlight.CompareAndSwap(false, true)
}

// EndRecompute releases the guard taken by TryBeginRecompute.
func EndRecompute() {
	recomputeInFlight.Store(false)
}

// RecomputeInProgress reports whether a recompute currently holds the guard.
// The admin endpoint checks it before submitting the background job so a
// request arriving while a recompute is running is rejected synchronously
// (HTTP 409). The authoritative guard remains the CompareAndSwap inside
// RecomputeV2Pairs: a request racing a submitted-but-not-yet-started job can
// slip past this check, in which case the second job fails with
// ErrRecomputeInProgress instead of the request getting a 409. The guard is
// deliberately NOT acquired at submit time — a job cancelled while still
// pending never runs its job function, which would leak the guard until
// restart.
func RecomputeInProgress() bool {
	return recomputeInFlight.Load()
}

// RetryFailedHashes clears the failed marker from previously-undecodable rows so
// the Phase-3 backfill task picks them up again (hash_version IS NULL is the
// backfill cursor). Returns the number of rows reset. This is the admin
// "retry failed hashes" action; it is a single cheap UPDATE.
func RetryFailedHashes(db *gorm.DB) (int64, error) {
	res := db.Model(&models.ImageHash{}).
		Where("status = ?", models.HashStatusFailed).
		Updates(map[string]any{
			"hash_version": gorm.Expr("NULL"),
			"status":       "",
		})
	return res.RowsAffected, res.Error
}

// RecomputeV2Pairs deletes every similarity pair whose both endpoints are v2 and
// rebuilds them by re-running the v2 matcher for each v2 "ok" row. It performs no
// image decoding (DB-only), so it is cheap enough to run for algorithm/constant
// changes. It is guarded against concurrent runs.
//
// batchSize bounds how many rows are loaded per page. shouldStop is polled between
// rows so the job can be cancelled; progress(done, total) is called after each row.
func RecomputeV2Pairs(db *gorm.DB, batchSize int, shouldStop func() bool, progress func(done, total int64)) error {
	if !TryBeginRecompute() {
		return ErrRecomputeInProgress
	}
	defer EndRecompute()

	if batchSize <= 0 {
		batchSize = 500
	}

	// Delete pairs where both endpoints are v2 rows. Legacy (v1-involving) pairs
	// are left intact — they are recreated only when their rows get backfilled.
	v2Ids := db.Model(&models.ImageHash{}).
		Select("resource_id").
		Where("hash_version = ?", HashVersionV2)
	if err := db.
		Where("resource_id1 IN (?)", v2Ids).
		Where("resource_id2 IN (?)", v2Ids).
		Delete(&models.ResourceSimilarity{}).Error; err != nil {
		return err
	}

	var total int64
	db.Model(&models.ImageHash{}).
		Where("hash_version = ? AND status = ?", HashVersionV2, models.HashStatusOK).
		Where("p_hash_int IS NOT NULL").
		Count(&total)
	if progress != nil {
		progress(0, total)
	}

	var done int64
	var lastID uint
	for {
		if shouldStop != nil && shouldStop() {
			return nil
		}
		var rows []models.ImageHash
		if err := db.
			Where("hash_version = ? AND status = ?", HashVersionV2, models.HashStatusOK).
			Where("p_hash_int IS NOT NULL").
			Where("id > ?", lastID).
			Order("id ASC").
			Limit(batchSize).
			Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			break
		}
		for i := range rows {
			r := &rows[i]
			lastID = r.ID
			if r.ResourceId == nil {
				continue
			}
			FindSimilaritiesV2(db, *r.ResourceId, r.GetPHash(), r.GetDHash(), r.GetAHash())
			done++
		}
		if progress != nil {
			progress(done, total)
		}
	}
	return nil
}
