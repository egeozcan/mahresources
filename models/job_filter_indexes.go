package models

import (
	"fmt"
	"strings"
	"time"

	"mahresources/constants"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// plannedIndex is one index a list query is planned around, created outside
// AutoMigrate (see EnsureJobFilterIndexes for why).
type plannedIndex struct {
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
//   - idx_jobs_state_entered and idx_jobs_visible_state_entered read one state
//     newest state change first, for an administrator and for an owner: the
//     listing in state-entered order reads each state of its filter as one
//     such seek (jobs.orderedBranches), which is how the Jobs panel reads its
//     groups on every refresh. idx_jobs_state_phase_entered and
//     idx_jobs_visible_phase_entered are their twins for the partial token,
//     which is a sliver of the succeeded Jobs, and idx_jobs_owner_state_entered
//     serves an administrator's own Jobs (owner=me), whose reads carry no
//     visibility predicate to lead with.
var jobFilterIndexes = []plannedIndex{
	{"idx_jobs_state_phase", "jobs", "state, phase, accepted_at, id"},
	{"idx_jobs_visible_phase", "jobs", "visibility_class, owner_user_id, state, phase, accepted_at, id"},
	{"idx_job_links_inbound", "job_links", "to_job_id, type, from_job_id"},
	{"idx_jobs_state_entered", "jobs", "state, state_entered_at, id"},
	{"idx_jobs_visible_state_entered", "jobs", "visibility_class, owner_user_id, state, state_entered_at, id"},
	{"idx_jobs_state_phase_entered", "jobs", "state, phase, state_entered_at, id"},
	{"idx_jobs_visible_phase_entered", "jobs", "visibility_class, owner_user_id, state, phase, state_entered_at, id"},
	{"idx_jobs_owner_state_entered", "jobs", "owner_user_id, state, state_entered_at, id"},
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
	return ensurePlannedIndexes(db, "job filter indexes", jobFilterIndexLockKey, jobFilterIndexes)
}

// ensurePlannedIndexes is EnsureJobFilterIndexes for any set of planned
// indexes; each set takes its own advisory lock.
func ensurePlannedIndexes(db *gorm.DB, what string, lockKey int64, indexes []plannedIndex) error {
	switch db.Dialector.Name() {
	case "postgres":
		return db.Connection(func(conn *gorm.DB) error {
			conn = conn.Session(&gorm.Session{})
			if err := pollAdvisoryLock(conn, lockKey); err != nil {
				return fmt.Errorf("lock %s: %w", what, err)
			}
			defer conn.Exec("SELECT pg_advisory_unlock(?)", lockKey)
			for _, index := range indexes {
				if err := ensurePostgresPlannedIndex(conn, index); err != nil {
					return err
				}
			}
			return nil
		})
	default:
		for _, index := range indexes {
			var table []string
			if err := db.Raw("SELECT tbl_name FROM sqlite_master WHERE type = 'index' AND name = ?", index.name).Scan(&table).Error; err != nil {
				return fmt.Errorf("inspect %s: %w", index.name, err)
			}
			var columns []string
			if err := db.Raw("SELECT name FROM pragma_index_info(?) ORDER BY seqno", index.name).Scan(&columns).Error; err != nil {
				return fmt.Errorf("inspect %s: %w", index.name, err)
			}
			// A unique or partial index on the right columns is not the index
			// either, as the PostgreSQL check below also refuses: a partial one
			// serves only the rows its WHERE admits.
			var plain []bool
			if len(table) == 1 {
				if err := db.Raw(`SELECT "unique" = 0 AND partial = 0 FROM pragma_index_list(?) WHERE name = ?`, table[0], index.name).Scan(&plain).Error; err != nil {
					return fmt.Errorf("inspect %s: %w", index.name, err)
				}
			}
			if len(table) == 1 && table[0] == index.table && strings.Join(columns, ", ") == index.columns && len(plain) == 1 && plain[0] {
				continue
			}
			// Missing, or under the name on another table, other columns, or with a
			// UNIQUE or a WHERE.
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

// pollAdvisoryLock takes a session advisory lock by polling pg_try_advisory_lock
// rather than blocking in pg_advisory_lock. CREATE INDEX CONCURRENTLY waits for
// every transaction older than its own, and a session blocked in
// pg_advisory_lock is one: a second server waiting for the lock and the first
// server's build then wait on each other until Postgres aborts one as a
// deadlock. Between polls the waiter has no statement running.
func pollAdvisoryLock(conn *gorm.DB, key int64) error {
	for {
		var locked bool
		if err := conn.Raw("SELECT pg_try_advisory_lock(?)", key).Scan(&locked).Error; err != nil {
			return err
		}
		if locked {
			return nil
		}
		time.Sleep(advisoryLockPoll)
	}
}

// advisoryLockPoll is how often a server waiting for another one's index
// build tries the lock again.
const advisoryLockPoll = 250 * time.Millisecond

// EnsureJobFilterIndexesOnOwnConnection is EnsureJobFilterIndexes on PostgreSQL
// through a one-connection handle of its own, opened from dsn and closed when the
// pass ends. The pass holds its connection for as long as it waits on another
// server's build and then builds three indexes; borrowed from the application's
// pool, that is a connection requests cannot have for minutes, and with a pool
// of one (-max-db-connections=1) it is all of them.
func EnsureJobFilterIndexesOnOwnConnection(dsn string) error {
	return onOwnConnection(dsn, "job filter index", EnsureJobFilterIndexes)
}

func onOwnConnection(dsn, what string, ensure func(*gorm.DB) error) error {
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return fmt.Errorf("open the %s connection: %w", what, err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("open the %s connection: %w", what, err)
	}
	defer sqlDB.Close()
	sqlDB.SetMaxOpenConns(1)
	return ensure(db)
}

// resourceListIndexes serve the resource list:
//
//   - idx_resources_owner_created reads one owner's resources newest first,
//     which is the default list order filtered by owner and the order of a
//     group's own resources. With only owner_id and created_at indexed, the
//     planner walks created_at and discards every other owner's rows; on a
//     library of 2.2M resources that was 639k rows, 401 ms, before the first
//     page of its largest owner.
var resourceListIndexes = []plannedIndex{
	{"idx_resources_owner_created", "resources", "owner_id, created_at"},
}

// resourceListIndexLockKey is the PostgreSQL advisory lock one server holds
// while it builds resourceListIndexes. It is the FTS prefix indexes' lock, so a
// server building one set waits for another server building the other: both
// build CONCURRENTLY on the resources table, and two such builds deadlock, which
// aborts one of them until its retry minutes later.
const resourceListIndexLockKey = constants.ResourceTableIndexBuildLockKey

// EnsureResourceListIndexes creates the resource list indexes that are missing,
// the way EnsureJobFilterIndexes does: CONCURRENTLY on PostgreSQL, so a build on
// a large resources table does not block uploads, and before serving on SQLite.
func EnsureResourceListIndexes(db *gorm.DB) error {
	return ensurePlannedIndexes(db, "resource list indexes", resourceListIndexLockKey, resourceListIndexes)
}

// EnsureResourceListIndexesOnOwnConnection is EnsureResourceListIndexes on
// PostgreSQL through a one-connection handle of its own, for the reason
// EnsureJobFilterIndexesOnOwnConnection gives.
func EnsureResourceListIndexesOnOwnConnection(dsn string) error {
	return onOwnConnection(dsn, "resource list index", EnsureResourceListIndexes)
}

func ensurePostgresPlannedIndex(conn *gorm.DB, index plannedIndex) error {
	var found []struct {
		Valid     bool
		Def       string
		Qualified string
	}
	// Looked up in the table's own schema, where CREATE INDEX puts it, which
	// current_schema() need not be: a search_path can name an existing schema
	// ahead of the one holding the tables. For the same reason a repair drops
	// it by its schema-qualified name: unqualified, the DROP would resolve
	// through search_path to a same-named index in another schema.
	if err := conn.Raw(`SELECT i.indisvalid AS valid, pg_get_indexdef(i.indexrelid) AS def,
		format('%I.%I', n.nspname, c.relname) AS qualified FROM pg_class c
		JOIN pg_namespace n ON n.oid = c.relnamespace
		JOIN pg_index i ON i.indexrelid = c.oid
		JOIN pg_class t ON t.oid = ?::regclass
		WHERE c.relname = ? AND c.relnamespace = t.relnamespace`, index.table, index.name).Scan(&found).Error; err != nil {
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
		if err := conn.Exec("DROP INDEX CONCURRENTLY IF EXISTS " + found[0].Qualified).Error; err != nil {
			return fmt.Errorf("drop %s: %w", index.name, err)
		}
	}
	statement := fmt.Sprintf("CREATE INDEX CONCURRENTLY IF NOT EXISTS %s ON %s (%s)", index.name, index.table, index.columns)
	if err := conn.Exec(statement).Error; err != nil {
		return fmt.Errorf("create %s: %w", index.name, err)
	}
	return nil
}
