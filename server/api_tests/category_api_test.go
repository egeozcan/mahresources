package api_tests

import (
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCategoryUpdatePartialJSONPreservesCustomFields(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a category with custom HTML fields populated
	cat := &models.Category{
		Name:          "Original Category",
		Description:   "Original desc",
		CustomHeader:  "<h2>Custom Header HTML</h2>",
		CustomSidebar: "<div>Sidebar content</div>",
		CustomSummary: "<p>Summary HTML</p>",
		CustomAvatar:  "<img src='avatar.png'>",
		MetaSchema:    `{"type":"object","properties":{"year":{"type":"number"}}}`,
	}
	tc.DB.Create(cat)

	// Send a partial JSON body that only changes the description
	partialBody := map[string]any{
		"ID":          cat.ID,
		"Description": "Updated desc",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/category", partialBody)
	assert.Equal(t, http.StatusOK, resp.Code)

	// The custom HTML fields should be preserved, not cleared
	var check models.Category
	tc.DB.First(&check, cat.ID)
	assert.Equal(t, "Updated desc", check.Description)
	assert.Equal(t, "Original Category", check.Name,
		"Editing only description should not clear the name")
	assert.Equal(t, "<h2>Custom Header HTML</h2>", check.CustomHeader,
		"Editing only description should not clear CustomHeader")
	assert.Equal(t, "<div>Sidebar content</div>", check.CustomSidebar,
		"Editing only description should not clear CustomSidebar")
	assert.Equal(t, "<p>Summary HTML</p>", check.CustomSummary,
		"Editing only description should not clear CustomSummary")
	assert.Equal(t, "<img src='avatar.png'>", check.CustomAvatar,
		"Editing only description should not clear CustomAvatar")
	assert.Equal(t, `{"type":"object","properties":{"year":{"type":"number"}}}`, check.MetaSchema,
		"Editing only description should not clear MetaSchema")
}
