package api_tests

import (
	"encoding/json"
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEditSelfRefRelationTypeRejectsCategoryMismatch verifies that
// EditRelationType rejects changes to a self-referential relation type
// that would make FromCategory != ToCategory.
//
// Self-referential types (BackRelationId == ID) use themselves as the
// back-relation. AddRelation auto-creates a back-relation with the groups
// swapped but the same type. If FromCategory != ToCategory, the back-
// relation's from_group would violate the type's FromCategory constraint
// (since it was originally the to_group of the opposite category).
//
// AddRelationType correctly rejects this at creation time, but
// EditRelationType does not enforce the invariant — allowing a previously
// valid self-referential type to become inconsistent.
func TestEditSelfRefRelationTypeRejectsCategoryMismatch(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create two categories
	catA := &models.Category{Name: "SelfRefCatA"}
	catB := &models.Category{Name: "SelfRefCatB"}
	tc.DB.Create(catA)
	tc.DB.Create(catB)
	require.NotZero(t, catA.ID)
	require.NotZero(t, catB.ID)
	require.NotEqual(t, catA.ID, catB.ID)

	// Create a valid self-referential relation type (FromCategory == ToCategory)
	createResp := tc.MakeRequest(http.MethodPost, "/v1/relationType", map[string]any{
		"Name":         "sibling",
		"FromCategory": catA.ID,
		"ToCategory":   catA.ID,
		"ReverseName":  "sibling", // same name = self-referential
	})
	require.Equal(t, http.StatusOK, createResp.Code, "creating self-ref type should succeed")

	var created models.GroupRelationType
	err := json.Unmarshal(createResp.Body.Bytes(), &created)
	require.NoError(t, err)
	require.NotZero(t, created.ID)

	// Verify it is truly self-referential
	var selfRef models.GroupRelationType
	tc.DB.First(&selfRef, created.ID)
	require.NotNil(t, selfRef.BackRelationId, "type should have a BackRelationId")
	require.Equal(t, selfRef.ID, *selfRef.BackRelationId,
		"BackRelationId should point to itself for self-referential type")

	// Now edit the self-referential type, changing ToCategory to a different
	// category. This should be rejected because it would break the
	// FromCategory == ToCategory invariant required by self-referential types.
	editResp := tc.MakeRequest(http.MethodPost, "/v1/relationType/edit", map[string]any{
		"Id":         created.ID,
		"ToCategory": catB.ID,
	})
	assert.NotEqual(t, http.StatusOK, editResp.Code,
		"editing a self-referential relation type to have different FromCategory and ToCategory should be rejected")

	// Even if the HTTP response doesn't indicate failure, verify the database
	// state: the type should still have matching categories.
	var afterEdit models.GroupRelationType
	tc.DB.First(&afterEdit, created.ID)
	if afterEdit.FromCategoryId != nil && afterEdit.ToCategoryId != nil {
		assert.Equal(t, *afterEdit.FromCategoryId, *afterEdit.ToCategoryId,
			"self-referential relation type must always have FromCategoryId == ToCategoryId; "+
				"EditRelationType allowed them to diverge")
	}
}
