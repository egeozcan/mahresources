package models

import (
	"mahresources/models/types"
	"time"

	"gorm.io/gorm"
)

type Category struct {
	ID        uint      `gorm:"primarykey"`
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time `gorm:"index"`
	GUID      *string   `gorm:"uniqueIndex;size:36" json:"guid,omitempty"`

	Name        string   `gorm:"uniqueIndex:unique_category_name"`
	Description string   `gorm:"index"`
	Groups      []*Group `gorm:"foreignKey:CategoryId;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`

	// CustomHeader is rendered at the top of the group detail page body, above the description.
	// Shortcodes are processed server-side; an Alpine entity variable is available.
	CustomHeader string `gorm:"type:text"`
	// CustomSidebar is rendered in the right sidebar of the group detail page.
	// Shortcodes are processed server-side; an Alpine entity variable is available.
	CustomSidebar string `gorm:"type:text"`
	// CustomSummary is rendered on group cards in list views, below the title.
	// Shortcodes are processed server-side; an Alpine entity variable is available.
	CustomSummary string `gorm:"type:text"`
	// CustomAvatar replaces the default initials avatar on group cards in list views.
	// Shortcodes are processed server-side; an Alpine entity variable is available.
	CustomAvatar string `gorm:"type:text"`
	// CustomMRQLResult is an HTML+shortcode template for rendering groups of this category
	// in [mrql] query results. Processed entirely server-side; Alpine directives are not
	// initialized in the rendered output.
	CustomMRQLResult string `gorm:"type:text"`
	// MetaSchema is a JSON schema for the meta field of groups in this category
	MetaSchema string `gorm:"type:text"`
	// SectionConfig is a JSON config controlling which sections are visible on group detail pages
	SectionConfig types.JSON `json:"sectionConfig"`
}

func (c *Category) BeforeCreate(tx *gorm.DB) error {
	if c.GUID == nil {
		guid := types.NewUUIDv7()
		c.GUID = &guid
	}
	return nil
}

func (c Category) GetId() uint {
	return c.ID
}

func (c Category) GetName() string {
	return c.Name
}

func (c Category) GetDescription() string {
	return c.Description
}
