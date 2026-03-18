package api_tests

import (
	"encoding/json"
	"fmt"
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCreateNoteWithNonExistentOwner verifies that creating a note with an
// OwnerId pointing to a group that does not exist returns an error rather
// than silently creating a note with a dangling foreign key.
func TestCreateNoteWithNonExistentOwner(t *testing.T) {
	tc := SetupTestEnv(t)

	const bogusGroupID = 999999

	// Sanity-check: the group really doesn't exist.
	var g models.Group
	assert.Error(t, tc.DB.First(&g, bogusGroupID).Error, "setup: group 999999 should not exist")

	// Attempt to create a note whose OwnerId references the non-existent group.
	resp := tc.MakeRequest(http.MethodPost, "/v1/note", map[string]any{
		"Name":    "Orphan-Owner Note",
		"OwnerId": bogusGroupID,
	})

	// The API must reject the request because the owner group doesn't exist.
	assert.NotEqual(t, http.StatusOK, resp.Code,
		"Creating a note with a non-existent OwnerId should fail, but the API returned 200 OK — "+
			"the note was silently created with a dangling foreign key")
}

// TestUpdateNoteToNonExistentOwner verifies that updating an existing note's
// OwnerId to a group that does not exist returns an error.
func TestUpdateNoteToNonExistentOwner(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a note with a valid owner first.
	owner := tc.CreateDummyGroup("Real Owner")
	note := tc.CreateDummyNote("Owned Note")
	note.OwnerId = &owner.ID
	tc.DB.Save(note)

	const bogusGroupID = 888888

	// Attempt to update the note's owner to a non-existent group.
	resp := tc.MakeRequest(http.MethodPost, "/v1/note", map[string]any{
		"ID":      note.ID,
		"Name":    note.Name,
		"OwnerId": bogusGroupID,
	})

	// The API must reject the request.
	assert.NotEqual(t, http.StatusOK, resp.Code,
		"Updating a note's OwnerId to a non-existent group should fail, but the API returned 200 OK")

	// Additionally verify the original OwnerId was NOT changed.
	var after models.Note
	tc.DB.First(&after, note.ID)
	if after.OwnerId != nil {
		assert.Equal(t, owner.ID, *after.OwnerId,
			"After a failed update, the note's OwnerId should remain unchanged")
	}
}

// TestCreateNoteWithNonExistentOwnerDanglingFK demonstrates the data-integrity
// consequence: when no validation occurs, the note ends up with an OwnerId
// that doesn't reference any group, and loading the Owner association yields nil.
func TestCreateNoteWithNonExistentOwnerDanglingFK(t *testing.T) {
	tc := SetupTestEnv(t)

	const bogusGroupID = 777777

	resp := tc.MakeRequest(http.MethodPost, "/v1/note", map[string]any{
		"Name":    "Dangling FK Note",
		"OwnerId": bogusGroupID,
	})

	// If the API allowed the creation (which is the bug), verify the damage.
	if resp.Code != http.StatusOK {
		// If the API correctly rejected it, the bug is fixed — skip.
		t.Skip("API correctly rejected non-existent OwnerId — bug is fixed")
	}

	var created models.Note
	err := json.Unmarshal(resp.Body.Bytes(), &created)
	assert.NoError(t, err)

	// Load the note with its Owner preloaded.
	url := fmt.Sprintf("/v1/note?id=%d", created.ID)
	getResp := tc.MakeRequest(http.MethodGet, url, nil)
	assert.Equal(t, http.StatusOK, getResp.Code)

	var loaded models.Note
	json.Unmarshal(getResp.Body.Bytes(), &loaded)

	// The note has an OwnerId set but the Owner association is nil —
	// a dangling foreign key that will confuse any code relying on Owner.
	assert.NotNil(t, loaded.OwnerId,
		"Note should have OwnerId set (the bogus value)")
	assert.Nil(t, loaded.Owner,
		"Owner should be nil because the referenced group doesn't exist — this is the dangling FK")

	t.Errorf("Note was created with OwnerId=%d but no such group exists — "+
		"this is a dangling foreign key that should have been rejected at creation time",
		bogusGroupID)
}
