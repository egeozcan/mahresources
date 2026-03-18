package api_tests

import (
	"encoding/json"
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateTagCanClearDescriptionToEmpty verifies that a user can remove a
// previously-set Description by sending an explicit empty string.
//
// BUG: UpdateTag guards the Description field with `if tagQuery.Description != ""`,
// so once a tag's Description has been set to a non-empty value it can never be
// cleared back to empty via the API. This is the same class of bug that was
// already fixed in UpdateCategory.
func TestUpdateTagCanClearDescriptionToEmpty(t *testing.T) {
	tc := SetupTestEnv(t)

	// Step 1: Create a tag with a non-empty description
	createBody := map[string]any{
		"Name":        "Tag With Desc",
		"Description": "Important description that should be removable",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/tag", createBody)
	require.Equal(t, http.StatusOK, resp.Code, "creating the tag should succeed")

	var created models.Tag
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))
	require.Equal(t, "Important description that should be removable", created.Description,
		"tag should be created with the supplied Description")

	// Step 2: Update the tag, explicitly clearing Description to ""
	updateBody := map[string]any{
		"ID":          created.ID,
		"Name":        "Tag With Desc",
		"Description": "",
	}
	resp = tc.MakeRequest(http.MethodPost, "/v1/tag", updateBody)
	require.Equal(t, http.StatusOK, resp.Code, "updating the tag should succeed")

	// Step 3: Verify the Description is now empty
	var updated models.Tag
	tc.DB.First(&updated, created.ID)

	assert.Equal(t, "", updated.Description,
		"Description should be cleared to empty string after explicit update with empty value; "+
			"UpdateTag currently ignores empty strings due to `if tagQuery.Description != \"\"` guard, "+
			"making it impossible to remove a description once set")
}
