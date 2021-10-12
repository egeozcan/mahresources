package models

import "gorm.io/gorm"

type Tag struct {
	gorm.Model
	Name      string      `gorm:"index"`
	Resources []*Resource `gorm:"many2many:resource_tags;"`
	Notes     []*Note     `gorm:"many2many:note_tags;"`
	Groups    []*Group    `gorm:"many2many:group_tags;"`
}

func (t Tag) GetId() uint {
	return t.ID
}

func (t Tag) GetName() string {
	return t.Name
}

type TagList []*Tag

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
