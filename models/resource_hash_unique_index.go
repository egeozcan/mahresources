package models

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// ResourceHashUniqueIndexName is the partial unique index that makes content
// deduplication hold across processes.
const ResourceHashUniqueIndexName = "idx_resources_hash_unique"

// EnsureResourceHashUnique adds a unique index over non-empty resources.hash.
//
// Deduplication by content hash was only ever guaranteed within one process: it
// rests on the in-memory per-hash idlock in AddResource, and Resource.Hash
// carries a plain `gorm:"index"`. Two processes sharing one database hold two
// idlocks, so both could read "not found" for the same bytes and both insert,
// and nothing below them said no. The client-side bulk upload widget makes that
// race ordinary rather than theoretical, since a batch is now many concurrent
// requests instead of one sequential loop.
//
// Three properties are deliberate:
//
//   - **Partial** — `WHERE hash <> ”`. Rows that were never hashed all carry
//     the empty string, and an unqualified unique index would reject every one
//     of them after the first.
//   - **Its own name**, leaving GORM's `idx_resources_hash` alone. AutoMigrate
//     never upgrades an existing index's uniqueness in place (see
//     EnsureImageHashResourceIdUnique), so reusing that name means fighting it
//     on every migration.
//   - **It never deletes anything.** A database that already holds duplicate
//     hashes keeps them: the index is skipped and the collisions are named in
//     the returned error, which main logs as a warning. Merging two resources is
//     a decision with a UI behind it, not one a boot-time fixup should make on
//     an operator's behalf — which is where this differs from
//     EnsureImageHashResourceIdUnique, whose rows are derived data. Once they
//     are merged, the next boot creates the index.
//
// Idempotent, and a cheap no-op once the index exists.
func EnsureResourceHashUnique(db *gorm.DB) error {
	exists, err := resourceHashUniqueIndexExists(db)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	// Only reached while the index is missing, so this scan is paid once per
	// deployment rather than on every startup.
	var duplicates []string
	if err := db.Raw(`
		SELECT hash
		FROM resources
		WHERE hash <> ''
		GROUP BY hash
		HAVING COUNT(*) > 1
		LIMIT 10
	`).Scan(&duplicates).Error; err != nil {
		return err
	}

	if len(duplicates) > 0 {
		return fmt.Errorf(
			"resources already contains duplicate content hashes, so the unique index was not created "+
				"and deduplication stays process-local: merge the resources sharing %s (and any others) "+
				"and restart to enable it",
			strings.Join(duplicates, ", "),
		)
	}

	return db.Exec(fmt.Sprintf(
		`CREATE UNIQUE INDEX IF NOT EXISTS %s ON resources (hash) WHERE hash <> ''`,
		ResourceHashUniqueIndexName,
	)).Error
}

func resourceHashUniqueIndexExists(db *gorm.DB) (bool, error) {
	var query string
	switch db.Dialector.Name() {
	case "postgres":
		query = `SELECT count(*) FROM pg_class WHERE relname = ? AND relkind = 'i'`
	default: // sqlite
		query = `SELECT count(*) FROM sqlite_master WHERE type = 'index' AND name = ?`
	}

	var found int64
	if err := db.Raw(query, ResourceHashUniqueIndexName).Scan(&found).Error; err != nil {
		return false, err
	}
	return found > 0, nil
}
