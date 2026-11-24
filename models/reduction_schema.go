package models

import (
	"fmt"

	"gorm.io/gorm"
)

// Statements AutoMigrate cannot express for the plan-as-tables schema (design D1).

// EnsureReductionKeyCollation gives reduction_clusters.key COLLATE "C" on
// Postgres, so SQL's order over keys is the byte order Go sorts them in (the
// apply token digests Clusters in key order). GORM has no collation tag, so it
// is an explicit statement after AutoMigrate; it is a no-op once the column
// reads "C", so a boot does not rewrite the table's indexes again. SQLite's
// BINARY collation already compares bytes.
func EnsureReductionKeyCollation(db *gorm.DB) error {
	if db.Dialector.Name() != "postgres" {
		return nil
	}
	var collation []string
	if err := db.Raw(`SELECT coll.collname FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid
		JOIN pg_namespace n ON n.oid = c.relnamespace AND n.oid = (SELECT relnamespace FROM pg_class WHERE oid = 'reduction_clusters'::regclass)
		JOIN pg_collation coll ON coll.oid = a.attcollation
		WHERE c.relname = 'reduction_clusters' AND a.attname = 'key' AND NOT a.attisdropped`).Scan(&collation).Error; err != nil {
		return fmt.Errorf("inspect reduction_clusters.key collation: %w", err)
	}
	if len(collation) == 1 && collation[0] == "C" {
		return nil
	}
	if err := db.Exec(`ALTER TABLE reduction_clusters ALTER COLUMN "key" TYPE varchar(64) COLLATE "C"`).Error; err != nil {
		return fmt.Errorf("collate reduction_clusters.key: %w", err)
	}
	return nil
}

// similarityDistanceIndexLockKey is the Postgres advisory lock one server holds
// while it builds the similarity distance indexes.
const similarityDistanceIndexLockKey = int64(0x4D5253494D445831)

// similarityDistanceIndexes serve the Near tier's neighbour query in the order
// it reads (design D5): each pair direction by the effective distance the read
// path already uses, COALESCE(p_distance, hamming_distance), then the other
// id. No pair writer changes and nothing is backfilled.
var similarityDistanceIndexes = []plannedIndex{
	{"idx_sim_r1_eff_dist", "resource_similarities", "resource_id1, (COALESCE(p_distance, hamming_distance)), resource_id2"},
	{"idx_sim_r2_eff_dist", "resource_similarities", "resource_id2, (COALESCE(p_distance, hamming_distance)), resource_id1"},
}

// EnsureSimilarityDistanceIndexes creates the two effective-distance indexes
// on resource_similarities when missing, the way EnsureJobFilterIndexes does:
// CONCURRENTLY on one dedicated connection under a polling advisory lock on
// Postgres (an index a failed build left invalid is dropped and rebuilt), and a
// plain build at startup on SQLite, which holds the writer lock for one table
// scan. Idempotent: IF NOT EXISTS, run every boot after AutoMigrate. Postgres
// then needs ANALYZE, which the caller runs.
func EnsureSimilarityDistanceIndexes(db *gorm.DB) error {
	return ensurePlannedIndexes(db, "similarity distance indexes", similarityDistanceIndexLockKey, similarityDistanceIndexes)
}

// EnsureSimilarityDistanceIndexesOnOwnConnection is EnsureSimilarityDistanceIndexes
// on Postgres through a one-connection handle of its own, for the reason
// EnsureJobFilterIndexesOnOwnConnection gives.
func EnsureSimilarityDistanceIndexesOnOwnConnection(dsn string) error {
	return onOwnConnection(dsn, "similarity distance index", func(db *gorm.DB) error {
		if err := EnsureSimilarityDistanceIndexes(db); err != nil {
			return err
		}
		return db.Exec("ANALYZE resource_similarities").Error
	})
}

// SimilarityDistanceIndexesValid reports whether both indexes exist and, on
// Postgres, read valid. A compute is refused until they do: without them the
// neighbour query is a sort over a hub's whole star.
func SimilarityDistanceIndexesValid(db *gorm.DB) (bool, error) {
	for _, index := range similarityDistanceIndexes {
		var defs []string
		var err error
		if db.Dialector.Name() == "postgres" {
			// Valid, plain (not unique) and exactly the planned columns: an index with
			// the planned name on other columns serves nothing.
			err = db.Raw(`SELECT pg_get_indexdef(i.indexrelid) FROM pg_class c JOIN pg_index i ON i.indexrelid = c.oid
				WHERE c.relname = ? AND i.indrelid = ?::regclass AND i.indisvalid AND NOT i.indisunique`, index.name, index.table).Scan(&defs).Error
		} else {
			err = db.Raw(`SELECT sql FROM sqlite_master WHERE type = 'index' AND name = ? AND tbl_name = ?`, index.name, index.table).Scan(&defs).Error
		}
		if err != nil {
			return false, err
		}
		if len(defs) != 1 || !index.definitionMatches(defs[0]) {
			return false, nil
		}
	}
	return true, nil
}
