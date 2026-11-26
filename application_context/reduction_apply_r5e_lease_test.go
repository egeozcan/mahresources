package application_context

import (
	"testing"

	"mahresources/models"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---- 5: the lease is renewed between the pre-merge veto hooks ----------------------

func TestStoreApplyRenewsTheLeaseBetweenBeforeDeleteHooksSQLite(t *testing.T) {
	ctx := withSettings(newReductionSQLiteContext(t))
	r := applyReduction(t, ctx)
	winner := makeResource(t, ctx.db, "w", opsRes{Hash: "h"})
	members := []models.ReductionMemberRow{opsMember(winner, "h")}
	for _, name := range []string{"l1", "l2", "l3"} {
		members = append(members, opsMember(makeResource(t, ctx.db, name, opsRes{Hash: "h"}), "h"))
	}
	insertAt(t, ctx, r, "k1", models.ReductionTierIdentical, 1, winner, members...)

	// The loop's own renewal never fires in this test: every renewal counted below
	// is one the pre-merge hooks asked for.
	origEvery := applyLeaseRenewEvery
	applyLeaseRenewEvery = 1 << 62
	renewals := 0
	reductionApplyRenewHook = func() error { renewals++; return nil }
	t.Cleanup(func() { applyLeaseRenewEvery, reductionApplyRenewHook = origEvery, nil })

	result, err := ctx.storeApplyReduction(r, applyRequest(r))
	require.NoError(t, err)
	require.Equal(t, 1, result.AppliedCount)
	assert.GreaterOrEqual(t, renewals, 3, "each Loser's before-delete hook is preceded by a fenced lease renewal")
}
