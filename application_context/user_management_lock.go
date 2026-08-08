package application_context

import (
	"strings"

	"mahresources/constants"

	"gorm.io/gorm"
)

// userManagementAdvisoryLockKey identifies the process-independent PostgreSQL
// transaction lock shared by every mutation that can cross the user/security
// boundary. A single lock intentionally makes row-lock ordering unambiguous:
// it is always acquired before group, administrator, user, session, or token
// rows are inspected or changed.
const userManagementAdvisoryLockKey int64 = 0x6d61687275736572 // "mahruser"

// lockUserManagementMutation must be the first database operation in a
// user-management transaction. PostgreSQL uses a transaction-scoped advisory
// lock. SQLite uses an empty no-op write to acquire its existing database writer
// serialization before any read that could become stale.
func (ctx *MahresourcesContext) lockUserManagementMutation(tx *gorm.DB) error {
	if ctx.Config.DbType == constants.DbTypePosgres {
		return tx.Exec("SELECT pg_advisory_xact_lock(?)", userManagementAdvisoryLockKey).Error
	}
	err := tx.Exec("UPDATE users SET id = id WHERE id = (SELECT MIN(id) FROM users)").Error
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "no such table") {
		// Legacy focused group contexts may intentionally omit auth tables; there
		// is no user/security boundary to serialize in that fixture.
		return nil
	}
	return err
}
