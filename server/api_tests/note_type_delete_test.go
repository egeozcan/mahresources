package api_tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"mahresources/models"
)

func TestDeleteNoteTypeDoesNotDeleteNotes(t *testing.T) {
	tc := SetupTestEnv(t)

	// Create a note type
	noteType := &models.NoteType{Name: "Meeting Notes"}
	assert.NoError(t, tc.DB.Create(noteType).Error)

	// Create a note with that note type
	note := &models.Note{
		Name:       "Standup 2024-01-15",
		NoteTypeId: &noteType.ID,
	}
	assert.NoError(t, tc.DB.Create(note).Error)
	noteID := note.ID

	// Verify the note exists and has the note type
	var check models.Note
	assert.NoError(t, tc.DB.First(&check, noteID).Error)
	assert.NotNil(t, check.NoteTypeId)
	assert.Equal(t, noteType.ID, *check.NoteTypeId)

	// Delete the note type
	err := tc.AppCtx.DeleteNoteType(noteType.ID)
	assert.NoError(t, err)

	// The note should still exist with NoteTypeId set to NULL (SET NULL),
	// NOT be cascade-deleted
	var afterDelete models.Note
	err = tc.DB.First(&afterDelete, noteID).Error
	assert.NoError(t, err,
		"note should still exist after its NoteType is deleted — must be SET NULL, not CASCADE")
	assert.Nil(t, afterDelete.NoteTypeId,
		"NoteTypeId should be set to NULL after note type deletion")
}
