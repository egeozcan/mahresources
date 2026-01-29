package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNoteBlock_TableName(t *testing.T) {
	block := NoteBlock{}
	assert.Equal(t, "note_blocks", block.TableName())
}

func TestNoteBlock_GetType(t *testing.T) {
	block := NoteBlock{Type: "text"}
	assert.Equal(t, "text", block.Type)
}
