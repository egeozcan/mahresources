package models

import (
	"fmt"
	"strings"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// jobFilterIndex is one index the Job list filters are planned around, created
// outside AutoMigrate.
type jobFilterIndex struct {
	name    string
	table   string
	columns string
}

// jobFilterIndexes serve the list filters added after the jobs tables were
// created:
//
//   - idx_jobs_state_phase and idx_jobs_visible_phase find the "partially
//     completed" Jobs (succeeded with phase partial), which can be a sliver of
//     the succeeded ones, for an administrator (whose reads carry no visibility
//     predicate) and an owner (whose reads lead with it, as idx_jobs_visible
//     does). accepted_at and id close both, as they close the other listing
//     indexes, so the newest-first page is read in index order.
//   - idx_job_links_inbound reads a link from its TO end ("has been retried",
//     "has not been retried"). The unique index leads with type and then from,
//     so it cannot narrow on to_job_id; this one carries from_job_id as well so
//     it covers the far-endpoint join, because SQLite prefers a covering index
//     and without it picked the unique one anyway.
var jobFilterIndexes = []jobFilterIndex{
	{"idx_jobs_state_phase", "jobs", "state, phase, accepted_at, id"},
	{"idx_jobs_visible_phase", "jobs", "visibility_class, owner_user_id, state, phase, accepted_at, id"},
	{"idx_job_links_inbound", "job_links", "to_job_id, type, from_job_id"},
}

// jobFilterIndexLockKey is the PostgreSQL advisory lock one server holds while
// it builds these indexes.
const jobFilterIndexLockKey = int64(0x4D524A4F42494458)

// EnsureJobFilterIndexes creates the Job list filter indexes that are missing.
//
// They are not AutoMigrate tags because AutoMigrate builds an index with a plain
// CREATE INDEX, and on PostgreSQL that holds a lock blocking every write to the
// table for as long as the build takes — minutes, on a jobs table of millions of
// rows, while other servers sharing the database are trying to record work. On
// PostgreSQL they are built CONCURRENTLY instead, which does not block writes;
// on SQLite, which has one writer anyway, a plain build is the same thing.
//
// A concurrent build that failed leaves an invalid index behind under the name,
// which IF NOT EXISTS would then skip forever, so an invalid one is dropped and
// built again — as is a valid one on other columns, which a name check alone
// would accept. An in-progress build also reads as invalid, which is why the whole
// pass runs under an advisory lock on one reserved connection: a server that
// finds another one building waits for it and then checks the result itself,
// rather than skipping and trusting a build that may yet fail. On PostgreSQL the
// server runs this off the startup path, on a connection of its own
// (EnsureJobFilterIndexesOnOwnConnection, from main's migrateJobCore); the
// filters are correct, only slower, until it finishes.
func EnsureJobFilterIndexes(db *gorm.DB) error {
	switch db.Dialector.Name() {
	case "postgres":
		return db.Connection(func(conn *gorm.DB) error {
			conn = conn.Session(&gorm.Session{})
			if err := conn.Exec("SELECT pg_advisory_lock(?)", jobFilterIndexLockKey).Error; err != nil {
				return fmt.Errorf("lock job filter indexes: %w", err)
			}
			defer conn.Exec("SELECT pg_advisory_unlock(?)", jobFilterIndexLockKey)
			for _, index := range jobFilterIndexes {
				if err := ensurePostgresJobFilterIndex(conn, index); err != nil {
					return err
				}
			}
			return nil
		})
	default:
		for _, index := range jobFilterIndexes {
			var table []string
			if err := db.Raw("SELECT tbl_name FROM sqlite_master WHERE type = 'index' AND name = ?", index.name).Scan(&table).Error; err != nil {
				return fmt.Errorf("inspect %s: %w", index.name, err)
			}
			var columns []string
			if err := db.Raw("SELECT name FROM pragma_index_info(?) ORDER BY seqno", index.name).Scan(&columns).Error; err != nil {
				return fmt.Errorf("inspect %s: %w", index.name, err)
			}
			if len(table) == 1 && table[0] == index.table && strings.Join(columns, ", ") == index.columns {
				continue
			}
			// Missing, or under the name on another table or other columns.
			if err := db.Exec("DROP INDEX IF EXISTS " + index.name).Error; err != nil {
				return fmt.Errorf("drop %s: %w", index.name, err)
			}
			statement := fmt.Sprintf("CREATE INDEX %s ON %s (%s)", index.name, index.table, index.columns)
			if err := db.Exec(statement).Error; err != nil {
				return fmt.Errorf("create %s: %w", index.name, err)
			}
		}
		return nil
	}
}

// EnsureJobFilterIndexesOnOwnConnection is EnsureJobFilterIndexes on PostgreSQL
// through a one-connection handle of its own, opened from dsn and closed when the
// pass ends. The pass holds its connection for as long as it waits on another
// server's build and then builds three indexes; borrowed from the application's
// pool, that is a connection requests cannot have for minutes, and with a pool
// of one (-max-db-connections=1) it is all of them.
func EnsureJobFilterIndexesOnOwnConnection(dsn string) error {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return fmt.Errorf("open the job filter index connection: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("open the job filter index connection: %w", err)
	}
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)
	return EnsureJobFilterIndexes(db)
}

func ensurePostgresJobFilterIndex(conn *gorm.DB, index jobFilterIndex) error {
	var found []struct {
		Valid bool
		Def   string
	}
	if err := conn.Raw(`SELECT i.indisvalid AS valid, pg_get_indexdef(i.indexrelid) AS def FROM pg_class c
		JOIN pg_index i ON i.indexrelid = c.oid
		JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relname = ? AND n.nspname = current_schema()`, index.name).Scan(&found).Error; err != nil {
		return fmt.Errorf("inspect %s: %w", index.name, err)
	}
	// The expected shape is a plain btree on the table over exactly these
	// columns; pg_get_indexdef spells it "... ON <schema>.<table> USING btree
	// (<columns>)".
	shape := fmt.Sprintf(" USING btree (%s)", index.columns)
	if len(found) == 1 && found[0].Valid && !strings.HasPrefix(found[0].Def, "CREATE UNIQUE") &&
		strings.HasSuffix(found[0].Def, shape) && strings.Contains(found[0].Def, "."+index.table+" USING") {
		return nil
	}
	if len(found) == 1 {
		// Invalid — left behind by a concurrent build that failed, and this
		// server holds the lock, so nothing else is building it — or valid on
		// other columns, which is not the index the filters are planned around.
		if err := conn.Exec("DROP INDEX CONCURRENTLY IF EXISTS " + index.name).Error; err != nil {
			return fmt.Errorf("drop %s: %w", index.name, err)
		}
	}
	statement := fmt.Sprintf("CREATE INDEX CONCURRENTLY IF NOT EXISTS %s ON %s (%s)", index.name, index.table, index.columns)
	if err := conn.Exec(statement).Error; err != nil {
		return fmt.Errorf("create %s: %w", index.name, err)
	}
	return nil
}
