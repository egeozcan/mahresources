package api_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/models"
	"mahresources/models/query_models"
)

// TestScanBypassesTheSubtreeScopeCallback pins a GORM behaviour that is a
// landmine wherever a scoped table is read.
//
// The subtree filter is registered on the Query, Update and Delete callback
// chains (see registerScopeCallbacks). Scan runs the *Row* chain, which has none
// of them — so a query that looks scoped in every other respect, model and all,
// returns rows from outside the caller's subtree. Find, Pluck and Count all go
// through Query and are filtered.
//
// This is not a test of the Resource Reduction. It is here because that feature
// reads scoped tables into custom result structs, which is exactly the shape that
// invites Scan, and because a GORM upgrade that changed this in either direction
// should be noticed rather than discovered.
func TestScanBypassesTheSubtreeScopeCallback(t *testing.T) {
	tc := SetupTestEnv(t)

	mine, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Mine"})
	require.NoError(t, err)
	theirs, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Theirs"})
	require.NoError(t, err)

	inside := addResourceWithBody(t, tc, "inside.txt", "inside")
	outside := addResourceWithBody(t, tc, "outside.txt", "outside")
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", inside.ID).Update("owner_id", mine.ID).Error)
	require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", outside.ID).Update("owner_id", theirs.ID).Error)

	db := scopedTo(t, tc, mine.ID).DBForProbe()

	var viaScan []struct{ ID uint }
	require.NoError(t, db.Model(&models.Resource{}).Select("resources.id").Scan(&viaScan).Error)
	assert.Len(t, viaScan, 2, "Scan runs the Row chain and is NOT filtered — the landmine this pins")

	// The three finishers that are.
	var viaFind []models.Resource
	require.NoError(t, db.Find(&viaFind).Error)
	assert.Len(t, viaFind, 1)

	var viaFindCustom []struct{ ID uint }
	require.NoError(t, db.Model(&models.Resource{}).Select("resources.id").Find(&viaFindCustom).Error)
	assert.Len(t, viaFindCustom, 1, "Find is filtered even into a destination that is not the model")

	var viaPluck []uint
	require.NoError(t, db.Model(&models.Resource{}).Pluck("resources.id", &viaPluck).Error)
	assert.Len(t, viaPluck, 1)

	var viaCount int64
	require.NoError(t, db.Model(&models.Resource{}).Count(&viaCount).Error)
	assert.EqualValues(t, 1, viaCount)
}

// A Winner must never be chosen on relationships the reviewer cannot see, and the
// margin the page prints must never state their exact number.
func TestAssociationCountsStopAtTheReviewersSubtree(t *testing.T) {
	tc := SetupTestEnv(t)

	mine, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Mine"})
	require.NoError(t, err)
	theirs, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Theirs"})
	require.NoError(t, err)

	quiet := addWithHash(t, tc, "quiet.txt", "quiet body", "shared-hash")
	linked := addWithHash(t, tc, "linked.txt", "linked body", "shared-hash")
	for _, id := range []uint{quiet.ID, linked.ID} {
		require.NoError(t, tc.DB.Model(&models.Resource{}).Where("id = ?", id).Update("owner_id", mine.ID).Error)
	}
	// `linked` is related to a Group outside the reviewer's subtree.
	require.NoError(t, tc.DB.Exec(
		"INSERT INTO groups_related_resources (resource_id, group_id) VALUES (?, ?)", linked.ID, theirs.ID).Error)

	owner, restricted := asAdmin()
	scoped := scopedTo(t, tc, mine.ID)
	red, err := scoped.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		Name:        "Hidden relationships",
		ResourceIds: []uint{quiet.ID, linked.ID},
		// Most associations wins — so if the hidden Group is counted, `linked` wins.
		WinnerRule: []string{models.WinnerCriterionAssociationsDesc},
	}, owner, restricted)
	require.NoError(t, err)

	fresh, err := scoped.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	_, err = scoped.RequestReductionCompute(red.ID, fresh.Version, owner, restricted, nil)
	require.NoError(t, err)
	plan := awaitReduction(t, tc, red.ID)

	require.Len(t, plan.Clusters, 1)
	cluster := plan.Clusters[0]
	assert.True(t, cluster.Undecided,
		"neither Resource has an association this reviewer can see, so no criterion decides")
	assert.Equal(t, quiet.ID, cluster.WinnerID, "and the tiebreaker of last resort is lowest id")
	assert.Empty(t, cluster.Margin, "so no count of hidden relationships is printed")

	// An administrator, who can see the Group, gets the other answer.
	adminRed, err := tc.AppCtx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		Name:        "Visible relationships",
		ResourceIds: []uint{quiet.ID, linked.ID},
		WinnerRule:  []string{models.WinnerCriterionAssociationsDesc},
	}, owner, restricted)
	require.NoError(t, err)
	adminFresh, err := tc.AppCtx.GetResourceReduction(adminRed.ID, owner, restricted)
	require.NoError(t, err)
	_, err = tc.AppCtx.RequestReductionCompute(adminRed.ID, adminFresh.Version, owner, restricted, nil)
	require.NoError(t, err)
	adminPlan := awaitReduction(t, tc, adminRed.ID)

	require.Len(t, adminPlan.Clusters, 1)
	assert.Equal(t, linked.ID, adminPlan.Clusters[0].WinnerID,
		"the relationship counts for somebody who can see it")
}
