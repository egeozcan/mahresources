package api_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/application_context"
	"mahresources/auth"
	"mahresources/models"
	"mahresources/models/query_models"
)

// scopedTo builds a context bound to a user confined to one Group's subtree.
//
// Subtree confinement is not automatic for a new table — scopeColumn maps only
// groups, resources and notes — so a Reduction's own row is guarded by the owner
// predicate and everything it *reaches* is guarded by the db handle's scope. Both
// halves need proving, and only the second one is what stops a confined reviewer
// destroying somebody else's files.
func scopedTo(t *testing.T, tc *TestContext, groupID uint) *application_context.MahresourcesContext {
	t.Helper()
	scope := groupID
	return tc.AppCtx.WithPrincipal(&auth.Principal{
		UserID:       42,
		Role:         models.RoleUser,
		ScopeGroupID: &scope,
	})
}

// A scope-limited reviewer sees and acts on only the Resources inside their
// subtree. A Resource outside it never enters the Extent, never becomes a Cluster
// member, and therefore can never be destroyed by their Reduction.
func TestAScopedReviewersReductionStopsAtTheirSubtree(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	mine, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Mine"})
	require.NoError(t, err)
	theirs, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Theirs"})
	require.NoError(t, err)

	// Three Resources with one content hash: two inside the subtree, one outside.
	inside := addWithHash(t, tc, "inside.txt", "inside body", "shared-hash")
	insideToo := addWithHash(t, tc, "inside-too.txt", "inside too body", "shared-hash")
	outside := addWithHash(t, tc, "outside.txt", "outside body", "shared-hash")
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id IN ?", []uint{inside.ID, insideToo.ID}).
		Update("owner_id", mine.ID).Error)
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", outside.ID).
		Update("owner_id", theirs.ID).Error)
	// The one outside the subtree is the biggest, so an unscoped Reduction would
	// make it the Winner and this one would too if scope were not applied.
	setDimensions(t, tc, outside.ID, 900, 900)
	setDimensions(t, tc, inside.ID, 400, 400)
	setDimensions(t, tc, insideToo.ID, 100, 100)

	scoped := scopedTo(t, tc, mine.ID)

	// The reviewer names all three ids explicitly — a stored id is not a
	// permission, and the Extent has to drop the one they may not see.
	red, err := scoped.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		Name:        "Confined",
		ResourceIds: []uint{inside.ID, insideToo.ID, outside.ID},
	}, owner, restricted)
	require.NoError(t, err)

	_, err = scoped.RequestReductionCompute(red.ID, owner, restricted, nil)
	require.NoError(t, err)
	plan := awaitReduction(t, tc, red.ID)

	require.Len(t, plan.Clusters, 1)
	cluster := plan.Clusters[0]
	assert.ElementsMatch(t, []uint{inside.ID, insideToo.ID}, memberIDs(cluster),
		"the Resource outside the subtree is not a member, whoever named its id")
	assert.Equal(t, inside.ID, cluster.WinnerID)
	assert.Equal(t, 2, plan.Coverage.ExtentSize, "and it never entered the Extent")

	// And applying destroys only what is inside.
	current, err := scoped.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	result, err := scoped.ApplyResourceReduction(&query_models.ReductionApply{
		ID: red.ID, Version: current.Version,
	}, owner, restricted)
	require.NoError(t, err)

	require.Len(t, result.Applied, 1)
	assert.Equal(t, []uint{insideToo.ID}, result.Applied[0].LoserIDs)
	assert.True(t, resourceExists(t, tc, outside.ID), "nothing outside the subtree was destroyed")
	assert.True(t, resourceExists(t, tc, inside.ID))
	assert.False(t, resourceExists(t, tc, insideToo.ID))
}

// A Reduction computed while the reviewer could see a Resource must not still let
// them destroy it once they cannot. The re-check is against the *current*
// principal, at apply as well as at render.
func TestAClusterIsRefusedWhenTheReviewersScopeHasShrunk(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	wide, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Wide"})
	require.NoError(t, err)
	narrow, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Narrow", OwnerId: wide.ID})
	require.NoError(t, err)

	winner := addWithHash(t, tc, "winner.txt", "winner body", "shared-hash")
	loser := addWithHash(t, tc, "loser.txt", "loser body", "shared-hash")
	setDimensions(t, tc, winner.ID, 400, 400)
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", winner.ID).Update("owner_id", narrow.ID).Error)
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", loser.ID).Update("owner_id", wide.ID).Error)

	// Computed while the reviewer could see the whole tree.
	wideCtx := scopedTo(t, tc, wide.ID)
	red, err := wideCtx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		Name:        "Shrinking",
		ResourceIds: []uint{winner.ID, loser.ID},
	}, owner, restricted)
	require.NoError(t, err)
	_, err = wideCtx.RequestReductionCompute(red.ID, owner, restricted, nil)
	require.NoError(t, err)
	plan := awaitReduction(t, tc, red.ID)
	require.Len(t, plan.Clusters, 1)
	require.Len(t, plan.Clusters[0].LoserIDs(), 1)

	// Their access is narrowed. The Loser is now outside what they may see.
	narrowCtx := scopedTo(t, tc, narrow.ID)

	review, err := narrowCtx.GetReductionReview(red.ID, owner, restricted, 1)
	require.NoError(t, err)
	require.Len(t, review.Clusters, 1)
	assert.Equal(t, 1, review.Clusters[0].Withheld,
		"the render says a member is outside what they may see")

	current, err := narrowCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	result, err := narrowCtx.ApplyResourceReduction(&query_models.ReductionApply{
		ID: red.ID, Version: current.Version,
	}, owner, restricted)
	require.NoError(t, err)

	assert.Empty(t, result.Applied)
	require.Len(t, result.Stale, 1)
	assert.True(t, resourceExists(t, tc, loser.ID),
		"a Resource they may no longer see is not destroyed by a Cluster computed when they could")
}
