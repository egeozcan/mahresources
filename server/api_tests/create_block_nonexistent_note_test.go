package api_tests

import (
	"encoding/json"
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestCreateBlockForNonExistentNoteReturnsError demonstrates that CreateBlock
// does not validate the existence of the parent note before inserting the block.
//
// Root cause:
// CreateBlock validates block type and content, auto-assigns a position, and
// then runs tx.Create(&block) — but it never checks that editor.NoteID refers
// to an actual Note row.  The only safety net is the database foreign-key
// constraint (NoteBlock.Note has OnDelete:CASCADE), but:
//   - SQLite FK enforcement depends on PRAGMA foreign_keys being ON, which is
//     unreliable in the codebase (see EnsureForeignKeysActive and its comments).
//   - CreateBlock does NOT call EnsureForeignKeysActive before the insert.
//   - Even when FK enforcement is active, the error message is a low-level
//     constraint violation rather than a clear 400-level "note not found".
//
// Impact:
// When FK enforcement is off (which the codebase documents can happen), the
// block is silently created as an orphan: it has a note_id that points to
// nothing.  Subsequent queries for the note's blocks will never find it (they
// filter by a valid note_id), and the block occupies space in the DB forever.
// The API returns 201 Created even though the operation is semantically invalid.
func TestCreateBlockForNonExistentNoteReturnsError(t *testing.T) {
	tc := SetupTestEnv(t)

	nonExistentNoteID := uint(99999)

	// Verify the note really does not exist
	var check models.Note
	result := tc.DB.First(&check, nonExistentNoteID)
	assert.Error(t, result.Error, "setup: note 99999 should not exist")

	// Attempt to create a text block referencing the non-existent note
	payload := map[string]interface{}{
		"noteId":   nonExistentNoteID,
		"type":     "text",
		"position": "n",
		"content":  map[string]string{"text": "Orphaned block"},
	}

	resp := tc.MakeRequest(http.MethodPost, "/v1/note/block", payload)

	// BUG: the API should reject this with a 4xx error because the note
	// does not exist, but it returns 201 and creates an orphaned block.
	assert.NotEqual(t, http.StatusCreated, resp.Code,
		"BUG: CreateBlock should reject a block whose noteId does not refer "+
			"to an existing note, but it returned 201 Created. "+
			"The block is now an orphan in the database.")

	// Double-check: if the block WAS created, that proves the bug
	if resp.Code == http.StatusCreated {
		var block models.NoteBlock
		err := json.Unmarshal(resp.Body.Bytes(), &block)
		assert.NoError(t, err)

		// The block exists in the DB but its NoteID is dangling
		var orphan models.NoteBlock
		tc.DB.First(&orphan, block.ID)
		assert.Equal(t, nonExistentNoteID, orphan.NoteID,
			"orphaned block was created with a dangling note_id")
	}
}
