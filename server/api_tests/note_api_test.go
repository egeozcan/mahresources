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
			Name:        "Meeting",
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

func TestNoteUpdateClearsResourceAssociations(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create three resources directly in the DB
	r1 := &models.Resource{Name: "Resource 1"}
	r2 := &models.Resource{Name: "Resource 2"}
	r3 := &models.Resource{Name: "Resource 3"}
	tc.DB.Create(r1)
	tc.DB.Create(r2)
	tc.DB.Create(r3)

	// Create a note with resources R1 and R2
	createPayload := query_models.NoteEditor{}
	createPayload.Name = "Note with resources"
	createPayload.Resources = []uint{r1.ID, r2.ID}

	resp := tc.MakeRequest(http.MethodPost, "/v1/note", createPayload)
	assert.Equal(t, http.StatusOK, resp.Code)

	var createdNote models.Note
	err := json.Unmarshal(resp.Body.Bytes(), &createdNote)
	assert.NoError(t, err)
	noteID := createdNote.ID

	// Verify initial resources
	var resourceCount int64
	tc.DB.Table("resource_notes").Where("note_id = ?", noteID).Count(&resourceCount)
	assert.Equal(t, int64(2), resourceCount, "note should have 2 resources after creation")

	// Update the note with only R3 (should replace R1,R2 with R3)
	updatePayload := query_models.NoteEditor{}
	updatePayload.ID = noteID
	updatePayload.Name = "Note with resources"
	updatePayload.Resources = []uint{r3.ID}

	resp = tc.MakeRequest(http.MethodPost, "/v1/note", updatePayload)
	assert.Equal(t, http.StatusOK, resp.Code)

	// After update, the note should have ONLY R3
	tc.DB.Table("resource_notes").Where("note_id = ?", noteID).Count(&resourceCount)
	assert.Equal(t, int64(1), resourceCount, "note should have exactly 1 resource after update, but old resources were not cleared")

	// Verify it's specifically R3
	var resourceIDs []uint
	tc.DB.Table("resource_notes").Where("note_id = ?", noteID).Pluck("resource_id", &resourceIDs)
	assert.Equal(t, []uint{r3.ID}, resourceIDs, "note should only have R3 after update")
}

func TestNoteSharedFilterDistinguishesTrueAndFalse(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create two notes
	sharedNote := tc.CreateDummyNote("Shared Note")
	unsharedNote := tc.CreateDummyNote("Unshared Note")

	// Share one of them
	_, err := tc.AppCtx.ShareNote(sharedNote.ID)
	assert.NoError(t, err)

	t.Run("Shared=true returns only shared notes", func(t *testing.T) {
		resp := tc.MakeRequest(http.MethodGet, "/v1/notes?Shared=true", nil)
		assert.Equal(t, http.StatusOK, resp.Code)

		var notes []models.Note
		json.Unmarshal(resp.Body.Bytes(), &notes)
		assert.Len(t, notes, 1, "Shared=true should return exactly 1 shared note")
		assert.Equal(t, sharedNote.ID, notes[0].ID)
	})

	t.Run("Shared=false returns only unshared notes", func(t *testing.T) {
		resp := tc.MakeRequest(http.MethodGet, "/v1/notes?Shared=false", nil)
		assert.Equal(t, http.StatusOK, resp.Code)

		var notes []models.Note
		json.Unmarshal(resp.Body.Bytes(), &notes)
		assert.Len(t, notes, 1, "Shared=false should return exactly 1 unshared note")
		assert.Equal(t, unsharedNote.ID, notes[0].ID)
	})

	t.Run("No Shared param returns all notes", func(t *testing.T) {
		resp := tc.MakeRequest(http.MethodGet, "/v1/notes", nil)
		assert.Equal(t, http.StatusOK, resp.Code)

		var notes []models.Note
		json.Unmarshal(resp.Body.Bytes(), &notes)
		assert.Len(t, notes, 2, "No filter should return all notes")
	})
}

func TestShareNote(t *testing.T) {
	tc := SetupTestEnv(t)
	note := tc.CreateDummyNote("Test Share Note")

	t.Run("Share note creates token", func(t *testing.T) {
		url := fmt.Sprintf("/v1/note/share?noteId=%d", note.ID)
		resp := tc.MakeRequest(http.MethodPost, url, nil)
		assert.Equal(t, http.StatusOK, resp.Code)

		var result map[string]interface{}
		json.Unmarshal(resp.Body.Bytes(), &result)
		assert.NotEmpty(t, result["shareToken"])
		assert.NotEmpty(t, result["shareUrl"])
	})

	t.Run("Share note returns same token on repeat", func(t *testing.T) {
		url := fmt.Sprintf("/v1/note/share?noteId=%d", note.ID)
		resp1 := tc.MakeRequest(http.MethodPost, url, nil)
		resp2 := tc.MakeRequest(http.MethodPost, url, nil)

		var result1, result2 map[string]interface{}
		json.Unmarshal(resp1.Body.Bytes(), &result1)
		json.Unmarshal(resp2.Body.Bytes(), &result2)
		assert.Equal(t, result1["shareToken"], result2["shareToken"])
	})

	t.Run("Unshare note removes token", func(t *testing.T) {
		url := fmt.Sprintf("/v1/note/share?noteId=%d", note.ID)
		tc.MakeRequest(http.MethodDelete, url, nil)

		// Verify note is no longer shared
		updatedNote, _ := tc.AppCtx.GetNote(note.ID)
		assert.Nil(t, updatedNote.ShareToken)
	})

	t.Run("Share nonexistent note returns error", func(t *testing.T) {
		url := "/v1/note/share?noteId=99999"
		resp := tc.MakeRequest(http.MethodPost, url, nil)
		assert.Equal(t, http.StatusNotFound, resp.Code)
	})
}

