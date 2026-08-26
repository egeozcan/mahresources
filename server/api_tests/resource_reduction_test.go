package api_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/application_context"
	"mahresources/models"
	"mahresources/models/query_models"
)

// ownedBy is the (ownerUserID, ownerRestricted) pair every Reduction call takes,
// spelled out here so the tests read as "as this user" rather than as two
// positional arguments.
func ownedBy(id uint) (*uint, bool) { return &id, true }

func asAdmin() (*uint, bool) { return nil, false }

func createReduction(t *testing.T, tc *TestContext, name string, resourceIDs []uint) *models.ResourceReduction {
	t.Helper()
	owner, restricted := asAdmin()
	red, err := tc.AppCtx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		Name:        name,
		ResourceIds: resourceIDs,
	}, owner, restricted)
	require.NoError(t, err)
	require.NotNil(t, red)
	return red
}

// A Reduction is created from a selection of Resources, named, and carries the
// defaults the design specifies: both tiers, and keepAsVersion on for
// Near-Identical and off for Identical. One flag could not hold both values,
// which is why there are two.
func TestCreateResourceReductionFromResources(t *testing.T) {
	tc := SetupTestEnv(t)

	a := addResourceWithBody(t, tc, "a.txt", "aaa")
	b := addResourceWithBody(t, tc, "b.txt", "bbb")

	red := createReduction(t, tc, "Holiday photos", []uint{a.ID, b.ID})

	assert.Equal(t, "Holiday photos", red.Name)
	assert.Equal(t, models.ReductionStatusDraft, red.Status)
	assert.Equal(t, models.MatchingModeBothTiers, red.MatchingMode)
	assert.False(t, red.KeepAsVersionIdentical, "a byte-identical Loser has nothing to preserve")
	assert.True(t, red.KeepAsVersionNear, "a Near-Identical Loser holds pixels worth a way back to")

	extent, err := application_context.DecodeReductionExtent(red.Extent)
	require.NoError(t, err)
	assert.ElementsMatch(t, []uint{a.ID, b.ID}, extent.ResourceIDs)
	assert.Empty(t, extent.GroupIDs)

	assert.Equal(t, models.DefaultWinnerRule(), application_context.DecodeWinnerRule(red.WinnerRule))
}

// A Reduction can be created from Groups instead, so "everything filed under
// Holidays" does not have to be enumerated. Group ids are stored, never their
// expansion — the descendants are resolved at compute time, because they change.
func TestCreateResourceReductionFromGroups(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	group, err := tc.AppCtx.CreateGroup(&query_models.GroupCreator{Name: "Holidays"})
	require.NoError(t, err)

	red, err := tc.AppCtx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		Name:     "Holidays",
		GroupIds: []uint{group.ID},
	}, owner, restricted)
	require.NoError(t, err)

	extent, err := application_context.DecodeReductionExtent(red.Extent)
	require.NoError(t, err)
	assert.ElementsMatch(t, []uint{group.ID}, extent.GroupIDs)
	assert.Empty(t, extent.ResourceIDs)
}

// Adding a further selection widens the Extent of an existing Reduction rather
// than making a second one, and it deduplicates: selecting overlapping sets twice
// is ordinary, and the Extent is a set.
func TestAddingASelectionWidensTheExtent(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	a := addResourceWithBody(t, tc, "a.txt", "aaa")
	b := addResourceWithBody(t, tc, "b.txt", "bbb")
	c := addResourceWithBody(t, tc, "c.txt", "ccc")

	red := createReduction(t, tc, "Growing", []uint{a.ID, b.ID})

	widened, err := tc.AppCtx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		ID:          red.ID,
		ResourceIds: []uint{b.ID, c.ID},
	}, owner, restricted)
	require.NoError(t, err)
	assert.Equal(t, red.ID, widened.ID, "widening does not make a second Reduction")

	extent, err := application_context.DecodeReductionExtent(widened.Extent)
	require.NoError(t, err)
	assert.ElementsMatch(t, []uint{a.ID, b.ID, c.ID}, extent.ResourceIDs)
	assert.Greater(t, widened.Version, red.Version, "a widened Extent moves the version")
}

