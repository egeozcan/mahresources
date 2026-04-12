package models

import (
	"mahresources/models/types"
	"time"
)

type ResourceCategory struct {
	ID        uint      `gorm:"primarykey"`
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time `gorm:"index"`

	Name        string      `gorm:"uniqueIndex:unique_resource_category_name"`
	Description string      `gorm:"index"`
	Resources   []*Resource `gorm:"foreignKey:ResourceCategoryId;constraint:OnUpdate:CASCADE;"`

	// CustomHeader is rendered at the top of the resource detail page body, above the description.
	// Shortcodes are processed server-side; an Alpine entity variable is available.
	CustomHeader string `gorm:"type:text"`
	// CustomSidebar is rendered in the resource detail page sidebar and lightbox panel.
	// Shortcodes are processed server-side; an Alpine entity variable is available.
	CustomSidebar string `gorm:"type:text"`
	// CustomSummary is rendered on resource cards in list views, below the title.
	// Shortcodes are processed server-side; an Alpine entity variable is available.
	CustomSummary string `gorm:"type:text"`
	// CustomAvatar is shown next to the category name on resource cards in list views.
	// Shortcodes are processed server-side; an Alpine entity variable is available.
	CustomAvatar string `gorm:"type:text"`
	// CustomMRQLResult is an HTML+shortcode template for rendering resources of this category
	// in [mrql] query results. Processed entirely server-side; Alpine directives are not
	// initialized in the rendered output.
	CustomMRQLResult string `gorm:"type:text"`
	// MetaSchema is a JSON schema for the meta field of resources in this category
	MetaSchema string `gorm:"type:text"`
	// AutoDetectRules is a JSON rule set for auto-detecting this category on upload
	AutoDetectRules string `gorm:"type:text"`
	// SectionConfig is a JSON config controlling which sections are visible on resource detail pages
	SectionConfig types.JSON `json:"sectionConfig"`
}

func (c ResourceCategory) GetId() uint {
	return c.ID
}

func (c ResourceCategory) GetName() string {
	return c.Name
}

func (c ResourceCategory) GetDescription() string {
	return c.Description
}
