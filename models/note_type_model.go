package models

import (
	"mahresources/models/types"
	"time"

	"gorm.io/gorm"
)

type NoteType struct {
	ID              uint      `gorm:"primarykey"`
	CreatedAt       time.Time `gorm:"index"`
	UpdatedAt       time.Time `gorm:"index"`
	CreatedByUserId *uint     `gorm:"index" json:"createdByUserId,omitempty"`
	GUID            *string   `gorm:"uniqueIndex;size:36" json:"guid,omitempty"`
	Name            string    `gorm:"index"`
	Description     string
	Notes           []*Note `gorm:"foreignKey:NoteTypeId;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
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
	// CustomListHeader is rendered at the top of note list pages when the list is
	// filtered to exactly this one note type. It is processed with the note type itself
	// as the entity: [property path="Name"] yields the type name, [meta] renders its
	// empty state (the type carries no meta), and [mrql] resolves against global scope.
	CustomListHeader string `gorm:"type:text"`
	// ApplyTemplatesToShares opts this note type's CustomHeader and CustomCSS into the
	// public /s/<token> share page. Default false: existing shares keep their appearance
	// until an author explicitly enables it. On share pages templates run in a restricted
	// mode — no [mrql] queries, no plugin shortcodes, read-only [meta] (see share_server.go).
	ApplyTemplatesToShares bool `gorm:"not null;default:false"`
	// CustomMRQLResult is an HTML+shortcode template for rendering notes of this type
	// in [mrql] query results. Processed entirely server-side; Alpine directives are not
	// initialized in the rendered output.
	CustomMRQLResult string `gorm:"type:text"`
	// CustomCSS is injected as a page-level <style> block on pages that render this note type's
	// templates (note detail page, note list pages, and [mrql] result cards that use a
	// CustomMRQLResult template), so the other Custom* slots can be styled globally. Shortcodes are
	// processed server-side; an entity variable is available.
	CustomCSS string `gorm:"type:text"`
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
