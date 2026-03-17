package api_tests

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"mahresources/models"
	"mahresources/models/query_models"
)

// TestMergeResourcesMultipleLosersVersionNumbersUnique verifies that when
// merging multiple losers into a winner, the transferred versions all receive
// unique version numbers. With two losers each having 2 versions, the winner
// should end up with its own version(s) plus 4 transferred versions, each
// with a distinct version_number.
func TestMergeResourcesMultipleLosersVersionNumbersUnique(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create the winner resource
	file1 := io.NopCloser(bytes.NewReader([]byte("winner-content")))
	winner, err := tc.AppCtx.AddResource(file1, "winner.txt", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "Winner"},
	})
	assert.NoError(t, err)

	// Create loser 1
	file2 := io.NopCloser(bytes.NewReader([]byte("loser1-content")))
	loser1, err := tc.AppCtx.AddResource(file2, "loser1.txt", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "Loser 1"},
	})
	assert.NoError(t, err)

	// Create loser 2
	file3 := io.NopCloser(bytes.NewReader([]byte("loser2-content")))
	loser2, err := tc.AppCtx.AddResource(file3, "loser2.txt", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "Loser 2"},
	})
	assert.NoError(t, err)

	// Add an extra version to each loser so each has 2 versions
	loser1ExtraVersion := models.ResourceVersion{
		ResourceID:    loser1.ID,
		VersionNumber: 2,
		Hash:          "loser1-v2-hash",
		HashType:      "SHA1",
		FileSize:      200,
		ContentType:   "text/plain",
		Location:      "/fake/loser1-v2",
		Comment:       "loser1 v2",
	}
	assert.NoError(t, tc.DB.Create(&loser1ExtraVersion).Error)

	loser2ExtraVersion := models.ResourceVersion{
		ResourceID:    loser2.ID,
		VersionNumber: 2,
		Hash:          "loser2-v2-hash",
		HashType:      "SHA1",
		FileSize:      300,
		ContentType:   "text/plain",
		Location:      "/fake/loser2-v2",
		Comment:       "loser2 v2",
	}
	assert.NoError(t, tc.DB.Create(&loser2ExtraVersion).Error)

	// Verify pre-merge version counts
	var winnerVersionCount int64
	tc.DB.Model(&models.ResourceVersion{}).Where("resource_id = ?", winner.ID).Count(&winnerVersionCount)
	assert.Equal(t, int64(1), winnerVersionCount, "winner should have 1 version (initial)")

	var loser1VersionCount int64
	tc.DB.Model(&models.ResourceVersion{}).Where("resource_id = ?", loser1.ID).Count(&loser1VersionCount)
	assert.Equal(t, int64(2), loser1VersionCount, "loser1 should have 2 versions")

	var loser2VersionCount int64
	tc.DB.Model(&models.ResourceVersion{}).Where("resource_id = ?", loser2.ID).Count(&loser2VersionCount)
	assert.Equal(t, int64(2), loser2VersionCount, "loser2 should have 2 versions")

	// Merge both losers into the winner
	err = tc.AppCtx.MergeResources(winner.ID, []uint{loser1.ID, loser2.ID})
	assert.NoError(t, err)

	// All versions should now belong to the winner
	var totalVersions int64
	tc.DB.Model(&models.ResourceVersion{}).Where("resource_id = ?", winner.ID).Count(&totalVersions)
	assert.Equal(t, int64(5), totalVersions,
		"winner should have 5 versions total (1 own + 2 from loser1 + 2 from loser2)")

	// The critical check: all version numbers must be unique
	var versions []models.ResourceVersion
	tc.DB.Where("resource_id = ?", winner.ID).Order("version_number ASC").Find(&versions)

	versionNumbers := make(map[int]int) // version_number -> count
	for _, v := range versions {
		versionNumbers[v.VersionNumber]++
	}

	for vn, count := range versionNumbers {
		assert.Equal(t, 1, count,
			"version_number %d appears %d times — all version numbers must be unique after merging multiple losers", vn, count)
	}

	// Additionally verify version numbers are sequential starting from 1
	// (winner's v1, then loser versions continuing from 2 onward)
	for i, v := range versions {
		assert.Equal(t, i+1, v.VersionNumber,
			"version numbers should be sequential; version at index %d has number %d", i, v.VersionNumber)
	}
}
