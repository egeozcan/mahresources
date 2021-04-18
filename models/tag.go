package models

import "gorm.io/gorm"

type Tag struct {
	gorm.Model
	Name      string      `gorm:"index"`
	Resources []*Resource `gorm:"many2many:resource_tags;"`
	Albums    []*Album    `gorm:"many2many:album_tags;"`
	People    []*Person   `gorm:"many2many:person_tags;"`
}

func (t Tag) GetId() uint {
	return t.ID
}

func (t Tag) GetName() string {
	return t.Name
}

type TagList []Tag

func (tags *TagList) ToNamedEntities() *[]NamedEntity {
	list := make([]NamedEntity, len(*tags))

	for i, v := range *tags {
		list[i] = v
	}

	return &list
}

func (tags *TagList) GetIds() *[]uint {
	list := make([]uint, len(*tags))

	for i, v := range *tags {
		list[i] = v.ID
	}

	return &list
}
