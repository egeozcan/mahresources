package models

import (
	"time"
)

type NoteType struct {
	ID          uint      `gorm:"primarykey"`
	CreatedAt   time.Time `gorm:"index"`
	UpdatedAt   time.Time `gorm:"index"`
	Name        string    `gorm:"index"`
	Description string
	Notes       []*Note `gorm:"foreignKey:NoteTypeId;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	// CustomHeader is used in the note page
	CustomHeader string `gorm:"type:text"`
	// CustomSidebar is used in the note page
	CustomSidebar string `gorm:"type:text"`
	// CustomSummary is used in the note list page
	CustomSummary string `gorm:"type:text"`
	// CustomAvatar is used when linking to a note with this type
	CustomAvatar string `gorm:"type:text"`
}

func (a NoteType) GetId() uint {
	return a.ID
}

func (a NoteType) GetName() string {
	return a.Name
}

func (a NoteType) GetDescription() string {
	return a.Description
}
