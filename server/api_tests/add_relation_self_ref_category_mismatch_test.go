package api_tests

import (
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAddRelationTypeSelfRefRejectsDifferentCategories verifies that
// AddRelationType rejects self-referential relation types (ReverseName == Name)
// when FromCategory != ToCategory.
//
// A self-referential type uses itself as the back-relation. AddRelation swaps
// the from/to groups for the back-relation but uses the same type. If
// FromCategory != ToCategory, the back-relation's from_group would always
// violate the type's FromCategory constraint. The fix prevents this at the
// type creation level.
func TestAddRelationTypeSelfRefRejectsDifferentCategories(t *testing.T) {
	tc := SetupTestEnv(t)

	catA := &models.Category{Name: "SRCatAlpha"}
	catB := &models.Category{Name: "SRCatBeta"}
	tc.DB.Create(catA)
	tc.DB.Create(catB)
	require.NotZero(t, catA.ID)
	require.NotZero(t, catB.ID)
	require.NotEqual(t, catA.ID, catB.ID)

	// Attempt to create a self-referential type with different categories.
	// This should be rejected because the back-relation would always violate
	// the type's category constraints.
	resp := tc.MakeRequest(http.MethodPost, "/v1/relationType", map[string]any{
		"Name":         "sr-invalid",
		"FromCategory": catA.ID,
		"ToCategory":   catB.ID,
		"ReverseName":  "sr-invalid", // same name = self-referential
	})
	assert.NotEqual(t, http.StatusOK, resp.Code,
		"self-referential relation type with FromCategory != ToCategory should be rejected")

	// Verify the type was NOT created
	var count int64
	tc.DB.Model(&models.GroupRelationType{}).Where("name = ?", "sr-invalid").Count(&count)
	assert.Equal(t, int64(0), count,
		"no relation type should exist after rejection")
}

// TestAddRelationTypeSelfRefAcceptsSameCategory verifies that self-referential
// types with FromCategory == ToCategory are still accepted.
func TestAddRelationTypeSelfRefAcceptsSameCategory(t *testing.T) {
	tc := SetupTestEnv(t)

	cat := &models.Category{Name: "SRSameCat"}
	tc.DB.Create(cat)
	require.NotZero(t, cat.ID)

	resp := tc.MakeRequest(http.MethodPost, "/v1/relationType", map[string]any{
		"Name":         "sr-valid",
		"FromCategory": cat.ID,
		"ToCategory":   cat.ID,
		"ReverseName":  "sr-valid",
	})
	assert.Equal(t, http.StatusOK, resp.Code,
		"self-referential type with same from/to category should succeed")

	var relType models.GroupRelationType
	err := tc.DB.Where("name = ?", "sr-valid").First(&relType).Error
	require.NoError(t, err)
	require.NotNil(t, relType.BackRelationId)
	assert.Equal(t, relType.ID, *relType.BackRelationId,
		"type should be self-referential")
}
