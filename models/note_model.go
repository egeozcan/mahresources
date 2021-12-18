package models

import (
	"mahresources/models/types"
	"time"
)

type Note struct {
	ID          uint `gorm:"primarykey"`
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Name        string `gorm:"index"`
	Description string
	Meta        types.JSON
	Tags        []*Tag      `gorm:"many2many:note_tags;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Resources   []*Resource `gorm:"many2many:resource_notes;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Groups      []*Group    `gorm:"many2many:groups_related_notes;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Owner       *Group      `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	OwnerId     *uint
	StartDate   *time.Time
	EndDate     *time.Time
	NoteType    *NoteType `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	NoteTypeId  *uint
}

func (a Note) getId() uint {
	return a.ID
}

func (a Note) GetName() string {
	return a.Name
}
