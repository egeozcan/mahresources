package hash_worker

import (
	"fmt"
	"log"
	"sync"

	"github.com/Nr90/imgsim"
	"mahresources/models"
)

// backfillV2Hashes incrementally converts existing v1 image_hashes rows (and
// previously-failed placeholder rows, which also have hash_version IS NULL) to
// v2: it re-decodes each image, computes the v2 fields, updates the row in place,
// and runs v2 chunk-index matching. It processes one batch (config.BatchSize) of
// the newest resources per call and is fully resumable — the hash_version IS NULL
// predicate is the cursor, and no transaction spans more than a single row.
//
// The 2.18M-image mahlayf deployment relies on this: it must never require a
// large one-shot migration, and must be pausable via the runtime setting.
func (w *HashWorker) backfillV2Hashes() {
	if w.config.BackfillPausedFn != nil && w.config.BackfillPausedFn() {
		return
	}

	// Newest first: most user-visible resources get the better hash earliest.
	// A resource has at most one image_hashes row (resource_id is unique), so the
	// join yields one row per resource.
	var resources []models.Resource
	if err := w.db.
		Joins("JOIN image_hashes ON image_hashes.resource_id = resources.id").
		Where("image_hashes.hash_version IS NULL").
		Where("resources.content_type IN ?", hashableContentTypesList).
		Order("resources.id DESC").
		Limit(w.config.BatchSize).
		Find(&resources).Error; err != nil {
		w.logError(fmt.Sprintf("Hash worker: error finding rows to backfill: %v", err), nil)
		return
	}

	if len(resources) == 0 {
		return
	}

	var totalRemaining int64
	w.db.Model(&models.ImageHash{}).
		Joins("JOIN resources ON resources.id = image_hashes.resource_id").
		Where("image_hashes.hash_version IS NULL").
		Where("resources.content_type IN ?", hashableContentTypesList).
		Count(&totalRemaining)

	w.logProgress(fmt.Sprintf("Hash worker: backfilling %d v2 hashes (remaining: %d)", len(resources), totalRemaining),
		map[string]interface{}{"batch_size": len(resources), "remaining": totalRemaining})

	sem := make(chan struct{}, w.config.WorkerCount)
	var wg sync.WaitGroup
	for _, resource := range resources {
		sem <- struct{}{}
		wg.Add(1)
		go func(r models.Resource) {
			defer wg.Done()
			defer func() { <-sem }()
			w.backfillOne(r)
		}(resource)
	}
	wg.Wait()
}

// backfillOne re-hashes a single existing row to v2 in place. A file that is
// missing or cannot be decoded is marked failed with hash_version=2 so it is not
// retried on the next cycle (exactly one retry for previously-failed rows).
func (w *HashWorker) backfillOne(resource models.Resource) {
	data, err := w.readResourceBytes(resource)
	if err != nil {
		log.Printf("Hash worker: backfill read error for resource %d: %v", resource.ID, err)
		w.markBackfillFailed(resource.ID)
		return
	}

	v2, err := ComputeV2Hashes(data)
	if err != nil {
		log.Printf("Hash worker: backfill hash error for resource %d: %v", resource.ID, err)
		w.markBackfillFailed(resource.ID)
		return
	}

	if err := w.db.Model(&models.ImageHash{}).
		Where("resource_id = ?", resource.ID).
		Updates(v2HashColumns(v2)).Error; err != nil {
		log.Printf("Hash worker: backfill update error for resource %d: %v", resource.ID, err)
		return
	}

	if v2.Status == models.HashStatusOK {
		w.findSimilaritiesV2(resource.ID, v2.PHash, v2.LegacyDHash, v2.LegacyAHash)
	}
}

// markBackfillFailed converts an existing row to a failed v2 row in place.
func (w *HashWorker) markBackfillFailed(resourceID uint) {
	if err := w.db.Model(&models.ImageHash{}).
		Where("resource_id = ?", resourceID).
		Updates(map[string]any{
			"hash_version": HashVersionV2,
			"status":       models.HashStatusFailed,
		}).Error; err != nil {
		log.Printf("Hash worker: error marking resource %d backfill-failed: %v", resourceID, err)
	}
}

// v2HashColumns returns the image_hashes column values for a v2 result, for use
// with GORM's map-based Updates (which, unlike struct updates, writes zero values).
func v2HashColumns(v2 *V2Hashes) map[string]any {
	chunks := SplitChunks(v2.PHash)
	legacyDHash := imgsim.Hash(v2.LegacyDHash)
	legacyAHash := imgsim.Hash(v2.LegacyAHash)
	return map[string]any{
		"hash_version": HashVersionV2,
		"p_hash_int":   int64(v2.PHash),
		"p_chunk0":     int32(chunks[0]),
		"p_chunk1":     int32(chunks[1]),
		"p_chunk2":     int32(chunks[2]),
		"p_chunk3":     int32(chunks[3]),
		"status":       v2.Status,
		"d_hash":       legacyDHash.String(),
		"a_hash":       legacyAHash.String(),
		"d_hash_int":   int64(v2.LegacyDHash),
		"a_hash_int":   int64(v2.LegacyAHash),
	}
}
