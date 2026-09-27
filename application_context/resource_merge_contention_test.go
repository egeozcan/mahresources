package application_context

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
)

// TestAMergeIsNotRefusedByACommitLandingAfterItsReads pins the merge's write order on
// SQLite. The merge reads its participants and then deletes the losers in one
// transaction; if that transaction read first, another connection committing in
// between (the hash worker, a thumbnail, a Job's bookkeeping) made its first write
// fail at once with SQLITE_BUSY_SNAPSHOT, which never reaches the busy handler, and a
// Resource Reduction apply recorded the Cluster as stale over nothing but contention.
//
// The competing commit is issued right after the merge's first read of resources. A
// merge that already holds the writer lock makes it wait until the merge commits; one
// that does not lets it commit at once and then loses its own write.
func TestAMergeIsNotRefusedByACommitLandingAfterItsReads(t *testing.T) {
	ctx := newWALTestContext(t, 0)
	winner := &models.Resource{Name: "winner", Hash: "hash-winner", Location: "loc-winner"}
	loser := &models.Resource{Name: "loser", Hash: "hash-loser", Location: "loc-loser"}
	require.NoError(t, ctx.db.Create(winner).Error)
	require.NoError(t, ctx.db.Create(loser).Error)

	sqlDB, err := ctx.db.DB()
	require.NoError(t, err)
	var once sync.Once
	competing := make(chan error, 1)
	var waited bool
	const name = "test:commit_after_merge_read"
	require.NoError(t, ctx.db.Callback().Query().After("gorm:query").Register(name, func(tx *gorm.DB) {
		if tx.Statement.Table != "resources" {
			return
		}
		once.Do(func() {
			finished := make(chan struct{})
			go func() {
				_, err := sqlDB.Exec(`UPDATE resource_categories SET description = description || '.' WHERE id = 1`)
				competing <- err
				close(finished)
			}()
			select {
			case <-finished:
			case <-time.After(300 * time.Millisecond):
				// Waiting on the merge's writer lock.
				waited = true
			}
		})
	}))
	t.Cleanup(func() { _ = ctx.db.Callback().Query().Remove(name) })

	require.NoError(t, ctx.MergeResources(winner.ID, []uint{loser.ID}, false))
	require.NoError(t, <-competing, "the competing commit should land once the merge has")
	assert.True(t, waited, "the competing commit landed inside the merge, so the merge did not hold the writer lock")

	var remaining int64
	require.NoError(t, ctx.db.Model(&models.Resource{}).Where("id = ?", loser.ID).Count(&remaining).Error)
	assert.Zero(t, remaining, "the loser should have been merged away")
}

// TestAMergeRetriedAfterContentionActsOnceForWhatItDeleted pins what a retried merge
// carries forward: nothing from the attempt that rolled back. The first attempt loses
// to lock contention at its last statement, the winner's backups write, after it has
// deleted the loser and collected that deletion's after-hook and file cleanup; the
// retry deletes it again. Only the committed attempt's effects may run, so
// after_resource_delete fires once for the loser, not twice.
func TestAMergeRetriedAfterContentionActsOnceForWhatItDeleted(t *testing.T) {
	ctx := newPluginHookTestContext(t, hookTagPlugin(map[string]string{
		"after_resource_delete": "after",
	}))
	winner := makeHookTestResource(t, ctx, "winner")
	loser := makeHookTestResource(t, ctx, "loser")

	var once sync.Once
	const name = "test:contention_at_merge_backups_write"
	require.NoError(t, ctx.db.Callback().Raw().After("gorm:raw").Register(name, func(tx *gorm.DB) {
		if !strings.Contains(tx.Statement.SQL.String(), "update resources set meta = json_patch") {
			return
		}
		once.Do(func() { _ = tx.AddError(errors.New("database is locked")) })
	}))
	t.Cleanup(func() { _ = ctx.db.Callback().Raw().Remove(name) })

	require.NoError(t, ctx.MergeResources(winner.ID, []uint{loser.ID}, false))
	assert.False(t, resourceExists(t, ctx, loser.ID))
	assert.Equal(t, 1, hookFires(t, ctx, "after"), "after_resource_delete fired for the rolled-back attempt too")
}
