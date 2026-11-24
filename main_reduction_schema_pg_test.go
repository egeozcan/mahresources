//go:build postgres

package main

import (
	"context"
	"testing"

	"mahresources/internal/testpgutil"
	"mahresources/models"
)

func TestAutoMigrateLeavesLegacyReductionRowsAtSchemaZeroPostgres(t *testing.T) {
	ctx := context.Background()
	container, err := testpgutil.StartContainer(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = container.Stop(ctx) })
	db, _ := container.CreateTestDBWithDSN(t)
	assertAutoMigrateLeavesLegacyRowsAtSchemaZero(t, db)
}

// reduction_clusters.key is COLLATE "C" on Postgres, read from pg_attribute (the
// collation GORM has no tag for), and the statement is idempotent.
func TestReductionClusterKeyIsCCollatedOnPostgres(t *testing.T) {
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

	collation := func() string {
		var got []string
		if err := db.Raw(`SELECT coll.collname FROM pg_attribute a
			JOIN pg_collation coll ON coll.oid = a.attcollation
			WHERE a.attrelid = 'reduction_clusters'::regclass AND a.attname = 'key'`).Scan(&got).Error; err != nil || len(got) != 1 {
			t.Fatalf("read attcollation: %v %v", got, err)
		}
		return got[0]
	}
	if collation() == "C" {
		t.Skip("the database default collation is already C, so the statement proves nothing here")
	}
	for pass := 0; pass < 2; pass++ {
		if err := models.EnsureReductionKeyCollation(db); err != nil {
			t.Fatalf("pass %d: %v", pass, err)
		}
		if got := collation(); got != "C" {
			t.Fatalf("pass %d: key collation = %q, want C", pass, got)
		}
	}
	// And it orders as bytes: "B" < "a" under C, the reverse under en_US.
	if err := db.Exec(`INSERT INTO reduction_clusters (reduction_id, generation, "key") VALUES (1, 'g', 'a'), (1, 'g', 'B')`).Error; err != nil {
		t.Fatal(err)
	}
	var keys []string
	if err := db.Raw(`SELECT "key" FROM reduction_clusters ORDER BY "key"`).Scan(&keys).Error; err != nil {
		t.Fatal(err)
	}
	if len(keys) != 2 || keys[0] != "B" || keys[1] != "a" {
		t.Fatalf("key order = %v, want [B a]", keys)
	}
}

// The similarity indexes are built, valid, idempotent, and an index a failed
// concurrent build left invalid is dropped and rebuilt.
func TestSimilarityDistanceIndexesArePostgresValidAndRepaired(t *testing.T) {
	ctx := context.Background()
	container, err := testpgutil.StartContainer(ctx)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { _ = container.Stop(ctx) })
	db, _ := container.CreateTestDBWithDSN(t)
	assertSimilarityDistanceIndexes(t, db)

	if err := db.Exec(`UPDATE pg_index SET indisvalid = false WHERE indexrelid = 'idx_sim_r1_eff_dist'::regclass`).Error; err != nil {
		t.Fatal(err)
	}
	if ok, err := models.SimilarityDistanceIndexesValid(db); err != nil || ok {
		t.Fatalf("an invalid index read valid: %v %v", ok, err)
	}
	if err := models.EnsureSimilarityDistanceIndexes(db); err != nil {
		t.Fatal(err)
	}
	if ok, err := models.SimilarityDistanceIndexesValid(db); err != nil || !ok {
		t.Fatalf("the repair left it invalid: %v %v", ok, err)
	}
}
