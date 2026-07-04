package models

import (
	"time"

	"gorm.io/gorm"

	"mahresources/models/types"
)

// TemplatePartial is a reusable, named snippet of template markup (HTML plus
// shortcodes) that category/type templates include via [partial name="…"]. The
// snippet expands with the including entity's context, so its own
// [meta]/[conditional]/[mrql]/[each] shortcodes resolve against that entity.
type TemplatePartial struct {
	ID              uint      `gorm:"primarykey" json:"id"`
	CreatedAt       time.Time `gorm:"index" json:"createdAt"`
	UpdatedAt       time.Time `gorm:"index" json:"updatedAt"`
	CreatedByUserId *uint     `gorm:"index" json:"createdByUserId,omitempty"`
	GUID            *string   `gorm:"uniqueIndex;size:36" json:"guid,omitempty"`
	// Name is a unique kebab-case identifier ([a-z][a-z0-9-]*) referenced by
	// [partial name="…"]. The kebab-case rule keeps references parseable and
	// lintable.
	Name        string `gorm:"uniqueIndex" json:"name"`
	Description string `json:"description"`
	// Content is the HTML + shortcode template body expanded at the include site.
	Content string `gorm:"type:text" json:"content"`
}

func (p *TemplatePartial) BeforeCreate(tx *gorm.DB) error {
	if p.GUID == nil {
		guid := types.NewUUIDv7()
		p.GUID = &guid
	}
	return nil
}

func (p TemplatePartial) GetId() uint            { return p.ID }
func (p TemplatePartial) GetName() string        { return p.Name }
func (p TemplatePartial) GetDescription() string { return p.Description }
