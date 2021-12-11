package models

import (
	"time"
)

type NoteType struct {
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	Name      string  `gorm:"index"`
	Notes     []*Note `gorm:"foreignKey:NoteTypeId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}
