package application_context

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"mahresources/models"
)

// (9) A compute is owner-qualified at the claim and at the landing, against the owner
// the request loaded. An owner deleted (created_by_user_id nulled, no version bump)
// between the load and the claim, or between the claim and the landing, no longer
// authorizes it: the landing's owner is the one captured at claim, never a re-read
// that already sees NULL.
func testReductionComputeOwnerDeletedBetweenRequestAndLanding(t *testing.T, ctx *MahresourcesContext) {
	require.NoError(t, ctx.db.AutoMigrate(&models.Resource{}, &models.ResourceSimilarity{}))
	require.NoError(t, models.EnsureSimilarityDistanceIndexes(ctx.db))
	deleteOwner := func(id uint) {
		require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET created_by_user_id = NULL WHERE id = ?", id).Error)
	}
	newOwned := func() *models.ResourceReduction {
		r := mkStreamReduction(t, ctx, streamSel{All: true})
		require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET created_by_user_id = ? WHERE id = ?", 77, r.ID).Error)
		return loadReductionRow(t, ctx.db, r.ID)
	}

	// Between the request's load and its claim.
	r := newOwned()
	reductionComputeAfterLoadHook = func() { deleteOwner(r.ID) }
	t.Cleanup(func() { reductionComputeAfterLoadHook = nil })
	_, err := ctx.RequestReductionCompute(r.ID, r.Version, nil, false, nil)
	require.ErrorIs(t, err, ErrReductionConflict)
	reductionComputeAfterLoadHook = nil
	require.NotEqual(t, models.ReductionStatusComputing, loadReductionRow(t, ctx.db, r.ID).Status, "nothing was claimed")

	// Between the claim and the landing.
	r = newOwned()
	reductionComputeAfterClaimHook = func() { deleteOwner(r.ID) }
	t.Cleanup(func() { reductionComputeAfterClaimHook = nil })
	_, err = ctx.RequestReductionCompute(r.ID, r.Version, nil, false, nil)
	// With a download queue the run is asynchronous, without one it ran inline and
	// returned its error: either way, wait for the row to leave `computing`.
	if err != nil {
		require.ErrorIs(t, err, ErrReductionConflict)
	}
	require.Eventually(t, func() bool {
		// A read racing the job's own write can see a shared-cache table lock; it is
		// retried, not a failure of the assertion.
		var status string
		if err := ctx.db.Raw(`SELECT status FROM resource_reductions WHERE id = ?`, r.ID).Scan(&status).Error; err != nil {
			return false
		}
		return status != models.ReductionStatusComputing
	}, 20*time.Second, 20*time.Millisecond)
	reductionComputeAfterClaimHook = nil
	after := loadReductionRow(t, ctx.db, r.ID)
	require.Nil(t, after.PlanGeneration, "the plan was not replaced by a run its owner no longer authorizes")
	require.Equal(t, models.ReductionStatusFailed, after.Status)
}

func TestReductionComputeOwnerDeletedBetweenRequestAndLandingSQLite(t *testing.T) {
	testReductionComputeOwnerDeletedBetweenRequestAndLanding(t, withSettings(newReductionSQLiteContext(t)))
}
