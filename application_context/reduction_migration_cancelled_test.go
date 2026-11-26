package application_context

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A runner started with a context that is already cancelled does no migration: it
// does not take over a spooled Reduction, write a Cluster of it or publish it. The
// plan stays legacy for the next start to finish.
func testMigrationRunnerHonoursACancelledContext(t *testing.T, ctx *MahresourcesContext) {
	red, _ := seedLegacyReduction(t, ctx, false)
	require.NoError(t, ctx.SpoolReductionMigrations())

	stop, cancel := context.WithCancel(context.Background())
	cancel()
	select {
	case <-ctx.StartReductionMigration(stop, 20*time.Millisecond):
	case <-time.After(10 * time.Second):
		t.Fatal("the runner never ended")
	}
	require.Equal(t, 0, planSchemaOf(t, ctx, red.ID), "a cancelled runner published a Reduction")
	var clusters int64
	require.NoError(t, ctx.db.Raw(`SELECT COUNT(*) FROM reduction_clusters WHERE reduction_id = ?`, red.ID).Scan(&clusters).Error)
	require.Zero(t, clusters, "a cancelled runner wrote Clusters")

	// and the same pass under a live context does migrate it
	live, stopLive := context.WithCancel(context.Background())
	t.Cleanup(stopLive)
	select {
	case <-ctx.StartReductionMigration(live, 20*time.Millisecond):
	case <-time.After(15 * time.Second):
		t.Fatal("the runner never finished")
	}
	require.Equal(t, 1, planSchemaOf(t, ctx, red.ID))
}

func TestMigrationRunnerHonoursACancelledContextSQLite(t *testing.T) {
	testMigrationRunnerHonoursACancelledContext(t, newReductionSQLiteContext(t))
}
