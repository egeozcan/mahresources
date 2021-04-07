package models

import "gorm.io/gorm"

type Person struct {
	gorm.Model
	Name             string      `gorm:"index"`
	RelatedResources []*Resource `gorm:"many2many:people_related_resources;"`
	RelatedAlbums    []*Album    `gorm:"many2many:people_related_albums;"`
	OwnResources     []Resource  `gorm:"foreignKey:OwnerId"`
	OwnAlbums        []Album     `gorm:"foreignKey:OwnerId"`
}
