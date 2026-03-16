package api_tests

import (
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResourceCategoryUpdatePartialJSONPreservesCustomFields(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a resource category with custom HTML fields populated
	rc := &models.ResourceCategory{
		Name:          "Original RC",
		Description:   "Original desc",
		CustomHeader:  "<h2>RC Header</h2>",
		CustomSidebar: "<div>RC Sidebar</div>",
		CustomSummary: "<p>RC Summary</p>",
		CustomAvatar:  "<img src='rc-avatar.png'>",
		MetaSchema:    `{"type":"object","properties":{"format":{"type":"string"}}}`,
	}
	tc.DB.Create(rc)

	// Send a partial JSON body that only changes the description
	partialBody := map[string]any{
		"ID":          rc.ID,
		"Description": "Updated desc",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/resourceCategory", partialBody)
	assert.Equal(t, http.StatusOK, resp.Code)

	// The custom HTML fields should be preserved, not cleared
	var check models.ResourceCategory
	tc.DB.First(&check, rc.ID)
	assert.Equal(t, "Updated desc", check.Description)
	assert.Equal(t, "Original RC", check.Name,
		"Name should be preserved")
	assert.Equal(t, "<h2>RC Header</h2>", check.CustomHeader,
		"CustomHeader should be preserved on partial update")
	assert.Equal(t, "<div>RC Sidebar</div>", check.CustomSidebar,
		"CustomSidebar should be preserved on partial update")
	assert.Equal(t, "<p>RC Summary</p>", check.CustomSummary,
		"CustomSummary should be preserved on partial update")
	assert.Equal(t, "<img src='rc-avatar.png'>", check.CustomAvatar,
		"CustomAvatar should be preserved on partial update")
	assert.Equal(t, `{"type":"object","properties":{"format":{"type":"string"}}}`, check.MetaSchema,
		"MetaSchema should be preserved on partial update")
}
