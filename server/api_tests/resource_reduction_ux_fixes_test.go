package api_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/application_context"
	"mahresources/contracts"
	"mahresources/models"
	"mahresources/models/query_models"
)

func listProgressOf(t *testing.T, ctx *application_context.MahresourcesContext, id uint) contracts.ReductionProgress {
	t.Helper()
	owner, restricted := asAdmin()
	red, err := ctx.GetResourceReduction(id, owner, restricted)
	require.NoError(t, err)
	out, err := ctx.GetReductionProgress([]models.ResourceReduction{*red})
	require.NoError(t, err)
	return out[id]
}

// UX-P1-2. Another Reduction merges away members of this one's Clusters. An
// unrestricted reviewer sees them as Stale, on the Stale tab and in the list
// counts alike; a scope-restricted one cannot tell deleted from out of scope, so
// the Cluster stays withheld in both.
func TestAClusterWhoseMemberWasDeletedElsewhereIsStaleInTabsAndListAlike(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	ids := clusterOf(t, tc, "ux12-hash", 2)
	first := createReduction(t, tc, "First", ids)
	computeReduction(t, tc, first.ID)
	second := createReduction(t, tc, "Second", ids)
	computeReduction(t, tc, second.ID)
	applyReduction(t, tc, second.ID)
	require.False(t, resourceExists(t, tc, ids[0]) && resourceExists(t, tc, ids[1]), "the other Reduction deleted a member")

	review := reviewOf(t, tc, first.ID, 1, nil)
	assert.Equal(t, 0, review.TabCounts.Open)
	assert.Equal(t, 1, review.TabCounts.Stale)
	assert.Equal(t, 0, review.ApplyFigures.Identical.Clusters)
	stale := reviewOf(t, tc, first.ID, 1, &query_models.ResourceReductionReviewQuery{Status: []string{"stale"}})
	require.Len(t, stale.Clusters, 1)
	assert.Contains(t, stale.Clusters[0].StaleReason, "no longer exists")
	assert.Zero(t, stale.Clusters[0].Withheld, "an unrestricted reviewer is told, not given a stub")

	progress := listProgressOf(t, tc.AppCtx, first.ID)
	assert.Equal(t, 0, progress.Open)
	assert.Equal(t, 0, progress.Approved)
	assert.Equal(t, 1, progress.Stale, "the list agrees with the tabs")

	// Applying it does nothing, and the figures the dialog showed (0) match.
	zero := uint(0)
	current, err := tc.AppCtx.GetResourceReduction(first.ID, owner, restricted)
	require.NoError(t, err)
	result, err := tc.AppCtx.ApplyResourceReduction(&query_models.ReductionApply{
		ID: first.ID, Version: current.Version, ExpectedClusters: &zero, ExpectedLosers: &zero,
	}, owner, restricted)
	require.NoError(t, err)
	assert.Empty(t, result.Applied)
}

func TestAScopedReviewerCannotTellDeletedFromOutOfScope(t *testing.T) {
	tc := SetupTestEnv(t)
	f := newReductionScopeFixture(t, tc)

	ids := clusterOf(t, tc, "ux12-scoped", 2)
	f.place(t, f.narrow, ids...)
	f.compute(t, ids)
	require.NoError(t, tc.DB.Where("id = ?", ids[1]).Delete(&models.Resource{}).Error)

	owner, restricted := asAdmin()
	review, err := f.narrowCtx.GetReductionReview(f.red.ID, owner, restricted, 1, nil)
	require.NoError(t, err)
	assert.Equal(t, contracts.ReductionTabCounts{}, review.TabCounts, "withheld: on no tab")
	assert.Equal(t, contracts.ReductionProgress{}, listProgressOf(t, f.narrowCtx, f.red.ID))
}

