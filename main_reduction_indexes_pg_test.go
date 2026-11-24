//go:build postgres

package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"mahresources/internal/testpgutil"
	"mahresources/models"
)

// A second instance's startup must not wait on another instance's CONCURRENTLY
// index build: ANALYZE resource_similarities takes SHARE UPDATE EXCLUSIVE, the
// build holds a conflicting lock for minutes, so the post-migrate ANALYZE leaves
// that table to the background builder.
func TestPostMigrateAnalyzeDoesNotWaitOnASimilarityIndexBuild(t *testing.T) {
	ctx := context.Background()
	container, err := testpgutil.StartContainer(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = container.Stop(ctx) })
	db, _ := container.CreateTestDBWithDSN(t)
	if err := migrateModels(db); err != nil {
		t.Fatal(err)
	}
	tx := db.Begin()
	t.Cleanup(func() { tx.Rollback() })
	if err := tx.Exec("LOCK TABLE resource_similarities IN SHARE UPDATE EXCLUSIVE MODE").Error; err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- models.AnalyzePerceptualHashTables(db) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the post-migrate ANALYZE blocked on the table another build holds")
	}
}

// An index with the planned name on other columns or another expression is not
// the planned index: readiness refuses it and the boot rebuilds it.
func TestSimilarityIndexWithTheRightNameButOtherColumnsIsRebuilt(t *testing.T) {
	ctx := context.Background()
	container, err := testpgutil.StartContainer(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = container.Stop(ctx) })
	db, _ := container.CreateTestDBWithDSN(t)
	if err := migrateModels(db); err != nil {
		t.Fatal(err)
	}
	if err := models.EnsureSimilarityDistanceIndexes(db); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("DROP INDEX idx_sim_r1_eff_dist").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE INDEX idx_sim_r1_eff_dist ON resource_similarities (resource_id1)").Error; err != nil {
		t.Fatal(err)
	}
	if ok, err := models.SimilarityDistanceIndexesValid(db); err != nil || ok {
		t.Fatalf("readiness accepted the wrong definition: ok=%v err=%v", ok, err)
	}
	if err := models.EnsureSimilarityDistanceIndexes(db); err != nil {
		t.Fatal(err)
	}
	var def string
	if err := db.Raw("SELECT pg_get_indexdef('idx_sim_r1_eff_dist'::regclass)").Scan(&def).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(def), "coalesce") {
		t.Fatalf("the index was not rebuilt: %s", def)
	}
	if ok, err := models.SimilarityDistanceIndexesValid(db); err != nil || !ok {
		t.Fatalf("readiness refused the rebuilt index: ok=%v err=%v", ok, err)
	}
}
