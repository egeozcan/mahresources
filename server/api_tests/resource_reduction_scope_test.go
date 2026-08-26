package api_tests

import (
	"encoding/json"
	"fmt"
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

	fresh, err := scoped.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	_, err = scoped.RequestReductionCompute(red.ID, fresh.Version, owner, restricted, nil)
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
	fresh, err := wideCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	_, err = wideCtx.RequestReductionCompute(red.ID, fresh.Version, owner, restricted, nil)
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

// A Cluster reaching outside the reviewer's access is invisible in every place a
// count or an identifier could carry it: the render, the checked totals that the
// apply confirmation names, and the apply result itself.
//
// The Cluster id is a SHA-1 of the tier and the member ids, and ids here are small
// integers, so publishing it beside one visible member recovers the hidden one.
func TestAWithheldClusterIsNeitherCountedNorNamed(t *testing.T) {
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

	wideCtx := scopedTo(t, tc, wide.ID)
	red, err := wideCtx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		Name:        "Hidden counts",
		ResourceIds: []uint{winner.ID, loser.ID},
	}, owner, restricted)
	require.NoError(t, err)
	fresh, err := wideCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	_, err = wideCtx.RequestReductionCompute(red.ID, fresh.Version, owner, restricted, nil)
	require.NoError(t, err)
	plan := awaitReduction(t, tc, red.ID)
	require.Len(t, plan.Clusters, 1)
	require.True(t, plan.Clusters[0].Checked, "an Identical Cluster arrives checked")

	narrowCtx := scopedTo(t, tc, narrow.ID)

	review, err := narrowCtx.GetReductionReview(red.ID, owner, restricted, 1)
	require.NoError(t, err)
	assert.Equal(t, 0, review.CheckedCount,
		"a Cluster they cannot see is not in the total the apply confirmation names")
	assert.Equal(t, 0, review.CheckedLoserCount)

	current, err := narrowCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	result, err := narrowCtx.ApplyResourceReduction(&query_models.ReductionApply{
		ID: red.ID, Version: current.Version,
	}, owner, restricted)
	require.NoError(t, err)

	require.Len(t, result.Stale, 1)
	outcome := result.Stale[0]
	assert.True(t, outcome.Withheld)
	assert.Empty(t, outcome.ClusterID, "the id is derived from the member ids and would recover them")
	assert.Zero(t, outcome.WinnerID)
	assert.Empty(t, outcome.LoserIDs)
	assert.True(t, resourceExists(t, tc, loser.ID))

	// The admin, who may see both, gets the whole picture.
	adminReview, err := tc.AppCtx.GetReductionReview(red.ID, owner, restricted, 1)
	require.NoError(t, err)
	assert.Equal(t, 0, adminReview.Clusters[0].Withheld)
}

// The withheld rule holds on the JSON surface too.
//
// Every server-rendered page also answers `.json` by serialising its whole
// template context, so a placeholder in the HTML would leave the Cluster's id,
// tier, Winner, member ids and reviewed hashes one URL suffix away.
func TestAWithheldClusterIsRedactedOnTheJSONSurfaceToo(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	wide, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Wide"})
	require.NoError(t, err)
	narrow, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Narrow", OwnerId: wide.ID})
	require.NoError(t, err)

	winner := addWithHash(t, tc, "winner.txt", "winner body", "hidden-content-hash")
	loser := addWithHash(t, tc, "loser.txt", "loser body", "hidden-content-hash")
	setDimensions(t, tc, winner.ID, 400, 400)
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", winner.ID).Update("owner_id", narrow.ID).Error)
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", loser.ID).Update("owner_id", wide.ID).Error)

	wideCtx := scopedTo(t, tc, wide.ID)
	red, err := wideCtx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		Name:        "JSON leak",
		ResourceIds: []uint{winner.ID, loser.ID},
	}, owner, restricted)
	require.NoError(t, err)
	fresh, err := wideCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	_, err = wideCtx.RequestReductionCompute(red.ID, fresh.Version, owner, restricted, nil)
	require.NoError(t, err)
	plan := awaitReduction(t, tc, red.ID)
	require.Len(t, plan.Clusters, 1)
	hiddenClusterID := plan.Clusters[0].ID

	narrowCtx := scopedTo(t, tc, narrow.ID)
	review, err := narrowCtx.GetReductionReview(red.ID, owner, restricted, 1)
	require.NoError(t, err)
	require.Len(t, review.Clusters, 1)

	// Whatever serialises this view — the HTML, or the .json twin of the same page
	// — finds nothing in it.
	body, err := json.Marshal(review.Clusters)
	require.NoError(t, err)
	rendered := string(body)
	assert.NotContains(t, rendered, hiddenClusterID, "not the derived Cluster id")
	assert.NotContains(t, rendered, "hidden-content-hash", "not the reviewed content hash")
	assert.NotContains(t, rendered, fmt.Sprintf(`"resourceId":%d`, loser.ID), "not the member ids")
	assert.NotContains(t, rendered, "identical", "not even the tier")
	assert.Contains(t, rendered, `"Withheld":1`)
}

