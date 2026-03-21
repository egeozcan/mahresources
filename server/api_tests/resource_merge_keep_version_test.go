package api_tests

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"mahresources/models"
	"mahresources/models/query_models"
)

// TestMergeResourcesKeepAsVersion verifies that when keepAsVersion=true, a new
// ResourceVersion is created from each loser's resource-level file data, so the
// total version count on the winner is winnerVersionsBefore + loserVersionsBefore + 1
// (the +1 being the new version from the loser's resource-level file).
func TestMergeResourcesKeepAsVersion(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create winner resource
	file1 := io.NopCloser(bytes.NewReader([]byte("winner-resource-content")))
	winner, err := tc.AppCtx.AddResource(file1, "winner.txt", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "Winner Resource"},
	})
	assert.NoError(t, err)

	// Create loser resource
	file2 := io.NopCloser(bytes.NewReader([]byte("loser-resource-content")))
	loser, err := tc.AppCtx.AddResource(file2, "loser.txt", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "Loser Resource"},
	})
	assert.NoError(t, err)

	// Record loser's resource-level file data
	loserHash := loser.Hash
	loserContentType := loser.ContentType

	// Count versions before merge
	var winnerVersionsBefore int64
	tc.DB.Model(&models.ResourceVersion{}).Where("resource_id = ?", winner.ID).Count(&winnerVersionsBefore)

	var loserVersionsBefore int64
	tc.DB.Model(&models.ResourceVersion{}).Where("resource_id = ?", loser.ID).Count(&loserVersionsBefore)

	// Merge loser into winner with keepAsVersion=true
	err = tc.AppCtx.MergeResources(winner.ID, []uint{loser.ID}, true)
	assert.NoError(t, err)

	// Winner should have winnerVersionsBefore + loserVersionsBefore + 1 versions
	// The +1 is the new version created from the loser's resource-level file
	var winnerVersionsAfter int64
	tc.DB.Model(&models.ResourceVersion{}).Where("resource_id = ?", winner.ID).Count(&winnerVersionsAfter)
	assert.Equal(t, winnerVersionsBefore+loserVersionsBefore+1, winnerVersionsAfter,
		"winner should have own versions + loser's existing versions + 1 new version from loser's resource-level file")

	// Verify the new version was created with the loser's hash and correct comment
	var keepVersionCount int64
	tc.DB.Model(&models.ResourceVersion{}).
		Where("resource_id = ? AND hash = ? AND comment LIKE ?", winner.ID, loserHash, "%Merged from: Loser Resource%").
		Count(&keepVersionCount)
	assert.Equal(t, int64(1), keepVersionCount,
		"a version with loser's hash and 'Merged from: Loser Resource' comment should exist on winner")

	// Also verify the content type matches
	var keepVersion models.ResourceVersion
	tc.DB.Model(&models.ResourceVersion{}).
		Where("resource_id = ? AND hash = ?", winner.ID, loserHash).
		First(&keepVersion)
	assert.Equal(t, loserContentType, keepVersion.ContentType,
		"the kept version should have the loser's content type")
}

// TestMergeResourcesKeepAsVersionFalse verifies that when keepAsVersion=false,
// no extra version is created from the loser's resource-level file data.
// The winner should only have winnerVersionsBefore + loserVersionsBefore versions.
func TestMergeResourcesKeepAsVersionFalse(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create winner resource
	file1 := io.NopCloser(bytes.NewReader([]byte("winner-content-false-test")))
	winner, err := tc.AppCtx.AddResource(file1, "winner2.txt", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "Winner Resource 2"},
	})
	assert.NoError(t, err)

	// Create loser resource
	file2 := io.NopCloser(bytes.NewReader([]byte("loser-content-false-test")))
	loser, err := tc.AppCtx.AddResource(file2, "loser2.txt", &query_models.ResourceCreator{
		ResourceQueryBase: query_models.ResourceQueryBase{Name: "Loser Resource 2"},
	})
	assert.NoError(t, err)

	// Count versions before merge
	var winnerVersionsBefore int64
	tc.DB.Model(&models.ResourceVersion{}).Where("resource_id = ?", winner.ID).Count(&winnerVersionsBefore)

	var loserVersionsBefore int64
	tc.DB.Model(&models.ResourceVersion{}).Where("resource_id = ?", loser.ID).Count(&loserVersionsBefore)

	// Merge loser into winner with keepAsVersion=false
	err = tc.AppCtx.MergeResources(winner.ID, []uint{loser.ID}, false)
	assert.NoError(t, err)

	// Winner should have exactly winnerVersionsBefore + loserVersionsBefore versions (no extra)
	var winnerVersionsAfter int64
	tc.DB.Model(&models.ResourceVersion{}).Where("resource_id = ?", winner.ID).Count(&winnerVersionsAfter)
	assert.Equal(t, winnerVersionsBefore+loserVersionsBefore, winnerVersionsAfter,
		"winner should have own versions + loser's existing versions only (no extra version when keepAsVersion=false)")
}
