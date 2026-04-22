//go:build json1 && fts5

package api_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/models"
)

// BH-037: Resource fetched via GetResource must preload ImageHash when one
// exists, so templates can render DHash/AHash values on the resource detail
// page without running a separate SQL query per view.
func TestGetResource_PreloadsImageHash(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a resource and an image_hashes row pointing at it.
	res := &models.Resource{Name: "img.png", ContentType: "image/png"}
	require.NoError(t, tc.DB.Create(res).Error)

	dhashInt := int64(0x0123456789abcdef)
	ahashInt := int64(0x1122334455667788)
	hash := &models.ImageHash{
		ResourceId: &res.ID,
		DHashInt:   &dhashInt,
		AHashInt:   &ahashInt,
		DHash:      "0123456789abcdef",
		AHash:      "1122334455667788",
	}
	require.NoError(t, tc.DB.Create(hash).Error)

	got, err := tc.AppCtx.GetResource(res.ID)
	require.NoError(t, err)
	require.NotNil(t, got)

	// Pre-fix: ImageHash was not declared on the Resource model, so
	// clause.Associations didn't preload it.
	require.NotNil(t, got.ImageHash,
		"ImageHash should be preloaded by GetResource so the detail template can render perceptual hashes")
	assert.Equal(t, "0123456789abcdef", got.ImageHash.DHash)
	assert.Equal(t, "1122334455667788", got.ImageHash.AHash)
	require.NotNil(t, got.ImageHash.DHashInt)
	assert.Equal(t, dhashInt, *got.ImageHash.DHashInt)
}

// Resources without an ImageHash row should not error — preload simply
// leaves the field nil.
func TestGetResource_NoImageHashLeavesFieldNil(t *testing.T) {
	tc := SetupTestEnv(t)

	res := &models.Resource{Name: "document.pdf", ContentType: "application/pdf"}
	require.NoError(t, tc.DB.Create(res).Error)

	got, err := tc.AppCtx.GetResource(res.ID)
	require.NoError(t, err)
	assert.Nil(t, got.ImageHash, "non-image resources should have no ImageHash and preload should leave the field nil")
}
