package models

import "gorm.io/gorm"

type Album struct {
	gorm.Model
	Name               string `gorm:"index"`
	Description        string
	Meta               string
	Preview            []byte
	PreviewContentType string
	Tags               []*Tag      `gorm:"many2many:album_tags;"`
	Resources          []*Resource `gorm:"many2many:resource_albums;"`
	People             []*Person   `gorm:"many2many:people_related_albums;"`
	OwnerId            uint
}
