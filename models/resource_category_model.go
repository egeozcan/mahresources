package models

import (
	"time"
)

type ResourceCategory struct {
	ID        uint      `gorm:"primarykey"`
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time `gorm:"index"`

	Name        string      `gorm:"uniqueIndex:unique_resource_category_name"`
	Description string      `gorm:"index"`
	Resources   []*Resource `gorm:"foreignKey:ResourceCategoryId;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`

	// CustomHeader is used in the resource category page
	CustomHeader string `gorm:"type:text"`
	// CustomSidebar is used in the resource category page
	CustomSidebar string `gorm:"type:text"`
	// CustomSummary is used in the resource category list page
	CustomSummary string `gorm:"type:text"`
	// CustomAvatar is used when linking to resources with this category
	CustomAvatar string `gorm:"type:text"`
	// MetaSchema is a JSON schema for the meta field of resources in this category
	MetaSchema string `gorm:"type:text"`
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
