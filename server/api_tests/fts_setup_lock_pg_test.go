//go:build postgres

package api_tests

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A server starting against a database whose FTS indexes already exist must not
// wait on another server's concurrent index build. CREATE INDEX IF NOT EXISTS
// takes a ShareLock on the table before it finds the index, and that conflicts
// with the ShareUpdateExclusiveLock a CREATE INDEX CONCURRENTLY holds for its
// whole build: the starting server waited, and every upload queued behind its
// pending lock until the other build finished.
func TestFTSSetupDoesNotWaitOnAConcurrentBuildWhenItsIndexesExist(t *testing.T) {
	tc := SetupPostgresTestEnv(t)
	require.NoError(t, tc.AppCtx.InitFTS(), "first setup creates the indexes")

	// What another server's CREATE INDEX CONCURRENTLY on resources holds.
	builder := tc.DB.Begin()
	require.NoError(t, builder.Error)
	defer builder.Rollback()
	require.NoError(t, builder.Exec("LOCK TABLE resources IN SHARE UPDATE EXCLUSIVE MODE").Error)

	done := make(chan error, 1)
	go func() { done <- tc.AppCtx.InitFTS() }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		builder.Rollback()
		<-done
		t.Fatal("FTS setup waited on a concurrent build of an index it did not need to create")
	}
}
