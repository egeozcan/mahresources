//go:build postgres && json1 && fts5

package application_context

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"mahresources/models"
)

// selectedGroupWithRelatedLoser seeds a Reduction whose only selection is a Group,
// and one Cluster whose Loser is inside it only by being related to that Group.
func selectedGroupWithRelatedLoser(t *testing.T, ctx *MahresourcesContext) (r *models.ResourceReduction, selected *models.Group, loser uint) {
	r = applyReduction(t, ctx)
	require.NoError(t, ctx.db.Exec("UPDATE resource_reductions SET extent_all = ? WHERE id = ?", false, r.ID).Error)
	r = loadReductionRow(t, ctx.db, r.ID)
	selected = &models.Group{Name: "selected"}
	require.NoError(t, ctx.db.Create(selected).Error)
	require.NoError(t, ctx.db.Create(&models.ReductionExtentGroup{ReductionID: r.ID, GroupID: selected.ID}).Error)
	_, loser = pairAt(t, ctx, r, "related", 1, nil)
	require.NoError(t, ctx.db.Exec("INSERT INTO groups_related_resources (group_id, resource_id) VALUES (?, ?)", selected.ID, loser).Error)
	return r, selected, loser
}

// An unlink that lands after the membership check must wait for the merge: the
// check holds a share lock on the relationship row, so the Loser is not deleted on
// the strength of a membership that is already gone.
func TestApplyMembershipRowIsLockedAgainstAnUnlinkPG(t *testing.T) {
	ctx := newOpsPGContext(t)
	r, selected, loser := selectedGroupWithRelatedLoser(t, ctx)

	unlinked := make(chan error, 1)
	var finishedInsideMerge bool
	reductionAfterExtentHook = func() {
		go func() { unlinked <- ctx.RemoveGroupsFromResource(loser, []uint{selected.ID}) }()
		select {
		case <-unlinked:
			finishedInsideMerge = true
		case <-time.After(750 * time.Millisecond):
		}
	}
	t.Cleanup(func() { reductionAfterExtentHook = nil })

	result, err := ctx.storeApplyReduction(r, applyRequest(r))
	require.NoError(t, err)
	require.False(t, finishedInsideMerge, "the unlink committed while the merge relied on the membership")
	require.Equal(t, []string{"related"}, appliedKeys(result.Applied))
	if !finishedInsideMerge {
		require.NoError(t, <-unlinked, "the unlink runs once the merge has committed")
	}
}

// An unlink that commits before the check is seen by it: the Cluster is stale.
func TestApplyUnlinkBeforeTheCheckLeavesTheSelectionPG(t *testing.T) {
	ctx := newOpsPGContext(t)
	r, selected, loser := selectedGroupWithRelatedLoser(t, ctx)

	restore := SetReductionPreconditionHookForTest(func() {
		require.NoError(t, ctx.RemoveGroupsFromResource(loser, []uint{selected.ID}))
	})
	t.Cleanup(restore)

	result, err := ctx.storeApplyReduction(r, applyRequest(r))
	require.NoError(t, err)
	require.Equal(t, []string{"related"}, appliedKeys(result.Stale))
	require.Equal(t, staleReasonLeftSelection, result.Stale[0].Reason)
	require.True(t, storeResourceExists(t, ctx, loser))
}
