package hash_worker

import (
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/spf13/afero"
	"mahresources/models"
)

// invertedImage builds a distinct image (color-inverted asymmetric layout) whose
// pHash is far from asymmetricImage's, so it must not match.
func invertedImage(w, h int) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			var c color.RGBA
			switch {
			case x < w/2 && y < h/2:
				c = color.RGBA{20, 20, 240, 255} // was red → now blue
			case x >= w/2 && y < h/2:
				c = color.RGBA{240, 20, 240, 255}
			case x < w/2 && y >= h/2:
				c = color.RGBA{240, 240, 20, 255}
			default:
				c = color.RGBA{10, 10, 10, 255}
			}
			img.Set(x, y, c)
		}
	}
	return img
}

// writeResourceImage creates a resource whose file lives in the worker's fs and
// returns it. content is the encoded image bytes.
func writeResourceImage(t *testing.T, w *HashWorker, name string, content []byte) models.Resource {
	t.Helper()
	r := models.Resource{Name: name, Location: name, ContentType: "image/jpeg"}
	if err := w.db.Create(&r).Error; err != nil {
		t.Fatalf("create resource: %v", err)
	}
	if err := afero.WriteFile(w.fs, r.GetCleanLocation(), content, 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	return r
}

// seedV1Row creates a legacy (hash_version NULL) image_hashes row for a resource.
func seedV1Row(t *testing.T, w *HashWorker, resourceID uint) {
	t.Helper()
	row := models.ImageHash{ResourceId: &resourceID, DHash: "abc", AHash: "def"}
	if err := w.db.Create(&row).Error; err != nil {
		t.Fatalf("seed v1 row: %v", err)
	}
}

func TestBackfill_ConvertsV1AndFindsPairs(t *testing.T) {
	w := testWorker(t)

	img := asymmetricImage(96, 96)
	// Two identical images → must match (p_distance 0) after backfill.
	jpegBytes := encodeJPEG(t, img, 92)
	r1 := writeResourceImage(t, w, "a.jpg", jpegBytes)
	r2 := writeResourceImage(t, w, "b.jpg", jpegBytes)
	// A distinct image → should not match the others.
	r3 := writeResourceImage(t, w, "c.jpg", encodeJPEG(t, invertedImage(96, 96), 92))

	seedV1Row(t, w, r1.ID)
	seedV1Row(t, w, r2.ID)
	seedV1Row(t, w, r3.ID)

	w.backfillV2Hashes()

	// All three rows should be converted to v2.
	var v1remaining int64
	w.db.Model(&models.ImageHash{}).Where("hash_version IS NULL").Count(&v1remaining)
	if v1remaining != 0 {
		t.Errorf("expected 0 v1 rows after backfill, got %d", v1remaining)
	}
	var v2count int64
	w.db.Model(&models.ImageHash{}).Where("hash_version = ?", 2).Count(&v2count)
	if v2count != 3 {
		t.Errorf("expected 3 v2 rows, got %d", v2count)
	}

	// r1 and r2 should be recorded as similar with a v2 p_distance.
	var pair models.ResourceSimilarity
	id1, id2 := r1.ID, r2.ID
	if id1 > id2 {
		id1, id2 = id2, id1
	}
	if err := w.db.Where("resource_id1 = ? AND resource_id2 = ?", id1, id2).First(&pair).Error; err != nil {
		t.Fatalf("expected similarity pair for r1,r2: %v", err)
	}
	if pair.PDistance == nil {
		t.Errorf("expected p_distance populated for backfilled pair")
	}

	// r3 should not pair with r1.
	var count int64
	a, b := r1.ID, r3.ID
	if a > b {
		a, b = b, a
	}
	w.db.Model(&models.ResourceSimilarity{}).Where("resource_id1 = ? AND resource_id2 = ?", a, b).Count(&count)
	if count != 0 {
		t.Errorf("expected r1 and r3 not to match, got %d pair(s)", count)
	}
}

func TestBackfill_MissingFileMarkedFailed(t *testing.T) {
	w := testWorker(t)

	// Resource row + v1 hash row, but no file on disk.
	r := models.Resource{Name: "gone.jpg", Location: "gone.jpg", ContentType: "image/jpeg"}
	if err := w.db.Create(&r).Error; err != nil {
		t.Fatalf("create resource: %v", err)
	}
	seedV1Row(t, w, r.ID)

	w.backfillV2Hashes()

	var row models.ImageHash
	if err := w.db.Where("resource_id = ?", r.ID).First(&row).Error; err != nil {
		t.Fatalf("load row: %v", err)
	}
	if !row.IsV2() {
		t.Errorf("expected failed row to be marked v2 (no infinite retry)")
	}
	if row.Status != models.HashStatusFailed {
		t.Errorf("status = %q, want failed", row.Status)
	}

	// A second pass must not pick it up again (hash_version is now set).
	var remaining int64
	w.db.Model(&models.ImageHash{}).Where("hash_version IS NULL").Count(&remaining)
	if remaining != 0 {
		t.Errorf("expected no rows left to backfill, got %d", remaining)
	}
}

func TestBackfill_RespectsPause(t *testing.T) {
	db := setupTestDB(t)
	paused := true
	w := New(db, afero.NewMemMapFs(), nil, Config{
		WorkerCount:           1,
		BatchSize:             100,
		PollInterval:          time.Hour,
		SimilarityThresholdFn: func() int { return 10 },
		AHashThresholdFn:      func() uint64 { return 5 },
		BackfillPausedFn:      func() bool { return paused },
		CacheSize:             100,
	}, nil)

	r := writeResourceImage(t, w, "a.jpg", encodeJPEG(t, asymmetricImage(64, 64), 90))
	seedV1Row(t, w, r.ID)

	// Paused: no conversion.
	w.backfillV2Hashes()
	var remaining int64
	w.db.Model(&models.ImageHash{}).Where("hash_version IS NULL").Count(&remaining)
	if remaining != 1 {
		t.Errorf("paused backfill converted rows: remaining = %d, want 1", remaining)
	}

	// Unpause: conversion proceeds.
	paused = false
	w.backfillV2Hashes()
	w.db.Model(&models.ImageHash{}).Where("hash_version IS NULL").Count(&remaining)
	if remaining != 0 {
		t.Errorf("unpaused backfill did not convert: remaining = %d, want 0", remaining)
	}
}
