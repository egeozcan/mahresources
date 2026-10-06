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

// TestDeleteBlockDoesNotResyncMentions demonstrates that deleting a text block
// does not trigger syncMentionsForNote, even though CreateBlock and
// UpdateBlockContent both do.
//
// When a text block is deleted, syncFirstTextBlockToDescriptionTx correctly
// updates the note's description to reflect the next text block. However,
// syncMentionsForNote is never called after the deletion. This means that if
// a mention association was removed and the remaining text blocks still contain
// that mention, the association will NOT be re-added — unlike what would happen
// after a CreateBlock or UpdateBlockContent.
//
// Scenario:
//  1. Create a note with two text blocks: A (first, no mentions) and B (second,
//     mentions a group via @[group:ID:Name]).
//  2. On creation of B, syncMentionsForNote fires and adds the group association.
//  3. Manually remove the group-note association.
//  4. Delete block A — block B becomes the first text block and its content is
//     synced to the note's description.
//  5. syncMentionsForNote should re-scan all text (description + blocks) and
//     re-add the group association. But DeleteBlock never calls it.
//
// Expected: After deleting block A, the group association is re-established
//
//	because block B still contains the mention.
//
// Actual:   The group association remains absent because DeleteBlock does not
//
//	call syncMentionsForNote.
func TestDeleteBlockDoesNotResyncMentions(t *testing.T) {
	tc := SetupTestEnv(t)

	// Step 1: Create a group and a note
	group := &models.Group{Name: "MentionedGroup", Meta: []byte(`{}`)}
	require.NoError(t, tc.DB.Create(group).Error)

	note := tc.CreateDummyNote("DeleteBlock Mention Test")

	// Step 2: Create text block A (first by position) — no mentions
	respA := tc.MakeRequest(http.MethodPost, "/v1/note/block", map[string]any{
		"noteId":   note.ID,
		"type":     "text",
		"position": "d",
		"content":  map[string]string{"text": "Plain text, no mentions"},
	})
	require.Equal(t, http.StatusCreated, respA.Code)
	var blockA models.NoteBlock
	require.NoError(t, json.Unmarshal(respA.Body.Bytes(), &blockA))

	// Step 3: Create text block B (second by position) — mentions the group
	mentionText := fmt.Sprintf("See @[group:%d:%s] for more", group.ID, group.Name)
	respB := tc.MakeRequest(http.MethodPost, "/v1/note/block", map[string]any{
		"noteId":   note.ID,
		"type":     "text",
		"position": "h",
		"content":  map[string]string{"text": mentionText},
	})
	require.Equal(t, http.StatusCreated, respB.Code)

	// Verify the group association was created by block B's mention sync
	var countAfterCreate int64
	tc.DB.Table("groups_related_notes").
		Where("group_id = ? AND note_id = ?", group.ID, note.ID).
		Count(&countAfterCreate)
	require.Equal(t, int64(1), countAfterCreate,
		"setup: creating block B with a mention should have synced the group association")

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

	// Step 5: Delete block A — block B becomes the first text block,
	// its content gets synced to the note's description.
	delURL := fmt.Sprintf("/v1/note/block?id=%d", blockA.ID)
	delResp := tc.MakeRequest(http.MethodDelete, delURL, nil)
	require.Equal(t, http.StatusNoContent, delResp.Code)

	// Verify the description was updated to block B's content
	var updatedNote models.Note
	require.NoError(t, tc.DB.First(&updatedNote, note.ID).Error)
	require.Equal(t, mentionText, updatedNote.Description,
		"setup: after deleting block A, the description should be synced from block B")

	// Step 6: Check that the group association was re-added by mention sync.
	// CreateBlock and UpdateBlockContent both call syncMentionsForNote after
	// their transactions, so the mention in block B would be re-synced.
	// DeleteBlock does NOT call syncMentionsForNote, so the association stays removed.
	var countAfterDelete int64
	tc.DB.Table("groups_related_notes").
		Where("group_id = ? AND note_id = ?", group.ID, note.ID).
		Count(&countAfterDelete)
	assert.Equal(t, int64(1), countAfterDelete,
		"BUG: DeleteBlock does not call syncMentionsForNote after deleting a text block. "+
			"Block B still contains @[group:%d:%s] and the description was updated to include it, "+
			"but the group-note association was not re-added. Both CreateBlock and UpdateBlockContent "+
			"call syncMentionsForNote after their transactions, but DeleteBlock does not.",
		group.ID, group.Name)
}
