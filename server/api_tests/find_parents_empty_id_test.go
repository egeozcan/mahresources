package api_tests

import (
	"encoding/json"
	"mahresources/models"
	"mahresources/models/query_models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFindParentsNonExistentGroupReturnsEmpty verifies that requesting
// parents of a non-existent group returns an empty list, not all groups.
func TestFindParentsNonExistentGroupReturnsEmpty(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create some groups so the database isn't empty
	catPayload := query_models.CategoryCreator{Name: "ParentsTestCat"}
	resp := tc.MakeRequest(http.MethodPost, "/v1/category", catPayload)
	require.Equal(t, http.StatusOK, resp.Code)
	var category models.Category
	json.Unmarshal(resp.Body.Bytes(), &category)

	for i := 0; i < 3; i++ {
		groupPayload := query_models.GroupCreator{
			Name:       "ParentsTestGroup",
			CategoryId: category.ID,
		}
		resp = tc.MakeRequest(http.MethodPost, "/v1/group", groupPayload)
		require.Equal(t, http.StatusOK, resp.Code)
	}

	// Request parents of a non-existent group
	resp = tc.MakeRequest(http.MethodGet, "/v1/group/parents?id=99999", nil)
	assert.Equal(t, http.StatusOK, resp.Code)

	var parents []models.Group
	json.Unmarshal(resp.Body.Bytes(), &parents)

	// Should return empty, NOT all groups
	assert.Equal(t, 0, len(parents),
		"FindParentsOfGroup with non-existent ID should return empty list, not all groups")
}
