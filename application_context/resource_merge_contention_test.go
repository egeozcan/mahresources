package application_context

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"mahresources/models"
)

// mergeStagedEffects is what a merge transaction has staged by the time it fails: the
// ids staged for deletion, the after-commit manifest rows, and the Reduction members
// marked gone. It is read through the transaction's own handle, so it sees the
// transaction's uncommitted writes.
type mergeStagedEffects struct{ staged, manifest, gone int64 }

func readMergeStagedEffects(tx *gorm.DB) mergeStagedEffects {
	db := tx.Session(&gorm.Session{NewDB: true})
	var e mergeStagedEffects
	db.Raw("SELECT COUNT(*) FROM reduction_deleted_ids").Scan(&e.staged)
	db.Raw("SELECT COUNT(*) FROM resource_cleanup_manifest").Scan(&e.manifest)
	db.Raw("SELECT COUNT(*) FROM reduction_members WHERE gone = ?", true).Scan(&e.gone)
	return e
}

// TestAMergeRetriedAfterContentionActsOnceForWhatItDeleted pins what a retried merge
// carries forward: nothing from the attempt that rolled back. The first attempt loses to
// lock contention at its LAST statement, the purge of the staged ids, which runs after
// the loser's rows are deleted, its ids staged, the Reduction members marked gone and
// the cleanup manifest written. The retry deletes the loser again, so only the committed
// attempt's effects may exist: the staging, the manifest and the gone marks of the
// first attempt are rolled back (the retry starts from none), and after_resource_delete
// fires once for the loser, not twice.
func TestAMergeRetriedAfterContentionActsOnceForWhatItDeleted(t *testing.T) {
	ctx := newPluginHookTestContext(t, hookTagPlugin(map[string]string{
		"after_resource_delete": "after",
	}))
	winner := makeHookTestResource(t, ctx, "winner")
	loser := makeHookTestResource(t, ctx, "loser")

	r := newReductionRow(t, ctx.db)
	require.NoError(t, ctx.db.Model(r).Update("plan_generation", "g-live").Error)
	cluster := hookCluster(t, ctx, r, "g-live", "k", winner.ID, member(winner.ID, "h", 1), member(loser.ID, "h", 1))

	var (
		mu         sync.Mutex
		injected   bool
		failedWith mergeStagedEffects
		retryStart *mergeStagedEffects
	)
	const purge = "test:contention_at_staged_id_purge"
	const retry = "test:observe_retry_start"
	require.NoError(t, ctx.db.Callback().Raw().Before("gorm:raw").Register(purge, func(tx *gorm.DB) {
		if !strings.Contains(strings.ToLower(tx.Statement.SQL.String()), "delete from reduction_deleted_ids") {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if injected {
			return
		}
		injected = true
		failedWith = readMergeStagedEffects(tx)
		_ = tx.AddError(errors.New("database is locked"))
	}))
	require.NoError(t, ctx.db.Callback().Create().Before("gorm:create").Register(retry, func(tx *gorm.DB) {
		if tx.Statement.Table != "reduction_deleted_ids" {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if injected && retryStart == nil {
			e := readMergeStagedEffects(tx)
			retryStart = &e
		}
	}))
	t.Cleanup(func() {
		_ = ctx.db.Callback().Raw().Remove(purge)
		_ = ctx.db.Callback().Create().Remove(retry)
	})

	require.NoError(t, ctx.MergeResources(winner.ID, []uint{loser.ID}, false))
	require.True(t, injected, "the contention was never injected: the matcher no longer sees the staged-id purge")

	assert.EqualValues(t, 1, failedWith.staged, "the failed attempt had staged the loser")
	assert.EqualValues(t, 1, failedWith.manifest, "the failed attempt had written its cleanup manifest")
	assert.EqualValues(t, 1, failedWith.gone, "the failed attempt had marked the loser's Cluster member gone")
	require.NotNil(t, retryStart, "the merge was never retried")
	assert.Zero(t, *retryStart, "what the failed attempt staged survived its rollback")

	assert.False(t, resourceExists(t, ctx, loser.ID))
	gone, _ := goneOf(t, ctx, cluster.Row.ID)
	assert.True(t, gone[loser.ID], "the committed attempt marks the member gone")
	assert.Equal(t, 1, hookFires(t, ctx, "after"), "after_resource_delete fired for the rolled-back attempt too")
	var left int64
	require.NoError(t, ctx.db.Model(&models.ResourceCleanupManifest{}).Count(&left).Error)
	assert.Zero(t, left, "the committed attempt's manifest is drained")
}
