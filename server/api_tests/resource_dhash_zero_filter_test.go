//go:build json1 && fts5

package api_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"mahresources/models"
	"mahresources/models/query_models"
)

// BH-037: ShowDhashZero filter must select only resources whose perceptual
// DHash is zero (BH-018 solid-colour false-positive class). This powers the
// admin-overview drill-down.
func TestResources_ShowDhashZero_FiltersToZeroDHashOnly(t *testing.T) {
	tc := SetupTestEnv(t)

	// Three resources: one with zero dhash (int), one with non-zero dhash,
	// one with legacy string "0000000000000000".
	resZero := &models.Resource{Name: "solid-colour.png", ContentType: "image/png"}
	resNonZero := &models.Resource{Name: "photo.jpg", ContentType: "image/jpeg"}
	resLegacyZero := &models.Resource{Name: "legacy-solid.png", ContentType: "image/png"}
	resNoHash := &models.Resource{Name: "document.pdf", ContentType: "application/pdf"}

	require.NoError(t, tc.DB.Create(resZero).Error)
	require.NoError(t, tc.DB.Create(resNonZero).Error)
	require.NoError(t, tc.DB.Create(resLegacyZero).Error)
	require.NoError(t, tc.DB.Create(resNoHash).Error)

	zero := int64(0)
	nonZero := int64(0x1234567890abcdef)
	require.NoError(t, tc.DB.Create(&models.ImageHash{ResourceId: &resZero.ID, DHashInt: &zero, DHash: "0000000000000000"}).Error)
	require.NoError(t, tc.DB.Create(&models.ImageHash{ResourceId: &resNonZero.ID, DHashInt: &nonZero, DHash: "1234567890abcdef"}).Error)
	// Legacy row: no int column populated, just the old hex string "0000..."
	require.NoError(t, tc.DB.Create(&models.ImageHash{ResourceId: &resLegacyZero.ID, DHash: "0000000000000000"}).Error)

	got, err := tc.AppCtx.GetResources(0, 100, &query_models.ResourceSearchQuery{
		ShowDhashZero: true,
	})
	require.NoError(t, err)

	names := make([]string, 0, len(got))
	for _, r := range got {
		names = append(names, r.Name)
	}

	assert.Contains(t, names, "solid-colour.png", "zero DHashInt should be included")
	assert.Contains(t, names, "legacy-solid.png", "legacy zero DHash string should be included")
	assert.NotContains(t, names, "photo.jpg", "non-zero DHash should NOT be included")
	assert.NotContains(t, names, "document.pdf", "resources without an ImageHash row should NOT be included")
}
