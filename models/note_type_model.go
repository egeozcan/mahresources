package models

import (
	"time"
)

type NoteType struct {
	ID          uint      `gorm:"primarykey"`
	CreatedAt   time.Time `gorm:"index"`
	UpdatedAt   time.Time `gorm:"index"`
	Name        string    `gorm:"index"`
	Description string
	Notes       []*Note `gorm:"foreignKey:NoteTypeId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

func (a NoteType) GetId() uint {
	return a.ID
}

func (a NoteType) GetName() string {
	return a.Name
}

func (a NoteType) GetDescription() string {
	return a.Description
}
