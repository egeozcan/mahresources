package models

import (
	"mahresources/models/types"
	"time"

	"gorm.io/gorm"
)

type NoteType struct {
	ID          uint      `gorm:"primarykey"`
	CreatedAt   time.Time `gorm:"index"`
	UpdatedAt   time.Time `gorm:"index"`
	GUID        *string   `gorm:"uniqueIndex;size:36" json:"guid,omitempty"`
	Name        string    `gorm:"index"`
	Description string
	Notes       []*Note `gorm:"foreignKey:NoteTypeId;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	// CustomHeader is rendered at the top of the note detail page body, above the description.
	// Shortcodes are processed server-side; an Alpine entity variable is available.
	CustomHeader string `gorm:"type:text"`
	// CustomSidebar is rendered in the note detail page sidebar (both default and wide layouts).
	// Shortcodes are processed server-side; an Alpine entity variable is available.
	CustomSidebar string `gorm:"type:text"`
	// CustomSummary is rendered on note cards in list views, below the title.
	// Shortcodes are processed server-side; an Alpine entity variable is available.
	CustomSummary string `gorm:"type:text"`
	// CustomAvatar replaces the default initials avatar on note cards in list views.
	// Shortcodes are processed server-side; an Alpine entity variable is available.
	CustomAvatar string `gorm:"type:text"`
	// CustomMRQLResult is an HTML+shortcode template for rendering notes of this type
	// in [mrql] query results. Processed entirely server-side; Alpine directives are not
	// initialized in the rendered output.
	CustomMRQLResult string `gorm:"type:text"`
	// MetaSchema defines the JSON Schema for notes of this type
	MetaSchema string `gorm:"type:text"`
	// SectionConfig controls which sections are visible on note detail pages
	SectionConfig types.JSON `gorm:"type:json"`
}

func (n *NoteType) BeforeCreate(tx *gorm.DB) error {
	if n.GUID == nil {
		guid := types.NewUUIDv7()
		n.GUID = &guid
	}
	return nil
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