// The Extent counts the page shows are counted as the current principal sees
// them. The stored Extent is not filtered, so its raw lengths would state exactly
// how many Resources and Groups a reviewer whose subtree shrank may no longer
// open — on the page and on its .json twin alike.
func TestTheExtentCountsShrinkWithTheReviewersAccess(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	wide, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Wide"})
	require.NoError(t, err)
	narrow, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Narrow", OwnerId: wide.ID})
	require.NoError(t, err)
	elsewhere, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Elsewhere"})
	require.NoError(t, err)

	visible := addResourceWithBody(t, tc, "visible.txt", "visible")
	hidden := addResourceWithBody(t, tc, "hidden.txt", "hidden")
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", visible.ID).Update("owner_id", narrow.ID).Error)
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", hidden.ID).Update("owner_id", elsewhere.ID).Error)

	wideCtx := scopedTo(t, tc, wide.ID)
	red, err := wideCtx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		Name:        "Shrinking extent",
		ResourceIds: []uint{visible.ID, hidden.ID},
		GroupIds:    []uint{wide.ID, elsewhere.ID},
	}, owner, restricted)
	require.NoError(t, err)

	admin, err := tc.AppCtx.GetReductionReview(red.ID, owner, restricted, 1)
	require.NoError(t, err)
	assert.Equal(t, 2, admin.SelectedResources, "an administrator sees the whole selection")
	assert.Equal(t, 2, admin.SelectedGroups)

	confined, err := scopedTo(t, tc, narrow.ID).GetReductionReview(red.ID, owner, restricted, 1)
	require.NoError(t, err)
	assert.Equal(t, 1, confined.SelectedResources,
		"and a confined reviewer sees only what is theirs, not a count of what is not")
	assert.Equal(t, 0, confined.SelectedGroups)
}

// Coverage answers a question about the *whole* Extent — how much of it could be
// examined — so it is answered only for a principal who can see the whole Extent.
//
// Comparing selection counts was not enough: narrowing a subtree can hide a
// Resource reached through a Group that is still visible, leaving every count
// unchanged while the figures describe more than the reviewer can now reach.
func TestCoverageIsWithheldFromEverySubtreeConfinedReviewer(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	wide, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Wide"})
	require.NoError(t, err)
	narrow, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Narrow", OwnerId: wide.ID})
	require.NoError(t, err)

	visible := addResourceWithBody(t, tc, "visible.txt", "visible")
	hidden := addResourceWithBody(t, tc, "hidden.txt", "hidden")
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", visible.ID).Update("owner_id", narrow.ID).Error)
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", hidden.ID).Update("owner_id", wide.ID).Error)

	wideCtx := scopedTo(t, tc, wide.ID)
	red, err := wideCtx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		Name:        "Shrinking coverage",
		ResourceIds: []uint{visible.ID, hidden.ID},
	}, owner, restricted)
	require.NoError(t, err)
	fresh, err := wideCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	_, err = wideCtx.RequestReductionCompute(red.ID, fresh.Version, owner, restricted, nil)
	require.NoError(t, err)
	awaitReduction(t, tc, red.ID)

	adminReview, err := tc.AppCtx.GetReductionReview(red.ID, owner, restricted, 1)
	require.NoError(t, err)
	assert.True(t, adminReview.CoverageTrusted, "an unconfined principal sees the whole Extent, so it sees the figures")
	assert.Equal(t, 2, adminReview.Coverage.ExtentSize)

	wideReview, err := wideCtx.GetReductionReview(red.ID, owner, restricted, 1)
	require.NoError(t, err)
	assert.False(t, wideReview.CoverageTrusted,
		"even the reviewer who computed it is confined, so the figures are not theirs to read")

	narrowReview, err := scopedTo(t, tc, narrow.ID).GetReductionReview(red.ID, owner, restricted, 1)
	require.NoError(t, err)
	assert.False(t, narrowReview.CoverageTrusted)

	// Redacted in the value, not only in the page. Every server-rendered page also
	// answers `.json` by serialising its whole template context, and this review is
	// what goes into it — so a figure hidden by a template condition would still be
	// one URL suffix away.
	assert.Zero(t, narrowReview.Coverage.ExtentSize)
	assert.Zero(t, narrowReview.Coverage.ContentHashed)
	assert.Zero(t, narrowReview.Coverage.PerceptualEligible)
	assert.Zero(t, narrowReview.ExtentSize)

	body, err := json.Marshal(narrowReview)
	require.NoError(t, err)
	assert.NotContains(t, string(body), `"extentSize":2`)
	assert.NotContains(t, string(body), `"contentHashed":2`)
}
