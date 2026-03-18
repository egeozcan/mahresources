package api_tests

import (
	"encoding/json"
	"fmt"
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeleteBlock_RejectsBlockBelongingToDifferentNote demonstrates that
// DeleteBlock does not validate note ownership, unlike UpdateBlockContent
// and UpdateBlockState which both check noteId.
//
// Root cause:
// DeleteBlockHandler (block_api_handlers.go) accepts only a block "id" query
// parameter. It does not accept a "noteId" parameter and performs no ownership
// validation. Compare with UpdateBlockContentHandler and UpdateBlockStateHandler,
// which both accept noteId and reject requests where the block does not belong
// to the specified note:
//
//	if existing.NoteID != noteId {
//	    http_utils.HandleError(errors.New("block does not belong to the specified note"), ...)
//	}
//
// DeleteBlockHandler lacks this check entirely.
//
// Impact:
// Any API caller can delete blocks belonging to any note by knowing (or
// guessing) the block's numeric ID. In a multi-tab editing scenario, or with
// plugins, a delete request intended for Note B can silently destroy a block
// on Note A. The description-sync side effect then fires on the wrong note,
// potentially blanking its description when no text blocks remain.
//
// Scenario:
//  1. Create two notes, each with a text block.
//  2. Attempt to delete Note A's block while passing noteId=NoteB.
//  3. Expected: the request is rejected (block does not belong to Note B).
//  4. Actual: the block on Note A is deleted regardless, and Note A's
//     description is blanked by the description-sync side effect.
func TestDeleteBlock_RejectsBlockBelongingToDifferentNote(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create two separate notes
	noteA := tc.CreateDummyNote("Note A - Owner")
	noteB := tc.CreateDummyNote("Note B - Attacker")

	// Create a text block on Note A
	createResp := tc.MakeRequest(http.MethodPost, "/v1/note/block", map[string]any{
		"noteId":   noteA.ID,
		"type":     "text",
		"position": "n",
		"content":  map[string]string{"text": "Important content on Note A"},
	})
	require.Equal(t, http.StatusCreated, createResp.Code)

	var blockOnNoteA models.NoteBlock
	require.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &blockOnNoteA))
	require.Equal(t, noteA.ID, blockOnNoteA.NoteID, "setup: block should belong to Note A")

	// Attempt to delete Note A's block while claiming to be operating on Note B.
	// A well-designed API should accept noteId and reject the request when the
	// block does not belong to the specified note — just like UpdateBlockContent
	// and UpdateBlockState already do.
	deleteURL := fmt.Sprintf("/v1/note/block?id=%d&noteId=%d", blockOnNoteA.ID, noteB.ID)
	deleteResp := tc.MakeRequest(http.MethodDelete, deleteURL, nil)

	// BUG: The handler ignores noteId entirely and deletes the block.
	// It should return 400 Bad Request with "block does not belong to the specified note".
	assert.NotEqual(t, http.StatusNoContent, deleteResp.Code,
		"BUG: DeleteBlock should reject deletion of blocks that belong to a different note. "+
			"UpdateBlockContent and UpdateBlockState both validate noteId ownership, but "+
			"DeleteBlock ignores noteId entirely. This allowed deleting block %d (owned by Note A, ID=%d) "+
			"while specifying noteId=%d (Note B). The block was silently destroyed.",
		blockOnNoteA.ID, noteA.ID, noteB.ID)

	// Verify the damage: the block on Note A was actually deleted
	var blockCount int64
	tc.DB.Model(&models.NoteBlock{}).Where("id = ?", blockOnNoteA.ID).Count(&blockCount)
	assert.Equal(t, int64(1), blockCount,
		"The block on Note A should still exist because the delete should have been rejected")

	// Additionally verify the side-effect damage: Note A's description was blanked
	// by the description-sync logic that fires after deleting the only text block.
	if blockCount == 0 {
		var noteACheck models.Note
		tc.DB.First(&noteACheck, noteA.ID)
		assert.NotEmpty(t, noteACheck.Description,
			"Note A's description was blanked as a side effect of the cross-note block deletion")
	}
}
