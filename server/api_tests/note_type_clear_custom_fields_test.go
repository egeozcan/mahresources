package api_tests

import (
	"encoding/json"
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestUpdateNoteTypeCanClearCustomHeaderToEmpty verifies that a user can
// remove a previously-set CustomHeader by sending an explicit empty string.
//
// BUG: CreateOrUpdateNoteType guards CustomHeader (and Description,
// CustomSidebar, CustomSummary, CustomAvatar) with `if value != ""`, so once
// any of these fields has been set to a non-empty string it can never be
// cleared back to empty via the API.
func TestUpdateNoteTypeCanClearCustomHeaderToEmpty(t *testing.T) {
	tc := SetupTestEnv(t)

	// Step 1: Create a note type with a non-empty CustomHeader
	createBody := map[string]any{
		"Name":         "Type With Header",
		"Description":  "Some description",
		"CustomHeader": "<h1>Big Header</h1>",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/note/noteType", createBody)
	require.Equal(t, http.StatusOK, resp.Code, "creating the note type should succeed")

	var created models.NoteType
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))
	require.Equal(t, "<h1>Big Header</h1>", created.CustomHeader,
		"note type should be created with the supplied CustomHeader")

	// Step 2: Update the note type, explicitly clearing CustomHeader to ""
	updateBody := map[string]any{
		"ID":           created.ID,
		"Name":         "Type With Header",
		"Description":  "Some description",
		"CustomHeader": "",
	}
	resp = tc.MakeRequest(http.MethodPost, "/v1/note/noteType", updateBody)
	require.Equal(t, http.StatusOK, resp.Code, "updating the note type should succeed")

	// Step 3: Verify the CustomHeader is now empty
	var updated models.NoteType
	tc.DB.First(&updated, created.ID)

	assert.Equal(t, "", updated.CustomHeader,
		"CustomHeader should be cleared to empty string after explicit update with empty value; "+
			"CreateOrUpdateNoteType currently ignores empty strings, making it impossible to remove a custom header once set")
}

// TestUpdateNoteTypeCanClearCustomSidebarToEmpty is the same bug but for CustomSidebar.
func TestUpdateNoteTypeCanClearCustomSidebarToEmpty(t *testing.T) {
	tc := SetupTestEnv(t)

	createBody := map[string]any{
		"Name":          "Type With Sidebar",
		"Description":   "Some description",
		"CustomSidebar": "<nav>Sidebar</nav>",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/note/noteType", createBody)
	require.Equal(t, http.StatusOK, resp.Code)

	var created models.NoteType
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))
	require.Equal(t, "<nav>Sidebar</nav>", created.CustomSidebar)

	updateBody := map[string]any{
		"ID":            created.ID,
		"Name":          "Type With Sidebar",
		"Description":   "Some description",
		"CustomSidebar": "",
	}
	resp = tc.MakeRequest(http.MethodPost, "/v1/note/noteType", updateBody)
	require.Equal(t, http.StatusOK, resp.Code)

	var updated models.NoteType
	tc.DB.First(&updated, created.ID)

	assert.Equal(t, "", updated.CustomSidebar,
		"CustomSidebar should be cleared to empty string after explicit update with empty value; "+
			"CreateOrUpdateNoteType currently ignores empty strings, making it impossible to remove a custom sidebar once set")
}

// TestUpdateNoteTypeCanClearDescriptionToEmpty tests that Description can be cleared.
func TestUpdateNoteTypeCanClearDescriptionToEmpty(t *testing.T) {
	tc := SetupTestEnv(t)

	createBody := map[string]any{
		"Name":        "Type With Desc",
		"Description": "Important description that should be removable",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/note/noteType", createBody)
	require.Equal(t, http.StatusOK, resp.Code)

	var created models.NoteType
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))
	require.Equal(t, "Important description that should be removable", created.Description)

	updateBody := map[string]any{
		"ID":          created.ID,
		"Name":        "Type With Desc",
		"Description": "",
	}
	resp = tc.MakeRequest(http.MethodPost, "/v1/note/noteType", updateBody)
	require.Equal(t, http.StatusOK, resp.Code)

	var updated models.NoteType
	tc.DB.First(&updated, created.ID)

	assert.Equal(t, "", updated.Description,
		"Description should be cleared to empty string after explicit update with empty value; "+
			"CreateOrUpdateNoteType currently ignores empty strings, making it impossible to remove a description once set")
}

// TestUpdateNoteTypeCanClearCustomSummaryToEmpty tests that CustomSummary can be cleared.
func TestUpdateNoteTypeCanClearCustomSummaryToEmpty(t *testing.T) {
	tc := SetupTestEnv(t)

	createBody := map[string]any{
		"Name":          "Type With Summary",
		"Description":   "Some description",
		"CustomSummary": "<p>Custom summary content</p>",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/note/noteType", createBody)
	require.Equal(t, http.StatusOK, resp.Code)

	var created models.NoteType
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))
	require.Equal(t, "<p>Custom summary content</p>", created.CustomSummary)

	updateBody := map[string]any{
		"ID":            created.ID,
		"Name":          "Type With Summary",
		"Description":   "Some description",
		"CustomSummary": "",
	}
	resp = tc.MakeRequest(http.MethodPost, "/v1/note/noteType", updateBody)
	require.Equal(t, http.StatusOK, resp.Code)

	var updated models.NoteType
	tc.DB.First(&updated, created.ID)

	assert.Equal(t, "", updated.CustomSummary,
		"CustomSummary should be cleared to empty string after explicit update with empty value; "+
			"CreateOrUpdateNoteType currently ignores empty strings, making it impossible to remove a custom summary once set")
}

// TestUpdateNoteTypeCanClearCustomAvatarToEmpty tests that CustomAvatar can be cleared.
func TestUpdateNoteTypeCanClearCustomAvatarToEmpty(t *testing.T) {
	tc := SetupTestEnv(t)

	createBody := map[string]any{
		"Name":         "Type With Avatar",
		"Description":  "Some description",
		"CustomAvatar": "<img src='avatar.png'>",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/note/noteType", createBody)
	require.Equal(t, http.StatusOK, resp.Code)

	var created models.NoteType
	require.NoError(t, json.Unmarshal(resp.Body.Bytes(), &created))
	require.Equal(t, "<img src='avatar.png'>", created.CustomAvatar)

	updateBody := map[string]any{
		"ID":           created.ID,
		"Name":         "Type With Avatar",
		"Description":  "Some description",
		"CustomAvatar": "",
	}
	resp = tc.MakeRequest(http.MethodPost, "/v1/note/noteType", updateBody)
	require.Equal(t, http.StatusOK, resp.Code)

	var updated models.NoteType
	tc.DB.First(&updated, created.ID)

	assert.Equal(t, "", updated.CustomAvatar,
		"CustomAvatar should be cleared to empty string after explicit update with empty value; "+
			"CreateOrUpdateNoteType currently ignores empty strings, making it impossible to remove a custom avatar once set")
}
