package api_tests

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/models"
	"mahresources/models/query_models"
)

// deletedFiles lists every file the merge/delete backup directory holds. Asserting
// on the count rather than on a reconstructed path keeps these tests off the exact
// naming scheme, which is not the behaviour under test.
func deletedFiles(t *testing.T, fs afero.Fs) []string {
	t.Helper()
	var found []string
	_ = afero.Walk(fs, "/deleted", func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		found = append(found, p)
		return nil
	})
	return found
}

func addResourceWithBody(t *testing.T, tc *TestContext, name, body string) *models.Resource {
	t.Helper()
	r, err := tc.AppCtx.AddResource(
		io.NopCloser(bytes.NewReader([]byte(body))),
		name,
		&query_models.ResourceCreator{
			ResourceQueryBase: query_models.ResourceQueryBase{Name: name},
		},
	)
	require.NoError(t, err)
	require.NotNil(t, r)
	return r
}

// TestMergeDoesNotBackUpAFileItIsNotRemoving pins the fix for the unconditional
// backup copy. With keepAsVersion=true the Loser's bytes are retained because a
// new ResourceVersion references its hash, so ShouldRemoveSource is false — and a
// backup of a file that is staying put is pure waste written into a directory
// with no readers and no retention sweep.
func TestMergeDoesNotBackUpAFileItIsNotRemoving(t *testing.T) {
	tc := SetupTestEnv(t)

	winner := addResourceWithBody(t, tc, "winner.txt", "winner-content")
	loser := addResourceWithBody(t, tc, "loser.txt", "loser-content")
	loserLocation := loser.GetCleanLocation()

	assert.Empty(t, deletedFiles(t, tc.Fs), "precondition: nothing in /deleted yet")

	assert.NoError(t, tc.AppCtx.MergeResources(winner.ID, []uint{loser.ID}, true))

	// The version keeps the hash alive, so the source must survive...
	exists, err := afero.Exists(tc.Fs, loserLocation)
	assert.NoError(t, err)
	assert.True(t, exists, "loser's file must be retained: a version references its hash")

	// ...and precisely because it survives, nothing should have been backed up.
	assert.Empty(t, deletedFiles(t, tc.Fs),
		"a file that is not being removed must not be copied into /deleted")
}

// TestMergeStillBacksUpAFileItRemoves is the complement: the backup must survive
// for the case it was actually written for. Without it, "stop copying" could be
// implemented by deleting the backup outright.
//
// Forcing that case takes work, and the reason is worth recording. A merge
// transfers every Loser version to the Winner unconditionally, and AddResource
// always creates an initial version — so after an ordinary merge the Loser's hash
// is still referenced by a version now owned by the Winner, and its file is
// correctly retained whatever keepAsVersion says. Dropping the Loser's versions
// first is the only way to reach a zero reference count.
func TestMergeStillBacksUpAFileItRemoves(t *testing.T) {
	tc := SetupTestEnv(t)

	winner := addResourceWithBody(t, tc, "winner.txt", "winner-content")
	loser := addResourceWithBody(t, tc, "loser.txt", "loser-content")
	loserLocation := loser.GetCleanLocation()

	// Drop the loser's versions so nothing will hold its hash after the merge.
	assert.NoError(t, tc.DB.Model(&models.Resource{}).
		Where("id = ?", loser.ID).Update("current_version_id", nil).Error)
	assert.NoError(t, tc.DB.Where("resource_id = ?", loser.ID).
		Delete(&models.ResourceVersion{}).Error)

	assert.NoError(t, tc.AppCtx.MergeResources(winner.ID, []uint{loser.ID}, false))

	exists, err := afero.Exists(tc.Fs, loserLocation)
	assert.NoError(t, err)
	assert.False(t, exists, "with nothing holding the hash, the source must be removed")

	backups := deletedFiles(t, tc.Fs)
	assert.Len(t, backups, 1, "a file that IS removed must still be backed up first")
	if len(backups) == 1 {
		assert.True(t, strings.Contains(backups[0], loser.Hash),
			"backup should be named for the loser's hash, got %q", backups[0])
	}
}

// TestMergeRetainsALoserFileItsVersionStillReferences pins the behaviour the test
// above had to work around, because it is the reason a merge reclaims no disk: the
// transferred version keeps the bytes alive, and keepAsVersion does not control
// this.
func TestMergeRetainsALoserFileItsVersionStillReferences(t *testing.T) {
	tc := SetupTestEnv(t)

	winner := addResourceWithBody(t, tc, "winner.txt", "winner-content")
	loser := addResourceWithBody(t, tc, "loser.txt", "loser-content")
	loserLocation := loser.GetCleanLocation()

	assert.NoError(t, tc.AppCtx.MergeResources(winner.ID, []uint{loser.ID}, false))

	var held int64
	assert.NoError(t, tc.DB.Model(&models.ResourceVersion{}).
		Where("resource_id = ? AND hash = ?", winner.ID, loser.Hash).Count(&held).Error)
	assert.Greater(t, held, int64(0),
		"the loser's version is transferred to the winner and still names its hash")

	exists, err := afero.Exists(tc.Fs, loserLocation)
	assert.NoError(t, err)
	assert.True(t, exists, "a referenced file must be retained even with keepAsVersion=false")

	assert.Empty(t, deletedFiles(t, tc.Fs), "and nothing retained should be backed up")
}

// TestMergeDoesNotCarryLoserAccumulatedBackupsOntoWinner pins the second path by
// which backups compound. Stripping the marshalled snapshot is not enough: the
// per-Loser metadata merge copies the Loser's *stored* meta onto the Winner, and
// the Winner's own keys only win where they exist.
func TestMergeDoesNotCarryLoserAccumulatedBackupsOntoWinner(t *testing.T) {
	tc := SetupTestEnv(t)

	winner := addResourceWithBody(t, tc, "winner.txt", "winner-content")
	loser := addResourceWithBody(t, tc, "loser.txt", "loser-content")

	// The loser is itself the product of an earlier merge, so it carries a backups key.
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
