package application_context

import (
	"context"
	"mahresources/jobs"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"mahresources/models"
)

// (1) On SQLite the periodic runner must never run the spool statement: it writes a
// whole legacy plan in one transaction under the database-wide writer lock, which
// D12 reserves for startup. A Reduction the startup spool missed stays legacy.
func TestReductionOnlineRunnerDoesNotSpoolOnSQLite(t *testing.T) {
	ctx := newReductionSQLiteContext(t)
	red, _ := seedLegacyReduction(t, ctx, false)

	stop, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	done := ctx.StartReductionMigration(stop, 20*time.Millisecond)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the runner has nothing it may do and should end")
	}
	var rows int64
	require.NoError(t, ctx.db.Raw(`SELECT COUNT(*) FROM reduction_migration_rows WHERE reduction_id = ?`, red.ID).Scan(&rows).Error)
	require.Zero(t, rows, "the online runner wrote spool rows")
	var migs int64
	require.NoError(t, ctx.db.Raw(`SELECT COUNT(*) FROM reduction_migrations WHERE reduction_id = ?`, red.ID).Scan(&migs).Error)
	require.Zero(t, migs)
	require.Equal(t, 0, planSchemaOf(t, ctx, red.ID), "still legacy until the next startup spool")

	// The startup spool, then the runner, still migrates it.
	require.NoError(t, ctx.SpoolReductionMigrations())
	stop2, cancel2 := context.WithCancel(context.Background())
	t.Cleanup(cancel2)
	select {
	case <-ctx.StartReductionMigration(stop2, 20*time.Millisecond):
	case <-time.After(15 * time.Second):
		t.Fatal("the runner never finished")
	}
	require.Equal(t, 1, planSchemaOf(t, ctx, red.ID))
}

// (2) A queued compute that meets the similarity indexes still building waits for
// them like the upgrade wait, bounded, and does not fail terminally.
func TestQueuedComputeWaitsForSimilarityIndexes(t *testing.T) {
	ctx := newReductionSQLiteContext(t)
	require.NoError(t, ctx.db.AutoMigrate(&models.Resource{}, &models.ResourceSimilarity{}))
	adapter := &reductionComputeAdapter{ctx: ctx, kind: JobKindReductionCompute}

	prevPoll, prevWait := reductionUpgradePoll, reductionIndexMaxWait
	reductionUpgradePoll, reductionIndexMaxWait = 5*time.Millisecond, 2*time.Second
	t.Cleanup(func() { reductionUpgradePoll, reductionIndexMaxWait = prevPoll, prevWait })

	go func() {
		time.Sleep(60 * time.Millisecond)
		_ = models.EnsureSimilarityDistanceIndexes(ctx.db)
	}()
	require.NoError(t, adapter.awaitSimilarityIndexes(context.Background(), jobs.Execution{}), "the run proceeds once the indexes are valid")

	// Bounded: indexes that never finish end the wait with the building error.
	require.NoError(t, ctx.db.Exec(`DROP INDEX idx_sim_r1_eff_dist`).Error)
	reductionIndexMaxWait = 40 * time.Millisecond
	err := adapter.awaitSimilarityIndexes(context.Background(), jobs.Execution{})
	require.ErrorIs(t, err, ErrReductionIndexesBuilding)

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	reductionIndexMaxWait = time.Minute
	require.ErrorIs(t, adapter.awaitSimilarityIndexes(cancelled, jobs.Execution{}), context.Canceled)
}
