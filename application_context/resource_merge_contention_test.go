package application_context

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

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
