package models

import "gorm.io/gorm"

type Note struct {
	gorm.Model
	Name        string `gorm:"index"`
	Description string
	Meta        string
	Tags        []*Tag      `gorm:"many2many:note_tags;"`
	Resources   []*Resource `gorm:"many2many:resource_notes;"`
	Groups      []*Group    `gorm:"many2many:groups_related_notes;"`
	OwnerId     uint
}

func (a Note) GetId() uint {
	return a.ID
}

func (a Note) GetName() string {
	return a.Name
}

type NoteList []*Note

func (notes *NoteList) ToNamedEntities() *[]NamedEntity {
	list := make([]NamedEntity, len(*notes))

	for i, v := range *notes {
		list[i] = v
	}

	return &list
}
