package models

import (
	"mahresources/models/types"
	"time"
)

type Tag struct {
	ID          uint        `gorm:"primarykey"`
	CreatedAt   time.Time   `gorm:"index"`
	UpdatedAt   time.Time   `gorm:"index"`
	Name        string      `gorm:"uniqueIndex:unique_tag_name"`
	Description string      `gorm:"index"`
	Meta        types.JSON
	Resources   []*Resource `gorm:"many2many:resource_tags;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Notes       []*Note     `gorm:"many2many:note_tags;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Groups      []*Group    `gorm:"many2many:group_tags;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
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
