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

// TestReorderBlocksDuplicatePositionsAccepted demonstrates that ReorderBlocks
// does not validate the uniqueness of new position values. If the caller
// submits positions that assign the same position string to two or more
// blocks, the operation succeeds without error, leaving the note's blocks
// in an ambiguous ordering state.
//
// This is a problem because position strings are used to sort blocks in
// presentation order (ORDER BY position ASC). When two blocks share the
// same position, their relative order is undefined and may differ between
// queries, leading to a non-deterministic block order that confuses users.
//
// Steps to reproduce:
//  1. Create a note with two text blocks at distinct positions.
//  2. Call ReorderBlocks assigning the SAME position to both blocks.
//  3. Observe that the operation succeeds (no error returned).
//  4. Fetch blocks: both blocks have identical position values, making
//     their sort order undefined.
//
// Expected: ReorderBlocks should reject duplicate position values with
//
//	an error like "duplicate positions are not allowed".
//
// Actual:   The operation succeeds, and both blocks end up with the
//
//	same position string.
func TestReorderBlocksDuplicatePositionsAccepted(t *testing.T) {
	tc := SetupTestEnv(t)

	// Step 1: Create a note with two text blocks at distinct positions
	note := tc.CreateDummyNote("Duplicate Position Note")

	respA := tc.MakeRequest(http.MethodPost, "/v1/note/block", map[string]any{
		"noteId":   note.ID,
		"type":     "text",
		"position": "d",
		"content":  map[string]string{"text": "Block A"},
	})
	require.Equal(t, http.StatusCreated, respA.Code)
	var blockA models.NoteBlock
	require.NoError(t, json.Unmarshal(respA.Body.Bytes(), &blockA))

	respB := tc.MakeRequest(http.MethodPost, "/v1/note/block", map[string]any{
		"noteId":   note.ID,
		"type":     "text",
		"position": "h",
		"content":  map[string]string{"text": "Block B"},
	})
	require.Equal(t, http.StatusCreated, respB.Code)
	var blockB models.NoteBlock
	require.NoError(t, json.Unmarshal(respB.Body.Bytes(), &blockB))

	// Verify both blocks have distinct positions initially
	var initialBlocks []models.NoteBlock
	tc.DB.Where("note_id = ?", note.ID).Order("position ASC").Find(&initialBlocks)
	require.Len(t, initialBlocks, 2)
	require.NotEqual(t, initialBlocks[0].Position, initialBlocks[1].Position,
		"setup: blocks should have different positions initially")

	// Step 2: Reorder both blocks to the SAME position
	reorderResp := tc.MakeRequest(http.MethodPost, "/v1/note/blocks/reorder", map[string]any{
		"noteId": note.ID,
		"positions": map[string]string{
			fmt.Sprintf("%d", blockA.ID): "m",
			fmt.Sprintf("%d", blockB.ID): "m",
		},
	})

	// Step 3: The API should reject duplicate positions.
	// BUG: It returns 204 No Content (success) instead of an error.
	assert.NotEqual(t, http.StatusNoContent, reorderResp.Code,
		"BUG: ReorderBlocks accepts duplicate position values without error. "+
			"Both blocks were assigned position \"m\", leaving their sort order "+
			"undefined. The endpoint should reject the request with a 400 Bad Request "+
			"when the positions map contains duplicate values.")
}
