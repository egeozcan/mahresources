package api_tests

import (
	"encoding/json"
	"fmt"
	"mahresources/models"
	"mahresources/models/query_models"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNoteFormEncodedPartialUpdatePreservesFields verifies that a form-encoded
// POST to /v1/note with only ID and Name does NOT clear NoteTypeId or OwnerId.
func TestNoteFormEncodedPartialUpdatePreservesFields(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a NoteType
	noteTypePayload := query_models.NoteTypeEditor{Name: "FormPartialTestType"}
	resp := tc.MakeRequest(http.MethodPost, "/v1/note/noteType", noteTypePayload)
	require.Equal(t, http.StatusOK, resp.Code)
	var noteType models.NoteType
	json.Unmarshal(resp.Body.Bytes(), &noteType)

	// Create a Category and Group (owner)
	catPayload := query_models.CategoryCreator{Name: "FormPartialTestCat"}
	resp = tc.MakeRequest(http.MethodPost, "/v1/category", catPayload)
	require.Equal(t, http.StatusOK, resp.Code)
	var category models.Category
	json.Unmarshal(resp.Body.Bytes(), &category)

	groupPayload := query_models.GroupCreator{Name: "FormPartialTestOwner", CategoryId: category.ID}
	resp = tc.MakeRequest(http.MethodPost, "/v1/group", groupPayload)
	require.Equal(t, http.StatusOK, resp.Code)
	var group models.Group
	json.Unmarshal(resp.Body.Bytes(), &group)

	// Create a Tag
	tagPayload := query_models.TagCreator{Name: "FormPartialTestTag"}
	resp = tc.MakeRequest(http.MethodPost, "/v1/tag", tagPayload)
	require.Equal(t, http.StatusOK, resp.Code)
	var tag models.Tag
	json.Unmarshal(resp.Body.Bytes(), &tag)

	// Create a note with NoteType, Owner, and Tag via JSON
	notePayload := query_models.NoteEditor{
		NoteCreator: query_models.NoteCreator{
			Name:       "FormPartialTestNote",
			NoteTypeId: noteType.ID,
			OwnerId:    group.ID,
			Tags:       []uint{tag.ID},
		},
	}
	resp = tc.MakeRequest(http.MethodPost, "/v1/note", notePayload)
	require.Equal(t, http.StatusOK, resp.Code)
	var note models.Note
	json.Unmarshal(resp.Body.Bytes(), &note)
	require.NotNil(t, note.NoteTypeId, "NoteTypeId should be set after creation")

	// Now send a form-encoded POST with ONLY ID and Name (simulating a partial update)
	formData := url.Values{}
	formData.Set("ID", fmt.Sprintf("%d", note.ID))
	formData.Set("Name", "FormPartialTestNote Updated")

	resp = tc.MakeFormRequest(http.MethodPost, "/v1/note", formData)
	assert.Equal(t, http.StatusOK, resp.Code)

	var updated models.Note
	json.Unmarshal(resp.Body.Bytes(), &updated)

	assert.Equal(t, "FormPartialTestNote Updated", updated.Name, "Name should be updated")
	assert.NotNil(t, updated.NoteTypeId, "NoteTypeId should be preserved after form-encoded partial update")
	if updated.NoteTypeId != nil {
		assert.Equal(t, noteType.ID, *updated.NoteTypeId, "NoteTypeId should match original")
	}
	assert.NotNil(t, updated.OwnerId, "OwnerId should be preserved after form-encoded partial update")
}
