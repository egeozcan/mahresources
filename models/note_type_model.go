package models

import (
	"time"
)

type NoteType struct {
	ID        uint      `gorm:"primarykey"`
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time `gorm:"index"`
	Name      string    `gorm:"index"`
	Notes     []*Note   `gorm:"foreignKey:NoteTypeId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}
