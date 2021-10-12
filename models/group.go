package models

import (
	"gorm.io/gorm"
	"strings"
	"unicode"
)

type Group struct {
	gorm.Model
	Name             string      `gorm:"index"`
	Description      string      `gorm:"index"`
	RelatedResources []*Resource `gorm:"many2many:groups_related_resources;"`
	RelatedNotes     []*Note     `gorm:"many2many:groups_related_notes;"`
	OwnResources     []Resource  `gorm:"foreignKey:OwnerId"`
	OwnNotes         []Note      `gorm:"foreignKey:OwnerId"`
	Tags             []*Tag      `gorm:"many2many:group_tags;"`
}

func (p Group) GetId() uint {
	return p.ID
}

func (p Group) GetName() string {
	return limit(strings.ToTitleSpecial(unicode.TurkishCase, p.Name), 200)
}

func (p Group) Initials() string {
	res := ""

	if len(p.Name) > 0 {
		r := firstRune(p.Name)
		res = string(r)
	}

	return strings.ToUpper(res)
}

type GroupList []*Group

func (groups *GroupList) ToNamedEntities() *[]NamedEntity {
	list := make([]NamedEntity, len(*groups))

	for i, v := range *groups {
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
