package api_tests

import (
	"encoding/json"
	"fmt"
	"mahresources/models"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCategoryFormEncodedPartialUpdatePreservesFields verifies that a
// form-encoded POST to /v1/category with only ID and Name does NOT clear
// Description, CustomHeader, or other fields.
//
// This is the form-encoded counterpart of TestCategoryUpdatePartialJSONPreservesCustomFields.
// The bug: CreateCategoryHandler only pre-fills fields for JSON requests
// (sentFields != nil), so form-encoded partial updates lose data.
func TestCategoryFormEncodedPartialUpdatePreservesFields(t *testing.T) {
	tc := SetupTestEnv(t)

	// Step 1: Create a category with all fields populated via JSON
	createBody := map[string]any{
		"Name":          "Original Category",
		"Description":   "Important description",
		"CustomHeader":  "<h2>Custom Header</h2>",
		"CustomSidebar": "<div>Sidebar</div>",
		"CustomSummary": "<p>Summary</p>",
		"CustomAvatar":  "<img src='avatar.png'>",
		"MetaSchema":    `{"type":"object"}`,
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/category", createBody)
	require.Equal(t, http.StatusOK, resp.Code, "creating category should succeed")

	var created models.Category
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))
	require.Equal(t, "Important description", created.Description)
	require.Equal(t, "<h2>Custom Header</h2>", created.CustomHeader)

	// Step 2: Send a form-encoded update with ONLY ID and Name
	formData := url.Values{}
	formData.Set("ID", fmt.Sprintf("%d", created.ID))
	formData.Set("Name", "Renamed Category")

	resp = tc.MakeFormRequest(http.MethodPost, "/v1/category", formData)
	require.Equal(t, http.StatusOK, resp.Code, "form-encoded update should succeed")

	// Step 3: Verify that fields not sent in the form are preserved
	var updated models.Category
	tc.DB.First(&updated, created.ID)

	assert.Equal(t, "Renamed Category", updated.Name,
		"Name should be updated")
	assert.Equal(t, "Important description", updated.Description,
		"Description should be preserved after form-encoded partial update")
	assert.Equal(t, "<h2>Custom Header</h2>", updated.CustomHeader,
		"CustomHeader should be preserved after form-encoded partial update")
	assert.Equal(t, "<div>Sidebar</div>", updated.CustomSidebar,
		"CustomSidebar should be preserved after form-encoded partial update")
	assert.Equal(t, "<p>Summary</p>", updated.CustomSummary,
		"CustomSummary should be preserved after form-encoded partial update")
	assert.Equal(t, "<img src='avatar.png'>", updated.CustomAvatar,
		"CustomAvatar should be preserved after form-encoded partial update")
	assert.Equal(t, `{"type":"object"}`, updated.MetaSchema,
		"MetaSchema should be preserved after form-encoded partial update")
}

// TestCategoryFormEncodedUpdateCanExplicitlyClearField verifies that if a
// form-encoded request explicitly sends an empty value for a field, that field
// IS cleared (not preserved). This distinguishes "absent" from "empty".
func TestCategoryFormEncodedUpdateCanExplicitlyClearField(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a category with Description populated
	createBody := map[string]any{
		"Name":         "Cat To Clear",
		"Description":  "Will be cleared",
		"CustomHeader": "<h1>Header</h1>",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/category", createBody)
	require.Equal(t, http.StatusOK, resp.Code)

	var created models.Category
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))

	// Send form with Description explicitly set to empty, but CustomHeader absent
	formData := url.Values{}
	formData.Set("ID", fmt.Sprintf("%d", created.ID))
	formData.Set("Name", "Cat To Clear")
	formData.Set("Description", "")

	resp = tc.MakeFormRequest(http.MethodPost, "/v1/category", formData)
	require.Equal(t, http.StatusOK, resp.Code)

	var updated models.Category
	tc.DB.First(&updated, created.ID)

	assert.Equal(t, "", updated.Description,
		"Description should be cleared when explicitly sent as empty")
	assert.Equal(t, "<h1>Header</h1>", updated.CustomHeader,
		"CustomHeader should be preserved when not sent in form")
}
