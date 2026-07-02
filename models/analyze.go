package models

import "gorm.io/gorm"

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
	return db.Exec("ANALYZE image_hashes, resource_similarities").Error
}
