package models

import (
	"mahresources/models/types"
	"time"
)

// NoteBlock represents a content block within a note
type NoteBlock struct {
	ID        uint       `gorm:"primarykey" json:"id"`
	CreatedAt time.Time  `gorm:"index" json:"createdAt"`
	UpdatedAt time.Time  `gorm:"index" json:"updatedAt"`
	NoteID    uint       `gorm:"index;not null" json:"noteId"`
	Note      *Note      `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;" json:"-"`
	Type      string     `gorm:"not null" json:"type"`
	Position  string     `gorm:"not null;index" json:"position"`
	Content   types.JSON `gorm:"not null;default:'{}'" json:"content"`
	State     types.JSON `gorm:"not null;default:'{}'" json:"state"`
}

func (NoteBlock) TableName() string {
	return "note_blocks"
}
