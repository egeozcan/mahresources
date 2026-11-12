package application_context

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"mahresources/models"
)

// ErrSavedSQLUnavailable refuses saved SQL, and the schema listing that
// supports writing it, when -auth is on and the PostgreSQL role behind
// DB_READONLY_DSN can do more than read the allowed tables. The wrapped error
// names the check that failed.
var ErrSavedSQLUnavailable = errors.New("saved SQL queries are disabled")

// savedSQLRoleCheckTTL is how long a verdict on the read-only role stands. A
// GRANT made while the server runs is noticed within this long; a fix is too.
const savedSQLRoleCheckTTL = time.Minute

// savedSQLRoleCheck caches the last verdict on the read-only role. It is shared
// by pointer, because the context is copied per principal and per transaction.
type savedSQLRoleCheck struct {
	mu sync.Mutex
	// checkedAt is when the check behind err started, not when it finished: a
	// verdict is as old as the privileges it saw.
	checkedAt time.Time
	err       error
	logged    string
}

// readOnlyRoleProblem returns why the PostgreSQL role behind the read-only
// handle is not restricted to the saved-SQL tables, or nil when it is. It is
// always nil on SQLite, where the read-only driver enforces the allow-list
// itself. A verdict is reused for savedSQLRoleCheckTTL.
func (ctx *MahresourcesContext) readOnlyRoleProblem() error {
	if ctx.readOnlyDB == nil || ctx.readOnlyDB.DriverName() != "postgres" || ctx.savedSQLRoleCheck == nil {
		return nil
	}
	return ctx.savedSQLRoleCheck.verdict(ctx, false)
}

func (c *savedSQLRoleCheck) verdict(ctx *MahresourcesContext, fresh bool) error {
	c.mu.Lock()
	if !fresh && !c.checkedAt.IsZero() && time.Since(c.checkedAt) < savedSQLRoleCheckTTL {
		err := c.err
		c.mu.Unlock()
		return err
	}
	c.mu.Unlock()

	// Checked without the lock: the check takes connections from both pools,
	// and a caller holding one while it waited on the lock would deadlock a
	// small pool. Two callers may check at once; store keeps whichever verdict
	// started later.
	started := time.Now()
	return c.store(started, ctx.verifyReadOnlyRole(), ctx.Config.AuthEnabled)
}

// store records err as the verdict of a check that started at started, unless
// a check that started later has already been stored: a slow check that began
// before a GRANT must not replace the failure a later check found. It returns
// the verdict in force afterwards.
func (c *savedSQLRoleCheck) store(started time.Time, err error, authEnabled bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.checkedAt.IsZero() && !started.After(c.checkedAt) {
		return c.err
	}
	c.err = err
	c.checkedAt = started

	// Logged when the verdict changes, not on every recheck.
	current := ""
	if c.err != nil {
		current = c.err.Error()
	}
	if current != c.logged {
		c.logged = current
		switch {
		case c.err == nil:
			log.Printf("Saved SQL: the DB_READONLY_DSN role is restricted to the saved-SQL tables")
		case authEnabled:
			log.Printf("WARNING: saved SQL queries are disabled: %v. Run `mahresources -print-readonly-role-sql=<role>` for the SQL that creates a correctly restricted role.", c.err)
		default:
			log.Printf("WARNING: saved SQL queries can read more than content tables: %v. They stay enabled because -auth is off; run `mahresources -print-readonly-role-sql=<role>` for the SQL that creates a correctly restricted role.", c.err)
		}
	}
	return c.err
}

func (ctx *MahresourcesContext) verifyReadOnlyRole() error {
	checkCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var mainRole, mainDatabase string
	if err := ctx.db.WithContext(checkCtx).Raw("SELECT current_user, current_database()").Row().Scan(&mainRole, &mainDatabase); err != nil {
		return fmt.Errorf("read the main connection's role: %w", err)
	}
	return models.VerifyPostgresReadOnlyRole(checkCtx, ctx.readOnlyDB, mainRole, mainDatabase)
}

// CheckSavedSQLRole checks the read-only role now, whatever the cached verdict,
// and logs the result. The server calls it at startup, after migration, so the
// check sees every table.
func (ctx *MahresourcesContext) CheckSavedSQLRole() error {
	if ctx.readOnlyDB == nil || ctx.readOnlyDB.DriverName() != "postgres" || ctx.savedSQLRoleCheck == nil {
		return nil
	}
	return ctx.savedSQLRoleCheck.verdict(ctx, true)
}

// savedSQLRefusal refuses saved SQL under -auth while the read-only role fails
// its check. With auth off every request is the administrator's, who could read
// those tables anyway, so it only warns (at startup and on /queries).
func (ctx *MahresourcesContext) savedSQLRefusal() error {
	if !ctx.Config.AuthEnabled {
		return nil
	}
	if problem := ctx.readOnlyRoleProblem(); problem != nil {
		return fmt.Errorf("%w: %w", ErrSavedSQLUnavailable, problem)
	}
	return nil
}

// ReadOnlyDBProblem describes why saved SQL is not restricted to the content
// tables, for the warning on the Queries pages; empty when it is.
func (ctx *MahresourcesContext) ReadOnlyDBProblem() string {
	if ctx.readOnlyDB == nil {
		return "no read-only connection is configured"
	}
	switch ctx.readOnlyDB.DriverName() {
	case models.ReadOnlySQLiteDriverName:
		return ""
	case "postgres":
		if problem := ctx.readOnlyRoleProblem(); problem != nil {
			return problem.Error()
		}
		return ""
	}
	return "the read-only connection was not opened by the hardened driver"
}
