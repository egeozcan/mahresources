package api_tests

import (
	"encoding/json"
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCreateBlockEmptyPositionOverwritesDescription demonstrates that
// CreateBlock with an empty Position string causes the new block to sort
// before all existing blocks (since "" < any non-empty string in SQL
// ORDER BY), and the description sync picks the new block's default
// content (empty text) as the note's description — silently wiping out
// the existing description.
//
// Steps to reproduce:
//  1. Create a note.
//  2. Create a text block at position "d" with content "Important content".
//     -> Note description syncs to "Important content".
//  3. Create a second text block with Position="" (empty) and default content.
//     -> syncFirstTextBlockToDescriptionTx finds the new block first
//        (empty string sorts before "d").
//     -> Note description is overwritten to "" (the new block's default text).
//  4. Verify the note description is still "Important content".
//     -> BUG: description was silently wiped to "".
func TestCreateBlockEmptyPositionOverwritesDescription(t *testing.T) {
	tc := SetupTestEnv(t)

	// Step 1: Create a note
	note := tc.CreateDummyNote("Empty Position Bug Note")

	// Step 2: Create a text block with meaningful content at position "d"
	resp := tc.MakeRequest(http.MethodPost, "/v1/note/block", map[string]any{
		"noteId":   note.ID,
		"type":     "text",
		"position": "d",
		"content":  map[string]string{"text": "Important content"},
	})
	require.Equal(t, http.StatusCreated, resp.Code, "first block creation should succeed")

	// Verify description was synced to "Important content"
	var check models.Note
	require.NoError(t, tc.DB.First(&check, note.ID).Error)
	require.Equal(t, "Important content", check.Description,
		"setup: description should be synced from the first text block")

	// Step 3: Create another text block with EMPTY position (no position specified)
	// The block type default content for text is {"text":""}, so the block has empty text.
	resp2 := tc.MakeRequest(http.MethodPost, "/v1/note/block", map[string]any{
		"noteId":   note.ID,
		"type":     "text",
		"position": "",
	})
	require.Equal(t, http.StatusCreated, resp2.Code, "second block creation should succeed")

	var newBlock models.NoteBlock
	require.NoError(t, json.Unmarshal(resp2.Body.Bytes(), &newBlock))
	assert.NotEqual(t, "", newBlock.Position, "the new block should have an auto-assigned position when none is provided")

	// Step 4: Verify the note description was NOT overwritten
	var afterCreate models.Note
	require.NoError(t, tc.DB.First(&afterCreate, note.ID).Error)

	// BUG: The empty-position block sorts before "d", so
	// syncFirstTextBlockToDescriptionTx picks it as the "first" text block
	// and overwrites the description with its empty text content.
	assert.Equal(t, "Important content", afterCreate.Description,
		"BUG: creating a text block with empty position should not overwrite "+
			"the note description with empty content; the existing first block "+
			"at position 'd' should remain the description source")
}