// A Reduction is owner-restricted: everyone sees their own, an administrator sees
// every one, and a row whose owner was deleted belongs to nobody. This is the
// DownloadHistoryQuery shape rather than the saved-query one, because a pending
// destructive decision is not a shared artifact.
func TestResourceReductionVisibilityIsOwnerRestricted(t *testing.T) {
	tc := SetupTestEnv(t)

	mine := createReduction(t, tc, "Mine", []uint{1})
	require.NoError(t, tc.DB.Model(&models.ResourceReduction{}).
		Where("id = ?", mine.ID).Update("created_by_user_id", 7).Error)

	theirs := createReduction(t, tc, "Theirs", []uint{1})
	require.NoError(t, tc.DB.Model(&models.ResourceReduction{}).
		Where("id = ?", theirs.ID).Update("created_by_user_id", 8).Error)

	orphan := createReduction(t, tc, "Orphan", []uint{1})
	require.NoError(t, tc.DB.Model(&models.ResourceReduction{}).
		Where("id = ?", orphan.ID).Update("created_by_user_id", nil).Error)

	sevenOwner, sevenRestricted := ownedBy(7)
	visible, err := tc.AppCtx.GetResourceReductions(0, 50, &query_models.ResourceReductionQuery{
		OwnerUserID: sevenOwner, OwnerRestricted: sevenRestricted,
	})
	require.NoError(t, err)
	names := make([]string, 0, len(visible))
	for _, r := range visible {
		names = append(names, r.Name)
	}
	assert.ElementsMatch(t, []string{"Mine"}, names)

	_, err = tc.AppCtx.GetResourceReduction(theirs.ID, sevenOwner, sevenRestricted)
	assert.ErrorIs(t, err, application_context.ErrReductionNotFound,
		"another user's Reduction is absent, not forbidden — ids are not probeable")

	_, err = tc.AppCtx.GetResourceReduction(orphan.ID, sevenOwner, sevenRestricted)
	assert.ErrorIs(t, err, application_context.ErrReductionNotFound, "an ownerless row is nobody's")

	adminOwner, adminRestricted := asAdmin()
	all, err := tc.AppCtx.GetResourceReductions(0, 50, &query_models.ResourceReductionQuery{
		OwnerUserID: adminOwner, OwnerRestricted: adminRestricted,
	})
	require.NoError(t, err)
	assert.Len(t, all, 3, "an administrator sees every Reduction, including the ownerless one")
}

// Deleting somebody else's Reduction is refused the same way reading it is.
func TestDeleteResourceReductionRespectsVisibility(t *testing.T) {
	tc := SetupTestEnv(t)

	red := createReduction(t, tc, "Theirs", []uint{1})
	require.NoError(t, tc.DB.Model(&models.ResourceReduction{}).
		Where("id = ?", red.ID).Update("created_by_user_id", 8).Error)

	owner, restricted := ownedBy(7)
	assert.ErrorIs(t, tc.AppCtx.DeleteResourceReduction(red.ID, owner, restricted),
		application_context.ErrReductionNotFound)

	adminOwner, adminRestricted := asAdmin()
	require.NoError(t, tc.AppCtx.DeleteResourceReduction(red.ID, adminOwner, adminRestricted))

	_, err := tc.AppCtx.GetResourceReduction(red.ID, adminOwner, adminRestricted)
	assert.ErrorIs(t, err, application_context.ErrReductionNotFound)
}

// Every write is a compare-and-set on the version integer. Three independent
// writers share one JSON document, and a last-writer-wins merge there silently
// discards a decision about which files to delete.
func TestResourceReductionWritesAreCompareAndSet(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	red := createReduction(t, tc, "Concurrent", []uint{1})
	staleVersion := red.Version

	first, err := tc.AppCtx.UpdateResourceReductionSettings(&query_models.ResourceReductionEditor{
		ID:      red.ID,
		Name:    "First writer",
		Version: staleVersion,
	}, owner, restricted)
	require.NoError(t, err)
	assert.Equal(t, staleVersion+1, first.Version)

	_, err = tc.AppCtx.UpdateResourceReductionSettings(&query_models.ResourceReductionEditor{
		ID:      red.ID,
		Name:    "Second writer",
		Version: staleVersion,
	}, owner, restricted)
	assert.ErrorIs(t, err, application_context.ErrReductionConflict)

	current, err := tc.AppCtx.GetResourceReduction(red.ID, owner, restricted)
	require.NoError(t, err)
	assert.Equal(t, "First writer", current.Name, "the refused write left nothing behind")
}

// An unknown criterion is dropped rather than silently carried into clustering,
// where it would change which Resource is deleted, and an empty rule falls back
// to the default.
func TestWinnerRuleIsNormalized(t *testing.T) {
	tc := SetupTestEnv(t)
	owner, restricted := asAdmin()

	red, err := tc.AppCtx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		Name:        "Rules",
		ResourceIds: []uint{1},
		WinnerRule:  []string{"size_desc", "not_a_criterion", "size_desc", "created_asc"},
	}, owner, restricted)
	require.NoError(t, err)

	assert.Equal(t,
		[]string{models.WinnerCriterionSizeDesc, models.WinnerCriterionCreatedAsc},
		application_context.DecodeWinnerRule(red.WinnerRule))

	empty, err := tc.AppCtx.CreateOrExtendResourceReduction(&query_models.ResourceReductionCreator{
		Name:        "Empty rule",
		ResourceIds: []uint{1},
		WinnerRule:  []string{"nonsense"},
	}, owner, restricted)
	require.NoError(t, err)
	assert.Equal(t, models.DefaultWinnerRule(), application_context.DecodeWinnerRule(empty.WinnerRule))
}
