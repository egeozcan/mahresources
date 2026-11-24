package application_context

import (
	"testing"

	"github.com/stretchr/testify/require"

	"mahresources/models"
)

// (3) Deleting a legacy Reduction after its spool was written but before it was
// published takes the spool with it, and the sweep removes spool rows whose
// Reduction no longer exists at all.
func testReductionDeleteRemovesItsSpool(t *testing.T, ctx *MahresourcesContext) {
	red, _ := seedLegacyReduction(t, ctx, false)
	other, _ := seedLegacyReduction(t, ctx, false)
	require.NoError(t, ctx.SpoolReductionMigrations())
	count := func(id uint) int64 {
		var n int64
		require.NoError(t, ctx.db.Raw(`SELECT COUNT(*) FROM reduction_migration_rows WHERE reduction_id = ?`, id).Scan(&n).Error)
		return n
	}
	require.NotZero(t, count(red.ID))
	otherBefore := count(other.ID)
	require.NotZero(t, otherBefore)

	require.NoError(t, ctx.DeleteResourceReduction(red.ID, nil, false))
	require.Zero(t, count(red.ID), "the deleted Reduction's spool was stranded")
	require.Equal(t, otherBefore, count(other.ID), "another Reduction's spool is untouched")

	// A spool already stranded (a crash, an older release) is swept.
	for i := 0; i < 3; i++ {
		require.NoError(t, ctx.db.Create(&models.ReductionMigrationRow{ReductionID: 987654, Generation: "dead", Kind: "member", ClusterOrd: 0, MemberOrd: i, ResourceID: uint(i + 1)}).Error)
	}
	require.NoError(t, ctx.SweepOrphanReductionSpool())
	require.Zero(t, count(987654), "spool of a Reduction that no longer exists was swept")
	require.Equal(t, otherBefore, count(other.ID), "a migration in flight keeps its spool")
}

func TestReductionDeleteRemovesItsSpoolSQLite(t *testing.T) {
	testReductionDeleteRemovesItsSpool(t, newReductionSQLiteContext(t))
}
