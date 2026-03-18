package api_tests

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"mahresources/models"
)

// TestRotateResourceSyncsHashType verifies that RotateResource updates the
// parent resource's hash_type field to match the version it creates.
//
// Bug: RotateResource creates a new version with HashType "SHA1" and syncs
// most resource fields (hash, location, content_type, width, height,
// file_size) but omits hash_type from the update map. If the resource had a
// legacy hash_type (e.g. "MD5" from a migration), the resource will have
// a SHA1 hash but still report its hash_type as "MD5".
//
// Compare with UploadNewVersion and RestoreVersion, which both correctly
// include "hash_type" in their resource update maps.
func TestRotateResourceSyncsHashType(t *testing.T) {
	tc := SetupTestEnv(t)

	// Migrate ResourceVersion table (not included in the default SetupTestEnv migration)
	require.NoError(t, tc.DB.AutoMigrate(&models.ResourceVersion{}))

	// Create a minimal 4x4 PNG image
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for x := 0; x < 4; x++ {
		for y := 0; y < 4; y++ {
			img.Set(x, y, color.RGBA{R: 255, G: 0, B: 0, A: 255})
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	imgBytes := buf.Bytes()

	// Write the image to the in-memory filesystem
	imgPath := "/resources/ab/cd/ef/abcdef1234567890abcdef1234567890abcdef12.png"
	fs, err := tc.AppCtx.GetFsForStorageLocation(nil)
	require.NoError(t, err)
	require.NoError(t, fs.MkdirAll(filepath.Dir(imgPath), 0755))
	f, err := fs.Create(imgPath)
	require.NoError(t, err)
	_, err = f.Write(imgBytes)
	require.NoError(t, err)
	require.NoError(t, f.Close())

	// Create an owner group
	owner := tc.CreateDummyGroup("rotate-owner")

	// Create a resource pointing to this image, with a LEGACY hash_type
	resource := &models.Resource{
		Name:        "rotate-test.png",
		Hash:        "abcdef1234567890abcdef1234567890abcdef12",
		HashType:    "MD5", // intentionally wrong — simulates legacy/migrated data
		Location:    imgPath,
		ContentType: "image/png",
		FileSize:    int64(len(imgBytes)),
		Width:       4,
		Height:      4,
		OwnerId:     &owner.ID,
	}
	require.NoError(t, tc.DB.Create(resource).Error)

	// Rotate the resource (any angle is fine; 90° is standard)
	err = tc.AppCtx.RotateResource(resource.ID, 90)
	require.NoError(t, err, "RotateResource should succeed")

	// Reload the resource from the database
	var updated models.Resource
	require.NoError(t, tc.DB.First(&updated, resource.ID).Error)

	// The hash must have changed (rotated image has different bytes)
	assert.NotEqual(t, resource.Hash, updated.Hash,
		"Hash should change after rotation")

	// Critical assertion: hash_type must be updated to "SHA1" because
	// the new version was created with SHA1 hashing.
	assert.Equal(t, "SHA1", updated.HashType,
		"RotateResource should sync hash_type from the new version to the resource; "+
			"currently RotateResource omits hash_type from its resource update map, "+
			"leaving the stale value %q", updated.HashType)
}
