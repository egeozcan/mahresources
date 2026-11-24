package application_context

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"mahresources/models"
)

// A renewal or fence that starts just before the lease expires and then waits for
// the row lock must judge the lease against a clock read after the wait (D11).
// The wait is injected as a clock that has moved past expiry once the row is held.
func testReductionLeaseJudgedAfterTheLock(t *testing.T, ctx *MahresourcesContext) {
	r := newReductionRow(t, ctx.db)
	s := ctx.reductionStore()
	gen := "gen-wait"
	started := time.Now()
	expiry := started.Add(time.Minute)
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET status = 'computing', compute_generation = ?, compute_lease_until = ? WHERE id = ?",
		gen, expiry.UnixNano(), r.ID).Error)
	orig := leaseNow
	t.Cleanup(func() { leaseNow = orig })
	leaseNow = func() time.Time { return expiry.Add(time.Second) }

	// the caller's clock is still before expiry, as it was before the wait
	ok, err := s.renewComputeLease(r.ID, gen, started, expiry.Add(-time.Millisecond))
	require.NoError(t, err)
	require.False(t, ok, "a renewal that waited past expiry must not revive the lease")
	var after models.ResourceReduction
	require.NoError(t, ctx.db.Select(reductionColumns).First(&after, r.ID).Error)
	require.Equal(t, expiry.UnixNano(), after.ComputeLeaseUntil)
	require.ErrorIs(t, s.fenceCompute(r.ID, gen, expiry.Add(-time.Millisecond)), ErrReductionLeaseLost,
		"a fence that waited past expiry must not admit the worker")
}

func TestReductionLeaseJudgedAfterTheLockSQLite(t *testing.T) {
	testReductionLeaseJudgedAfterTheLock(t, newReductionSQLiteContext(t))
}
