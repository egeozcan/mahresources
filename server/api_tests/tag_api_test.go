package api_tests

import (
	"encoding/json"
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTagUpdatePartialJSONPreservesDescription(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a tag with both name and description
	tag := &models.Tag{Name: "Original Tag", Description: "Important description"}
	tc.DB.Create(tag)

	// Send a partial JSON edit that only changes the name
	partialBody := map[string]any{
		"ID":   tag.ID,
		"Name": "Renamed Tag",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/tag", partialBody)
	assert.Equal(t, http.StatusOK, resp.Code)

	var updated models.Tag
	json.Unmarshal(resp.Body.Bytes(), &updated)

	var check models.Tag
	tc.DB.First(&check, tag.ID)
	assert.Equal(t, "Renamed Tag", check.Name)
	assert.Equal(t, "Important description", check.Description,
		"Editing only name should not clear the description")
}