func TestDeleteGroupDoesNotDeleteOwnedNotes(t *testing.T) {
	tc := SetupTestEnv(t)

	group := tc.CreateDummyGroup("NoteOwnerGroup")
	note := &models.Note{Name: "Owned Note", OwnerId: &group.ID}
	tc.DB.Create(note)

	// Delete the group
	err := tc.AppCtx.DeleteGroup(group.ID)
	assert.NoError(t, err)

	// The note should still exist (owner set to NULL), NOT be cascade-deleted
	var check models.Note
	result := tc.DB.First(&check, note.ID)
	assert.NoError(t, result.Error,
		"Owned note should survive group deletion (SET NULL), not be cascade-deleted")
	assert.Nil(t, check.OwnerId,
		"Note's OwnerId should be NULL after owner group is deleted")
}

func TestNoteNameFilterTreatsUnderscoreLiterally(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create notes: one with underscore, one similar but no underscore
	tc.DB.Create(&models.Note{Name: "report_final"})
	tc.DB.Create(&models.Note{Name: "reportXfinal"})

	// Filter notes by name containing the literal underscore
	resp := tc.MakeRequest(http.MethodGet, "/v1/notes?Name=report_final", nil)
	assert.Equal(t, http.StatusOK, resp.Code)

	var notes []*models.Note
	json.Unmarshal(resp.Body.Bytes(), &notes)

	assert.Equal(t, 1, len(notes),
		"Name filter should treat _ as literal character, not LIKE wildcard; both report_final and reportXfinal were returned")
}

func TestNoteGroupFilterIncludesUnownedNotes(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a group
	group := tc.CreateDummyGroup("Filter Group")

	// Create a note WITHOUT an owner, then relate it to the group via the junction table
	note := &models.Note{Name: "Unowned Note", Description: "has no owner"}
	tc.DB.Create(note)
	tc.DB.Exec("INSERT INTO groups_related_notes (group_id, note_id) VALUES (?, ?)", group.ID, note.ID)

	// Query notes filtered by this group
	url := fmt.Sprintf("/v1/notes?Groups=%d", group.ID)
	resp := tc.MakeRequest(http.MethodGet, url, nil)
	assert.Equal(t, http.StatusOK, resp.Code)

	var notes []*models.Note
	json.Unmarshal(resp.Body.Bytes(), &notes)

	assert.Equal(t, 1, len(notes),
		"Note with NULL owner_id related to group via junction table should be returned by group filter")
	if len(notes) > 0 {
		assert.Equal(t, note.ID, notes[0].ID)
	}
}

