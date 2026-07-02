package hash_worker

import (
	"testing"

	"mahresources/models"
)

// TestHashAndStore_UpsertsExistingRow: when a hash row already exists for the
// resource (e.g. a failed placeholder from an earlier attempt whose file has
// since been restored), hashAndStoreSimilarities must update it in place so the
// persisted row matches the hashes that were cached and matched — not silently
// keep the stale row.
func TestHashAndStore_UpsertsExistingRow(t *testing.T) {
	w := testWorker(t)
	r := writeResourceImage(t, w, "a.jpg", encodeJPEG(t, asymmetricImage(96, 96), 92))

	ver := HashVersionV2
	stale := models.ImageHash{ResourceId: &r.ID, HashVersion: &ver, Status: models.HashStatusFailed}
	if err := w.db.Create(&stale).Error; err != nil {
		t.Fatalf("seed stale row: %v", err)
	}

	w.hashAndStoreSimilarities(r)

	var row models.ImageHash
	if err := w.db.Where("resource_id = ?", r.ID).First(&row).Error; err != nil {
		t.Fatalf("load row: %v", err)
	}
	if row.ID != stale.ID {
		t.Errorf("row should be updated in place, got new id %d (was %d)", row.ID, stale.ID)
	}
	if row.Status != models.HashStatusOK {
		t.Errorf("status = %q, want ok", row.Status)
	}
	if row.PHashInt == nil || *row.PHashInt == 0 {
		t.Errorf("p_hash_int should be populated, got %v", row.PHashInt)
	}
	if row.DHashInt == nil || row.PChunk0 == nil {
		t.Errorf("legacy + chunk columns should be populated")
	}
}
