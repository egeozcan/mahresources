package models

import (
	"gorm.io/gorm"
	"strings"
	"unicode"
)

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
	return limit(strings.ToTitleSpecial(unicode.TurkishCase, p.Name+" "+p.Surname), 200)
}

func (p Person) Initials() string {
	res := ""

	if len(p.Name) > 0 {
		r := firstRune(p.Name)
		res = string(r)
	}

	if len(p.Surname) > 0 {
		r := firstRune(p.Surname)
		res += string(r)
	}

	return strings.ToUpper(res)
}

type PersonList []*Person

func (people *PersonList) ToNamedEntities() *[]NamedEntity {
	list := make([]NamedEntity, len(*people))

	for i, v := range *people {
		list[i] = v
	}

	return &list
}

func firstRune(str string) (r rune) {
	for _, r = range str {
		return
	}
	return
}

func limit(str string, maxLen int) string {
	if len(str) < maxLen {
		return str
	}

	res := ""
	lenWithDots := maxLen - 3

	for i, s := range str {
		if i >= lenWithDots {
			return res + "..."
		}

		res += string(s)
	}

	return res
}
