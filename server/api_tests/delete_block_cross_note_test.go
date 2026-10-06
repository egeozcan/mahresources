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

	assert.Equal(t, http.StatusBadRequest, deleteResp.Code,
		"DeleteBlock should reject deletion of blocks that belong to a different note. This allowed deleting block %d (owned by Note A, ID=%d) while specifying noteId=%d (Note B).",
		blockOnNoteA.ID, noteA.ID, noteB.ID)

	// The rejected request must leave the block in place
	var blockCount int64
	tc.DB.Model(&models.NoteBlock{}).Where("id = ?", blockOnNoteA.ID).Count(&blockCount)
	assert.Equal(t, int64(1), blockCount,
		"The block on Note A should still exist because the delete should have been rejected")

	// ...and so must Note A's description, which the sync logic would blank
	var noteACheck models.Note
	tc.DB.First(&noteACheck, noteA.ID)
	assert.NotEmpty(t, noteACheck.Description,
		"Note A's description was blanked as a side effect of the cross-note block deletion")
}

func TestDeleteBlock_RequiresNoteId(t *testing.T) {
	tc := SetupTestEnv(t)

	noteA := tc.CreateDummyNote("Note A")

	createResp := tc.MakeRequest(http.MethodPost, "/v1/note/block", map[string]any{
		"noteId":   noteA.ID,
		"type":     "text",
		"position": "n",
		"content":  map[string]string{"text": "Important content"},
	})
	require.Equal(t, http.StatusCreated, createResp.Code)

	var blockOnNoteA models.NoteBlock
	require.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &blockOnNoteA))

	// Attempt to delete without noteId
	deleteURL := fmt.Sprintf("/v1/note/block?id=%d", blockOnNoteA.ID)
	deleteResp := tc.MakeRequest(http.MethodDelete, deleteURL, nil)

	assert.Equal(t, http.StatusBadRequest, deleteResp.Code, "DeleteBlock should require noteId")

	// Verify block still exists
	var count int64
	tc.DB.Model(&models.NoteBlock{}).Where("id = ?", blockOnNoteA.ID).Count(&count)
	assert.Equal(t, int64(1), count)
}

// The POST form alternative shares the handler, so it must enforce the same
// ownership rule: no noteId and a foreign noteId are both refused.
func TestDeleteBlockPost_EnforcesNoteOwnership(t *testing.T) {
	tc := SetupTestEnv(t)

	noteA := tc.CreateDummyNote("Note A")
	noteB := tc.CreateDummyNote("Note B")

	createResp := tc.MakeRequest(http.MethodPost, "/v1/note/block", map[string]any{
		"noteId":   noteA.ID,
		"type":     "text",
		"position": "n",
		"content":  map[string]string{"text": "Keep me"},
	})
	require.Equal(t, http.StatusCreated, createResp.Code)

	var block models.NoteBlock
	require.NoError(t, json.Unmarshal(createResp.Body.Bytes(), &block))

	for name, url := range map[string]string{
		"missing noteId": fmt.Sprintf("/v1/note/block/delete?id=%d", block.ID),
		"foreign noteId": fmt.Sprintf("/v1/note/block/delete?id=%d&noteId=%d", block.ID, noteB.ID),
	} {
		resp := tc.MakeRequest(http.MethodPost, url, nil)
		assert.Equal(t, http.StatusBadRequest, resp.Code, name)
	}

	var count int64
	tc.DB.Model(&models.NoteBlock{}).Where("id = ?", block.ID).Count(&count)
	assert.Equal(t, int64(1), count, "a refused delete must leave the block in place")

	resp := tc.MakeRequest(http.MethodPost, fmt.Sprintf("/v1/note/block/delete?id=%d&noteId=%d", block.ID, noteA.ID), nil)
	assert.Equal(t, http.StatusNoContent, resp.Code)
}
