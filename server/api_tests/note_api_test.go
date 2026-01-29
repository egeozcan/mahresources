package api_tests

import (
	"encoding/json"
	"fmt"
	"mahresources/models"
	"mahresources/models/query_models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoteEndpoints(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a dummy note directly in DB
	initialNote := tc.CreateDummyNote("Initial Note")

	t.Run("List Notes", func(t *testing.T) {
		resp := tc.MakeRequest(http.MethodGet, "/v1/notes", nil)
		assert.Equal(t, http.StatusOK, resp.Code)

		var notes []models.Note
		err := json.Unmarshal(resp.Body.Bytes(), &notes)
		assert.NoError(t, err)
		assert.Len(t, notes, 1)
		assert.Equal(t, initialNote.Name, notes[0].Name)
	})

	t.Run("Get Note", func(t *testing.T) {
		url := fmt.Sprintf("/v1/note?id=%d", initialNote.ID)
		resp := tc.MakeRequest(http.MethodGet, url, nil)
		assert.Equal(t, http.StatusOK, resp.Code)

		var note models.Note
		err := json.Unmarshal(resp.Body.Bytes(), &note)
		assert.NoError(t, err)
		assert.Equal(t, initialNote.ID, note.ID)
	})

	var newNoteID uint

	t.Run("Create Note", func(t *testing.T) {
		payload := query_models.NoteEditor{}
		payload.Name = "New API Note"
		payload.Description = "Created via API"
		payload.OwnerId = initialNote.ID
		
		// Fix OwnerId to be a valid group
		group := tc.CreateDummyGroup("Owner Group")
		payload.OwnerId = group.ID

		resp := tc.MakeRequest(http.MethodPost, "/v1/note", payload)
		
		assert.Equal(t, http.StatusOK, resp.Code)

		var createdNote models.Note
		err := json.Unmarshal(resp.Body.Bytes(), &createdNote)
		assert.NoError(t, err)
		assert.Equal(t, "New API Note", createdNote.Name)
		newNoteID = createdNote.ID
	})

	t.Run("Update Note", func(t *testing.T) {
		payload := query_models.NoteEditor{}
		payload.ID = newNoteID
		payload.Name = "Updated API Note"
		
		group := tc.CreateDummyGroup("Another Group")
		payload.OwnerId = group.ID

		resp := tc.MakeRequest(http.MethodPost, "/v1/note", payload)
		assert.Equal(t, http.StatusOK, resp.Code)

		var updatedNote models.Note
		tc.DB.First(&updatedNote, newNoteID)
		assert.Equal(t, "Updated API Note", updatedNote.Name)
	})

	t.Run("Edit Name", func(t *testing.T) {
		url := fmt.Sprintf("/v1/note/editName?id=%d", newNoteID)
		payload := map[string]string{"Name": "Renamed Note"}
		
		resp := tc.MakeRequest(http.MethodPost, url, payload)
		assert.Equal(t, http.StatusOK, resp.Code)

		var n models.Note
		tc.DB.First(&n, newNoteID)
		assert.Equal(t, "Renamed Note", n.Name)
	})

	t.Run("Get Note Meta Keys", func(t *testing.T) {
		t.Skip("Skipping due to missing json_each extension in test sqlite driver")
		resp := tc.MakeRequest(http.MethodGet, "/v1/notes/meta/keys", nil)
		assert.Equal(t, http.StatusOK, resp.Code)
	})

	t.Run("Delete Note", func(t *testing.T) {
		url := fmt.Sprintf("/v1/note/delete?Id=%d", newNoteID)
		resp := tc.MakeRequest(http.MethodPost, url, nil)
		assert.Equal(t, http.StatusOK, resp.Code)

		var check models.Note
		result := tc.DB.First(&check, newNoteID)
		assert.Error(t, result.Error)
	})

	// NoteTypes sub-resource
	t.Run("NoteTypes CRUD", func(t *testing.T) {
		// Create
		ntPayload := query_models.NoteTypeEditor{
			Name: "Meeting",
			Description: "Meeting notes",
		}
		resp := tc.MakeRequest(http.MethodPost, "/v1/note/noteType", ntPayload)
		assert.Equal(t, http.StatusOK, resp.Code)

		var nt models.NoteType
		json.Unmarshal(resp.Body.Bytes(), &nt)
		assert.NotZero(t, nt.ID)

		// List
		respList := tc.MakeRequest(http.MethodGet, "/v1/note/noteTypes", nil)
		assert.Equal(t, http.StatusOK, respList.Code)

		// Delete
		delUrl := fmt.Sprintf("/v1/note/noteType/delete?Id=%d", nt.ID)
		tc.MakeRequest(http.MethodPost, delUrl, nil)

		var check models.NoteType
		assert.Error(t, tc.DB.First(&check, nt.ID).Error)
	})
}

func TestShareNote(t *testing.T) {
	tc := SetupTestEnv(t)
	note := tc.CreateDummyNote("Test Share Note")

	t.Run("Share note creates token", func(t *testing.T) {
		url := fmt.Sprintf("/v1/note/share?id=%d", note.ID)
		resp := tc.MakeRequest(http.MethodPost, url, nil)
		assert.Equal(t, http.StatusOK, resp.Code)

		var result map[string]interface{}
		json.Unmarshal(resp.Body.Bytes(), &result)
		assert.NotEmpty(t, result["shareToken"])
		assert.NotEmpty(t, result["shareUrl"])
	})

	t.Run("Share note returns same token on repeat", func(t *testing.T) {
		url := fmt.Sprintf("/v1/note/share?id=%d", note.ID)
		resp1 := tc.MakeRequest(http.MethodPost, url, nil)
		resp2 := tc.MakeRequest(http.MethodPost, url, nil)

		var result1, result2 map[string]interface{}
		json.Unmarshal(resp1.Body.Bytes(), &result1)
		json.Unmarshal(resp2.Body.Bytes(), &result2)
		assert.Equal(t, result1["shareToken"], result2["shareToken"])
	})

	t.Run("Unshare note removes token", func(t *testing.T) {
		url := fmt.Sprintf("/v1/note/share?id=%d", note.ID)
		tc.MakeRequest(http.MethodDelete, url, nil)

		// Verify note is no longer shared
		updatedNote, _ := tc.AppCtx.GetNote(note.ID)
		assert.Nil(t, updatedNote.ShareToken)
	})

	t.Run("Share nonexistent note returns error", func(t *testing.T) {
		url := "/v1/note/share?id=99999"
		resp := tc.MakeRequest(http.MethodPost, url, nil)
		assert.Equal(t, http.StatusNotFound, resp.Code)
	})
}
