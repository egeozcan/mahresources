package models

import "gorm.io/gorm"

type Album struct {
	gorm.Model
	Name               string `gorm:"index"`
	Meta               string
	Preview            []byte
	PreviewContentType string
	Tags               []*Tag      `gorm:"many2many:album_tags;"`
	Resources          []*Resource `gorm:"many2many:resource_albums;"`
	OwnerId            uint
}