// UX-P1-5. Ejecting the only remaining Loser removes nothing, so the Cluster is no
// longer approved.
func TestEjectingTheLastLoserClearsApproval(t *testing.T) {
	tc := SetupTestEnv(t)
	ids := clusterOf(t, tc, "ux15-hash", 2)
	setDimensions(t, tc, ids[0], 900, 900)
	setDimensions(t, tc, ids[1], 100, 100)
	red := createReduction(t, tc, "Eject", ids)
	plan := computeReduction(t, tc, red.ID)
	require.True(t, plan.Clusters[0].Checked)
	plan = override(t, tc, red.ID, plan.Clusters[0].ID, application_context.ReductionActionEject, ids[1])
	assert.False(t, plan.Clusters[0].Checked)
}

// UX-P2. Approval belongs to a proposal; promoting changes the proposal.
func TestPromotingClearsApproval(t *testing.T) {
	tc := SetupTestEnv(t)
	ids := clusterOf(t, tc, "ux2-promote", 2)
	setDimensions(t, tc, ids[0], 900, 900)
	setDimensions(t, tc, ids[1], 100, 100)
	red := createReduction(t, tc, "Promote", ids)
	plan := computeReduction(t, tc, red.ID)
	require.True(t, plan.Clusters[0].Checked)
	plan = override(t, tc, red.ID, plan.Clusters[0].ID, application_context.ReductionActionPromote, ids[1])
	assert.False(t, plan.Clusters[0].Checked)
}

// UX-P2. "Approve all Identical (n)" counts what a click would change.
func TestApproveAllIdenticalCountsOnlyWhatWouldChange(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()
	a := clusterOf(t, tc, "ux2-a", 2)
	b := clusterOf(t, tc, "ux2-b", 2)
	c := clusterOf(t, tc, "ux2-c", 2)
	red := createReduction(t, tc, "Approve all", append(append(append([]uint{}, a...), b...), c...))
	plan := computeReduction(t, tc, red.ID)
	assert.Equal(t, 0, reviewOf(t, tc, red.ID, 1, nil).Bulk.IdenticalApprovable, "all three arrive approved")

	override(t, tc, red.ID, clusterFor(plan, a[0]).ID, application_context.ReductionActionUncheck, 0)
	review := reviewOf(t, tc, red.ID, 1, nil)
	assert.Equal(t, 1, review.Bulk.IdenticalApprovable)

	// expectedCount follows the same figure.
	_, err := tc.AppCtx.BulkOverrideReductionClusters(&query_models.ReductionBulkOverride{
		ID: red.ID, Version: versionOf(t, tc, red.ID), Action: "check", AllIdentical: true, ExpectedCount: uintPtr(3),
	}, owner, restricted)
	assert.ErrorIs(t, err, application_context.ErrReductionCountMismatch)
	result, err := tc.AppCtx.BulkOverrideReductionClusters(&query_models.ReductionBulkOverride{
		ID: red.ID, Version: versionOf(t, tc, red.ID), Action: "check", AllIdentical: true, ExpectedCount: uintPtr(1),
	}, owner, restricted)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Changed)
}

// UX-P2. A reopened, unapproved Cluster is not "Reviewed".
func TestStateLabelsSayWhatTheClusterIs(t *testing.T) {
	tc := SetupTestEnv(t)
	ids := clusterOf(t, tc, "ux2-label", 2)
	red := createReduction(t, tc, "Labels", ids)
	plan := computeReduction(t, tc, red.ID)
	id := plan.Clusters[0].ID
	label := func() string { return reviewOf(t, tc, red.ID, 1, nil).Clusters[0].StateLabel }

	assert.Equal(t, "Approved", label())
	override(t, tc, red.ID, id, application_context.ReductionActionSkip, 0)
	override(t, tc, red.ID, id, application_context.ReductionActionReopen, 0)
	assert.Equal(t, "Not approved", label())
	override(t, tc, red.ID, id, application_context.ReductionActionCheck, 0)
	assert.Equal(t, "Approved", label())
}
