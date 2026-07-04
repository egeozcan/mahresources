package models

import "gorm.io/gorm"

// EnsureImageHashResourceIdUnique repairs Postgres deployments where
// image_hashes.resource_id carries a *non-unique* index instead of the unique
// index the model declares (gorm:"uniqueIndex" on ImageHash.ResourceId).
//
// GORM's AutoMigrate never upgrades an existing index's uniqueness in place: any
// database first created with a plain index named idx_image_hashes_resource_id
// keeps that non-unique index forever. The hash worker's upsert then does
// ON CONFLICT (resource_id), which Postgres rejects with SQLSTATE 42P10
// ("no unique or exclusion constraint matching the ON CONFLICT specification")
// on every save. No hash row is ever written, so the same resources are
// re-hashed every poll cycle — a permanent CPU spin (observed on a 2.1M-row
// deployment pinning ~3 cores). The same missing constraint also lets duplicate
// resource_id rows accumulate (the markResourceFailed path uses a no-target
// ON CONFLICT DO NOTHING, which inserts unconditionally without a unique index).
//
// This fixup is idempotent and self-healing: it dedups any rows sharing a
// resource_id (keeping the most complete row) and replaces the non-unique index
// with a unique one of the same name GORM expects, so subsequent AutoMigrate
// runs treat it as already present. It is a no-op once the unique index exists,
// and a no-op on SQLite (whose AutoMigrate creates the unique index correctly).
// Called from main.go right after AutoMigrate.
func EnsureImageHashResourceIdUnique(db *gorm.DB) error {
	if db.Dialector.Name() != "postgres" {
		return nil
	}

	// Already have a unique index on exactly (resource_id)? Nothing to do — this
	// keeps steady-state boots cheap (no full-table window scan every startup).
	var uniqueCount int64
	if err := db.Raw(`
		SELECT count(*)
		FROM pg_index i
		JOIN pg_class t ON t.oid = i.indrelid
		JOIN pg_attribute a ON a.attrelid = t.oid AND a.attnum = i.indkey[0]
		WHERE t.relname = 'image_hashes'
		  AND i.indisunique
		  AND i.indnatts = 1
		  AND a.attname = 'resource_id'
	`).Scan(&uniqueCount).Error; err != nil {
		return err
	}
	if uniqueCount > 0 {
		return nil
	}

	return db.Transaction(func(tx *gorm.DB) error {
		// 1. Deduplicate: keep the single best row per resource_id (a real v2 hash
		//    over a legacy/failed placeholder, then the newest), delete the rest.
		if err := tx.Exec(`
			DELETE FROM image_hashes ih
			USING (
				SELECT id,
				       row_number() OVER (
				           PARTITION BY resource_id
				           ORDER BY (p_hash_int IS NOT NULL) DESC,
				                    (status = 'ok') DESC,
				                    (d_hash_int IS NOT NULL) DESC,
				                    id DESC
				       ) AS rn
				FROM image_hashes
				WHERE resource_id IS NOT NULL
			) d
			WHERE ih.id = d.id AND d.rn > 1
		`).Error; err != nil {
			return err
		}

		// 2. Drop the stale non-unique index (the name AutoMigrate uses for the
		//    uniqueIndex tag) so we can recreate it as unique under the same name.
		if err := tx.Exec(`DROP INDEX IF EXISTS idx_image_hashes_resource_id`).Error; err != nil {
			return err
		}

		// 3. Create the unique index under the name GORM's uniqueIndex tag expects,
		//    so future AutoMigrate runs find it and ON CONFLICT (resource_id) works.
		if err := tx.Exec(`
			CREATE UNIQUE INDEX IF NOT EXISTS idx_image_hashes_resource_id
			ON image_hashes (resource_id)
		`).Error; err != nil {
			return err
		}

		return nil
	})
}