func TestNoteUpdateExplicitEmptyTagsClears(t *testing.T) {
	tc := SetupTestEnv(t)

	tag := &models.Tag{Name: "Removable Tag"}
	tc.DB.Create(tag)

	// Create a note with a tag
	createResp := tc.MakeRequest(http.MethodPost, "/v1/note", map[string]any{
		"Name": "Note With Tag",
		"Tags": []uint{tag.ID},
	})
	assert.Equal(t, http.StatusOK, createResp.Code)
	var created models.Note
	json.Unmarshal(createResp.Body.Bytes(), &created)

	var before models.Note
	tc.DB.Preload("Tags").First(&before, created.ID)
	assert.Equal(t, 1, len(before.Tags), "note should start with 1 tag")

	// Send a JSON update with an explicit empty Tags array — should CLEAR tags
	clearBody := map[string]any{
		"ID":   created.ID,
		"Name": "Note With Tag",
		"Tags": []uint{},
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/note", clearBody)
	assert.Equal(t, http.StatusOK, resp.Code)

	var after models.Note
	tc.DB.Preload("Tags").First(&after, created.ID)
	assert.Equal(t, 0, len(after.Tags),
		"Sending explicit empty Tags array should clear all tags")
}

func TestNoteUpdatePartialJSONPreservesTagAssociations(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create tags
	tag1 := &models.Tag{Name: "Note Tag A"}
	tag2 := &models.Tag{Name: "Note Tag B"}
	tc.DB.Create(tag1)
	tc.DB.Create(tag2)

	// Create a note with tags via API
	createResp := tc.MakeRequest(http.MethodPost, "/v1/note", map[string]any{
		"Name": "Tagged Note",
		"Tags": []uint{tag1.ID, tag2.ID},
	})
	assert.Equal(t, http.StatusOK, createResp.Code)
	var created models.Note
	json.Unmarshal(createResp.Body.Bytes(), &created)

	// Verify tags were assigned
	var checkBefore models.Note
	tc.DB.Preload("Tags").First(&checkBefore, created.ID)
	assert.Equal(t, 2, len(checkBefore.Tags), "note should start with 2 tags")

	// Send a partial JSON update that only changes the description
	partialBody := map[string]any{
		"ID":          created.ID,
		"Name":        "Tagged Note",
		"Description": "Updated description",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/note", partialBody)
	assert.Equal(t, http.StatusOK, resp.Code)

	// The tags should be preserved, not cleared
	var checkAfter models.Note
	tc.DB.Preload("Tags").First(&checkAfter, created.ID)
	assert.Equal(t, "Updated description", checkAfter.Description)
	assert.Equal(t, 2, len(checkAfter.Tags),
		"Editing only description should not clear tag associations — partial JSON must preserve unset arrays")
}

func TestNoteTypeUpdatePreservesCustomFields(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a note type with custom HTML fields
	nt := &models.NoteType{
		Name:          "Original NoteType",
		Description:   "Original desc",
		CustomHeader:  "<h2>NT Header</h2>",
		CustomSidebar: "<div>NT Sidebar</div>",
		CustomSummary: "<p>NT Summary</p>",
		CustomAvatar:  "<img src='nt.png'>",
	}
	tc.DB.Create(nt)

	// Send a partial JSON edit that only changes the name
	partialBody := map[string]any{
		"ID":   nt.ID,
		"Name": "Renamed NoteType",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/note/noteType/edit", partialBody)
	assert.Equal(t, http.StatusOK, resp.Code)

	var check models.NoteType
	tc.DB.First(&check, nt.ID)
	assert.Equal(t, "Renamed NoteType", check.Name)
	assert.Equal(t, "Original desc", check.Description,
		"Editing only name should not clear Description")
	assert.Equal(t, "<h2>NT Header</h2>", check.CustomHeader,
		"Editing only name should not clear CustomHeader")
	assert.Equal(t, "<div>NT Sidebar</div>", check.CustomSidebar,
		"Editing only name should not clear CustomSidebar")
	assert.Equal(t, "<p>NT Summary</p>", check.CustomSummary,
		"Editing only name should not clear CustomSummary")
	assert.Equal(t, "<img src='nt.png'>", check.CustomAvatar,
		"Editing only name should not clear CustomAvatar")
}

func TestNoteUpdatePartialJSONPreservesNoteTypeId(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a note type
	nt := &models.NoteType{Name: "Meeting Notes"}
	tc.DB.Create(nt)

	// Create a note with that note type
	note := tc.CreateDummyNote("Typed Note")
	note.NoteTypeId = &nt.ID
	tc.DB.Save(note)

	// Verify note type is set
	var before models.Note
	tc.DB.First(&before, note.ID)
	assert.NotNil(t, before.NoteTypeId, "note should start with a NoteTypeId")

	// Send a partial JSON edit that only changes the description
	partialBody := map[string]any{
		"ID":          note.ID,
		"Name":        "Typed Note",
		"Description": "Updated desc",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/note", partialBody)
	assert.Equal(t, http.StatusOK, resp.Code)

	// NoteTypeId should be preserved, not cleared to nil
	var after models.Note
	tc.DB.First(&after, note.ID)
	assert.Equal(t, "Updated desc", after.Description)
	if assert.NotNil(t, after.NoteTypeId,
		"Editing only description should not clear NoteTypeId") {
		assert.Equal(t, nt.ID, *after.NoteTypeId)
	}
}

func TestNoteUpdatePartialJSONPreservesOtherFields(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a note with name, description, and meta populated
	note := tc.CreateDummyNote("Original Name")
	note.Description = "Original Desc"
	note.Meta = []byte(`{"key":"value"}`)
	tc.DB.Save(note)

	// Send a partial JSON body that only changes the name
	// (simulates CLI: mr note edit ID --name "New Name")
	partialBody := map[string]any{
		"ID":   note.ID,
		"Name": "Updated Name",
	}
	resp := tc.MakeRequest(http.MethodPost, "/v1/note", partialBody)
	assert.Equal(t, http.StatusOK, resp.Code)

	// The description and meta should be preserved, not cleared
	var check models.Note
	tc.DB.First(&check, note.ID)
	assert.Equal(t, "Updated Name", check.Name)
	assert.Equal(t, "Original Desc", check.Description,
		"Editing only name should not clear the description — partial JSON must preserve unset fields")
	assert.JSONEq(t, `{"key":"value"}`, string(check.Meta),
		"Editing only name should not reset meta to default")
}
