package application_context

import (
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// commitBeforeEveryWriteTo has another connection commit immediately before each
// transaction's first INSERT or UPDATE to one table, and counts them.
//
// That is what a busy deployment does on its own: the compute Job's bookkeeping, a
// hash or thumbnail worker, a job migration pass commit between a writer's reads and
// its write. A transaction that read first holds a WAL snapshot that commit has just
// made stale, and promoting it fails at once with SQLITE_BUSY_SNAPSHOT: the busy
// handler is never invoked, so busy_timeout does nothing. It fires for every
// transaction, not once, because a retry that reads again first meets the same commit
// again under sustained load; only a transaction whose first statement is the write
// survives it.
//
// Only before a transaction's first write to the table: a later one in the same
// transaction already holds the writer lock, so nothing can commit in front of it and
// the competing write would just wait out busy_timeout. The writers under test make
// that write their transaction's first.
//
// The competing write goes through the raw pool, so it runs on another connection and
// does not re-enter this callback.
func commitBeforeEveryWriteTo(t *testing.T, ctx *MahresourcesContext, table string) *atomic.Int32 {
	t.Helper()
	sqlDB, err := ctx.db.DB()
	require.NoError(t, err)
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
		if _, err := sqlDB.Exec(`UPDATE resource_categories SET description = description || '.' WHERE id = 1`); err != nil {
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
