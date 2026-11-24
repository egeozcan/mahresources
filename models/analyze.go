package models

import (
	"strings"

	"gorm.io/gorm"
)

// AnalyzePerceptualHashTables refreshes Postgres planner statistics for the
// tables carrying the image-similarity-v2 indexed columns (image_hashes
// p_chunk0..3, resource_similarities p_distance). Newly added columns have no
// statistics until autoanalyze triggers at ~10% row churn; until then the
// planner wildly overestimates chunk-IN lookups and seq-scans the whole table
// for every v2 candidate query (observed: 334ms vs 0.135ms per query on a
// 2.1M-row deployment, throttling the backfill ~4x). Called from main.go right
// after AutoMigrate. No-op on SQLite, whose ANALYZE is a full table scan and
// would slow startup on large databases.
func AnalyzePerceptualHashTables(db *gorm.DB) error {
	if db.Dialector.Name() != "postgres" {
		return nil
	}
	if err := db.Exec("ANALYZE image_hashes").Error; err != nil {
		return err
	}
	// resource_similarities may be locked by another instance's CONCURRENTLY index
	// build (SHARE UPDATE EXCLUSIVE, which ANALYZE also takes). Waiting would hold
	// boot hostage to that build, so the ANALYZE gives up after a short
	// lock_timeout; the background index builder analyzes the table after its
	// build in that case.
	err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SET LOCAL lock_timeout = '2s'").Error; err != nil {
			return err
		}
		return tx.Exec("ANALYZE resource_similarities").Error
	})
	if err != nil && isLockTimeout(err) {
		return nil
	}
	return err
}

// isLockTimeout reports an expired lock_timeout (SQLSTATE 55P03).
func isLockTimeout(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "55P03") || strings.Contains(msg, "lock timeout")
}
