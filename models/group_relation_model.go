package models

import (
	"mahresources/models/types"
	"time"

	"gorm.io/gorm"
)

type GroupRelationType struct {
	ID             uint      `gorm:"primarykey"`
	CreatedAt      time.Time `gorm:"index"`
	UpdatedAt      time.Time `gorm:"index"`
	GUID           *string   `gorm:"uniqueIndex;size:36" json:"guid,omitempty"`
	Name           string    `gorm:"uniqueIndex:unique_rel_type"`
	Description    string
	FromCategory   *Category          `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	FromCategoryId *uint              `gorm:"uniqueIndex:unique_rel_type"`
	ToCategory     *Category          `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	ToCategoryId   *uint              `gorm:"uniqueIndex:unique_rel_type"`
	BackRelation   *GroupRelationType `gorm:"constraint:OnUpdate:CASCADE,OnDelete:SET NULL;"`
	BackRelationId *uint
}

type GroupRelation struct {
	ID             uint `gorm:"primarykey"`
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Name           string
	Description    string
	FromGroup      *Group             `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	FromGroupId    *uint              `gorm:"uniqueIndex:unique_rel"`
	ToGroup        *Group             `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	ToGroupId      *uint              `gorm:"uniqueIndex:unique_rel,check:ToGroupId <> FromGroupId"`
	RelationType   *GroupRelationType `gorm:"constraint:OnUpdate:CASCADE,OnDelete:CASCADE;"`
	RelationTypeId *uint              `gorm:"uniqueIndex:unique_rel"`
}

func (r *GroupRelationType) BeforeCreate(tx *gorm.DB) error {
	if r.GUID == nil {
		guid := types.NewUUIDv7()
		r.GUID = &guid
	}
	return nil
}

func (r GroupRelation) GetId() uint {
	return r.ID
}

func (r GroupRelation) GetName() string {
	return r.Name
}

func (r GroupRelation) GetDescription() string {
	return r.Description
}

func (r GroupRelationType) GetId() uint {
	return r.ID
}

func (r GroupRelationType) GetName() string {
	return r.Name
}

func (r GroupRelationType) GetDescription() string {
	return r.Description
}
