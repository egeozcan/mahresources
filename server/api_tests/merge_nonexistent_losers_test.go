package api_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/models"
)

func TestMergeGroupsNonExistentLoserFails(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	winner := &models.Group{Name: "Winner Group"}
	tc.DB.Create(winner)

	// Attempt merge with a non-existent loser ID
	err := tc.AppCtx.MergeGroups(winner.ID, []uint{999999})
	require.Error(t, err, "merge with non-existent loser should fail")
	assert.Contains(t, err.Error(), "not found",
		"error message should indicate losers were not found")
}

func TestMergeResourcesNonExistentLoserFails(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	winner := &models.Resource{Name: "Winner Resource", Meta: []byte(`{}`), OwnMeta: []byte(`{}`)}
	tc.DB.Create(winner)

	// Attempt merge with a non-existent loser ID
	err := tc.AppCtx.MergeResources(winner.ID, []uint{999999}, false)
	require.Error(t, err, "merge with non-existent loser should fail")
	assert.Contains(t, err.Error(), "not found",
		"error message should indicate losers were not found")
}

func TestMergeGroupsPartialNonExistentLoserFails(t *testing.T) {
	tc := SetupTestEnv(t)
	requireJsonPatch(t, tc.DB)

	winner := &models.Group{Name: "Winner"}
	tc.DB.Create(winner)
	realLoser := &models.Group{Name: "Real Loser"}
	tc.DB.Create(realLoser)

	// One real, one fake -- should still fail
	err := tc.AppCtx.MergeGroups(winner.ID, []uint{realLoser.ID, 999999})
	assert.Error(t, err, "merge with partially non-existent losers should fail")
}
