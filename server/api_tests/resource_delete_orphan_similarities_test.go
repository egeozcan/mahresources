package api_tests

import (
	"mahresources/models"
	"path"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestDeleteResourceOrphansResourceSimilarities demonstrates that deleting a
// resource leaves orphaned rows in the resource_similarities table.
//
// Root cause:
// DeleteResource uses db.Select(clause.Associations).Delete(&resource) to
// remove the resource and its declared GORM associations (Tags, Notes, Groups,
// Previews, Versions). However, ResourceSimilarity is NOT declared as an
// association on the Resource model — it is a separate entity that references
// resources via ResourceID1 and ResourceID2 foreign keys.
//
// The ResourceSimilarity model declares OnDelete:CASCADE FK constraints, but
// SQLite FK cascades do not fire reliably inside transactions (a well-known
// issue documented throughout the codebase — see EnsureForeignKeysActive and
// the many explicit cleanup steps in DeleteGroup, DeleteNoteType, etc.).
// DeleteResource does NOT call EnsureForeignKeysActive, nor does it explicitly
// delete resource_similarities rows before removing the resource.
//
// Impact:
// After deleting a resource that has similarity records, those records remain
// in the database pointing to a non-existent resource ID. For deployments with
// millions of resources, these orphaned rows accumulate over time, wasting
// storage and potentially corrupting similarity query results (the API could
// return similarity pairs where one resource no longer exists).
func TestDeleteResourceOrphansResourceSimilarities(t *testing.T) {
	tc := SetupTestEnv(t)

	// AutoMigrate ResourceSimilarity (not included in the default test setup)
	err := tc.DB.AutoMigrate(&models.ResourceSimilarity{})
	assert.NoError(t, err)

	// Get the in-memory filesystem so we can create dummy resource files.
	// DeleteResource copies files to a /deleted/ backup folder, so the files
	// must exist on the filesystem.
	fs, fsErr := tc.AppCtx.GetFsForStorageLocation(nil)
	assert.NoError(t, fsErr)

	// Create two resources directly in the DB with dummy file locations.
	res1 := &models.Resource{
		Name:        "Image A",
		Hash:        "aabbccddee0011223344aabbccddee0011223344",
		HashType:    "SHA1",
		Location:    "/resources/aa/bb/cc/aabbccddee0011223344aabbccddee0011223344.png",
		ContentType: "image/png",
		FileSize:    100,
		Meta:        []byte("{}"),
		OwnMeta:     []byte("{}"),
	}
	res2 := &models.Resource{
		Name:        "Image B",
		Hash:        "11223344556677889900aabbccddeeff00112233",
		HashType:    "SHA1",
		Location:    "/resources/11/22/33/11223344556677889900aabbccddeeff00112233.png",
		ContentType: "image/png",
		FileSize:    200,
		Meta:        []byte("{}"),
		OwnMeta:     []byte("{}"),
	}

	tc.DB.Create(res1)
	tc.DB.Create(res2)
	assert.NotZero(t, res1.ID)
	assert.NotZero(t, res2.ID)

	// Create dummy files on the in-memory filesystem so DeleteResource doesn't
	// fail trying to open the file for backup.
	for _, loc := range []string{res1.Location, res2.Location} {
		assert.NoError(t, fs.MkdirAll(path.Dir(loc), 0755))
		f, createErr := fs.Create(loc)
		assert.NoError(t, createErr)
		_, _ = f.Write([]byte("dummy file content"))
		_ = f.Close()
	}

	// Create a ResourceSimilarity record linking the two resources.
	// ResourceID1 must be less than ResourceID2 per the model invariant.
	id1, id2 := res1.ID, res2.ID
	if id1 > id2 {
		id1, id2 = id2, id1
	}
	sim := &models.ResourceSimilarity{
		ResourceID1:     id1,
		ResourceID2:     id2,
		HammingDistance: 5,
	}
	err = tc.DB.Create(sim).Error
	assert.NoError(t, err)
	assert.NotZero(t, sim.ID)

	// Verify the similarity row exists before deletion.
	var countBefore int64
	tc.DB.Model(&models.ResourceSimilarity{}).
		Where("resource_id1 = ? OR resource_id2 = ?", res1.ID, res1.ID).
		Count(&countBefore)
	assert.Equal(t, int64(1), countBefore, "setup: should have 1 similarity row referencing res1")

	// Delete res1 via the application context.
	err = tc.AppCtx.DeleteResource(res1.ID)
	assert.NoError(t, err, "DeleteResource should succeed")

	// Verify the resource itself is gone.
	var resourceCheck models.Resource
	result := tc.DB.First(&resourceCheck, res1.ID)
	assert.Error(t, result.Error, "resource should be deleted from the database")

	// BUG: The similarity row should have been cleaned up (either by explicit
	// DELETE or by a working FK cascade), but it remains orphaned because:
	// 1. ResourceSimilarity is not a GORM association on Resource
	// 2. SQLite FK cascades don't fire reliably inside transactions
	// 3. DeleteResource doesn't explicitly delete resource_similarities rows
	var countAfter int64
	tc.DB.Model(&models.ResourceSimilarity{}).
		Where("resource_id1 = ? OR resource_id2 = ?", res1.ID, res1.ID).
		Count(&countAfter)
	assert.Equal(t, int64(0), countAfter,
		"resource_similarities rows referencing the deleted resource should be cleaned up, "+
			"but they remain orphaned because DeleteResource does not explicitly delete them "+
			"and SQLite FK cascades do not fire reliably inside transactions")
}
