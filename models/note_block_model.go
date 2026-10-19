package models

import (
	"encoding/json"
	"mahresources/models/types"
	"time"
)

// NoteBlock represents a content block within a note
type NoteBlock struct {
	ID              uint       `gorm:"primarykey" json:"id"`
	CreatedAt       time.Time  `gorm:"index" json:"createdAt"`
	UpdatedAt       time.Time  `gorm:"index" json:"updatedAt"`
	CreatedByUserId *uint      `gorm:"index" json:"createdByUserId,omitempty"`
	NoteID          uint       `gorm:"index;index:idx_note_type_position,priority:1;index:idx_note_position,priority:1;not null" json:"noteId"`
	Note            *Note      `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	Type            string     `gorm:"index:idx_note_type_position,priority:2;not null" json:"type"`
	Position        string     `gorm:"index:idx_note_type_position,priority:3;index:idx_note_position,priority:2;size:64;not null" json:"position"`
	Content         types.JSON `gorm:"not null;default:'{}'" json:"content"`
	State           types.JSON `gorm:"not null;default:'{}'" json:"state"`
}

// TextContent is a text block's markdown: Content's "text" field, or "" when
// the block has none (any other block type, or content that is not an object).
func (b NoteBlock) TextContent() string {
	var content struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(b.Content, &content); err != nil {
		return ""
	}
	return content.Text
}

func (NoteBlock) TableName() string {
	return "note_blocks"
}
