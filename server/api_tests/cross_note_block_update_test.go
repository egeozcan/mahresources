package api_tests

import (
	"encoding/json"
	"fmt"
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestUpdateBlockContent_RejectsBlockBelongingToDifferentNote demonstrates that
// UpdateBlockContent does not validate note ownership of the block.
//
// Root cause:
// UpdateBlockContent (block_context.go) accepts only a blockID and content.
// It looks up the block by primary key, validates the content against the block
// type, and saves — but it never checks that the caller is operating within the
// context of the correct note. The HTTP handler (UpdateBlockContentHandler) also
// only reads an "id" query parameter; there is no noteId parameter at all.
//
// Compare with ReorderBlocks, which explicitly verifies:
//
//	"one or more block IDs do not belong to the specified note"
//
// UpdateBlockContent and UpdateBlockState lack this check entirely.
//
// Impact:
// Any API caller can silently modify the content of blocks belonging to any
// note, as long as they know (or guess) the block's numeric ID. In a multi-user
// or plugin context, this breaks the assumption that block mutations are scoped
// to the note being edited. It also means the description-sync and mention-sync
// side effects fire on the wrong note, potentially corrupting its description
// with content the note's author never wrote.
func TestUpdateBlockContent_RejectsBlockBelongingToDifferentNote(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create two separate notes
	noteA := tc.CreateDummyNote("Note A")
	noteB := tc.CreateDummyNote("Note B")

	// Create a text block on Note A
	createResp := tc.MakeRequest(http.MethodPost, "/v1/note/block", map[string]any{
		"noteId":   noteA.ID,
		"type":     "text",
		"position": "n",
		"content":  map[string]string{"text": "Original content on Note A"},
	})
	assert.Equal(t, http.StatusCreated, createResp.Code)

	var blockOnNoteA models.NoteBlock
	err := json.Unmarshal(createResp.Body.Bytes(), &blockOnNoteA)
	assert.NoError(t, err)
	assert.Equal(t, noteA.ID, blockOnNoteA.NoteID, "setup: block should belong to Note A")

	// Now, pretending to be editing Note B, update the block that belongs to Note A.
	// A well-designed API should require a noteId parameter and reject the request
	// when the block does not belong to the specified note.
	//
	// Since the current API does not even accept a noteId parameter, we call the
	// endpoint as-is. The bug is that this succeeds — the block on Note A is
	// silently mutated even though the caller has no business editing Note A.
	updateURL := fmt.Sprintf("/v1/note/block?id=%d&noteId=%d", blockOnNoteA.ID, noteB.ID)
	updateResp := tc.MakeRequest(http.MethodPut, updateURL, map[string]any{
		"content": map[string]string{"text": "Hijacked content from Note B context"},
	})

	// BUG: The API should reject this with an error because the block does not
	// belong to Note B's context. Instead it returns 200 and mutates the block.
	assert.NotEqual(t, http.StatusOK, updateResp.Code,
		"BUG: UpdateBlockContent should reject updates to blocks that belong to a different note. "+
			"The API allows any caller to modify any block by ID alone, with no note-ownership check. "+
			"This silently mutated a block on Note A (ID=%d) without any note context validation.",
		noteA.ID)

	// Additionally verify the corruption: the block's content was changed AND
	// Note A's description was overwritten by the cross-note update's sync logic.
	if updateResp.Code == http.StatusOK {
		var updatedBlock models.NoteBlock
		json.Unmarshal(updateResp.Body.Bytes(), &updatedBlock)

		assert.Contains(t, string(updatedBlock.Content), "Hijacked content",
			"The block content was overwritten by a cross-note update")

		var noteACheck models.Note
		tc.DB.First(&noteACheck, noteA.ID)
		assert.NotContains(t, noteACheck.Description, "Hijacked",
			"Note A's description was corrupted by the cross-note block update's description sync")

		// Note B should be completely unaffected
		var noteBCheck models.Note
		tc.DB.First(&noteBCheck, noteB.ID)
		_ = noteBCheck // Note B is never touched, confirming the mutation is on the wrong note
	}
}

// TestUpdateBlockState_RejectsBlockBelongingToDifferentNote demonstrates the
// same note-ownership gap exists for UpdateBlockState.
//
// UpdateBlockState also takes only a blockID and state payload, with no note
// context validation. A caller can toggle checkboxes or modify any UI state
// on blocks belonging to arbitrary notes.
func TestUpdateBlockState_RejectsBlockBelongingToDifferentNote(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create two separate notes
	noteA := tc.CreateDummyNote("Note A - State")
	noteB := tc.CreateDummyNote("Note B - State")

	// Create a todos block on Note A
	createResp := tc.MakeRequest(http.MethodPost, "/v1/note/block", map[string]any{
		"noteId":   noteA.ID,
		"type":     "todos",
		"position": "n",
		"content":  map[string]any{"items": []map[string]string{{"id": "t1", "label": "Task 1"}}},
	})
	assert.Equal(t, http.StatusCreated, createResp.Code)

	var todosBlockOnNoteA models.NoteBlock
	err := json.Unmarshal(createResp.Body.Bytes(), &todosBlockOnNoteA)
	assert.NoError(t, err)
	assert.Equal(t, noteA.ID, todosBlockOnNoteA.NoteID, "setup: todos block should belong to Note A")

	// From Note B's context, update the state of the todos block on Note A.
	stateURL := fmt.Sprintf("/v1/note/block/state?id=%d&noteId=%d", todosBlockOnNoteA.ID, noteB.ID)
	stateResp := tc.MakeRequest(http.MethodPatch, stateURL, map[string]any{
		"state": map[string]any{"checked": []string{"t1"}},
	})

	// BUG: The API should reject this, but it returns 200 and mutates the block.
	assert.NotEqual(t, http.StatusOK, stateResp.Code,
		"BUG: UpdateBlockState should reject state updates to blocks that belong to a different note. "+
			"The API allows any caller to modify any block's state by ID alone, with no note-ownership check. "+
			"This silently mutated a todos block on Note A (ID=%d).",
		noteA.ID)

	// Verify the corruption if the update succeeded
	if stateResp.Code == http.StatusOK {
		var updatedBlock models.NoteBlock
		json.Unmarshal(stateResp.Body.Bytes(), &updatedBlock)

		assert.Contains(t, string(updatedBlock.State), "t1",
			"The block state was modified by a cross-note update")

		_ = noteB // Note B is never touched
	}
}
