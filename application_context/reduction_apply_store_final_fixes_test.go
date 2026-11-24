package application_context

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Final-review fixes for the apply loop: each test is one finding.

// ---- 12: a renewal error that is not lock contention stops the loop ----------------

func testStoreApplyLoopStopsOnARenewalError(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	_, l1 := pairAt(t, ctx, r, "k1", 1, nil)
	_, l2 := pairAt(t, ctx, r, "k2", 2, nil)

	origEvery := applyLeaseRenewEvery
	applyLeaseRenewEvery = 0
	t.Cleanup(func() { applyLeaseRenewEvery, reductionApplyRenewHook = origEvery, nil })
	reductionApplyRenewHook = func() error { return errors.New("the Reduction was deleted") }

	result, err := ctx.storeApplyReduction(r, applyRequest(r))
	require.NoError(t, err)
	assert.Zero(t, result.AppliedCount, "nothing starts once ownership cannot be proved")
	assert.Equal(t, 1, result.Errors)
	assert.True(t, storeResourceExists(t, ctx, l1))
	assert.True(t, storeResourceExists(t, ctx, l2))
}

func testStoreApplyLoopRidesOutRenewalContention(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	_, l1 := pairAt(t, ctx, r, "k1", 1, nil)
	_, l2 := pairAt(t, ctx, r, "k2", 2, nil)

	origEvery := applyLeaseRenewEvery
	applyLeaseRenewEvery = 0
	t.Cleanup(func() { applyLeaseRenewEvery, reductionApplyRenewHook = origEvery, nil })
	reductionApplyRenewHook = func() error { return errors.New("database is locked") }

	result, err := ctx.storeApplyReduction(r, applyRequest(r))
	require.NoError(t, err)
	assert.Equal(t, 2, result.AppliedCount, "transient contention does not end the batch")
	assert.Zero(t, result.Errors)
	assert.False(t, storeResourceExists(t, ctx, l1))
	assert.False(t, storeResourceExists(t, ctx, l2))
}

// ---- 13: reconciliation renews the lease, and a lost one abandons the batch --------

func testStoreReconcileRenewsTheApplyLease(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	r = startBatch(t, ctx, r, "b1", 5*time.Second)
	before := loadReductionRow(t, ctx.db, r.ID).ApplyLeaseUntil
	run := &applyRun{reductionID: r.ID, batch: "b1", generation: opsGen, startedAt: time.Now()}

	_, _, err := ctx.storeReconcileClaimsFor(r.ID, applyBatchClaimFence(run), "b1")
	require.NoError(t, err)
	after := loadReductionRow(t, ctx.db, r.ID).ApplyLeaseUntil
	assert.Greater(t, after, before+int64(time.Minute), "a reconciliation chunk pushes the lease out")
}

func testStoreApplyAbandonsWhenReconciliationLosesTheLease(t *testing.T, ctx *MahresourcesContext) {
	r := applyReduction(t, ctx)
	_, l1 := pairAt(t, ctx, r, "k1", 1, nil)

	reductionApplyBeforeReconcileHook = func() {
		require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET apply_lease_until = ? WHERE id = ?", time.Now().Add(-time.Second).UnixNano(), r.ID).Error)
	}
	t.Cleanup(func() { reductionApplyBeforeReconcileHook = nil })

	result, err := ctx.storeApplyReduction(r, applyRequest(r))
	require.ErrorIs(t, err, ErrReductionLeaseLost)
	assert.Nil(t, result)
	assert.True(t, storeResourceExists(t, ctx, l1), "nothing was merged")
	rec := lastApplyOf(t, ctx, r)
	assert.Nil(t, rec.FinishedAt, "an interrupted batch is not recorded as finished")
	assert.Zero(t, loadReductionRow(t, ctx.db, r.ID).ApplyLeaseUntil, "the lease is given back")
}

func TestStoreApplyLoopStopsOnARenewalErrorSQLite(t *testing.T) {
	testStoreApplyLoopStopsOnARenewalError(t, withSettings(newReductionSQLiteContext(t)))
}

func TestStoreApplyLoopRidesOutRenewalContentionSQLite(t *testing.T) {
	testStoreApplyLoopRidesOutRenewalContention(t, withSettings(newReductionSQLiteContext(t)))
}

func TestStoreReconcileRenewsTheApplyLeaseSQLite(t *testing.T) {
	testStoreReconcileRenewsTheApplyLease(t, withSettings(newReductionSQLiteContext(t)))
}

func TestStoreApplyAbandonsWhenReconciliationLosesTheLeaseSQLite(t *testing.T) {
	testStoreApplyAbandonsWhenReconciliationLosesTheLease(t, withSettings(newReductionSQLiteContext(t)))
}
