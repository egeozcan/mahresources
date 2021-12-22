package models

import (
	"mahresources/models/types"
	"time"
)

type Resource struct {
	ID               uint `gorm:"primarykey"`
	CreatedAt        time.Time
	UpdatedAt        time.Time
	Name             string `gorm:"index"`
	OriginalName     string `gorm:"index"`
	OriginalLocation string `gorm:"index"`
	Hash             string `gorm:"index"`
	HashType         string `gorm:"index"`
	Location         string `gorm:"index"`
	StorageLocation  *string
	Description      string
	Meta             types.JSON
	Width            uint
	Height           uint
	FileSize         int64
	Category         string   `gorm:"index"`
	ContentType      string   `gorm:"index"`
	ContentCategory  string   `gorm:"index"`
	Tags             []*Tag   `gorm:"many2many:resource_tags;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Notes            []*Note  `gorm:"many2many:resource_notes;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Groups           []*Group `gorm:"many2many:groups_related_resources;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Owner            *Group   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	OwnerId          *uint
	Previews         []*Preview `gorm:"foreignKey:ResourceId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}
