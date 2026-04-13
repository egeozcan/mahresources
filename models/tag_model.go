package models

import (
	"mahresources/models/types"
	"time"

	"gorm.io/gorm"
)

type Tag struct {
	ID          uint        `gorm:"primarykey"`
	CreatedAt   time.Time   `gorm:"index"`
	UpdatedAt   time.Time   `gorm:"index"`
	GUID        *string     `gorm:"uniqueIndex;size:36" json:"guid,omitempty"`
	Name        string      `gorm:"uniqueIndex:unique_tag_name"`
	Description string      `gorm:"index"`
	Meta        types.JSON
	Resources   []*Resource `gorm:"many2many:resource_tags;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Notes       []*Note     `gorm:"many2many:note_tags;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Groups      []*Group    `gorm:"many2many:group_tags;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

func (t *Tag) BeforeCreate(tx *gorm.DB) error {
	if t.GUID == nil {
		guid := types.NewUUIDv7()
		t.GUID = &guid
	}
	return nil
}

func (t Tag) GetId() uint {
	return t.ID
}

func (t Tag) GetName() string {
	return t.Name
}

func (t Tag) GetDescription() string {
	return t.Description
}
