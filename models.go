package main

import "gorm.io/gorm"

type Resource struct {
	gorm.Model
	Name string  `gorm:"index"`
	Hash string  `gorm:"index"`
	HashType string  `gorm:"index"`
	Location string
	Description string
	Meta string
	Width uint
	Height uint
	FileSize int64
	Category string  `gorm:"index"`
	ContentType string  `gorm:"index"`
	ContentCategory string  `gorm:"index"`
	Preview []byte
	PreviewContentType string
	Tags []*Tag      `gorm:"many2many:resource_tags;"`
	Albums []*Album  `gorm:"many2many:resource_albums;"`
	People []*Person `gorm:"many2many:people_related_resources;"`
	OwnerId uint
}

type Album struct {
	gorm.Model
	Name string  `gorm:"index"`
	Meta string
	Preview []byte
	PreviewContentType string
	Tags []*Tag `gorm:"many2many:album_tags;"`
	Resources []*Resource `gorm:"many2many:resource_albums;"`
	OwnerId uint
}

type Tag struct {
	gorm.Model
	Name string  `gorm:"index"`
	Resources []*Resource `gorm:"many2many:resource_tags;"`
	Albums []*Album `gorm:"many2many:album_tags;"`
}

type Person struct {
	gorm.Model
	Name string  `gorm:"index"`
	RelatedResources []*Resource `gorm:"many2many:people_related_resources;"`
	OwnResources []Resource `gorm:"foreignKey:OwnerId"`
	OwnAlbums []Album `gorm:"foreignKey:OwnerId"`
}
