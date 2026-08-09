package application_context

import (
	"strconv"
	"strings"
	"time"

	"mahresources/constants"

	"gorm.io/gorm"
)

// userManagementAdvisoryLockKey identifies the process-independent PostgreSQL
// transaction lock shared by every mutation that can cross the user/security
// boundary. A single lock intentionally makes row-lock ordering unambiguous:
// it is always acquired before group, administrator, user, session, or token
// rows are inspected or changed.
const userManagementAdvisoryLockKey int64 = 0x6d61687275736572 // "mahruser"

// sqliteUserManagementLockStatement is the SQLite serialization write issued by
// lockUserManagementMutation, and sqliteUserRowLockStatement the per-row variant
// used where a single owner must be pinned. They are named constants because
// concurrency tests key their barriers on the exact statement: a hand-copied
// literal stops matching the moment the statement is reformatted, and a barrier
// that never fires turns a clean failure into a package-wide test timeout.
const (
	sqliteUserManagementLockStatement = "UPDATE users SET id = id WHERE id = (SELECT MIN(id) FROM users)"
	sqliteUserRowLockStatement        = "UPDATE users SET id = id WHERE id = ?"
)

// lockUserManagementMutation must be the first database operation in a
// user-management transaction. PostgreSQL uses a transaction-scoped advisory
// lock. SQLite uses an empty no-op write to acquire its existing database writer
// serialization before any read that could become stale.
func (ctx *MahresourcesContext) lockUserManagementMutation(tx *gorm.DB) error {
	if ctx.Config.DbType == constants.DbTypePosgres {
		return tx.Exec("SELECT pg_advisory_xact_lock(?)", userManagementAdvisoryLockKey).Error
	}
	err := tx.Exec(sqliteUserManagementLockStatement).Error
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "no such table") {
		// Legacy focused group contexts may intentionally omit auth tables; there
		// is no user/security boundary to serialize in that fixture.
		return nil
	}
	return err
}

// lockUserManagementMutationBounded is lockUserManagementMutation for request
// paths that must fail fast rather than queue behind a long administrative
// transaction. MergeGroups, BulkDeleteGroups and DeleteGroup hold this same lock
// for the whole of one potentially minutes-long transaction, and
// pg_advisory_xact_lock waits forever; a bounded wait turns that into a
// retryable error instead of a hung request. SQLite needs nothing extra: the
// connection's busy_timeout already bounds the wait.
//
// SET LOCAL takes no locks of its own, so running it first preserves the
// lock-ordering invariant documented above.
func (ctx *MahresourcesContext) lockUserManagementMutationBounded(tx *gorm.DB, wait time.Duration) error {
	if ctx.Config.DbType == constants.DbTypePosgres {
		milliseconds := wait.Milliseconds()
		if milliseconds < 1 {
			milliseconds = 1
		}
		// lock_timeout accepts no bind parameters; the value is an internal
		// constant, never request input.
		if err := tx.Exec("SET LOCAL lock_timeout = '" + strconv.FormatInt(milliseconds, 10) + "ms'").Error; err != nil {
			return err
		}
	}
	return ctx.lockUserManagementMutation(tx)
}
