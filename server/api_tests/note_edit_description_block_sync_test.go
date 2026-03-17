package api_tests

import (
	"encoding/json"
	"fmt"
	"mahresources/models"
	"mahresources/models/query_models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestNoteEditDescriptionDoesNotSyncToBlock proves that the
// /v1/note/editDescription endpoint bypasses the block-sync logic.
//
// When a note has text blocks, its Description and the first text block's
// content are kept in sync by two mechanisms:
//   - CreateOrUpdateNote writes Description -> first text block (note_context.go)
//   - Block mutations write first text block -> Description (syncFirstTextBlockToDescriptionTx)
//
// The generic UpdateDescription (basic_entity_context.go) used by
// /v1/note/editDescription updates ONLY the description column. It does NOT
// sync the change to the first text block. As a result:
//   1. The description and first text block become out of sync.
//   2. The next block mutation (e.g., creating a second block) triggers
//      syncFirstTextBlockToDescriptionTx, which overwrites the description
//      with the STALE block content — silently losing the user's edit.
func TestNoteEditDescriptionDoesNotSyncToBlock(t *testing.T) {
	tc := SetupTestEnv(t)

	// Step 1: Create a note with an initial description
	note := tc.CreateDummyNote("Block Sync Note")
	require.NotZero(t, note.ID)

	// Step 2: Create a text block on the note.
	// The block's content matches the note's initial description.
	initialDesc := note.Description // "Test Description" from CreateDummyNote
	blockContent, _ := json.Marshal(map[string]string{"text": initialDesc})
	block := tc.CreateDummyBlock(note.ID, "text", string(blockContent), "n")
	require.NotZero(t, block.ID)

	// Step 3: Edit the note description via the quick-edit endpoint.
	// This uses the generic UpdateDescription which does a raw DB update.
	newDesc := "Completely new description set via editDescription"
	editURL := fmt.Sprintf("/v1/note/editDescription?id=%d", note.ID)
	resp := tc.MakeRequest(http.MethodPost, editURL, map[string]string{
		"Description": newDesc,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	// Verify the note's description was updated in the DB
	var noteAfterEdit models.Note
	tc.DB.First(&noteAfterEdit, note.ID)
	require.Equal(t, newDesc, noteAfterEdit.Description,
		"editDescription should have updated the description column")

	// Step 4: Verify the first text block was NOT updated (BUG).
	// If the sync worked correctly, the block content should also be newDesc.
	var blockAfterEdit models.NoteBlock
	tc.DB.First(&blockAfterEdit, block.ID)
	var blockTextContent struct {
		Text string `json:"text"`
	}
	json.Unmarshal(blockAfterEdit.Content, &blockTextContent)

	// This assertion DEMONSTRATES the bug: the block still has the OLD content.
	// A correct implementation would sync the new description to the block.
	assert.Equal(t, newDesc, blockTextContent.Text,
		"BUG: editDescription did not sync the new description to the first text block; "+
			"block still has the old content %q while note.Description is %q",
		blockTextContent.Text, newDesc)
}

// TestNoteEditDescriptionCausesDataLoss demonstrates that the desync from
// editDescription leads to silent data loss when a subsequent block operation
// triggers the reverse sync (block -> description).
func TestNoteEditDescriptionCausesDataLoss(t *testing.T) {
	tc := SetupTestEnv(t)

	// Step 1: Create a note
	note := tc.CreateDummyNote("Data Loss Note")

	// Step 2: Create a text block matching the initial description
	initialDesc := note.Description
	blockContent, _ := json.Marshal(map[string]string{"text": initialDesc})
	tc.CreateDummyBlock(note.ID, "text", string(blockContent), "a")

	// Step 3: Edit the description via the quick-edit endpoint
	newDesc := "Important updated description"
	editURL := fmt.Sprintf("/v1/note/editDescription?id=%d", note.ID)
	resp := tc.MakeRequest(http.MethodPost, editURL, map[string]string{
		"Description": newDesc,
	})
	require.Equal(t, http.StatusOK, resp.Code)

	// Verify the edit took effect
	var checkNote models.Note
	tc.DB.First(&checkNote, note.ID)
	require.Equal(t, newDesc, checkNote.Description)

	// Step 4: Create a second text block using the application context directly.
	// This triggers syncFirstTextBlockToDescriptionTx which reads the FIRST
	// text block (still with OLD content) and overwrites the note's description.
	secondBlockContent, _ := json.Marshal(map[string]string{"text": "Second block"})
	_, err := tc.AppCtx.CreateBlock(&query_models.NoteBlockEditor{
		NoteID:   note.ID,
		Type:     "text",
		Content:  secondBlockContent,
		Position: "z",
	})
	require.NoError(t, err, "creating a second text block should succeed")

	// Step 5: Check the note's description — it should still be newDesc,
	// but the bug causes it to revert to the OLD first block's content.
	var noteAfterBlockCreate models.Note
	tc.DB.First(&noteAfterBlockCreate, note.ID)

	assert.Equal(t, newDesc, noteAfterBlockCreate.Description,
		"BUG: creating a second text block reverted the note description from %q back to %q; "+
			"the editDescription change was silently lost because syncFirstTextBlockToDescriptionTx "+
			"overwrote it with the stale first block content",
		newDesc, noteAfterBlockCreate.Description)
}
