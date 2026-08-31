package application_context

import (
	"mahresources/constants"

	"gorm.io/gorm"
)

// groupTreeAdvisoryLockKey identifies the process-independent PostgreSQL
// transaction lock shared by every mutation that can re-parent a group: the
// ordinary group update's cycle walk and the mass edit's group owner op. A
// cycle is not a property of one row but of the whole ancestor CHAIN between
// the new owner and the target, and row locks cannot freeze a chain that a
// concurrent re-parent is extending while the walk is still reading it. One
// lock makes the walk-then-write sequence unambiguous: it is always acquired
// before any group row involved in a re-parent is inspected or changed.
//
// SQLite needs nothing here: its single-writer discipline plus WAL snapshot
// isolation turns a concurrent re-parent landing mid-transaction into a failed
// write (SQLITE_BUSY_SNAPSHOT) instead of a silently stale check.
const groupTreeAdvisoryLockKey int64 = 0x6d61687267727473 // "mahrgrts"

// lockGroupTreeMutation must run before any group row involved in a re-parent
// is read or locked — an advisory lock taken after row locks could invert
// against another transaction holding the advisory lock and waiting for those
// rows. The GLOBAL advisory order is: the user-management lock first when a
// transaction needs it (MergeGroups does; UpdateGroup and the mass edit do
// not), then the tree lock, then model rows. Transactions that never take the
// user-management lock (UpdateGroup, the mass edit) take the tree lock as
// their first operation.
func (ctx *MahresourcesContext) lockGroupTreeMutation(tx *gorm.DB) error {
	if ctx.Config.DbType == constants.DbTypePosgres {
		return tx.Exec("SELECT pg_advisory_xact_lock(?)", groupTreeAdvisoryLockKey).Error
	}
	return nil
}
