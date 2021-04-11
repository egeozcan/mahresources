package models

import "gorm.io/gorm"

type Person struct {
	gorm.Model
	Name             string      `gorm:"index"`
	Surname          string      `gorm:"index"`
	Description      string      `gorm:"index"`
	RelatedResources []*Resource `gorm:"many2many:people_related_resources;"`
	RelatedAlbums    []*Album    `gorm:"many2many:people_related_albums;"`
	OwnResources     []Resource  `gorm:"foreignKey:OwnerId"`
	OwnAlbums        []Album     `gorm:"foreignKey:OwnerId"`
	Tags             []*Tag      `gorm:"many2many:person_tags;"`
}

func (p Person) GetId() uint {
	return p.ID
}

func (p Person) GetName() string {
	return p.Name + " " + p.Surname
}

type PersonList []Person

func (people *PersonList) ToNamedEntities() *[]NamedEntity {
	list := make([]NamedEntity, len(*people))

	for i, v := range *people {
		list[i] = v
	}

	return &list
}
