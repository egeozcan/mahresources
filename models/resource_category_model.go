package models

import (
	"mahresources/models/types"
	"time"

	"gorm.io/gorm"
)

type ResourceCategory struct {
	ID              uint      `gorm:"primarykey"`
	CreatedAt       time.Time `gorm:"index"`
	UpdatedAt       time.Time `gorm:"index"`
	CreatedByUserId *uint     `gorm:"index" json:"createdByUserId,omitempty"`
	GUID            *string   `gorm:"uniqueIndex;size:36" json:"guid,omitempty"`

	Name        string      `gorm:"uniqueIndex:unique_resource_category_name"`
	Description string      `gorm:"index"`
	Resources   []*Resource `gorm:"foreignKey:ResourceCategoryId;constraint:OnUpdate:CASCADE;"`

	// CustomHeader is rendered at the top of the resource detail page body, above the description.
	// Shortcodes are processed server-side; an Alpine entity variable is available.
	CustomHeader string `gorm:"type:text"`
	// CustomDetailFooter is rendered at the bottom of the resource detail page body, below
	// every built-in section and above the resource_detail_after plugin slot. Unlike that
	// plugin slot it is per-resource. Shortcodes are processed server-side; an Alpine entity
	// variable is available.
	CustomDetailFooter string `gorm:"type:text"`
	// CustomSidebar is rendered in the resource detail page sidebar and lightbox panel.
	// Shortcodes are processed server-side; an Alpine entity variable is available.
	CustomSidebar string `gorm:"type:text"`
	// CustomPreview is rendered in the resource detail sidebar directly above the built-in
	// preview image, for file types the built-in preview cannot show: a PDF or model viewer,
	// an audio waveform, an embed for a link resource. It does not replace the preview image,
	// which SectionConfig's PreviewImage toggle still governs independently. Shortcodes are
	// processed server-side; an Alpine entity variable is available.
	CustomPreview string `gorm:"type:text"`
	// CustomLightbox replaces CustomSidebar in the lightbox details panel. When empty the
	// panel falls back to CustomSidebar, so setting it is only needed when the dark, narrow
	// lightbox needs different markup than the detail page sidebar. Both are expanded
	// server-side before the JSON response is serialized (processShortcodesForJSON).
	CustomLightbox string `gorm:"type:text"`
	// CustomSummary is rendered on resource cards in list views, below the title.
	// Shortcodes are processed server-side; an Alpine entity variable is available.
	CustomSummary string `gorm:"type:text"`
	// CustomAvatar is shown next to the category name on resource cards in list views.
	// Shortcodes are processed server-side; an Alpine entity variable is available.
	CustomAvatar string `gorm:"type:text"`
	// CustomHoverCard replaces CustomSummary in the hover card shown when a resource link is
	// hovered. When empty the hover card falls back to CustomSummary, so setting it is only
	// needed when the hover card should differ from the list card. The hover card is
	// injected via innerHTML and Alpine.initTree runs on it, so entity-scoped directives
	// hydrate. Shortcodes are processed server-side.
	CustomHoverCard string `gorm:"type:text"`
	// CustomCell is rendered as one extra trailing cell per row in the resources details
	// table view (?display=details). The column header reads "Custom" and the column is
	// emitted only when the list is filtered to exactly this one category, so a mixed list
	// keeps the built-in columns. Output should be a bare cell body, not a <td>. Shortcodes
	// are processed server-side; keep it short — it shares a horizontally scrolling table.
	CustomCell string `gorm:"type:text"`
	// CustomListHeader is rendered at the top of resource list pages when the list is
	// filtered to exactly this one category. It is processed with the category itself
	// as the entity: [property path="Name"] yields the category name, [meta] renders its
	// empty state (the category carries no meta), and [mrql] resolves against global scope.
	CustomListHeader string `gorm:"type:text"`
	// CustomListFooter is rendered at the bottom of resource list pages when the list is
	// filtered to exactly this one category, below the results and the pager. Like
	// CustomListHeader it is processed with the category itself as the entity:
	// [property path="Name"] yields the category name, [meta] renders its empty state, and
	// [mrql] resolves against global scope.
	CustomListFooter string `gorm:"type:text"`
	// CustomMRQLResult is an HTML+shortcode template for rendering resources of this category
	// in [mrql] query results. Processed entirely server-side; Alpine directives are not
	// initialized in the rendered output.
	CustomMRQLResult string `gorm:"type:text"`
	// CustomCSS is injected as a page-level <style> block on pages that render this category's
	// templates (resource detail page, resource list pages, and [mrql] result cards that use a
	// CustomMRQLResult template), so the other Custom* slots can be styled globally. Shortcodes are
	// processed server-side; an entity variable is available.
	CustomCSS string `gorm:"type:text"`
	// MetaSchema is a JSON schema for the meta field of resources in this category
	MetaSchema string `gorm:"type:text"`
	// AutoDetectRules is a JSON rule set for auto-detecting this category on upload
	AutoDetectRules string `gorm:"type:text"`
	// SectionConfig is a JSON config controlling which sections are visible on resource detail pages
	SectionConfig types.JSON `json:"sectionConfig"`
}

func (c *ResourceCategory) BeforeCreate(tx *gorm.DB) error {
	if c.GUID == nil {
		guid := types.NewUUIDv7()
		c.GUID = &guid
	}
	return nil
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
