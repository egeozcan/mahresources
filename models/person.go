package models

import "gorm.io/gorm"

type Person struct {
	gorm.Model
	Name             string      `gorm:"index"`
	RelatedResources []*Resource `gorm:"many2many:people_related_resources;"`
	OwnResources     []Resource  `gorm:"foreignKey:OwnerId"`
	OwnAlbums        []Album     `gorm:"foreignKey:OwnerId"`
}
