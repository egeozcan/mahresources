package models

import (
	"time"
)

type Category struct {
	ID        uint      `gorm:"primarykey"`
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time `gorm:"index"`

	Name        string   `gorm:"uniqueIndex:unique_category_name"`
	Description string   `gorm:"index"`
	Groups      []*Group `gorm:"foreignKey:CategoryId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`

	// CustomHeader is used in the group page
	CustomHeader string `gorm:"type:text"`
	// CustomSidebar is used in the group page
	CustomSidebar string `gorm:"type:text"`
	// CustomSummary is used in the group list page
	CustomSummary string `gorm:"type:text"`
	// CustomAvatar is used when linking to a group with this category
	CustomAvatar string `gorm:"type:text"`
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
