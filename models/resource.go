package models

import "gorm.io/gorm"

type Resource struct {
	gorm.Model
	Name               string `gorm:"index"`
	Hash               string `gorm:"index"`
	HashType           string `gorm:"index"`
	Location           string
	Description        string
	Meta               string
	Width              uint
	Height             uint
	FileSize           int64
	Category           string `gorm:"index"`
	ContentType        string `gorm:"index"`
	ContentCategory    string `gorm:"index"`
	Preview            []byte
	PreviewContentType string
	Tags               []*Tag    `gorm:"many2many:resource_tags;"`
	Albums             []*Album  `gorm:"many2many:resource_albums;"`
	People             []*Person `gorm:"many2many:people_related_resources;"`
	OwnerId            uint
}
