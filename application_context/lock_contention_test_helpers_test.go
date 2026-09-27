package application_context

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/constants"
	"mahresources/models"
)

// commitBeforeEveryWriteTo has another connection try to commit immediately before
// each transaction's first INSERT or UPDATE to one table, and counts the attempts.
//
// That is what a busy deployment does on its own: the compute Job's bookkeeping, a
// hash or thumbnail worker, a job migration pass commit between a writer's reads and
// its write. A transaction that read first under a deferred BEGIN holds a WAL snapshot
// that commit has just made stale, and promoting it fails at once with
// SQLITE_BUSY_SNAPSHOT: the busy handler is never invoked, so busy_timeout does
// nothing. The server's driver begins every write transaction IMMEDIATE, so under it
// the attempt is refused (the transaction already holds the writer lock) and the write
// under test must still succeed; a handle with a deferred BEGIN lets the commit land,
// and then only a transaction whose first statement is the write survives it.
//
// Only before a transaction's first write to the table: a later one in the same
// transaction already holds the writer lock either way.
//
// The competing write goes through its own connection, which never waits for a lock,
// so it neither re-enters this callback nor stalls the transaction for busy_timeout.
func commitBeforeEveryWriteTo(t *testing.T, ctx *MahresourcesContext, table string) *atomic.Int32 {
	t.Helper()
	competitor := noWaitConnection(t, ctx.db)
	var fired atomic.Int32
	var mu sync.Mutex
	seen := map[gorm.ConnPool]bool{}
	competingCommit := func(tx *gorm.DB) {
		if tx.Statement.Table != table {
			return
		}
		mu.Lock()
		first := !seen[tx.Statement.ConnPool]
		seen[tx.Statement.ConnPool] = true
		mu.Unlock()
		if !first {
			return
		}
		fired.Add(1)
		if _, err := competitor.Exec(`UPDATE resource_categories SET description = description || '.' WHERE id = 1`); err != nil && !strings.Contains(err.Error(), "database is locked") {
			t.Errorf("competing commit: %v", err)
		}
	}
	updateName, createName := "test:commit_before_update_"+table, "test:commit_before_create_"+table
	require.NoError(t, ctx.db.Callback().Update().Before("gorm:update").Register(updateName, competingCommit))
	require.NoError(t, ctx.db.Callback().Create().Before("gorm:create").Register(createName, competingCommit))
	t.Cleanup(func() {
		_ = ctx.db.Callback().Update().Remove(updateName)
		_ = ctx.db.Callback().Create().Remove(createName)
	})
	return &fired
}

// noWaitConnection opens a second connection to db's SQLite file that never waits
// for a lock: a write through it commits at once or fails with "database is locked".
func noWaitConnection(t *testing.T, db *gorm.DB) *sql.DB {
	t.Helper()
	var path string
	require.NoError(t, db.Raw("SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&path).Error)
	require.NotEmpty(t, path, "the database is not a file")
	competitor, err := sql.Open("sqlite3", "file:"+path+"?_busy_timeout=0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = competitor.Close() })
	return competitor
}

// openProductionSQLiteFile opens a SQLite database file through
// CreateDatabaseConnection, so it has the server's driver: WAL, busy_timeout, and
// transactions that take the writer lock at BEGIN unless declared read-only.
func openProductionSQLiteFile(t *testing.T) (*gorm.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "production.db")
	db, _, err := models.CreateDatabaseConnection(constants.DbTypeSqlite, "file:"+path+"?_journal_mode=WAL", "", 0)
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db, path
}

// holdWriterLock has a separate connection take the SQLite writer lock on the
// file at path and keep it until the test ends.
func holdWriterLock(t *testing.T, path string) {
	t.Helper()
	holder, err := sql.Open("sqlite3", "file:"+path+"?_busy_timeout=0")
	require.NoError(t, err)
	conn, err := holder.Conn(context.Background())
	require.NoError(t, err)
	_, err = conn.ExecContext(context.Background(), "BEGIN IMMEDIATE")
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		_ = conn.Close()
		_ = holder.Close()
	})
}
