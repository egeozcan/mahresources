//go:build postgres

package api_tests

import (
	"testing"

	"github.com/spf13/afero"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/models"
)

// TestMergeDoesNotCarryLoserAccumulatedBackupsOntoWinnerPG is the Postgres twin of
// the SQLite test of the same name. SetupTestEnv is SQLite on every build tag, so
// without this file the Postgres branch of the meta-merge SQL would never execute
// in CI and a syntax error there would ship green.
//
// What it actually pins is the *snapshot* strip, which compounds on both engines.
// The meta-merge strip cannot be caught here: Postgres's `||` shallow-replaces the
// whole backups key at the final write, so the carried-in copy is overwritten with
// or without the fix. Verified by reverting each half independently — only the
// snapshot revert turns this red.
func TestMergeDoesNotCarryLoserAccumulatedBackupsOntoWinnerPG(t *testing.T) {
	tc := SetupPostgresTestEnv(t)

	winner := addResourceWithBody(t, tc, "winner.txt", "winner-content-pg")
	loser := addResourceWithBody(t, tc, "loser.txt", "loser-content-pg")

	priorBackup := `{"backups":{"resource_999":{"ID":999,"Name":"long gone"}},"keep":"me"}`
	assert.NoError(t, tc.DB.Model(&models.Resource{}).
		Where("id = ?", loser.ID).
		Update("meta", priorBackup).Error)

	assert.NoError(t, tc.AppCtx.MergeResources(winner.ID, []uint{loser.ID}, false))

	var merged models.Resource
	assert.NoError(t, tc.DB.First(&merged, winner.ID).Error)
	metaStr := string(merged.Meta)

	assert.Contains(t, metaStr, `"keep"`, "the loser's ordinary meta must still merge across")
	assert.NotContains(t, metaStr, "resource_999",
		"the loser's accumulated backups must not compound onto the winner")
}

// TestMergeDoesNotBackUpAFileItIsNotRemovingPG pins the conditional backup copy on
// Postgres too: the reference count runs different SQL per engine via GORM, and the
// retention decision it feeds is what gates the copy.
func TestMergeDoesNotBackUpAFileItIsNotRemovingPG(t *testing.T) {
	tc := SetupPostgresTestEnv(t)

	winner := addResourceWithBody(t, tc, "winner.txt", "winner-content-pg2")
	loser := addResourceWithBody(t, tc, "loser.txt", "loser-content-pg2")
	loserLocation := loser.GetCleanLocation()

	assert.NoError(t, tc.AppCtx.MergeResources(winner.ID, []uint{loser.ID}, true))

	exists, err := afero.Exists(tc.Fs, loserLocation)
	assert.NoError(t, err)
	assert.True(t, exists, "loser's file must be retained: a version references its hash")

	assert.Empty(t, deletedFiles(t, tc.Fs),
		"a file that is not being removed must not be copied into /deleted")
}

// TestCountHashReferencesIsPerStorageLocationPG runs the two-store discrimination
// against Postgres. The predicate is GORM-built and the NULL-versus-empty-string
// handling is where the engines are most likely to differ.
func TestCountHashReferencesIsPerStorageLocationPG(t *testing.T) {
	tc := SetupPostgresTestEnv(t)

	const sharedHash = "cccccccccccccccccccccccccccccccccccccccc"
	alt := "photos"
	blank := ""

	require.NoError(t, tc.DB.Create(&models.Resource{
		Name: "on the main store", Hash: sharedHash, HashType: "SHA1",
		Location: "/resources/cc/cc/cc/" + sharedHash, ResourceCategoryId: 1,
	}).Error)
	require.NoError(t, tc.DB.Create(&models.Resource{
		Name: "on the alt store", Hash: sharedHash, HashType: "SHA1",
		Location: "/resources/cc/cc/cc/" + sharedHash, StorageLocation: &alt,
		ResourceCategoryId: 1,
	}).Error)
	require.NoError(t, tc.DB.Create(&models.Resource{
		Name: "unrelated, different hash", Hash: "dddddddddddddddddddddddddddddddddddddddd",
		HashType: "SHA1", Location: "/resources/dd/dd/dd/d", StorageLocation: &blank,
		ResourceCategoryId: 1,
	}).Error)

	mainCount, err := tc.AppCtx.CountHashReferences(sharedHash, nil)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), mainCount)

	altCount, err := tc.AppCtx.CountHashReferences(sharedHash, &alt)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), altCount)
}
