package api_tests

import (
	"fmt"
	"mahresources/models"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEditDescriptionDoesNotSyncMentions demonstrates that the
// /v1/note/editDescription endpoint does not trigger mention syncing.
//
// When a user edits a note's description via the inline edit endpoint and
// includes an @-mention (e.g., @[tag:1:MyTag]), the referenced entity should
// be added as a relation to the note. The full note editor (CreateOrUpdateNote)
// calls syncMentionsForNote after saving, but the generic UpdateDescription
// (basic_entity_context.go) does not — so mentions added via inline editing
// are silently ignored.
//
// Steps to reproduce:
//  1. Create a tag and a note with no associations between them.
//  2. Edit the note's description via /v1/note/editDescription, including an
//     @-mention of the tag in the new description text.
//  3. Verify the note-tag association was created.
//
// Expected: The tag mention is synced and a note_tags row is created.
// Actual:   No note_tags row is created — the mention is ignored.
func TestEditDescriptionDoesNotSyncMentions(t *testing.T) {
	tc := SetupTestEnv(t)

	// Step 1: Create a tag and a note with no association
	tag := &models.Tag{Name: "MentionedTag", Meta: []byte(`{}`)}
	require.NoError(t, tc.DB.Create(tag).Error)

	note := tc.CreateDummyNote("Mention Sync Note")

	// Verify no tag association exists initially
	var countBefore int64
	tc.DB.Table("note_tags").Where("note_id = ? AND tag_id = ?", note.ID, tag.ID).Count(&countBefore)
	require.Equal(t, int64(0), countBefore, "setup: note should have no tag association initially")

	// Step 2: Edit description via inline edit endpoint with an @-mention
	newDesc := fmt.Sprintf("This note references @[tag:%d:%s] in the description", tag.ID, tag.Name)
	editURL := fmt.Sprintf("/v1/note/editDescription?id=%d", note.ID)
	resp := tc.MakeRequest(http.MethodPost, editURL, map[string]string{
		"Description": newDesc,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	// Verify the description was actually updated
	var updatedNote models.Note
	require.NoError(t, tc.DB.First(&updatedNote, note.ID).Error)
	require.Equal(t, newDesc, updatedNote.Description,
		"setup: editDescription should have updated the description column")

	// Step 3: Check if the mention was synced as a tag association
	var countAfter int64
	tc.DB.Table("note_tags").Where("note_id = ? AND tag_id = ?", note.ID, tag.ID).Count(&countAfter)
	assert.Equal(t, int64(1), countAfter,
		"BUG: editDescription does not trigger syncMentionsForNote — the @[tag:%d:%s] mention "+
			"in the description was not synced as a note-tag association. The full note editor "+
			"(CreateOrUpdateNote) calls syncMentionsForNote, but UpdateDescription in "+
			"basic_entity_context.go does not.",
		tag.ID, tag.Name)
}

// TestEditDescriptionDoesNotSyncMentionsForGroups verifies the same bug
// affects groups: editing a group's description via /v1/group/editDescription
// with an @-mention does not sync the mention as a relation.
func TestEditDescriptionDoesNotSyncMentionsForGroups(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a tag and a group
	tag := &models.Tag{Name: "GroupMentionTag", Meta: []byte(`{}`)}
	require.NoError(t, tc.DB.Create(tag).Error)

	group := tc.CreateDummyGroup("Mention Group")

	// Verify no tag association exists initially
	var countBefore int64
	tc.DB.Table("group_tags").Where("group_id = ? AND tag_id = ?", group.ID, tag.ID).Count(&countBefore)
	require.Equal(t, int64(0), countBefore, "setup: group should have no tag association initially")

	// Edit description via inline edit endpoint with @-mention
	newDesc := fmt.Sprintf("Group references @[tag:%d:%s] here", tag.ID, tag.Name)
	editURL := fmt.Sprintf("/v1/group/editDescription?id=%d", group.ID)
	resp := tc.MakeRequest(http.MethodPost, editURL, map[string]string{
		"Description": newDesc,
	})
	assert.Equal(t, http.StatusOK, resp.Code)

	// Verify the description was updated
	var updatedGroup models.Group
	require.NoError(t, tc.DB.First(&updatedGroup, group.ID).Error)
	require.Equal(t, newDesc, updatedGroup.Description,
		"setup: editDescription should have updated the description column")

	// Check if the mention was synced
	var countAfter int64
	tc.DB.Table("group_tags").Where("group_id = ? AND tag_id = ?", group.ID, tag.ID).Count(&countAfter)
	assert.Equal(t, int64(1), countAfter,
		"BUG: editDescription for groups does not trigger syncMentionsForGroup — the tag mention was not synced")
}
