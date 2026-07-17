package models

import (
	"mahresources/models/types"
	"strings"
	"time"

	"gorm.io/gorm"
)

type Note struct {
	ID              uint      `gorm:"primarykey"`
	CreatedAt       time.Time `gorm:"index"`
	UpdatedAt       time.Time `gorm:"index"`
	CreatedByUserId *uint     `gorm:"index" json:"createdByUserId,omitempty"`
	GUID            *string   `gorm:"uniqueIndex;size:36" json:"guid,omitempty"`
	Name            string    `gorm:"index;index:idx_notes_lower_name,expression:LOWER(name)"`
	Description     string
	Meta            types.JSON
	Tags            []*Tag      `gorm:"many2many:note_tags;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Resources       []*Resource `gorm:"many2many:resource_notes;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Groups          []*Group    `gorm:"many2many:groups_related_notes;constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	Owner           *Group      `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	OwnerId         *uint       `gorm:"index"`
	StartDate       *time.Time  `gorm:"index"`
	EndDate         *time.Time  `gorm:"index"`
	NoteType        *NoteType   `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	NoteTypeId      *uint       `gorm:"index"`
	ShareToken      *string     `gorm:"uniqueIndex;size:32" json:"shareToken,omitempty"`
	// ShareCreatedAt records when the current ShareToken was minted. Nullable:
	// existing rows (minted before BH-035) remain NULL; the /admin/shares UI
	// renders "(unknown)" for them rather than back-filling with an inaccurate
	// NOW() during migration. Set in ShareNote, cleared in UnshareNote.
	ShareCreatedAt *time.Time   `gorm:"index" json:"shareCreatedAt,omitempty"`
	Blocks         []*NoteBlock `gorm:"foreignKey:NoteID" json:"blocks,omitempty"`

	// RenderedHTML is a transient field populated by the API when render=1 is set.
	RenderedHTML string `gorm:"-" json:"renderedHTML,omitempty"`

	// HasShare is a transient, list-only flag populated by the notes-list context
	// provider so cards can signal "this note is shared" without ShareToken being
	// serialized into the HTML (BH-038). It is never persisted.
	HasShare bool `gorm:"-" json:"hasShare,omitempty"`
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

// HasTextBlock reports whether the note has at least one text block. The first
// text block mirrors the note's Description, so the description should be
// rendered directly only when no text block exists. Gating on this (instead of
// "no blocks at all") keeps the description visible for notes that have only
// non-text blocks, while still avoiding double-rendering when a text block owns it.
func (a Note) HasTextBlock() bool {
	for _, b := range a.Blocks {
		if b != nil && b.Type == "text" {
			return true
		}
	}
	return false
}

func (a Note) Initials() string {
	res := ""

	if len(a.Name) > 0 {
		r := firstRune(a.Name)
		res = string(r)
	}

	return strings.ToUpper(res)
}
