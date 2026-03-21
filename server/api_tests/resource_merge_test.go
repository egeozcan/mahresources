package api_tests

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"mahresources/models"
	"mahresources/models/query_models"
)

func TestMergeResourcesTransfersVersions(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create two resources
	file1 := io.NopCloser(bytes.NewReader([]byte("winner-resource-content")))
	winner, err := tc.AppCtx.AddResource(file1, "winner.txt", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "Winner"},
	})
	assert.NoError(t, err)

	file2 := io.NopCloser(bytes.NewReader([]byte("loser-resource-content")))
	loser, err := tc.AppCtx.AddResource(file2, "loser.txt", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "Loser"},
	})
	assert.NoError(t, err)

	// Add an extra version to the loser to simulate version history
	loserVersion := models.ResourceVersion{
		ResourceID:    loser.ID,
		VersionNumber: 99,
		Hash:          "loser-extra-version-hash",
		HashType:      "SHA1",
		FileSize:      200,
		ContentType:   "text/plain",
		Location:      "/fake/loser-v99",
		Comment:       "loser extra version",
	}
	assert.NoError(t, tc.DB.Create(&loserVersion).Error)

	// Count versions before merge
	var winnerVersionsBefore int64
	tc.DB.Model(&models.ResourceVersion{}).Where("resource_id = ?", winner.ID).Count(&winnerVersionsBefore)

	var loserVersionsBefore int64
	tc.DB.Model(&models.ResourceVersion{}).Where("resource_id = ?", loser.ID).Count(&loserVersionsBefore)
	assert.Greater(t, loserVersionsBefore, int64(0), "loser should have versions before merge")

	totalBefore := winnerVersionsBefore + loserVersionsBefore

	// Merge loser into winner
	err = tc.AppCtx.MergeResources(winner.ID, []uint{loser.ID}, false)
	assert.NoError(t, err)

	// The loser's versions should have been transferred to the winner, not deleted
	var winnerVersionsAfter int64
	tc.DB.Model(&models.ResourceVersion{}).Where("resource_id = ?", winner.ID).Count(&winnerVersionsAfter)
	assert.Equal(t, totalBefore, winnerVersionsAfter,
		"winner should have all versions (own + loser's) after merge — versions must not be silently deleted")

	// Verify the loser's extra version specifically survived
	var extraVersionCount int64
	tc.DB.Model(&models.ResourceVersion{}).
		Where("resource_id = ? AND comment = ?", winner.ID, "loser extra version").
		Count(&extraVersionCount)
	assert.Equal(t, int64(1), extraVersionCount,
		"loser's named version should be transferred to winner")
}
