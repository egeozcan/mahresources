package models

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestSimilarityIndexReadinessChecksTheDefinitionOnSQLite(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:simidx?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE resource_similarities (resource_id1 integer, resource_id2 integer, p_distance integer, hamming_distance integer)").Error; err != nil {
		t.Fatal(err)
	}
	if err := EnsureSimilarityDistanceIndexes(db); err != nil {
		t.Fatal(err)
	}
	if ok, err := SimilarityDistanceIndexesValid(db); err != nil || !ok {
		t.Fatalf("fresh indexes: ok=%v err=%v", ok, err)
	}
	db.Exec("DROP INDEX idx_sim_r1_eff_dist")
	if err := db.Exec("CREATE INDEX idx_sim_r1_eff_dist ON resource_similarities (resource_id1)").Error; err != nil {
		t.Fatal(err)
	}
	if ok, _ := SimilarityDistanceIndexesValid(db); ok {
		t.Fatal("readiness accepted an index with the right name on other columns")
	}
	if err := EnsureSimilarityDistanceIndexes(db); err != nil {
		t.Fatal(err)
	}
	if ok, err := SimilarityDistanceIndexesValid(db); err != nil || !ok {
		t.Fatalf("after the repair: ok=%v err=%v", ok, err)
	}
}
