//go:build postgres

package main

import (
	"context"
	"strings"
	"testing"

	"mahresources/internal/testpgutil"
	"mahresources/models"
)

// A cast on the columns changes the expression's type: an index over
// COALESCE(p_distance::text, hamming_distance::text) cannot serve the planned
// COALESCE(p_distance, hamming_distance), so readiness refuses it and the boot
// rebuilds it.
func TestSimilarityIndexWithATextCastIsRefusedAndRebuilt(t *testing.T) {
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
	if err := db.Exec("CREATE INDEX idx_sim_r1_eff_dist ON resource_similarities (resource_id1, (COALESCE(p_distance::text, hamming_distance::text)), resource_id2)").Error; err != nil {
		t.Fatal(err)
	}
	if ok, err := models.SimilarityDistanceIndexesValid(db); err != nil || ok {
		t.Fatalf("readiness accepted a text-cast expression: ok=%v err=%v", ok, err)
	}
	if err := models.EnsureSimilarityDistanceIndexes(db); err != nil {
		t.Fatal(err)
	}
	var def string
	if err := db.Raw("SELECT pg_get_indexdef('idx_sim_r1_eff_dist'::regclass)").Scan(&def).Error; err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(def), "::text") {
		t.Fatalf("the cast index was not rebuilt: %s", def)
	}
	if ok, err := models.SimilarityDistanceIndexesValid(db); err != nil || !ok {
		t.Fatalf("readiness refused the rebuilt index: ok=%v err=%v", ok, err)
	}
}
