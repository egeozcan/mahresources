package models

import "gorm.io/gorm"

type Tag struct {
	gorm.Model
	Name      string      `gorm:"index"`
	Resources []*Resource `gorm:"many2many:resource_tags;"`
	Albums    []*Album    `gorm:"many2many:album_tags;"`
}
