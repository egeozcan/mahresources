package models

import (
	"mahresources/models/types"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
)

type Group struct {
	ID        uint      `gorm:"primarykey"`
	CreatedAt time.Time `gorm:"index"`
	UpdatedAt time.Time `gorm:"index"`
	GUID      *string   `gorm:"uniqueIndex;size:36" json:"guid,omitempty"`

	Name        string `gorm:"index"`
	Description string
	URL         *types.URL `gorm:"index"`

	Meta types.JSON

	Owner   *Group `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	OwnerId *uint  `gorm:"index"`

	RelatedResources []*Resource `gorm:"many2many:groups_related_resources;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	RelatedNotes     []*Note     `gorm:"many2many:groups_related_notes;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	RelatedGroups    []*Group    `gorm:"many2many:group_related_groups;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`

	OwnResources []*Resource `gorm:"foreignKey:OwnerId;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	OwnNotes     []*Note     `gorm:"foreignKey:OwnerId;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	OwnGroups    []*Group    `gorm:"foreignKey:OwnerId;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`

	Relationships []*GroupRelation `gorm:"foreignKey:FromGroupId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`
	BackRelations []*GroupRelation `gorm:"foreignKey:ToGroupId;constraint:OnUpdate:CASCADE,OnDelete:CASCADE"`

	Tags       []*Tag `gorm:"many2many:group_tags;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	CategoryId *uint
	Category   *Category `gorm:"foreignKey:CategoryId;constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`

	// RenderedHTML is a transient field populated by the API when render=1 is set.
	RenderedHTML string `gorm:"-" json:"renderedHTML,omitempty"`
}

func (g *Group) BeforeCreate(tx *gorm.DB) error {
	if g.GUID == nil {
		guid := types.NewUUIDv7()
		g.GUID = &guid
	}
	return nil
}

func (g Group) GetId() uint {
	return g.ID
}

func (g Group) GetName() string {
	return limit(g.Name, 200)
}

func (g Group) GetDescription() string { return g.Description }

func limit(str string, maxLen int) string {
	runeCount := utf8.RuneCountInString(str)
	if runeCount <= maxLen {
		return str
	}

	res := ""
	lenWithDots := maxLen - 3
	runeIdx := 0

	for _, s := range str {
		if runeIdx >= lenWithDots {
			return res + "..."
		}

		res += string(s)
		runeIdx++
	}

	return res
}
