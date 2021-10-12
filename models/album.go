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
	Groups             []*Group    `gorm:"many2many:groups_related_albums;"`
	OwnerId            uint
}

func (a Album) GetId() uint {
	return a.ID
}

func (a Album) GetName() string {
	return a.Name
}

type AlbumList []*Album

func (albums *AlbumList) ToNamedEntities() *[]NamedEntity {
	list := make([]NamedEntity, len(*albums))

	for i, v := range *albums {
		list[i] = v
	}

	return &list
}
