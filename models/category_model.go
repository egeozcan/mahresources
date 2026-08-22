package models

import (
	"mahresources/models/types"
	"time"

	"gorm.io/gorm"
)

type Category struct {
	ID              uint      `gorm:"primarykey"`
	CreatedAt       time.Time `gorm:"index"`
	UpdatedAt       time.Time `gorm:"index"`
	CreatedByUserId *uint     `gorm:"index" json:"createdByUserId,omitempty"`
	GUID            *string   `gorm:"uniqueIndex;size:36" json:"guid,omitempty"`

	Name        string   `gorm:"uniqueIndex:unique_category_name"`
	Description string   `gorm:"index"`
	Groups      []*Group `gorm:"foreignKey:CategoryId;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`

	// CustomHeader is rendered at the top of the group detail page body, above the description.
	// Shortcodes are processed server-side; an Alpine entity variable is available.
	CustomHeader string `gorm:"type:text"`
	// CustomDetailFooter is rendered at the bottom of the group detail page body, below
	// every built-in section and above the group_detail_after plugin slot. Unlike that
	// plugin slot it is per-group. Shortcodes are processed server-side; an Alpine entity
	// variable is available.
	CustomDetailFooter string `gorm:"type:text"`
	// CustomSidebar is rendered in the right sidebar of the group detail page.
	// Shortcodes are processed server-side; an Alpine entity variable is available.
	CustomSidebar string `gorm:"type:text"`
	// CustomSummary is rendered on group cards in list views, below the title.
	// Shortcodes are processed server-side; an Alpine entity variable is available.
	CustomSummary string `gorm:"type:text"`
	// CustomAvatar replaces the default initials avatar on group cards in list views.
	// Shortcodes are processed server-side; an Alpine entity variable is available.
	CustomAvatar string `gorm:"type:text"`
	// CustomHoverCard replaces CustomSummary in the hover card shown when a group link is
	// hovered. When empty the hover card falls back to CustomSummary, so setting it is only
	// needed when the hover card should differ from the list card. The hover card is
	// injected via innerHTML and Alpine.initTree runs on it, so entity-scoped directives
	// hydrate. Shortcodes are processed server-side.
	CustomHoverCard string `gorm:"type:text"`
	// CustomOwnEntities replaces the body of the "Own Entities" section on the group detail
	// page, which otherwise lists owned notes, sub-groups and resources as card grids. When
	// empty the built-in body renders. The section's own visibility is still governed by
	// SectionConfig: setting this while OwnEntities is "off" renders nothing. Shortcodes are
	// processed server-side; an Alpine entity variable is available.
	CustomOwnEntities string `gorm:"type:text"`
	// CustomListHeader is rendered at the top of group list pages when the list is
	// filtered to exactly this one category. It is processed with the category itself
	// as the entity: [property path="Name"] yields the category name, [meta] renders its
	// empty state (the category carries no meta), and [mrql] resolves against global scope.
	CustomListHeader string `gorm:"type:text"`
	// CustomListFooter is rendered at the bottom of group list pages when the list is
	// filtered to exactly this one category, below the results and the pager. Like
	// CustomListHeader it is processed with the category itself as the entity:
	// [property path="Name"] yields the category name, [meta] renders its empty state, and
	// [mrql] resolves against global scope.
	CustomListFooter string `gorm:"type:text"`
	// CustomMRQLResult is an HTML+shortcode template for rendering groups of this category
	// in [mrql] query results. Processed entirely server-side; Alpine directives are not
	// initialized in the rendered output.
	CustomMRQLResult string `gorm:"type:text"`
	// CustomCSS is injected as a page-level <style> block on pages that render this category's
	// templates (group detail page, group list pages, and [mrql] result cards that use a
	// CustomMRQLResult template), so the other Custom* slots can be styled globally. Shortcodes are
	// processed server-side; an entity variable is available.
	CustomCSS string `gorm:"type:text"`
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
