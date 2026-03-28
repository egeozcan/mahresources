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

// TestQueryPartialUpdatePreservesDescription verifies that a partial JSON update
// to a query that does not include Description preserves the existing value.
//
// BUG: The query handler pre-fills Name, Text, and Template from the existing
// record, but NOT Description.
func TestQueryPartialUpdatePreservesDescription(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a query with a description
	createBody := map[string]any{
		"Name":        "Query With Desc",
		"Text":        "SELECT 1",
		"Description": "Important query description",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/query", createBody)
	require.Equal(t, http.StatusOK, resp.Code, "creating the query should succeed")

	var created models.Query
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))
	require.Equal(t, "Important query description", created.Description)

	// Partial update: only change the name (Description not sent)
	updateBody := map[string]any{
		"ID":   created.ID,
		"Name": "Renamed Query",
	}
	resp = tc.MakeRequest(http.MethodPost, "/v1/query", updateBody)
	require.Equal(t, http.StatusOK, resp.Code)

	var updated models.Query
	tc.DB.First(&updated, created.ID)

	assert.Equal(t, "Renamed Query", updated.Name, "Name should be updated")
	assert.Equal(t, "Important query description", updated.Description,
		"Description should be preserved when not included in partial JSON update")
}

// TestNoteCanClearDescriptionExplicitly verifies that sending an explicit empty
// Description clears it, rather than restoring the old value.
//
// BUG: The note handler uses `if editor.Description == ""` which prevents
// clearing Description once set.
func TestNoteCanClearDescriptionExplicitly(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a note with a description via JSON
	createBody := map[string]any{
		"Name":        "Note With Desc",
		"Description": "A description that should be clearable",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/note", createBody)
	require.Equal(t, http.StatusOK, resp.Code)

	var created models.Note
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))
	require.Equal(t, "A description that should be clearable", created.Description)

	// Update: explicitly send empty Description
	updateBody := map[string]any{
		"ID":          created.ID,
		"Name":        "Note With Desc",
		"Description": "",
	}
	resp = tc.MakeRequest(http.MethodPost, "/v1/note", updateBody)
	require.Equal(t, http.StatusOK, resp.Code)

	var updated models.Note
	tc.DB.First(&updated, created.ID)

	assert.Equal(t, "", updated.Description,
		"Description should be cleared to empty string when explicitly sent as empty in JSON")
}

// TestNoteCanClearDescriptionViaForm verifies the same clearing works for
// form-encoded requests.
func TestNoteCanClearDescriptionViaForm(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a note with a description via JSON
	createBody := map[string]any{
		"Name":        "Note Form Clear Desc",
		"Description": "Form-clearable description",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/note", createBody)
	require.Equal(t, http.StatusOK, resp.Code)

	var created models.Note
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))

	// Send form-encoded update with empty Description
	formData := url.Values{}
	formData.Set("ID", fmt.Sprintf("%d", created.ID))
	formData.Set("Name", "Note Form Clear Desc")
	formData.Set("Description", "")

	resp = tc.MakeFormRequest(http.MethodPost, "/v1/note", formData)
	require.Equal(t, http.StatusOK, resp.Code)

	var updated models.Note
	tc.DB.First(&updated, created.ID)

	assert.Equal(t, "", updated.Description,
		"Description should be cleared when form-encoded request explicitly sends empty Description")
}

// TestGroupCanClearDescriptionExplicitly verifies that sending an explicit empty
// Description on a group clears it.
//
// BUG: The group handler uses `if editor.Description == ""` which prevents clearing.
func TestGroupCanClearDescriptionExplicitly(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a group with a description
	createBody := map[string]any{
		"Name":        "Group With Desc",
		"Description": "Group description to clear",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/group", createBody)
	require.Equal(t, http.StatusOK, resp.Code)

	var created models.Group
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))
	require.Equal(t, "Group description to clear", created.Description)

	// Update: explicitly send empty Description
	updateBody := map[string]any{
		"ID":          created.ID,
		"Name":        "Group With Desc",
		"Description": "",
	}
	resp = tc.MakeRequest(http.MethodPost, "/v1/group", updateBody)
	require.Equal(t, http.StatusOK, resp.Code)

	var updated models.Group
	tc.DB.First(&updated, created.ID)

	assert.Equal(t, "", updated.Description,
		"Description should be cleared to empty string when explicitly sent as empty in JSON")
}

// TestGroupCanClearURLExplicitly verifies that sending an explicit empty URL
// on a group clears it, rather than restoring the old value.
//
// BUG: The group handler checks `if editor.URL == "" && existing.URL != nil`
// but doesn't check if the form/JSON explicitly sent an empty URL.
func TestGroupCanClearURLExplicitly(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a group with a URL
	createBody := map[string]any{
		"Name": "Group With URL",
		"URL":  "https://example.com",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/group", createBody)
	require.Equal(t, http.StatusOK, resp.Code)

	var created models.Group
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))
	require.NotNil(t, created.URL, "URL should be set after creation")

	// Update: explicitly send empty URL to clear it
	updateBody := map[string]any{
		"ID":   created.ID,
		"Name": "Group With URL",
		"URL":  "",
	}
	resp = tc.MakeRequest(http.MethodPost, "/v1/group", updateBody)
	require.Equal(t, http.StatusOK, resp.Code)

	var updated models.Group
	tc.DB.First(&updated, created.ID)

	assert.Nil(t, updated.URL,
		"URL should be cleared (nil) when explicitly sent as empty in JSON; "+
			"currently the handler restores the old URL because it only checks for empty string")
}

// TestResourceCanClearDescriptionExplicitly verifies that sending an explicit
// empty Description on a resource clears it.
//
// BUG: The resource handler uses `if editor.Description == ""` which prevents clearing.
func TestResourceCanClearDescriptionExplicitly(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a resource directly in DB (resource creation via API requires file upload)
	resource := &models.Resource{
		Name:        "Resource With Desc",
		Description: "Resource description to clear",
	}
	tc.DB.Create(resource)
	require.NotZero(t, resource.ID)

	// Update: explicitly send empty Description via JSON
	updateBody := map[string]any{
		"ID":          resource.ID,
		"Name":        "Resource With Desc",
		"Description": "",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/resource/edit", updateBody)
	require.Equal(t, http.StatusOK, resp.Code)

	var updated models.Resource
	tc.DB.First(&updated, resource.ID)

	assert.Equal(t, "", updated.Description,
		"Description should be cleared to empty string when explicitly sent as empty in JSON")
}
