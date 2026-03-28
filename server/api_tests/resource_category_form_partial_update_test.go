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

// TestResourceCategoryFormEncodedPartialUpdatePreservesFields verifies that a
// form-encoded POST to /v1/resourceCategory with only ID and Name does NOT
// clear Description, CustomHeader, or other fields.
//
// The bug: CreateResourceCategoryHandler only pre-fills fields for JSON
// requests (sentFields != nil), so form-encoded partial updates lose data.
func TestResourceCategoryFormEncodedPartialUpdatePreservesFields(t *testing.T) {
	tc := SetupTestEnv(t)

	// Step 1: Create a resource category with all fields populated via JSON
	createBody := map[string]any{
		"Name":          "Original RC",
		"Description":   "Important RC description",
		"CustomHeader":  "<h2>RC Header</h2>",
		"CustomSidebar": "<div>RC Sidebar</div>",
		"CustomSummary": "<p>RC Summary</p>",
		"CustomAvatar":  "<img src='rc-avatar.png'>",
		"MetaSchema":    `{"type":"object"}`,
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", createBody)
	require.Equal(t, http.StatusOK, resp.Code, "creating resource category should succeed")

	var created models.ResourceCategory
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))
	require.Equal(t, "Important RC description", created.Description)
	require.Equal(t, "<h2>RC Header</h2>", created.CustomHeader)

	// Step 2: Send a form-encoded update with ONLY ID and Name
	formData := url.Values{}
	formData.Set("ID", fmt.Sprintf("%d", created.ID))
	formData.Set("Name", "Renamed RC")

	resp = tc.MakeFormRequest(http.MethodPost, "/v1/resourceCategory", formData)
	require.Equal(t, http.StatusOK, resp.Code, "form-encoded update should succeed")

	// Step 3: Verify that fields not sent in the form are preserved
	var updated models.ResourceCategory
	tc.DB.First(&updated, created.ID)

	assert.Equal(t, "Renamed RC", updated.Name,
		"Name should be updated")
	assert.Equal(t, "Important RC description", updated.Description,
		"Description should be preserved after form-encoded partial update")
	assert.Equal(t, "<h2>RC Header</h2>", updated.CustomHeader,
		"CustomHeader should be preserved after form-encoded partial update")
	assert.Equal(t, "<div>RC Sidebar</div>", updated.CustomSidebar,
		"CustomSidebar should be preserved after form-encoded partial update")
	assert.Equal(t, "<p>RC Summary</p>", updated.CustomSummary,
		"CustomSummary should be preserved after form-encoded partial update")
	assert.Equal(t, "<img src='rc-avatar.png'>", updated.CustomAvatar,
		"CustomAvatar should be preserved after form-encoded partial update")
	assert.Equal(t, `{"type":"object"}`, updated.MetaSchema,
		"MetaSchema should be preserved after form-encoded partial update")
}

// TestResourceCategoryFormEncodedUpdateCanExplicitlyClearField verifies that
// explicitly sending an empty value in a form clears that field.
func TestResourceCategoryFormEncodedUpdateCanExplicitlyClearField(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a resource category with Description populated
	createBody := map[string]any{
		"Name":         "RC To Clear",
		"Description":  "Will be cleared",
		"CustomHeader": "<h1>Header</h1>",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", createBody)
	require.Equal(t, http.StatusOK, resp.Code)

	var created models.ResourceCategory
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))

	// Send form with Description explicitly set to empty, but CustomHeader absent
	formData := url.Values{}
	formData.Set("ID", fmt.Sprintf("%d", created.ID))
	formData.Set("Name", "RC To Clear")
	formData.Set("Description", "")

	resp = tc.MakeFormRequest(http.MethodPost, "/v1/resourceCategory", formData)
	require.Equal(t, http.StatusOK, resp.Code)

	var updated models.ResourceCategory
	tc.DB.First(&updated, created.ID)

	assert.Equal(t, "", updated.Description,
		"Description should be cleared when explicitly sent as empty")
	assert.Equal(t, "<h1>Header</h1>", updated.CustomHeader,
		"CustomHeader should be preserved when not sent in form")
}
