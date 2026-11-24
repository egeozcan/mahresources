package application_context

import (
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Round 5E review fixes for the apply store: each test is one finding.

// ---- 1: the settlement context starts when settlement begins ----------------------

func TestStoreApplySettlementOutlivesALongMergeLoopSQLite(t *testing.T) {
	ctx := withSettings(newReductionSQLiteContext(t))
	r := applyReduction(t, ctx)
	pairAt(t, ctx, r, "k1", 1, nil)

	orig := reductionApplySettleTimeout
	reductionApplySettleTimeout = 300 * time.Millisecond
	t.Cleanup(func() { reductionApplySettleTimeout = orig })
	reductionApplyAfterClaimHook = func() { time.Sleep(600 * time.Millisecond) }
	t.Cleanup(func() { reductionApplyAfterClaimHook = nil })

	result, err := ctx.storeApplyReduction(r, applyRequest(r))
	require.NoError(t, err)
	assert.Equal(t, 1, result.AppliedCount)
	rec := lastApplyOf(t, ctx, r)
	assert.NotNil(t, rec.FinishedAt, "a batch that outran the settlement timeout still records its finish")
	assert.Zero(t, loadReductionRow(t, ctx.db, r.ID).ApplyLeaseUntil, "and gives the lease back")
}

// ---- 2: a clipped reason does not keep the original allocation alive ---------------

func TestClipReductionReasonDoesNotAliasTheOriginal(t *testing.T) {
	big := strings.Repeat("x", 1<<20)
	clipped := clipReductionReason(big)
	require.Len(t, clipped, reductionReasonBytes)
	assert.NotEqual(t, uintptr(unsafe.Pointer(unsafe.StringData(big))), uintptr(unsafe.Pointer(unsafe.StringData(clipped))), "the clip is a copy, not a view of the 1 MiB reason")
}
