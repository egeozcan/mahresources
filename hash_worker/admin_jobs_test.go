package hash_worker

import (
	"testing"

	"mahresources/models"
)

func TestRetryFailedHashes(t *testing.T) {
	w := testWorker(t)

	// A failed row and a healthy v2 row.
	ver := HashVersionV2
	failed := models.ImageHash{ResourceId: uptr(1), HashVersion: &ver, Status: models.HashStatusFailed}
	ok := models.ImageHash{ResourceId: uptr(2), HashVersion: &ver, Status: models.HashStatusOK}
	if err := w.db.Create(&failed).Error; err != nil {
		t.Fatal(err)
	}
	if err := w.db.Create(&ok).Error; err != nil {
		t.Fatal(err)
	}

	reset, err := RetryFailedHashes(w.db)
	if err != nil {
		t.Fatalf("RetryFailedHashes: %v", err)
	}
	if reset != 1 {
		t.Errorf("reset = %d, want 1", reset)
	}

	var reloaded models.ImageHash
	w.db.First(&reloaded, failed.ID)
	if reloaded.HashVersion != nil {
		t.Errorf("failed row hash_version should be NULL after retry, got %v", *reloaded.HashVersion)
	}
	if reloaded.Status != "" {
		t.Errorf("failed row status should be cleared, got %q", reloaded.Status)
	}

	// The healthy row is untouched.
	var okReloaded models.ImageHash
	w.db.First(&okReloaded, ok.ID)
	if okReloaded.HashVersion == nil || okReloaded.Status != models.HashStatusOK {
		t.Errorf("healthy row should be untouched")
	}
}

func uptr(v uint) *uint { return &v }

func TestRecomputeV2Pairs(t *testing.T) {
	w := testWorker(t)

	// Two similar (pHash distance 0) and one distinct.
	insertV2Hash(t, w, 1, 0x0, 0, 0, models.HashStatusOK)
	insertV2Hash(t, w, 2, 0x0, 0, 0, models.HashStatusOK)
	insertV2Hash(t, w, 3, 0xFFFFFFFFFFFFFFFF, 0, 0, models.HashStatusOK)

	// A stale/incorrect v2-v2 pair that recompute must delete and rebuild.
	badP := uint8(63)
	stale := models.ResourceSimilarity{ResourceID1: 1, ResourceID2: 3, HammingDistance: 63, PDistance: &badP}
	if err := w.db.Create(&stale).Error; err != nil {
		t.Fatal(err)
	}

	var lastDone, lastTotal int64
	if err := RecomputeV2Pairs(w.db, 100, nil, func(done, total int64) {
		lastDone, lastTotal = done, total
	}); err != nil {
		t.Fatalf("RecomputeV2Pairs: %v", err)
	}

	// The stale (1,3) pair (distance 64 > 11) must be gone.
	var badCount int64
	w.db.Model(&models.ResourceSimilarity{}).Where("resource_id1 = 1 AND resource_id2 = 3").Count(&badCount)
	if badCount != 0 {
		t.Errorf("stale (1,3) pair should be deleted, found %d", badCount)
	}

	// The (1,2) pair (distance 0) must exist with p_distance 0.
	var good models.ResourceSimilarity
	if err := w.db.Where("resource_id1 = 1 AND resource_id2 = 2").First(&good).Error; err != nil {
		t.Fatalf("expected (1,2) pair: %v", err)
	}
	if good.PDistance == nil || *good.PDistance != 0 {
		t.Errorf("(1,2) p_distance = %v, want 0", good.PDistance)
	}

	if lastTotal != 3 {
		t.Errorf("progress total = %d, want 3", lastTotal)
	}
	if lastDone != 3 {
		t.Errorf("progress done = %d, want 3", lastDone)
	}
}

func TestRecomputeV2Pairs_ConcurrentGuard(t *testing.T) {
	w := testWorker(t)
	// Simulate an in-flight run.
	if !recomputeInFlight.CompareAndSwap(false, true) {
		t.Fatal("expected to acquire recompute guard")
	}
	defer recomputeInFlight.Store(false)

	err := RecomputeV2Pairs(w.db, 100, nil, nil)
	if err != ErrRecomputeInProgress {
		t.Errorf("expected ErrRecomputeInProgress, got %v", err)
	}
}
