package models

import (
	"mahresources/models/types"
	"strings"
	"time"
)

type Group struct {
	ID        uint `gorm:"primarykey"`
	CreatedAt time.Time
	UpdatedAt time.Time

	Name        string `gorm:"index"`
	Description string `gorm:"index"`
	Meta        types.JSON

	Owner   *Group `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	OwnerId *uint

	RelatedResources []*Resource `gorm:"many2many:groups_related_resources;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	RelatedNotes     []*Note     `gorm:"many2many:groups_related_notes;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	RelatedGroups    []*Group    `gorm:"many2many:group_related_groups;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`

	OwnResources []*Resource `gorm:"foreignKey:OwnerId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	OwnNotes     []*Note     `gorm:"foreignKey:OwnerId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	OwnGroups    []*Group    `gorm:"foreignKey:OwnerId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`

	Relationships []*GroupRelation `gorm:"foreignKey:FromGroupId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	BackRelations []*GroupRelation `gorm:"foreignKey:ToGroupId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`

	Tags       []*Tag `gorm:"many2many:group_tags;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	CategoryId *uint
	Category   *Category `gorm:"foreignKey:CategoryId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
}

func (g Group) getId() uint {
	return g.ID
}

func (g Group) GetName() string {
	return limit(g.Name, 200)
}

func (g Group) initials() string {
	res := ""

	if len(g.Name) > 0 {
		r := firstRune(g.Name)
		res = string(r)
	}

	return strings.ToUpper(res)
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
