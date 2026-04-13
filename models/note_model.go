package models

import (
	"mahresources/models/types"
	"strings"
	"time"

	"gorm.io/gorm"
)

type Note struct {
	ID          uint      `gorm:"primarykey"`
	CreatedAt   time.Time `gorm:"index"`
	UpdatedAt   time.Time `gorm:"index"`
	GUID        *string   `gorm:"uniqueIndex;size:36" json:"guid,omitempty"`
	Name        string    `gorm:"index"`
	Description string
	Meta        types.JSON
	Tags        []*Tag      `gorm:"many2many:note_tags;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Resources   []*Resource `gorm:"many2many:resource_notes;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Groups      []*Group    `gorm:"many2many:groups_related_notes;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Owner       *Group      `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	OwnerId     *uint
	StartDate   *time.Time
	EndDate     *time.Time
	NoteType    *NoteType `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	NoteTypeId  *uint
	ShareToken  *string      `gorm:"uniqueIndex;size:32" json:"shareToken,omitempty"`
	Blocks      []*NoteBlock `gorm:"foreignKey:NoteID" json:"blocks,omitempty"`

	// RenderedHTML is a transient field populated by the API when render=1 is set.
	RenderedHTML string `gorm:"-" json:"renderedHTML,omitempty"`
}

func (n *Note) BeforeCreate(tx *gorm.DB) error {
	if n.GUID == nil {
		guid := types.NewUUIDv7()
		n.GUID = &guid
	}
	return nil
}

func (a Note) GetId() uint {
	return a.ID
}

func (a Note) GetName() string {
	return a.Name
}

func (a Note) GetDescription() string {
	return a.Description
}

func (a Note) Initials() string {
	res := ""

	if len(a.Name) > 0 {
		r := firstRune(a.Name)
		res = string(r)
	}

	return strings.ToUpper(res)
}
