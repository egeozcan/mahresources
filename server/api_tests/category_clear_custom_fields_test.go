package api_tests

import (
	"encoding/json"
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateCategoryCanClearCustomHeaderToEmpty verifies that a user can
// remove a previously-set CustomHeader by sending an explicit empty string.
//
// BUG: UpdateCategory guards every field with `if value != ""`, so once
// CustomHeader (or CustomSidebar, Description, etc.) has been set to a
// non-empty string it can never be cleared back to empty via the API.
func TestUpdateCategoryCanClearCustomHeaderToEmpty(t *testing.T) {
	tc := SetupTestEnv(t)

	// Step 1: Create a category with a non-empty CustomHeader
	createBody := map[string]any{
		"Name":         "Cat With Header",
		"Description":  "Some description",
		"CustomHeader": "<h1>Big Header</h1>",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/category", createBody)
	require.Equal(t, http.StatusOK, resp.Code, "creating the category should succeed")

	var created models.Category
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))
	require.Equal(t, "<h1>Big Header</h1>", created.CustomHeader,
		"category should be created with the supplied CustomHeader")

	// Step 2: Update the category, explicitly clearing CustomHeader to ""
	updateBody := map[string]any{
		"ID":           created.ID,
		"Name":         "Cat With Header",
		"Description":  "Some description",
		"CustomHeader": "",
	}
	resp = tc.MakeRequest(http.MethodPost, "/v1/category", updateBody)
	require.Equal(t, http.StatusOK, resp.Code, "updating the category should succeed")

	// Step 3: Verify the CustomHeader is now empty
	var updated models.Category
	tc.DB.First(&updated, created.ID)

	assert.Equal(t, "", updated.CustomHeader,
		"CustomHeader should be cleared to empty string after explicit update with empty value; "+
			"UpdateCategory currently ignores empty strings, making it impossible to remove a custom header once set")
}

// TestUpdateCategoryCanClearCustomSidebarToEmpty is the same bug but for CustomSidebar.
func TestUpdateCategoryCanClearCustomSidebarToEmpty(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a category with a non-empty CustomSidebar
	createBody := map[string]any{
		"Name":          "Cat With Sidebar",
		"Description":   "Some description",
		"CustomSidebar": "<nav>Sidebar</nav>",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/category", createBody)
	require.Equal(t, http.StatusOK, resp.Code)

	var created models.Category
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))
	require.Equal(t, "<nav>Sidebar</nav>", created.CustomSidebar)

	// Update: explicitly clear CustomSidebar
	updateBody := map[string]any{
		"ID":            created.ID,
		"Name":          "Cat With Sidebar",
		"Description":   "Some description",
		"CustomSidebar": "",
	}
	resp = tc.MakeRequest(http.MethodPost, "/v1/category", updateBody)
	require.Equal(t, http.StatusOK, resp.Code)

	var updated models.Category
	tc.DB.First(&updated, created.ID)

	assert.Equal(t, "", updated.CustomSidebar,
		"CustomSidebar should be cleared to empty string after explicit update with empty value; "+
			"UpdateCategory currently ignores empty strings, making it impossible to remove a custom sidebar once set")
}

// TestUpdateCategoryCanClearDescriptionToEmpty tests that Description can be cleared.
func TestUpdateCategoryCanClearDescriptionToEmpty(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a category with a non-empty Description
	createBody := map[string]any{
		"Name":        "Cat With Desc",
		"Description": "Important description that should be removable",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/category", createBody)
	require.Equal(t, http.StatusOK, resp.Code)

	var created models.Category
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))
	require.Equal(t, "Important description that should be removable", created.Description)

	// Update: explicitly clear Description
	updateBody := map[string]any{
		"ID":          created.ID,
		"Name":        "Cat With Desc",
		"Description": "",
	}
	resp = tc.MakeRequest(http.MethodPost, "/v1/category", updateBody)
	require.Equal(t, http.StatusOK, resp.Code)

	var updated models.Category
	tc.DB.First(&updated, created.ID)

	assert.Equal(t, "", updated.Description,
		"Description should be cleared to empty string after explicit update with empty value; "+
			"UpdateCategory currently ignores empty strings, making it impossible to remove a description once set")
}
