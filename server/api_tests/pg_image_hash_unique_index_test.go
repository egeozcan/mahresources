//go:build postgres

package api_tests

import (
	"testing"

	"gorm.io/gorm/clause"

	"mahresources/models"
)

// TestEnsureImageHashResourceIdUnique_HealsNonUniqueIndex reproduces the
// production incident on mahlayf: a deployment whose image_hashes.resource_id
// carries a *non-unique* index (GORM's AutoMigrate never upgrades an existing
// index's uniqueness in place). The hash worker's ON CONFLICT (resource_id)
// upsert then fails with SQLSTATE 42P10 on every save, re-hashing the same
// resources forever and pinning the CPU. The missing constraint also lets
// duplicate resource_id rows accumulate.
//
// The test forces that broken state, asserts the upsert genuinely fails, runs
// the self-healing fixup, and asserts the dupes are gone, a unique index exists,
// and the upsert now succeeds.
func TestEnsureImageHashResourceIdUnique_HealsNonUniqueIndex(t *testing.T) {
	tc := SetupPostgresTestEnv(t)
	db := tc.DB

	// A resource to satisfy the image_hashes -> resources FK.
	res := &models.Resource{Name: "img", ContentType: "image/jpeg", ResourceCategoryId: 1}
	if err := db.Create(res).Error; err != nil {
		t.Fatalf("create resource: %v", err)
	}

	// Recreate the legacy broken state: drop the correct unique index that
	// AutoMigrate created and replace it with a plain (non-unique) index of the
	// same name, then insert two rows for the same resource_id.
	if err := db.Exec(`DROP INDEX IF EXISTS idx_image_hashes_resource_id`).Error; err != nil {
		t.Fatalf("drop unique index: %v", err)
	}
	if err := db.Exec(`CREATE INDEX idx_image_hashes_resource_id ON image_hashes (resource_id)`).Error; err != nil {
		t.Fatalf("create non-unique index: %v", err)
	}
	ver := 2 // HashVersionV2
	for i := 0; i < 2; i++ {
		if err := db.Create(&models.ImageHash{ResourceId: &res.ID, HashVersion: &ver, Status: models.HashStatusOK}).Error; err != nil {
			t.Fatalf("seed duplicate hash row %d: %v", i, err)
		}
	}

	// Precondition: the upsert the hash worker uses must fail against the
	// non-unique index (this is the actual production symptom, SQLSTATE 42P10).
	upsert := func() error {
		row := &models.ImageHash{ResourceId: &res.ID, HashVersion: &ver, Status: models.HashStatusOK}
		return db.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "resource_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"status"}),
		}).Create(row).Error
	}
	if err := upsert(); err == nil {
		t.Fatal("expected ON CONFLICT (resource_id) upsert to fail before the fixup, but it succeeded")
	}

	// Heal.
	if err := models.EnsureImageHashResourceIdUnique(db); err != nil {
		t.Fatalf("EnsureImageHashResourceIdUnique: %v", err)
	}

	// Duplicates collapsed to exactly one row for the resource.
	var count int64
	if err := db.Model(&models.ImageHash{}).Where("resource_id = ?", res.ID).Count(&count).Error; err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 1 {
		t.Errorf("after fixup: %d rows for resource_id %d, want 1", count, res.ID)
	}

	// A unique index on exactly (resource_id) now exists.
	var uniqueIdx int64
	if err := db.Raw(`
		SELECT count(*)
		FROM pg_index i
		JOIN pg_class t ON t.oid = i.indrelid
		JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = i.indkey[0]
		WHERE t.relname = 'image_hashes' AND i.indisunique AND i.indnatts = 1 AND a.attname = 'resource_id'
	`).Scan(&uniqueIdx).Error; err != nil {
		t.Fatalf("probe unique index: %v", err)
	}
	if uniqueIdx == 0 {
		t.Error("after fixup: no unique index on image_hashes(resource_id)")
	}

	// The upsert now works (the fix that stops the CPU spin).
	if err := upsert(); err != nil {
		t.Errorf("ON CONFLICT (resource_id) upsert still fails after fixup: %v", err)
	}

	// Idempotent: a second run is a clean no-op.
	if err := models.EnsureImageHashResourceIdUnique(db); err != nil {
		t.Errorf("second EnsureImageHashResourceIdUnique (idempotency): %v", err)
	}
}
