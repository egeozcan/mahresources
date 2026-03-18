package api_tests

import (
	"encoding/json"
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateResourceCategoryCanClearDescriptionToEmpty verifies that a user can
// remove a previously-set Description by sending an explicit empty string.
//
// BUG: UpdateResourceCategory guards every optional field with `if query.Field != ""`,
// so once Description has been set to a non-empty string it can never be cleared
// back to empty via the API.  This is inconsistent with UpdateCategory, UpdateTag,
// and CreateOrUpdateNoteType, which all allow clearing optional fields.
func TestUpdateResourceCategoryCanClearDescriptionToEmpty(t *testing.T) {
	tc := SetupTestEnv(t)

	// Step 1: Create a resource category with a non-empty Description
	createBody := map[string]any{
		"Name":        "RC With Desc",
		"Description": "Important description that should be removable",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", createBody)
	require.Equal(t, http.StatusOK, resp.Code, "creating the resource category should succeed")

	var created models.ResourceCategory
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))
	require.Equal(t, "Important description that should be removable", created.Description,
		"resource category should be created with the supplied Description")

	// Step 2: Update the resource category, explicitly clearing Description to ""
	updateBody := map[string]any{
		"ID":          created.ID,
		"Name":        "RC With Desc",
		"Description": "",
	}
	resp = tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", updateBody)
	require.Equal(t, http.StatusOK, resp.Code, "updating the resource category should succeed")

	// Step 3: Verify the Description is now empty
	var updated models.ResourceCategory
	tc.DB.First(&updated, created.ID)

	assert.Equal(t, "", updated.Description,
		"Description should be cleared to empty string after explicit update with empty value; "+
			"UpdateResourceCategory currently ignores empty strings, making it impossible to remove a description once set")
}

// TestUpdateResourceCategoryCanClearCustomHeaderToEmpty is the same bug but for CustomHeader.
func TestUpdateResourceCategoryCanClearCustomHeaderToEmpty(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a resource category with a non-empty CustomHeader
	createBody := map[string]any{
		"Name":         "RC With Header",
		"CustomHeader": "<h1>Big Header</h1>",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", createBody)
	require.Equal(t, http.StatusOK, resp.Code)

	var created models.ResourceCategory
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))
	require.Equal(t, "<h1>Big Header</h1>", created.CustomHeader)

	// Update: explicitly clear CustomHeader
	updateBody := map[string]any{
		"ID":           created.ID,
		"Name":         "RC With Header",
		"CustomHeader": "",
	}
	resp = tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", updateBody)
	require.Equal(t, http.StatusOK, resp.Code)

	var updated models.ResourceCategory
	tc.DB.First(&updated, created.ID)

	assert.Equal(t, "", updated.CustomHeader,
		"CustomHeader should be cleared to empty string after explicit update with empty value; "+
			"UpdateResourceCategory currently ignores empty strings, making it impossible to remove a custom header once set")
}

// TestUpdateResourceCategoryCanClearCustomSidebarToEmpty is the same bug but for CustomSidebar.
func TestUpdateResourceCategoryCanClearCustomSidebarToEmpty(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a resource category with a non-empty CustomSidebar
	createBody := map[string]any{
		"Name":          "RC With Sidebar",
		"CustomSidebar": "<nav>Sidebar</nav>",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", createBody)
	require.Equal(t, http.StatusOK, resp.Code)

	var created models.ResourceCategory
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))
	require.Equal(t, "<nav>Sidebar</nav>", created.CustomSidebar)

	// Update: explicitly clear CustomSidebar
	updateBody := map[string]any{
		"ID":            created.ID,
		"Name":          "RC With Sidebar",
		"CustomSidebar": "",
	}
	resp = tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", updateBody)
	require.Equal(t, http.StatusOK, resp.Code)

	var updated models.ResourceCategory
	tc.DB.First(&updated, created.ID)

	assert.Equal(t, "", updated.CustomSidebar,
		"CustomSidebar should be cleared to empty string after explicit update with empty value; "+
			"UpdateResourceCategory currently ignores empty strings, making it impossible to remove a custom sidebar once set")
}

// TestUpdateResourceCategoryCanClearMetaSchemaToEmpty is the same bug but for MetaSchema.
func TestUpdateResourceCategoryCanClearMetaSchemaToEmpty(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a resource category with a non-empty MetaSchema
	createBody := map[string]any{
		"Name":       "RC With Schema",
		"MetaSchema": `{"type":"object","properties":{"format":{"type":"string"}}}`,
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", createBody)
	require.Equal(t, http.StatusOK, resp.Code)

	var created models.ResourceCategory
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))
	require.Equal(t, `{"type":"object","properties":{"format":{"type":"string"}}}`, created.MetaSchema)

	// Update: explicitly clear MetaSchema
	updateBody := map[string]any{
		"ID":         created.ID,
		"Name":       "RC With Schema",
		"MetaSchema": "",
	}
	resp = tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", updateBody)
	require.Equal(t, http.StatusOK, resp.Code)

	var updated models.ResourceCategory
	tc.DB.First(&updated, created.ID)

	assert.Equal(t, "", updated.MetaSchema,
		"MetaSchema should be cleared to empty string after explicit update with empty value; "+
			"UpdateResourceCategory currently ignores empty strings, making it impossible to remove a meta schema once set")
}
