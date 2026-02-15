package models

import (
	"mahresources/models/types"
	"time"
)

type Series struct {
	ID        uint       `gorm:"primarykey"`
	CreatedAt time.Time  `gorm:"index"`
	UpdatedAt time.Time  `gorm:"index"`
	Name      string     `gorm:"index"`
	Slug      string     `gorm:"uniqueIndex"`
	Meta      types.JSON
	Resources []*Resource `gorm:"foreignKey:SeriesID;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
}

func (s Series) GetId() uint {
	return s.ID
}

func (s Series) GetName() string {
	return s.Name
}

func (s Series) GetDescription() string {
	return ""
}
