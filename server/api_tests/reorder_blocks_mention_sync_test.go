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

// TestReorderBlocksDoesNotResyncMentions demonstrates that reordering blocks
// does not trigger syncMentionsForNote, even though ReorderBlocks calls
// syncFirstTextBlockToDescriptionTx (which may change the note's description
// to a different text block that contains mentions).
//
// CreateBlock, UpdateBlockContent, and DeleteBlock all call syncMentionsForNote
// after their transactions. ReorderBlocks does not.
//
// Scenario:
//  1. Create a note with two text blocks:
//     - Block A at position "d": contains a @[group:ID:Name] mention
//     - Block B at position "h": plain text, no mentions
//     Block A is first by position, so the description is synced from A.
//  2. On creation of block A, syncMentionsForNote fires and adds the group
//     association.
//  3. Manually remove the group-note association.
//  4. Reorder: swap positions so block B becomes first (position "d") and
//     block A becomes second (position "h").
//  5. After reorder, syncFirstTextBlockToDescriptionTx runs and updates the
//     description to block B's content (plain text). But since B is now the
//     first text block, block A's mention is still present in the note's
//     blocks. syncMentionsForNote should re-scan all text blocks and
//     re-establish the group association. But ReorderBlocks never calls it.
//
// Expected: After reorder, the group association is re-established because
//
//	block A still contains the mention.
//
// Actual:   The group association remains absent because ReorderBlocks does
//
//	not call syncMentionsForNote.
func TestReorderBlocksDoesNotResyncMentions(t *testing.T) {
	tc := SetupTestEnv(t)

	// Step 1: Create a group and a note
	group := &models.Group{Name: "ReorderMentionGroup", Meta: []byte(`{}`)}
	require.NoError(t, tc.DB.Create(group).Error)

	note := tc.CreateDummyNote("ReorderBlocks Mention Test")

	// Step 2: Create text block A (first by position) — mentions the group
	mentionText := fmt.Sprintf("Ref @[group:%d:%s] here", group.ID, group.Name)
	respA := tc.MakeRequest(http.MethodPost, "/v1/note/block", map[string]any{
		"noteId":   note.ID,
		"type":     "text",
		"position": "d",
		"content":  map[string]string{"text": mentionText},
	})
	require.Equal(t, http.StatusCreated, respA.Code)
	var blockA models.NoteBlock
	require.NoError(t, json.Unmarshal(respA.Body.Bytes(), &blockA))

	// Step 3: Create text block B (second by position) — no mentions
	respB := tc.MakeRequest(http.MethodPost, "/v1/note/block", map[string]any{
		"noteId":   note.ID,
		"type":     "text",
		"position": "h",
		"content":  map[string]string{"text": "Plain text, no mentions"},
	})
	require.Equal(t, http.StatusCreated, respB.Code)
	var blockB models.NoteBlock
	require.NoError(t, json.Unmarshal(respB.Body.Bytes(), &blockB))

	// Verify the group association was created by block A's mention sync
	var countAfterCreate int64
	tc.DB.Table("groups_related_notes").
		Where("group_id = ? AND note_id = ?", group.ID, note.ID).
		Count(&countAfterCreate)
	require.Equal(t, int64(1), countAfterCreate,
		"setup: creating block A with a mention should have synced the group association")

	// Step 4: Manually remove the group-note association
	require.NoError(t,
		tc.DB.Exec("DELETE FROM groups_related_notes WHERE group_id = ? AND note_id = ?",
			group.ID, note.ID).Error)

	var countAfterRemove int64
	tc.DB.Table("groups_related_notes").
		Where("group_id = ? AND note_id = ?", group.ID, note.ID).
		Count(&countAfterRemove)
	require.Equal(t, int64(0), countAfterRemove,
		"setup: group association should be gone after manual removal")

	// Step 5: Reorder — swap positions so B comes first, A comes second.
	// This changes which text block is "first" and triggers description sync.
	reorderResp := tc.MakeRequest(http.MethodPost, "/v1/note/blocks/reorder", map[string]any{
		"noteId": note.ID,
		"positions": map[string]string{
			fmt.Sprintf("%d", blockA.ID): "h", // A moves to second
			fmt.Sprintf("%d", blockB.ID): "d", // B moves to first
		},
	})
	require.Equal(t, http.StatusNoContent, reorderResp.Code)

	// Verify the description was updated to block B's content (the new first text block)
	var updatedNote models.Note
	require.NoError(t, tc.DB.First(&updatedNote, note.ID).Error)
	require.Equal(t, "Plain text, no mentions", updatedNote.Description,
		"setup: after reorder, the description should be synced from block B (now first)")

	// Step 6: Check that the group association was re-added by mention sync.
	// Block A still contains @[group:ID:Name]. syncMentionsForNote scans ALL
	// text blocks (not just the description), so it should re-add the association.
	// But ReorderBlocks never calls syncMentionsForNote.
	var countAfterReorder int64
	tc.DB.Table("groups_related_notes").
		Where("group_id = ? AND note_id = ?", group.ID, note.ID).
		Count(&countAfterReorder)
	assert.Equal(t, int64(1), countAfterReorder,
		"BUG: ReorderBlocks does not call syncMentionsForNote after reordering text blocks. "+
			"Block A still contains @[group:%d:%s] and syncMentionsForNote scans all text blocks, "+
			"but the group-note association was not re-added. CreateBlock, UpdateBlockContent, and "+
			"DeleteBlock all call syncMentionsForNote after their transactions, but ReorderBlocks does not.",
		group.ID, group.Name)
}
