//go:build postgres && json1 && fts5

package application_context

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A compute blocked on a Resource row another transaction holds must stop when its
// worker is cancelled, and release the Reduction it had locked. Its statements and
// its lease renewals carry the worker's context, so the cancel reaches the server;
// before, they ran on a background context and the Reduction stayed locked (and
// every other writer of it queued behind) until the blocker let go.
func TestReductionComputeCancelReleasesTheReductionWhileBlockedPG(t *testing.T) {
	ctx := newOpsPGContext(t)
	for i := 0; i < 2; i++ {
		mkStreamRes(t, ctx.db, streamRes{Hash: "same", W: 10, H: 10, Size: int64(100 + i)})
	}
	r := mkStreamReduction(t, ctx, streamSel{All: true, Identical: true})
	cur := loadReductionRow(t, ctx.db, r.ID)
	_, gen, err := ctx.beginReductionComputeStore(cur.ID, cur.Version)
	require.NoError(t, err)

	// another transaction holds every Resource row, so the Cluster insertion's FOR SHARE waits
	blocker := ctx.db.Begin()
	require.NoError(t, blocker.Error)
	released := false
	release := func() {
		if !released {
			released = true
			_ = blocker.Rollback().Error
		}
	}
	t.Cleanup(release)
	require.NoError(t, blocker.Exec(`SELECT id FROM resources FOR UPDATE`).Error)

	jobCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- ctx.runReductionComputeOwned(jobCtx, cur.ID, "", gen, nil, nil, false) }()

	require.Eventually(t, func() bool {
		var waiting int
		require.NoError(t, ctx.db.Raw(`SELECT COUNT(*) FROM pg_stat_activity WHERE wait_event_type = 'Lock' AND datname = current_database()`).Scan(&waiting).Error)
		return waiting > 0
	}, 20*time.Second, 50*time.Millisecond, "the compute never blocked on the held Resource rows")

	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("the cancelled compute stayed blocked behind the held lock")
	}

	// the Reduction is free now, while the blocker still holds the Resources
	probe := ctx.db.Begin()
	require.NoError(t, probe.Error)
	defer probe.Rollback()
	require.NoError(t, probe.Exec(`SET LOCAL lock_timeout = '2s'`).Error)
	require.NoError(t, probe.Exec(`SELECT id FROM resource_reductions WHERE id = ? FOR UPDATE`, cur.ID).Error,
		"the cancelled compute still holds the Reduction row")
	release()
}
