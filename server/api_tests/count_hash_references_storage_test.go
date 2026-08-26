package api_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"mahresources/models"
)

// TestCountHashReferencesIsPerStorageLocation pins the fix for a reference count
// that was blind to which filesystem a file lives on. Storage is content-addressed,
// so one hash means one path — but only within a single filesystem. The same hash
// held on the main store and on an alternative store is two distinct files behind
// one count, so deleting either one saw a non-zero count and removed neither. That
// leaks a file with nothing in the database pointing at it.
func TestCountHashReferencesIsPerStorageLocation(t *testing.T) {
	tc := SetupTestEnv(t)

	const sharedHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	alt := "photos"

	assert.NoError(t, tc.DB.Create(&models.Resource{
		Name: "on the main store", Hash: sharedHash, HashType: "SHA1",
		Location: "/resources/aa/aa/aa/" + sharedHash, ResourceCategoryId: 1,
	}).Error)
	assert.NoError(t, tc.DB.Create(&models.Resource{
		Name: "on the alt store", Hash: sharedHash, HashType: "SHA1",
		Location: "/resources/aa/aa/aa/" + sharedHash, StorageLocation: &alt,
		ResourceCategoryId: 1,
	}).Error)

	// An unrelated row on the main store, and its storage_location must be the empty
	// string rather than NULL. If the predicate is not parenthesised, the OR binds
	// looser than the AND and `storage_location = ''` sweeps this row into every
	// count — but only if it can match, and NULL = '' never does. With NULL here the
	// assertion below holds whether or not the parentheses are present.
	blank := ""
	assert.NoError(t, tc.DB.Create(&models.Resource{
		Name: "unrelated, different hash", Hash: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
		HashType: "SHA1", Location: "/resources/bb/bb/bb/b", StorageLocation: &blank,
		ResourceCategoryId: 1,
	}).Error)

	mainCount, err := tc.AppCtx.CountHashReferences(sharedHash, nil)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), mainCount,
		"the main store holds one file with this hash, not two")

	altCount, err := tc.AppCtx.CountHashReferences(sharedHash, &alt)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), altCount,
		"the alt store holds one file with this hash, not two")

	// A pointer to the empty string means the main store, same as nil.
	empty := ""
	emptyCount, err := tc.AppCtx.CountHashReferences(sharedHash, &empty)
	assert.NoError(t, err)
	assert.Equal(t, int64(1), emptyCount,
		"an empty storage location is the main store, not a third one")
}
