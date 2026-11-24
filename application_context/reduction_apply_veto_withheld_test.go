package application_context

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"mahresources/models"
)

// A merge refused after a Loser moved outside the caller's subtree (a before-delete
// veto, say) must not be reported to that caller: the demotion re-checks what the
// caller may see, and a Cluster now withheld has its claim released with no outcome.
func testDemoteWithholdsAClusterThatLeftTheSubtree(t *testing.T, ctx *MahresourcesContext) {
	mine := &models.Group{Name: "mine"}
	other := &models.Group{Name: "other"}
	require.NoError(t, ctx.db.Create(mine).Error)
	require.NoError(t, ctx.db.Create(other).Error)
	account := accountPrincipal(t, ctx, "u", models.RoleUser, &mine.ID)
	p := ctx.WithPrincipal(account)

	r := newPlanReduction(t, ctx.db)
	w := makeResource(t, ctx.db, "w", opsRes{Hash: "h", Owner: &mine.ID})
	l := makeResource(t, ctx.db, "l", opsRes{Hash: "h", Owner: &mine.ID})
	insertOps(t, ctx, r, "veto", models.ReductionTierIdentical, w, opsMember(w, "h"), opsMember(l, "h"))
	visible := w2(t, ctx, &mine.ID)
	insertOps(t, ctx, r, "control", models.ReductionTierIdentical, visible[0], opsMember(visible[0], "h"), opsMember(visible[1], "h"))
	r = startBatch(t, ctx, r, "b1", time.Hour)
	stamp(t, ctx, "b1", "veto", "control")

	claimed, gate, err := p.storeClaimCluster(r.ID, "b1", "veto")
	require.NoError(t, err)
	require.Equal(t, claimWon, gate)
	control, gate, err := p.storeClaimCluster(r.ID, "b1", "control")
	require.NoError(t, err)
	require.Equal(t, claimWon, gate)

	// the Loser moves out of the caller's subtree after the claim
	require.NoError(t, ctx.db.Exec("UPDATE resources SET owner_id = ? WHERE id = ?", other.ID, l).Error)

	require.Equal(t, demoteWithheld, p.storeDemote(r.ID, "b1", "veto", claimed.Row.ClaimID, staleReasonMergeRefused+": vetoed", nil))
	row := rowOf(t, ctx, "veto")
	require.Equal(t, models.ReductionClusterOpen, row.State, "the claim was released, not marked stale")
	require.True(t, row.Checked)
	require.Empty(t, row.ClaimID)
	require.Empty(t, row.OutcomeBatch, "no outcome is recorded for a Cluster the caller cannot see")

	// a Cluster still wholly visible is demoted as before
	require.Equal(t, demoteRecorded, p.storeDemote(r.ID, "b1", "control", control.Row.ClaimID, staleReasonMergeRefused+": vetoed", nil))
	require.Equal(t, models.ReductionClusterStale, rowOf(t, ctx, "control").State)
}

// w2 is two Resources owned by the group.
func w2(t *testing.T, ctx *MahresourcesContext, owner *uint) []uint {
	t.Helper()
	return []uint{
		makeResource(t, ctx.db, "cw", opsRes{Hash: "k", Owner: owner}),
		makeResource(t, ctx.db, "cl", opsRes{Hash: "k", Owner: owner}),
	}
}

func TestDemoteWithholdsAClusterThatLeftTheSubtreeSQLite(t *testing.T) {
	testDemoteWithholdsAClusterThatLeftTheSubtree(t, withSettings(newReductionSQLiteContext(t)))
}
