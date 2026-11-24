package application_context

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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
